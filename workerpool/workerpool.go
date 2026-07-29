package workerpool

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/surendratiwari3/paota/config"
	"github.com/surendratiwari3/paota/internal/backend"
	"github.com/surendratiwari3/paota/internal/broker"
	"github.com/surendratiwari3/paota/internal/factory"
	"github.com/surendratiwari3/paota/internal/task"
	"github.com/surendratiwari3/paota/internal/workergroup"
	"github.com/surendratiwari3/paota/logger"
	"github.com/surendratiwari3/paota/schema"
)

const (
	// reconnectInitialDelay/reconnectMaxDelay bound the exponential backoff used
	// when the consumer has to be restarted after losing the broker. The old
	// code slept a flat three seconds, which meant every worker in a fleet
	// reconnected in lockstep the moment RabbitMQ came back.
	reconnectInitialDelay = 500 * time.Millisecond
	reconnectMaxDelay     = 30 * time.Second
)

// WorkerPool stores all configuration for tasks workers
type WorkerPool struct {
	backend        backend.Backend
	broker         broker.Broker
	factory        factory.IFactory
	config         *config.Config
	taskRegistrar  task.TaskRegistrarInterface
	started        atomic.Bool
	workerPoolID   string
	concurrency    uint
	nameSpace      string
	contextType    reflect.Type
	workerGroup    workergroup.WorkerGroupInterface
	configProvider config.ConfigProvider

	// stopChan is closed exactly once by Stop and releases both Start and the
	// consumer supervisor.
	stopChan     chan struct{}
	stopOnce     sync.Once
	consumerDone chan struct{}
}

// globalFactory defined just for unit test cases
var (
	globalFactory factory.IFactory
)

type WorkerPoolOptions struct {
}

// NewWorkerPool creates WorkerPool instance
func NewWorkerPool(ctx interface{}, concurrency uint, nameSpace string) (Pool, error) {
	cnfProvider := config.GetConfigProvider()
	if cnfProvider == nil {
		return nil, errors.New("config is not provided")
	}
	return createWorkerPool(ctx, concurrency, nameSpace, cnfProvider)
}

func createWorkerPool(ctx interface{}, concurrency uint, nameSpace string, configProvider config.ConfigProvider) (Pool, error) {

	if err := validateContextType(ctx); err != nil {
		return nil, err
	}

	if configProvider == nil {
		return nil, errors.New("config is not provided")
	}

	cfg := configProvider.GetConfig()
	if cfg == nil {
		return nil, errors.New("config is not provided")
	}

	// A pool with zero workers can never make progress; treat it as a
	// single-worker pool rather than silently accepting nothing.
	if concurrency == 0 {
		concurrency = 1
	}

	workerPoolId := uuid.New().String()

	workerPool := &WorkerPool{
		concurrency:    concurrency,
		config:         cfg,
		contextType:    reflect.TypeOf(ctx),
		nameSpace:      nameSpace,
		workerPoolID:   workerPoolId,
		factory:        globalFactory,
		configProvider: configProvider,
		stopChan:       make(chan struct{}),
		consumerDone:   make(chan struct{}),
	}

	if workerPool.factory == nil {
		workerPool.factory = &factory.Factory{}
	}

	factoryBroker, err := workerPool.factory.CreateBroker(configProvider)
	if err != nil {
		logger.ApplicationLogger.Error("broker creation failed", err)
		return nil, err
	}
	workerPool.broker = factoryBroker

	// Backend is optional so we ignore the error
	err = workerPool.factory.CreateStore(configProvider)
	if err != nil {
		logger.ApplicationLogger.Error("store creation failed", err)
		return nil, err
	}

	taskRegistrar := workerPool.factory.CreateTaskRegistrar(factoryBroker, configProvider)
	if taskRegistrar == nil {
		logger.ApplicationLogger.Error("task registrar creation failed")
		return nil, errors.New("failed to start the worker pool")
	}
	workerPool.taskRegistrar = taskRegistrar

	workerPool.workerGroup = workergroup.NewWorkerGroup(concurrency, taskRegistrar, nameSpace)

	return workerPool, nil
}

func NewWorkerPoolWithConfig(ctx interface{}, concurrency uint, nameSpace string, conf config.Config) (Pool, error) {
	configProvider := config.NewConfigProvider()
	err := configProvider.SetApplicationConfig(conf)
	if err != nil {
		logger.ApplicationLogger.Error("config error", err)
		return nil, err
	}
	return createWorkerPool(ctx, concurrency, nameSpace, configProvider)
}

// NewWorkerPoolWithOptions : TODO future with options
func NewWorkerPoolWithOptions(ctx interface{}, concurrency uint, namespace string, workerPoolOpts WorkerPoolOptions) (Pool, error) {
	return NewWorkerPool(ctx, concurrency, namespace)
}

// GetBroker returns broker
func (wp *WorkerPool) GetBroker() broker.Broker {
	return wp.broker
}

// SetBroker sets broker
func (wp *WorkerPool) SetBroker(broker broker.Broker) {
	wp.broker = broker
}

// GetBackend returns backend
func (wp *WorkerPool) GetBackend() backend.Backend {
	return wp.backend
}

// SetBackend sets backend
func (wp *WorkerPool) SetBackend(backend backend.Backend) {
	wp.backend = backend
}

// SendTaskWithContext will inject the trace context in the signature headers before publishing it
func (wp *WorkerPool) SendTaskWithContext(ctx context.Context, signature *schema.Signature) (*schema.State, error) {
	if err := wp.taskRegistrar.SendTaskWithContext(ctx, signature); err != nil {
		return nil, fmt.Errorf("publish message error: %w", err)
	}
	return schema.NewPendingTaskState(signature), nil
}

// validateContextType will panic if context is invalid
func validateContextType(ctx interface{}) error {
	ctxType := reflect.TypeOf(ctx)
	if ctxType == nil || ctxType.Kind() != reflect.Struct {
		return errors.New("work: Context needs to be a struct type")
	}
	return nil
}

// Start runs the worker pool until Stop is called or the process receives
// SIGINT/SIGTERM. It never terminates the host process itself: the previous
// implementation called os.Exit(0) from a background goroutine, which killed
// the caller's application during an ordinary shutdown.
func (wp *WorkerPool) Start() error {
	if !wp.started.CompareAndSwap(false, true) {
		return nil
	}

	logger.ApplicationLogger.Info("worker pool called")

	wp.workerGroup.Start()

	go wp.superviseConsumer()

	logger.ApplicationLogger.Info("worker pool started")

	// Wait for a signal to quit, or for Stop to be called elsewhere.
	// os.Kill (SIGKILL) can never be delivered to a process, so SIGTERM — what
	// container runtimes actually send — is registered instead.
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signalChan)

	select {
	case sig := <-signalChan:
		logger.ApplicationLogger.Info("received signal, shutting down worker pool: ", sig)
	case <-wp.stopChan:
	}

	wp.Stop()

	return nil
}

// superviseConsumer keeps the broker consumer running, reconnecting with
// exponential backoff and jitter for as long as the pool is started.
func (wp *WorkerPool) superviseConsumer() {
	defer close(wp.consumerDone)

	delay := reconnectInitialDelay

	for {
		if wp.isStopping() {
			return
		}

		err := wp.broker.StartConsumer(context.Background(), wp.workerGroup)
		if err != nil {
			logger.ApplicationLogger.Error("consumer exited:", err)
		}

		if wp.isStopping() {
			logger.ApplicationLogger.Info("worker pool stopping, consumer goroutine exiting")
			return
		}

		if err == nil {
			// A clean exit that was not requested still means we have no
			// consumer, so restart promptly.
			delay = reconnectInitialDelay
		}

		logger.ApplicationLogger.Warning("broker connection lost, retrying in ", delay)
		if !wp.sleepOrStop(delay) {
			return
		}

		delay *= 2
		if delay > reconnectMaxDelay {
			delay = reconnectMaxDelay
		}
	}
}

func (wp *WorkerPool) isStopping() bool {
	select {
	case <-wp.stopChan:
		return true
	default:
		return !wp.started.Load()
	}
}

// sleepOrStop waits for d, spreading reconnects out with a little jitter.
// It returns false if the pool was stopped while waiting.
func (wp *WorkerPool) sleepOrStop(d time.Duration) bool {
	timer := time.NewTimer(jitterDuration(d))
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-wp.stopChan:
		return false
	}
}

// jitterDuration returns a duration in [d/2, d) so a fleet of workers does not
// reconnect to a recovering broker all at the same instant.
func jitterDuration(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	half := d / 2
	if half <= 0 {
		return d
	}
	return half + time.Duration(rand.Int63n(int64(half)))
}

func (wp *WorkerPool) RegisterTasks(namedTaskFuncs map[string]interface{}) error {
	return wp.taskRegistrar.RegisterTasks(namedTaskFuncs)
}

func (wp *WorkerPool) IsTaskRegistered(name string) bool {
	return wp.taskRegistrar.IsTaskRegistered(name)
}

// Stop stops the workers and associated processes.
//
// Shutdown order matters: the consumer is stopped first so no new deliveries
// arrive, in-flight deliveries are then allowed to drain, and only afterwards
// are the workers torn down. Doing it the other way round handed jobs to
// already-closed worker channels.
//
// Stop is idempotent and safe to call from any goroutine.
func (wp *WorkerPool) Stop() {
	if !wp.started.Load() {
		return
	}

	wp.stopOnce.Do(func() {
		wp.started.Store(false)
		close(wp.stopChan)

		// Stop consuming first; this also waits for deliveries already taken
		// off the socket to be handed to the worker group.
		wp.broker.StopConsumer()

		// Let the supervisor goroutine finish before tearing down workers.
		select {
		case <-wp.consumerDone:
		case <-time.After(reconnectMaxDelay):
			logger.ApplicationLogger.Warning("timed out waiting for consumer supervisor to exit")
		}

		wp.workerGroup.Stop()

		logger.ApplicationLogger.Info("worker pool stopped")
	})
}
