package importer

import (
	"context"
	"fmt"
	"sort"
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
// resources. It is read-only: Match never calls Create or Update.
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
func (c *OutputCatalog) Match(ctx context.Context, query provider.OutputMatchQuery) (resource.Address, provider.OutputMatch, error) {
	if c == nil || c.bindings == nil || c.lookup == nil {
		return resource.Address{}, provider.OutputMatchNone, nil
	}
	query.Provider = strings.TrimSpace(query.Provider)
	query.ResourceType = strings.TrimSpace(query.ResourceType)
	query.Output = strings.TrimSpace(query.Output)
	if query.Provider == "" || query.ResourceType == "" || query.Output == "" {
		return resource.Address{}, provider.OutputMatchNone, nil
	}

	bound, err := c.bindings.Bindings(query.Provider, query.ResourceType)
	if err != nil {
		return resource.Address{}, 0, err
	}
	sort.Slice(bound, func(i, j int) bool {
		return bound[i].Address.String() < bound[j].Address.String()
	})

	var matches []resource.Address
	for _, item := range bound {
		if item.Address.Provider != query.Provider || item.Address.Type != query.ResourceType {
			continue
		}
		ok, err := c.outputEquals(ctx, item, query)
		if err != nil {
			return resource.Address{}, 0, err
		}
		if ok {
			matches = append(matches, item.Address)
		}
	}
	switch len(matches) {
	case 0:
		return resource.Address{}, provider.OutputMatchNone, nil
	case 1:
		return matches[0], provider.OutputMatchUnique, nil
	default:
		return resource.Address{}, provider.OutputMatchAmbiguous, nil
	}
}

func (c *OutputCatalog) outputEquals(ctx context.Context, item RemoteBinding, query provider.OutputMatchQuery) (bool, error) {
	p, err := c.lookup(item.Address)
	if err != nil {
		return false, fmt.Errorf("import output catalog: %s: %w", item.Address, err)
	}
	if p == nil {
		return false, fmt.Errorf("import output catalog: %s: provider is nil", item.Address)
	}
	spec, ok := provider.FindOutput(provider.OutputsOf(p, item.Address.Type), query.Output)
	if !ok || spec.Sensitive {
		return false, nil
	}

	live, err := p.Import(ctx, item.Address, item.RemoteID)
	if err != nil {
		return false, fmt.Errorf("import output catalog: read %s: %w", item.Address, err)
	}
	raw, ok := live.Computed[query.Output]
	if !ok {
		return false, nil
	}
	got, err := outputString(raw)
	if err != nil {
		return false, nil
	}
	return got == query.Value, nil
}

func outputString(v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("not a string")
	}
	return s, nil
}
