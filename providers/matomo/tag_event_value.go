package matomo

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"unicode/utf8"

	"github.com/dziblo-music/agoraform/internal/resource"
)

func optionalEventValue(res resource.Resource) error {
	v, ok := res.Attributes[AttrEventValue]
	if !ok {
		return nil
	}
	if resolved, ok := resource.AsResolved(v); ok {
		return validateVariableRef(res.Address, AttrEventValue, resolved.Address)
	}
	if ref, ok := resource.AsRef(v); ok {
		return validateVariableRef(res.Address, AttrEventValue, ref.Address)
	}

	s, err := eventValueLiteralString(v)
	if err != nil {
		return fmt.Errorf("resource %s: attribute %q must be a numeric value or a resource reference to a %s.%s resource", res.Address, AttrEventValue, Name, TypeVariable)
	}
	if s == "" {
		return nil
	}
	if err := rejectEdgeWhitespace(res.Address, AttrEventValue, s); err != nil {
		return err
	}
	if utf8.RuneCountInString(s) > MaxEventFieldLen {
		return fmt.Errorf("resource %s: attribute %q must be at most %d characters", res.Address, AttrEventValue, MaxEventFieldLen)
	}

	n, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return fmt.Errorf("resource %s: attribute %q must be numeric or a resource reference to a %s.%s resource", res.Address, AttrEventValue, Name, TypeVariable)
	}
	return nil
}

func eventValueLiteralString(v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "", nil
	case string:
		return x, nil
	case json.Number:
		return x.String(), nil
	case int:
		return strconv.FormatInt(int64(x), 10), nil
	case int8:
		return strconv.FormatInt(int64(x), 10), nil
	case int16:
		return strconv.FormatInt(int64(x), 10), nil
	case int32:
		return strconv.FormatInt(int64(x), 10), nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case uint:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint8:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint64:
		return strconv.FormatUint(x, 10), nil
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32), nil
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64), nil
	default:
		return "", fmt.Errorf("unsupported event value type %T", v)
	}
}
