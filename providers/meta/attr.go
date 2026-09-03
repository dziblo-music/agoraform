package meta

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dziblo-music/agoraform/internal/resource"
)

func requiredString(res resource.Resource, key string) (string, error) {
	v, ok := res.Attributes[key]
	if !ok {
		return "", fmt.Errorf("resource %s: missing required attribute %q", res.Address, key)
	}
	s, err := coerceString(v)
	if err != nil {
		return "", fmt.Errorf("resource %s: attribute %q must be a non-empty string", res.Address, key)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("resource %s: attribute %q must be a non-empty string", res.Address, key)
	}
	return s, nil
}

func optionalFloat(res resource.Resource, key string) (float64, bool, error) {
	v, ok := res.Attributes[key]
	if !ok {
		return 0, false, nil
	}
	n, err := coerceFloat(v)
	if err != nil {
		return 0, true, fmt.Errorf("resource %s: attribute %q must be a number", res.Address, key)
	}
	return n, true, nil
}

func coerceString(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case json.Number:
		return x.String(), nil
	case int:
		return strconv.Itoa(x), nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10), nil
		}
		return "", fmt.Errorf("must be a string")
	default:
		return "", fmt.Errorf("must be a string")
	}
}

func coerceFloat(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case json.Number:
		return x.Float64()
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil {
			return 0, err
		}
		return n, nil
	default:
		return 0, fmt.Errorf("must be a number")
	}
}

func normalizeObjectID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", fmt.Errorf("id is required")
	}
	if !digitsOnly(id) {
		return "", fmt.Errorf("id %q must be a numeric Meta object identifier", id)
	}
	if id[0] == '0' && len(id) > 1 {
		return "", fmt.Errorf("id %q must be a numeric Meta object identifier", id)
	}
	return id, nil
}

func digitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func boundIdentity(res resource.Resource) (string, bool, error) {
	if res.Identity.IsZero() {
		return "", false, nil
	}
	id, err := normalizeObjectID(res.Identity.ID)
	if err != nil {
		return "", true, fmt.Errorf("resource %s: persisted identity is invalid: %w", res.Address, err)
	}
	return id, true, nil
}

func joinSorted(values []string) string {
	if len(values) == 0 {
		return ""
	}
	copied := append([]string(nil), values...)
	sort.Strings(copied)
	return strings.Join(copied, ", ")
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}

func logicalRef(v any) resource.Ref {
	if resolved, ok := resource.AsResolved(v); ok {
		return resource.Ref{Address: resolved.Address}
	}
	ref, _ := resource.AsRef(v)
	return ref
}

func objectIDFromAny(v any) (string, bool) {
	switch x := v.(type) {
	case map[string]any:
		if id, err := coerceString(x["id"]); err == nil {
			if normalized, nerr := normalizeObjectID(id); nerr == nil {
				return normalized, true
			}
		}
	case resource.Attributes:
		return objectIDFromAny(map[string]any(x))
	default:
		if s, err := coerceString(x); err == nil {
			if normalized, nerr := normalizeObjectID(s); nerr == nil {
				return normalized, true
			}
		}
	}
	return "", false
}
