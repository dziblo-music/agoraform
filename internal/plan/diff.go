package plan

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"

	"github.com/dziblo-music/agoraform/internal/resource"
)

func diffAttributes(prefix string, before, after any) []AttributeDiff {
	if valuesEqual(before, after) {
		return nil
	}

	beforeMap, beforeIsMap := asMap(before)
	afterMap, afterIsMap := asMap(after)
	if (beforeIsMap || before == nil) && (afterIsMap || after == nil) && (beforeIsMap || afterIsMap) {
		if !beforeIsMap {
			beforeMap = map[string]any{}
		}
		if !afterIsMap {
			afterMap = map[string]any{}
		}
		return diffMaps(prefix, beforeMap, afterMap)
	}

	beforeList, beforeIsList := asList(before)
	afterList, afterIsList := asList(after)
	if (beforeIsList || before == nil) && (afterIsList || after == nil) && (beforeIsList || afterIsList) {
		if !beforeIsList {
			beforeList = nil
		}
		if !afterIsList {
			afterList = nil
		}
		return diffLists(prefix, beforeList, afterList)
	}

	return []AttributeDiff{{
		Path:   prefix,
		Before: cloneDiffValue(before),
		After:  cloneDiffValue(after),
	}}
}

func diffMaps(prefix string, before, after map[string]any) []AttributeDiff {
	keys := make([]string, 0, len(before)+len(after))
	seen := make(map[string]struct{}, len(before)+len(after))
	for _, m := range []map[string]any{before, after} {
		for k := range m {
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var diffs []AttributeDiff
	for _, key := range keys {
		path := joinPath(prefix, key)
		b, bOK := before[key]
		a, aOK := after[key]
		if !bOK {
			b = nil
		}
		if !aOK {
			a = nil
		}
		diffs = append(diffs, diffAttributes(path, b, a)...)
	}
	return diffs
}

func diffLists(prefix string, before, after []any) []AttributeDiff {
	n := len(before)
	if len(after) > n {
		n = len(after)
	}
	var diffs []AttributeDiff
	for i := 0; i < n; i++ {
		path := prefix + "[" + strconv.Itoa(i) + "]"
		var b, a any
		if i < len(before) {
			b = before[i]
		}
		if i < len(after) {
			a = after[i]
		}
		diffs = append(diffs, diffAttributes(path, b, a)...)
	}
	return diffs
}

func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

func asMap(v any) (map[string]any, bool) {
	if v == nil {
		return nil, false
	}
	switch m := v.(type) {
	case map[string]any:
		return m, true
	default:
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Map && rv.Type().Key().Kind() == reflect.String {
			out := make(map[string]any, rv.Len())
			for _, mk := range rv.MapKeys() {
				out[mk.String()] = rv.MapIndex(mk).Interface()
			}
			return out, true
		}
		return nil, false
	}
}

func asList(v any) ([]any, bool) {
	if v == nil {
		return nil, false
	}
	switch list := v.(type) {
	case []any:
		return list, true
	default:
		rv := reflect.ValueOf(v)
		if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
			return nil, false
		}
		out := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = rv.Index(i).Interface()
		}
		return out, true
	}
}

func valuesEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if fa, ok := asNumber(a); ok {
		if fb, ok := asNumber(b); ok {
			return fa == fb
		}
	}

	am, aMap := asMap(a)
	bm, bMap := asMap(b)
	if aMap && bMap {
		return mapsEqual(am, bm)
	}

	al, aList := asList(a)
	bl, bList := asList(b)
	if aList && bList {
		if len(al) != len(bl) {
			return false
		}
		for i := range al {
			if !valuesEqual(al[i], bl[i]) {
				return false
			}
		}
		return true
	}

	if ra, ok := resource.AsRef(a); ok {
		if rb, ok := resource.AsRef(b); ok {
			return ra.Address == rb.Address && ra.Output == rb.Output
		}
	}

	return reflect.DeepEqual(a, b)
}

func mapsEqual(a, b map[string]any) bool {
	keys := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}
	for k := range keys {
		av, aOK := a[k]
		bv, bOK := b[k]
		if !aOK {
			av = nil
		}
		if !bOK {
			bv = nil
		}
		if !valuesEqual(av, bv) {
			return false
		}
	}
	return true
}

func asNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

func cloneDiffValue(v any) any {
	if v == nil {
		return nil
	}
	if m, ok := asMap(v); ok {
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[k] = cloneDiffValue(val)
		}
		return out
	}
	if list, ok := asList(v); ok {
		out := make([]any, len(list))
		for i, val := range list {
			out[i] = cloneDiffValue(val)
		}
		return out
	}
	return v
}

func formatValue(v any) string {
	if v == nil {
		return "(absent)"
	}
	if n, ok := asNumber(v); ok {
		if n == float64(int64(n)) && n >= -1e15 && n <= 1e15 {
			return strconv.FormatInt(int64(n), 10)
		}
		return strconv.FormatFloat(n, 'g', -1, 64)
	}
	switch x := v.(type) {
	case string:
		return strconv.Quote(x)
	case resource.Ref:
		return formatRef(x)
	case bool:
		return strconv.FormatBool(x)
	default:
		if m, ok := asMap(v); ok {
			return formatMap(m)
		}
		if list, ok := asList(v); ok {
			return formatList(list)
		}
		return fmt.Sprint(x)
	}
}

func formatRef(ref resource.Ref) string {
	addr := strconv.Quote(ref.String())
	if !ref.HasOutput() {
		return addr
	}
	return "{$ref: " + addr + ", output: " + strconv.Quote(ref.Output) + "}"
}

func formatMap(m map[string]any) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := "{"
	for i, k := range keys {
		if i > 0 {
			out += ", "
		}
		out += strconv.Quote(k) + ": " + formatValue(m[k])
	}
	out += "}"
	return out
}

func formatList(list []any) string {
	out := "["
	for i, v := range list {
		if i > 0 {
			out += ", "
		}
		out += formatValue(v)
	}
	out += "]"
	return out
}
