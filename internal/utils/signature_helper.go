package utils

func GetString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func GetInt(v interface{}) int {
	if i, ok := v.(int32); ok {
		return int(i)
	}
	if i, ok := v.(int64); ok {
		return int(i)
	}
	if i, ok := v.(int); ok {
		return i
	}
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 0
}

func GetUInt8(v interface{}) uint8 {
	if i, ok := v.(int); ok {
		return uint8(i)
	}
	if f, ok := v.(float64); ok {
		return uint8(f)
	}
	return 0
}

func GetBool(v interface{}) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func GetInt64Ok(v interface{}) (int64, bool) {
	if i, ok := v.(int64); ok {
		return i, true
	}
	if f, ok := v.(float64); ok {
		return int64(f), true
	}
	return 0, false
}
