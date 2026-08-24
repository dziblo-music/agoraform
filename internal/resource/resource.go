package resource

// Attributes is a provider-neutral map of resource attribute values.
//
// Values are YAML-like scalars, lists, or nested maps. The core does not
// interpret provider-specific schemas; providers own that meaning.
type Attributes map[string]any

// Clone returns a shallow-nested copy of the attribute map.
func (a Attributes) Clone() Attributes {
	if a == nil {
		return Attributes{}
	}
	out := make(Attributes, len(a))
	for k, v := range a {
		out[k] = cloneValue(v)
	}
	return out
}

// Resource is a desired resource from configuration.
//
// Attributes are configurable fields declared in the manifest. Computed
// (read-only) values do not belong here.
//
// Identity is not configuration. Core code may attach a persisted
// provider-native identity from local state before calling a provider.
// Manifests must not declare identity fields.
type Resource struct {
	Address    Address
	Identity   Identity
	Attributes Attributes
}

// RemoteResource is a provider-reported live resource.
//
// Attributes holds configurable fields as observed remotely. Computed holds
// read-only fields that providers report but that configuration must not set.
// Identity is an opaque provider-native remote identifier, when the provider
// has one.
type RemoteResource struct {
	Address    Address
	Identity   Identity
	Attributes Attributes
	Computed   Attributes
}

// Identity is an opaque provider-native remote identifier.
//
// The core treats Identity.ID as an uninterpreted string. Providers decide
// the format; it must never include credentials.
type Identity struct {
	ID string
}

// IsZero reports whether the identity is unset.
func (id Identity) IsZero() bool {
	return id.ID == ""
}

func cloneValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = cloneValue(val)
		}
		return out
	case Attributes:
		return x.Clone()
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = cloneValue(val)
		}
		return out
	case Ref:
		return x
	case Resolved:
		return Resolved{
			Address:  x.Address,
			Identity: x.Identity,
			Outputs:  x.Outputs.Clone(),
		}
	default:
		return v
	}
}
