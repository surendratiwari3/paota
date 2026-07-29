package utils

import (
	"encoding/json"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetString(t *testing.T) {
	assert.Equal(t, "hello", GetString("hello"))
	assert.Equal(t, "bytes", GetString([]byte("bytes")))
	assert.Equal(t, "", GetString(nil))
	assert.Equal(t, "", GetString(42))
	assert.Equal(t, "", GetString(struct{}{}))
}

func TestGetInt(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected int
	}{
		{"int", 7, 7},
		{"int8", int8(7), 7},
		{"int16", int16(7), 7},
		// amqp091-go puts a Go int on the wire as a 32-bit signed integer, so
		// this is what a consumer actually receives.
		{"int32", int32(7), 7},
		{"int64", int64(7), 7},
		{"uint8", uint8(7), 7},
		{"uint32", uint32(7), 7},
		{"float64", float64(7.9), 7},
		{"json.Number", json.Number("7"), 7},
		{"negative", int32(-3), -3},
		{"nil", nil, 0},
		{"string", "7", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, GetInt(tt.input))
		})
	}
}

func TestGetUInt8(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected uint8
	}{
		{"uint8", uint8(9), 9},
		// A priority header round-tripped through RabbitMQ comes back as an
		// int32; this used to decode to 0.
		{"int32", int32(9), 9},
		{"int64", int64(9), 9},
		{"int", 9, 9},
		{"float64", float64(9), 9},
		{"max", int32(255), 255},
		{"overflow is rejected", int32(256), 0},
		{"negative is rejected", int32(-1), 0},
		{"nil", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, GetUInt8(tt.input))
		})
	}
}

func TestGetBool(t *testing.T) {
	assert.True(t, GetBool(true))
	assert.False(t, GetBool(false))
	assert.False(t, GetBool(nil))
	assert.False(t, GetBool("true"))
	assert.True(t, GetBool(json.Number("1")))
	assert.False(t, GetBool(json.Number("0")))
}

func TestGetInt64Ok(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected int64
		ok       bool
	}{
		{"int64", int64(1700000000), 1700000000, true},
		{"int32", int32(1700000000), 1700000000, true},
		{"int", 1700000000, 1700000000, true},
		{"float64", float64(1700000000), 1700000000, true},
		{"json.Number", json.Number("1700000000"), 1700000000, true},
		{"string is not a number", "1700000000", 0, false},
		{"nil", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := GetInt64Ok(tt.input)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestHeaderTypesSurviveAmqpEncoding documents why the helpers accept every
// integer width: the type a header is published with is not the type it is
// consumed as.
func TestHeaderTypesSurviveAmqpEncoding(t *testing.T) {
	table := amqp.Table{
		"retry_count": 3,           // Go int   -> wire int32
		"priority":    int(5),      // Go int   -> wire int32
		"created_at":  int64(1700), // Go int64 -> wire int64
		"task_name":   "sendEmail", // string
		"ignore":      true,        // bool
		"retries":     uint8(2),    // Go uint8 -> wire byte
	}
	require.NoError(t, table.Validate())

	assert.Equal(t, 3, GetInt(table["retry_count"]))
	assert.Equal(t, uint8(5), GetUInt8(table["priority"]))
	assert.Equal(t, "sendEmail", GetString(table["task_name"]))
	assert.True(t, GetBool(table["ignore"]))
	assert.Equal(t, 2, GetInt(table["retries"]))

	createdAt, ok := GetInt64Ok(table["created_at"])
	assert.True(t, ok)
	assert.Equal(t, int64(1700), createdAt)
}
