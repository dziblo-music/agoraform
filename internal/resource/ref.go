package resource

import (
	"sort"
	"strconv"
)

// Ref is an explicit reference to another logical resource address.
//
// Refs are configuration. They name a desired Agoraform resource, not a
// provider-native identity. Runtime provider/resource execution is responsible
// for resolving any native identity needed from a Ref; manifests must not
// embed those identities.
//
// Output, when set, selects one declared non-sensitive named output from the
// referenced resource instead of the full runtime Resolved binding.
type Ref struct {
	Address Address
	Output  string
}

// String returns the canonical logical address of the referenced resource.
// It never includes the optional output selector.
func (r Ref) String() string {
	return r.Address.String()
}

// HasOutput reports whether the reference selects a named output.
func (r Ref) HasOutput() bool {
	return r.Output != ""
}

// IsZero reports whether the reference has no address.
func (r Ref) IsZero() bool {
	return r.Address.IsZero()
}

// AsRef reports whether v is an explicit resource reference.
//
// Only Ref values are references. Plain strings remain provider-owned values,
// even when they happen to have the form provider.type.name.
func AsRef(v any) (Ref, bool) {
	ref, ok := v.(Ref)
	if !ok || ref.IsZero() {
		return Ref{}, false
	}
	return ref, true
}

// WalkRefs visits every resource reference in v.
//
// path is a dotted or indexed attribute path, for example "trigger" or
// "triggers[0]". Map keys are visited in sorted order so walks are
// deterministic. fn must not retain path after it returns.
func WalkRefs(v any, fn func(path string, addr Address)) {
	if fn == nil {
		return
	}
	WalkRefValues(v, func(path string, ref Ref) {
		fn(path, ref.Address)
	})
}

// WalkRefValues visits every resource reference in v, including any output
// selector. Map keys are visited in sorted order. fn must not retain path
// after it returns.
func WalkRefValues(v any, fn func(path string, ref Ref)) {
	if fn == nil {
		return
	}
	walkRefs(v, "", fn)
}

// MapRefs clones v, replacing each Ref with the value returned by fn.
//
// Map keys are visited in sorted order. The original value is not mutated.
// When v is Attributes, the result is Attributes.
func MapRefs(v any, fn func(path string, ref Ref) (any, error)) (any, error) {
	if fn == nil {
		return cloneValue(v), nil
	}
	return mapRefs(v, "", fn)
}

func mapRefs(v any, path string, fn func(path string, ref Ref) (any, error)) (any, error) {
	if v == nil {
		return nil, nil
	}
	if ref, ok := AsRef(v); ok {
		return fn(path, ref)
	}
	switch x := v.(type) {
	case Attributes:
		m, err := mapRefMapValues(map[string]any(x), path, fn)
		if err != nil {
			return nil, err
		}
		return Attributes(m), nil
	case map[string]any:
		return mapRefMapValues(x, path, fn)
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			mapped, err := mapRefs(item, path+"["+strconv.Itoa(i)+"]", fn)
			if err != nil {
				return nil, err
			}
			out[i] = mapped
		}
		return out, nil
	default:
		return cloneValue(v), nil
	}
}

func mapRefMapValues(m map[string]any, prefix string, fn func(path string, ref Ref) (any, error)) (map[string]any, error) {
	if m == nil {
		return map[string]any{}, nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]any, len(m))
	for _, k := range keys {
		mapped, err := mapRefs(m[k], joinRefPath(prefix, k), fn)
		if err != nil {
			return nil, err
		}
		out[k] = mapped
	}
	return out, nil
}

func walkRefs(v any, path string, fn func(path string, ref Ref)) {
	if v == nil {
		return
	}
	if ref, ok := AsRef(v); ok {
		fn(path, ref)
		return
	}
	switch x := v.(type) {
	case Attributes:
		walkRefMap(map[string]any(x), path, fn)
	case map[string]any:
		walkRefMap(x, path, fn)
	case []any:
		for i, item := range x {
			walkRefs(item, path+"["+strconv.Itoa(i)+"]", fn)
		}
	}
}

func walkRefMap(m map[string]any, prefix string, fn func(path string, ref Ref)) {
	if len(m) == 0 {
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		walkRefs(m[k], joinRefPath(prefix, k), fn)
	}
}

func joinRefPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}
