package resource

// Resolved is a runtime binding of a logical resource reference to a
// provider-native identity and optional computed outputs.
//
// Apply produces Resolved values immediately before Create/Update. They are
// not configuration: manifests, plans, and import YAML keep Ref values.
type Resolved struct {
	Address  Address
	Identity Identity
	Outputs  Attributes
}

// String returns the canonical logical address. It never includes the
// provider-native identity.
func (r Resolved) String() string {
	return r.Address.String()
}

// IsZero reports whether the binding has no address.
func (r Resolved) IsZero() bool {
	return r.Address.IsZero()
}

// AsResolved reports whether v is a runtime-resolved resource reference.
func AsResolved(v any) (Resolved, bool) {
	resolved, ok := v.(Resolved)
	if !ok || resolved.IsZero() {
		return Resolved{}, false
	}
	return resolved, true
}

// Select returns a clone of the named output when present.
func (r Resolved) Select(name string) (any, bool) {
	if name == "" || r.Outputs == nil {
		return nil, false
	}
	v, ok := r.Outputs[name]
	if !ok {
		return nil, false
	}
	return cloneValue(v), true
}
