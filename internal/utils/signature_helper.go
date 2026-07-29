package utils

import "encoding/json"

// The helpers below convert values pulled out of an amqp.Table back into Go
// types. AMQP field tables are self-describing, and the wire type a value comes
// back as is not necessarily the one it was published with: amqp091-go encodes
// a Go `int` as a 32-bit signed integer ('I') and a `uint8` as a short-short
// ('b'), and other publishers (Python, Java, ...) may use any of the integer
// widths. Each helper therefore accepts every integer representation rather
// than the single one the local publisher happens to emit — otherwise a
// perfectly valid header silently decodes to the zero value.

// GetString returns v as a string, or "" if v is not a string.
func GetString(v interface{}) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	}
	return ""
}

// GetInt returns v as an int, or 0 if v does not hold an integer value.
func GetInt(v interface{}) int {
	i, ok := GetInt64Ok(v)
	if !ok {
		return 0
	}
	return int(i)
}

// GetUInt8 returns v as a uint8, or 0 if v does not hold an integer value or
// the value does not fit in a uint8.
func GetUInt8(v interface{}) uint8 {
	i, ok := GetInt64Ok(v)
	if !ok || i < 0 || i > 255 {
		return 0
	}
	return uint8(i)
}

// GetBool returns v as a bool, or false if v is not a bool.
func GetBool(v interface{}) bool {
	switch b := v.(type) {
	case bool:
		return b
	case json.Number:
		n, err := b.Int64()
		return err == nil && n != 0
	}
	return false
}

// GetInt64Ok returns v as an int64 along with whether the conversion succeeded.
func GetInt64Ok(v interface{}) (int64, bool) {
	switch i := v.(type) {
	case int:
		return int64(i), true
	case int8:
		return int64(i), true
	case int16:
		return int64(i), true
	case int32:
		return int64(i), true
	case int64:
		return i, true
	case uint:
		return int64(i), true
	case uint8:
		return int64(i), true
	case uint16:
		return int64(i), true
	case uint32:
		return int64(i), true
	case uint64:
		return int64(i), true
	case float32:
		return int64(i), true
	case float64:
		return int64(i), true
	case json.Number:
		n, err := i.Int64()
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}
