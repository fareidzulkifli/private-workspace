package phase1

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type ColumnKind string

const (
	KindText      ColumnKind = "text"
	KindTimestamp ColumnKind = "timestamp"
	KindDate      ColumnKind = "date"
	KindBool      ColumnKind = "bool"
	KindJSON      ColumnKind = "json"
	KindReal      ColumnKind = "real"
	KindInteger   ColumnKind = "integer"
)

func NormalizeValue(value any, kind ColumnKind, fallback any) (any, error) {
	if value == nil {
		return fallback, nil
	}

	switch kind {
	case KindText:
		return toString(value)
	case KindTimestamp:
		return normalizeTimestamp(value)
	case KindDate:
		return normalizeDate(value)
	case KindBool:
		return normalizeBool(value)
	case KindJSON:
		return normalizeJSON(value, fallback)
	case KindReal:
		return normalizeReal(value)
	case KindInteger:
		return normalizeInteger(value)
	default:
		return nil, fmt.Errorf("unsupported column kind %q", kind)
	}
}

func toString(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	case [16]byte:
		return formatUUID(v), nil
	case fmt.Stringer:
		return v.String(), nil
	case time.Time:
		return v.UTC().Format(time.RFC3339Nano), nil
	default:
		return fmt.Sprintf("%v", value), nil
	}
}

func formatUUID(value [16]byte) string {
	var out [36]byte
	hex.Encode(out[0:8], value[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], value[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], value[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], value[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], value[10:16])
	return string(out[:])
}

func normalizeTimestamp(value any) (string, error) {
	switch v := value.(type) {
	case time.Time:
		return v.UTC().Format(time.RFC3339Nano), nil
	case string:
		if strings.TrimSpace(v) == "" {
			return "", nil
		}
		parsed, err := parseTime(v)
		if err != nil {
			return "", err
		}
		return parsed.UTC().Format(time.RFC3339Nano), nil
	case []byte:
		return normalizeTimestamp(string(v))
	default:
		return "", fmt.Errorf("cannot normalize %T as timestamp", value)
	}
}

func normalizeDate(value any) (string, error) {
	switch v := value.(type) {
	case time.Time:
		return v.Format(time.DateOnly), nil
	case string:
		if strings.TrimSpace(v) == "" {
			return "", nil
		}
		if len(v) >= len(time.DateOnly) {
			candidate := v[:len(time.DateOnly)]
			if _, err := time.Parse(time.DateOnly, candidate); err == nil {
				return candidate, nil
			}
		}
		parsed, err := parseTime(v)
		if err != nil {
			return "", err
		}
		return parsed.Format(time.DateOnly), nil
	case []byte:
		return normalizeDate(string(v))
	default:
		return "", fmt.Errorf("cannot normalize %T as date", value)
	}
}

func normalizeBool(value any) (int, error) {
	switch v := value.(type) {
	case bool:
		if v {
			return 1, nil
		}
		return 0, nil
	case int:
		return boolInt(v)
	case int16:
		return boolInt(int(v))
	case int32:
		return boolInt(int(v))
	case int64:
		return boolInt(int(v))
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		if err != nil {
			return 0, err
		}
		if parsed {
			return 1, nil
		}
		return 0, nil
	case []byte:
		return normalizeBool(string(v))
	default:
		return 0, fmt.Errorf("cannot normalize %T as bool", value)
	}
}

func boolInt(value int) (int, error) {
	if value == 0 || value == 1 {
		return value, nil
	}
	return 0, fmt.Errorf("boolean integer must be 0 or 1, got %d", value)
}

func normalizeJSON(value any, fallback any) (string, error) {
	if value == nil {
		if fallback == nil {
			return "null", nil
		}
		return fmt.Sprintf("%v", fallback), nil
	}

	var raw []byte
	switch v := value.(type) {
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		rawBytes, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		raw = rawBytes
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		raw = []byte("[]")
	}

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", err
	}
	compact, err := json.Marshal(decoded)
	if err != nil {
		return "", err
	}
	return string(compact), nil
}

func normalizeReal(value any) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(strings.TrimSpace(v), 64)
	case []byte:
		return normalizeReal(string(v))
	default:
		return 0, fmt.Errorf("cannot normalize %T as real", value)
	}
}

func normalizeInteger(value any) (int64, error) {
	switch v := value.(type) {
	case int:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	case string:
		return strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	case []byte:
		return normalizeInteger(string(v))
	default:
		return 0, fmt.Errorf("cannot normalize %T as integer", value)
	}
}

func parseTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07",
		"2006-01-02 15:04:05.999999-07",
		"2006-01-02 15:04:05-07",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999Z07:00",
		"2006-01-02 15:04:05Z07:00",
		time.DateOnly,
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time format %q", value)
}
