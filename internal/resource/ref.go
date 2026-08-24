package resource

import (
	"sort"
	"strconv"
)

// Ref is an explicit reference to another logical resource address.
//
// Refs are configuration. They name a desired Agoraform resource, not a
// provider-native identity. Providers resolve a Ref to a remote identity
// at runtime; manifests must not embed those identities.
type Ref struct {
	Address Address
}

// String returns the canonical logical address of the referenced resource.
func (r Ref) String() string {
	return r.Address.String()
}

// IsZero reports whether the reference has no address.
func (r Ref) IsZero() bool {
	return r.Address.IsZero()
}

// AsRef reports whether v is a resource reference.
//
// A Ref value is always a reference. A string is a reference when it is a
// well-formed logical resource address (provider.type.name).
func AsRef(v any) (Ref, bool) {
	switch x := v.(type) {
	case Ref:
		if x.IsZero() {
			return Ref{}, false
		}
		return x, true
	case string:
		addr, err := ParseAddress(x)
		if err != nil {
			return Ref{}, false
		}
		return Ref{Address: addr}, true
	default:
		return Ref{}, false
	}
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
	walkRefs(v, "", fn)
}

func walkRefs(v any, path string, fn func(path string, addr Address)) {
	if v == nil {
		return
	}
	if ref, ok := AsRef(v); ok {
		fn(path, ref.Address)
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

func walkRefMap(m map[string]any, prefix string, fn func(path string, addr Address)) {
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
