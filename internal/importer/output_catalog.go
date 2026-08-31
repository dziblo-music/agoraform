package importer

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

// RemoteBinding is a logical address together with its persisted
// provider-native identity.
type RemoteBinding struct {
	Address  resource.Address
	RemoteID string
}

// RemoteBindings lists already-bound identities for a provider resource type.
//
// *state.Store can be adapted to this interface. Bindings must be returned
// in a deterministic order.
type RemoteBindings interface {
	Bindings(provider, resourceType string) ([]RemoteBinding, error)
}

// OutputCatalog matches declared non-sensitive outputs of already-bound
// resources. It is read-only: Match never calls Create or Update. The catalog
// is ephemeral and is not persisted.
type OutputCatalog struct {
	bindings RemoteBindings
	lookup   Lookup
}

// NewOutputCatalog returns a matcher over identities from bindings.
// lookup is used only to Import already-bound resources by identity.
func NewOutputCatalog(bindings RemoteBindings, lookup Lookup) *OutputCatalog {
	return &OutputCatalog{bindings: bindings, lookup: lookup}
}

// Match implements provider.OutputMatcher.
func (c *OutputCatalog) Match(ctx context.Context, query provider.OutputMatchQuery) (resource.Ref, provider.OutputMatch, error) {
	if c == nil || c.bindings == nil || c.lookup == nil {
		return resource.Ref{}, provider.OutputMatchNone, nil
	}
	query.Provider = strings.TrimSpace(query.Provider)
	query.ResourceType = strings.TrimSpace(query.ResourceType)
	selected := query.SelectedOutput()
	constraints := query.Constraints()
	if query.Provider == "" || query.ResourceType == "" || selected == "" || len(constraints) == 0 {
		return resource.Ref{}, provider.OutputMatchNone, nil
	}

	bound, err := c.bindings.Bindings(query.Provider, query.ResourceType)
	if err != nil {
		return resource.Ref{}, 0, err
	}
	sort.Slice(bound, func(i, j int) bool {
		return bound[i].Address.String() < bound[j].Address.String()
	})

	var matches []resource.Address
	for _, item := range bound {
		if item.Address.Provider != query.Provider || item.Address.Type != query.ResourceType {
			continue
		}
		ok, err := c.outputsEqual(ctx, item, selected, constraints)
		if err != nil {
			return resource.Ref{}, 0, err
		}
		if ok {
			matches = append(matches, item.Address)
		}
	}
	switch len(matches) {
	case 0:
		return resource.Ref{}, provider.OutputMatchNone, nil
	case 1:
		return resource.Ref{Address: matches[0], Output: selected}, provider.OutputMatchUnique, nil
	default:
		return resource.Ref{}, provider.OutputMatchAmbiguous, nil
	}
}

func (c *OutputCatalog) outputsEqual(ctx context.Context, item RemoteBinding, selected string, constraints map[string]string) (bool, error) {
	p, err := c.lookup(item.Address)
	if err != nil {
		return false, fmt.Errorf("import output catalog: %s: %w", item.Address, err)
	}
	if p == nil {
		return false, fmt.Errorf("import output catalog: %s: provider is nil", item.Address)
	}
	specs := provider.OutputsOf(p, item.Address.Type)
	if !safeOutput(specs, selected) {
		return false, nil
	}
	names := make([]string, 0, len(constraints))
	for name := range constraints {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !safeOutput(specs, name) {
			return false, nil
		}
	}

	live, err := p.Import(ctx, item.Address, item.RemoteID)
	if err != nil {
		return false, fmt.Errorf("import output catalog: read %s: %w", item.Address, err)
	}
	for _, name := range names {
		spec, _ := provider.FindOutput(specs, name)
		raw, ok := live.Computed[name]
		if !ok {
			return false, nil
		}
		if !provider.KindMatches(raw, spec.Kind) {
			return false, nil
		}
		got, ok := outputLiteral(raw)
		if !ok || got != constraints[name] {
			return false, nil
		}
	}
	return true, nil
}

func safeOutput(specs []provider.OutputSpec, name string) bool {
	spec, ok := provider.FindOutput(specs, name)
	return ok && !spec.Sensitive
}

func outputLiteral(v any) (string, bool) {
	switch n := v.(type) {
	case string:
		return n, true
	case bool:
		return strconv.FormatBool(n), true
	case int:
		return strconv.Itoa(n), true
	case int8:
		return strconv.FormatInt(int64(n), 10), true
	case int16:
		return strconv.FormatInt(int64(n), 10), true
	case int32:
		return strconv.FormatInt(int64(n), 10), true
	case int64:
		return strconv.FormatInt(n, 10), true
	case uint:
		return strconv.FormatUint(uint64(n), 10), true
	case uint8:
		return strconv.FormatUint(uint64(n), 10), true
	case uint16:
		return strconv.FormatUint(uint64(n), 10), true
	case uint32:
		return strconv.FormatUint(uint64(n), 10), true
	case uint64:
		return strconv.FormatUint(n, 10), true
	case float32:
		return strconv.FormatFloat(float64(n), 'g', -1, 32), true
	case float64:
		return strconv.FormatFloat(n, 'g', -1, 64), true
	default:
		return "", false
	}
}
