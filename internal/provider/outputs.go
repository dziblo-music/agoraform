package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/dziblo-music/agoraform/internal/resource"
)

// OutputKind is the declared runtime kind of a named resource output.
type OutputKind string

const (
	// OutputKindString is a UTF-8 string output.
	OutputKindString OutputKind = "string"

	// OutputKindNumber is a numeric output.
	OutputKindNumber OutputKind = "number"

	// OutputKindBool is a boolean output.
	OutputKindBool OutputKind = "bool"
)

// OutputSpec describes one declared resource output.
//
// Only explicitly declared non-sensitive outputs may be selected by a
// manifest output reference. Arbitrary computed fields are not outputs.
type OutputSpec struct {
	Name      string
	Kind      OutputKind
	Sensitive bool
}

// OutputCatalog is an optional provider hook that declares the named
// outputs a resource type may expose.
type OutputCatalog interface {
	Outputs(resourceType string) []OutputSpec
}

// OutputMatchQuery asks for a unique already-bound resource whose declared
// non-sensitive outputs equal the requested values.
//
// Output names the selected output on a unique match Ref. When Equals is
// empty, Output and Value form a single-field query. When Equals is set, every
// named value must match the same resource (AND). Providers must not guess
// when the result is none or ambiguous.
type OutputMatchQuery struct {
	Provider     string
	ResourceType string
	Output       string
	Value        string
	Equals       map[string]string
}

// Constraints returns the named output values that must all match.
// Empty names are omitted. The returned map is a copy.
func (q OutputMatchQuery) Constraints() map[string]string {
	out := make(map[string]string)
	if len(q.Equals) > 0 {
		for name, value := range q.Equals {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			out[name] = value
		}
		return out
	}
	name := strings.TrimSpace(q.Output)
	if name == "" {
		return out
	}
	out[name] = q.Value
	return out
}

// SelectedOutput returns the output name placed on a unique match Ref.
// When Output is empty and Constraints has exactly one name, that name is used.
func (q OutputMatchQuery) SelectedOutput() string {
	if name := strings.TrimSpace(q.Output); name != "" {
		return name
	}
	constraints := q.Constraints()
	if len(constraints) != 1 {
		return ""
	}
	for name := range constraints {
		return name
	}
	return ""
}

// OutputMatch is the deterministic result of an output relationship lookup.
type OutputMatch int

const (
	// OutputMatchNone means no bound resource produced Value for Output.
	OutputMatchNone OutputMatch = iota
	// OutputMatchUnique means exactly one bound resource produced Value.
	OutputMatchUnique
	// OutputMatchAmbiguous means more than one bound resource produced Value.
	OutputMatchAmbiguous
)

// OutputMatcher looks up already-bound resources by declared safe outputs.
//
// Implementations must not mutate remote systems. A unique match returns a
// logical Ref selecting query.SelectedOutput. Zero and multiple matches are
// distinct from errors; callers must not guess in those cases.
type OutputMatcher interface {
	Match(ctx context.Context, query OutputMatchQuery) (resource.Ref, OutputMatch, error)
}

// OutputsOf returns the declared outputs for resourceType, or nil when
// the reader does not implement OutputCatalog.
func OutputsOf(reader Reader, resourceType string) []OutputSpec {
	if reader == nil {
		return nil
	}
	catalog, ok := reader.(OutputCatalog)
	if !ok {
		return nil
	}
	return catalog.Outputs(resourceType)
}

// FindOutput returns the declared spec with the given name.
func FindOutput(specs []OutputSpec, name string) (OutputSpec, bool) {
	for _, spec := range specs {
		if spec.Name == name {
			return spec, true
		}
	}
	return OutputSpec{}, false
}

// KindOf reports the output kind of v when it is a supported scalar.
func KindOf(v any) (OutputKind, bool) {
	switch v.(type) {
	case string:
		return OutputKindString, true
	case bool:
		return OutputKindBool, true
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return OutputKindNumber, true
	default:
		return "", false
	}
}

// KindMatches reports whether v has the declared output kind.
func KindMatches(v any, kind OutputKind) bool {
	got, ok := KindOf(v)
	return ok && got == kind
}

// ValidateOutputRefs checks that every output selector names a declared
// non-sensitive output on its target resource. Address-only references
// are ignored. lookup may be nil when there are no output references.
func ValidateOutputRefs(resources []resource.Resource, lookup func(resource.Address) (Reader, error)) error {
	byAddr := make(map[string]resource.Resource, len(resources))
	for _, res := range resources {
		byAddr[res.Address.String()] = res
	}

	var first error
	for _, res := range resources {
		resource.WalkRefValues(res.Attributes, func(path string, ref resource.Ref) {
			if first != nil || !ref.HasOutput() {
				return
			}
			target, ok := byAddr[ref.Address.String()]
			if !ok {
				first = fmt.Errorf("resource %q attribute %q references unknown resource %q", res.Address, displayOutputPath(path), ref.Address)
				return
			}
			if lookup == nil {
				first = fmt.Errorf("resource %q attribute %q: provider lookup is required to validate output %q", res.Address, displayOutputPath(path), ref.Output)
				return
			}
			reader, err := lookup(target.Address)
			if err != nil {
				first = fmt.Errorf("resource %q attribute %q: %w", res.Address, displayOutputPath(path), err)
				return
			}
			spec, ok := FindOutput(OutputsOf(reader, target.Address.Type), ref.Output)
			if !ok {
				first = fmt.Errorf("resource %q attribute %q: %s has no declared output %q", res.Address, displayOutputPath(path), ref.Address, ref.Output)
				return
			}
			if spec.Sensitive {
				first = fmt.Errorf("resource %q attribute %q: output %q on %s is sensitive and cannot be referenced", res.Address, displayOutputPath(path), ref.Output, ref.Address)
			}
		})
		if first != nil {
			return first
		}
	}
	return nil
}

func displayOutputPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "(attributes)"
	}
	return path
}
