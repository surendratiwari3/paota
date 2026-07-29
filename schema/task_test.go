package schema

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSignature(t *testing.T) {
	args := []Arg{{Name: "to", Type: "string", Value: "someone@example.com"}}

	sig, err := NewSignature("sendEmail", args)

	require.NoError(t, err)
	require.NotNil(t, sig)
	assert.Equal(t, "sendEmail", sig.Name)
	assert.Equal(t, args, sig.Args)
	assert.True(t, strings.HasPrefix(sig.UUID, "task_"), "uuid should be prefixed: %s", sig.UUID)
}

func TestNewSignatureGeneratesUniqueUUIDs(t *testing.T) {
	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		sig, err := NewSignature("task", nil)
		require.NoError(t, err)
		_, duplicate := seen[sig.UUID]
		assert.False(t, duplicate, "duplicate uuid generated: %s", sig.UUID)
		seen[sig.UUID] = struct{}{}
	}
}

func TestBytesToSignature(t *testing.T) {
	createdAt := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	original := &Signature{
		UUID:        "task_1",
		Name:        "sendEmail",
		RoutingKey:  "paota_tasks",
		Priority:    3,
		RetryCount:  2,
		RetriesDone: 1,
		TaskTimeOut: 30,
		CreatedAt:   &createdAt,
	}

	encoded, err := json.Marshal(original)
	require.NoError(t, err)

	decoded, err := BytesToSignature(encoded)

	require.NoError(t, err)
	assert.Equal(t, original.UUID, decoded.UUID)
	assert.Equal(t, original.Name, decoded.Name)
	assert.Equal(t, original.RoutingKey, decoded.RoutingKey)
	assert.Equal(t, original.Priority, decoded.Priority)
	assert.Equal(t, original.RetryCount, decoded.RetryCount)
	assert.Equal(t, original.RetriesDone, decoded.RetriesDone)
	require.NotNil(t, decoded.CreatedAt)
	assert.True(t, createdAt.Equal(*decoded.CreatedAt))
}

func TestBytesToSignatureRawArgsRoundTrip(t *testing.T) {
	original := &Signature{
		Name:    "sendEmail",
		RawArgs: json.RawMessage(`{"to":"someone@example.com","attempts":9007199254740993}`),
	}

	encoded, err := json.Marshal(original)
	require.NoError(t, err)

	decoded, err := BytesToSignature(encoded)

	require.NoError(t, err)
	// Decoding uses json.Number, so large integers inside RawArgs are not
	// mangled by a float64 round trip.
	assert.JSONEq(t, string(original.RawArgs), string(decoded.RawArgs))
}

func TestBytesToSignatureErrors(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"empty input", []byte{}},
		{"not json", []byte("this is not json")},
		{"truncated json", []byte(`{"Name":`)},
		{"wrong type for field", []byte(`{"Name":123}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig, err := BytesToSignature(tt.input)
			assert.Error(t, err)
			assert.Nil(t, sig)
		})
	}
}

func TestTaskStates(t *testing.T) {
	sig := &Signature{UUID: "task_1", Name: "sendEmail"}

	tests := []struct {
		name     string
		state    *State
		expected string
	}{
		{"pending", NewPendingTaskState(sig), "Pending"},
		{"complete", NewCompleteTaskState(sig), "Complete"},
		{"failed", NewFailedTaskState(sig), "Failed"},
		{"cancelled", NewCancelledTaskState(sig), "Cancelled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.state)
			assert.Equal(t, tt.expected, tt.state.Status)
			assert.Equal(t, *sig, tt.state.Request)
		})
	}
}

// TestTaskStateCopiesSignature makes sure a state snapshot is not aliased to
// the live signature, which keeps retry bookkeeping from mutating a state that
// was already handed to the caller.
func TestTaskStateCopiesSignature(t *testing.T) {
	sig := &Signature{UUID: "task_1", RetriesDone: 0}
	state := NewPendingTaskState(sig)

	sig.RetriesDone = 5

	assert.Equal(t, 0, state.Request.RetriesDone)
}
