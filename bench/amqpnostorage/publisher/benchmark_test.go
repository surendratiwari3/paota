package publisher

import (
	"context"
	"encoding/json"
	"github.com/surendratiwari3/paota/bench/amqpnostorage"
	"github.com/surendratiwari3/paota/internal/schema"
	"github.com/surendratiwari3/paota/workerpool"
	"testing"
)

func BenchmarkAmqpNoStore(b *testing.B) {
	workerPool := amqpnostorage.SetupWorkerPool()
	for n := 0; n < b.N; n++ {
		Publisher(workerPool)
	}
}

func Publisher(pool workerpool.Pool) {

	// Replace this with the received user record
	user := amqpnostorage.UserRecord{
		ID:    "1",
		Name:  "John Doe",
		Email: "john.doe@example.com",
	}

	// Convert the struct to a JSON string
	userJSON, err := json.Marshal(user)
	if err != nil {
		//
	}

	returnNil := &schema.Signature{
		Name: "returnNil",
		Args: []schema.Arg{
			{
				Type:  "string",
				Value: string(userJSON),
			},
		},
		IgnoreWhenTaskNotRegistered: true,
	}

	for i := 0; i < 10000; i++ {
		pool.SendTaskWithContext(context.Background(), returnNil)
	}
}
