package meta

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

func parseRule(v any) (any, error) {
	if v == nil {
		return nil, fmt.Errorf("must be a Meta rule object")
	}
	switch x := v.(type) {
	case string:
		raw := strings.TrimSpace(x)
		if raw == "" {
			return nil, fmt.Errorf("must be a Meta rule object")
		}
		var decoded any
		dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
		dec.UseNumber()
		if err := dec.Decode(&decoded); err != nil {
			return nil, fmt.Errorf("must be a valid Meta rule JSON object")
		}
		return canonicalizeJSON(decoded)
	default:
		return canonicalizeJSON(x)
	}
}

func encodeRule(v any) (string, error) {
	canonical, err := parseRule(v)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("could not encode Meta rule")
	}
	return string(raw), nil
}

func canonicalizeJSON(v any) (any, error) {
	switch x := v.(type) {
	case map[string]any:
		if len(x) == 0 {
			return nil, fmt.Errorf("must be a Meta rule object")
		}
		out := make(map[string]any, len(x))
		for key, val := range x {
			canon, err := canonicalizeJSON(val)
			if err != nil {
				return nil, err
			}
			out[key] = canon
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			canon, err := canonicalizeJSON(val)
			if err != nil {
				return nil, err
			}
			out[i] = canon
		}
		return out, nil
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i, nil
		}
		f, err := x.Float64()
		if err != nil {
			return nil, fmt.Errorf("invalid numeric rule value")
		}
		return f, nil
	case float64:
		if x == float64(int64(x)) {
			return int64(x), nil
		}
		return x, nil
	case float32:
		return canonicalizeJSON(float64(x))
	case int:
		return int64(x), nil
	case int32:
		return int64(x), nil
	case int64:
		return x, nil
	case string, bool, nil:
		return x, nil
	default:
		return nil, fmt.Errorf("unsupported value in Meta rule")
	}
}

func rulesEqual(a, b any) bool {
	left, err := parseRule(a)
	if err != nil {
		return false
	}
	right, err := parseRule(b)
	if err != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
}
