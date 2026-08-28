package destroy

import (
	"context"
	"fmt"
	"sort"

	"github.com/dziblo-music/agoraform/internal/graph"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

// Kind is a planned destroy outcome.
type Kind string

const (
	// KindDestroy is a supported provider-native deletion.
	KindDestroy Kind = "destroy"

	// KindRemove is a supported provider-native remove/archive/disable.
	KindRemove Kind = "remove"

	// KindUnsupported means destroy is not implemented for the resource.
	KindUnsupported Kind = "unsupported"

	// KindProviderOwned means the platform owns the object and Agoraform
	// cannot delete it.
	KindProviderOwned Kind = "provider-owned"

	// KindNotManaged means the manifest resource has no local state binding.
	KindNotManaged Kind = "not-managed"
)

// Change is one planned destroy outcome.
type Change struct {
	Address  resource.Address
	Kind     Kind
	Identity resource.Identity
	Resource resource.Resource
}

// Plan is a deterministic destroy plan plus any provider finalizations that
// must run after successful destructive mutations.
type Plan struct {
	Changes       []Change
	Finalizations []provider.FinalizationPlan
	Preserved     []resource.Address
}

// Lookup resolves the provider for a resource address.
type Lookup func(addr resource.Address) (provider.Provider, error)

// Identities supplies persisted provider-native identities.
type Identities interface {
	Identity(addr resource.Address) (resource.Identity, bool, error)
	Addresses() ([]resource.Address, error)
}

// Store persists identity removal after confirmed remote teardown.
type Store interface {
	Identities
	Remove(addr resource.Address) error
}

// Build constructs the complete destroy plan before any mutation.
//
// The manifest defines the requested destroy set and dependency graph. State
// supplies identities. Invalid graphs produce an error and an empty plan.
func Build(ctx context.Context, desired []resource.Resource, lookup Lookup, ids Identities) (*Plan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if lookup == nil && len(desired) > 0 {
		return nil, fmt.Errorf("destroy: provider lookup is required")
	}
	if ids == nil {
		return nil, fmt.Errorf("destroy: state store is required")
	}

	g, err := graph.Build(desired)
	if err != nil {
		return nil, fmt.Errorf("destroy: %w", err)
	}

	byAddr, err := desiredByAddress(desired)
	if err != nil {
		return nil, err
	}

	changes := make([]Change, 0, len(desired))
	for _, addr := range g.ReverseOrder() {
		res := byAddr[addr.String()]
		change, err := planChange(res, lookup, ids)
		if err != nil {
			return nil, err
		}
		changes = append(changes, change)
	}

	preserved, err := preservedAddresses(desired, ids)
	if err != nil {
		return nil, err
	}

	return &Plan{Changes: changes, Preserved: preserved}, nil
}

func planChange(res resource.Resource, lookup Lookup, ids Identities) (Change, error) {
	change := Change{Address: res.Address, Resource: res}
	id, ok, err := ids.Identity(res.Address)
	if err != nil {
		return Change{}, fmt.Errorf("destroy %s: %w", res.Address, err)
	}
	if !ok {
		change.Kind = KindNotManaged
		return change, nil
	}
	change.Identity = id
	res.Identity = id
	change.Resource = res

	p, err := lookup(res.Address)
	if err != nil {
		return Change{}, fmt.Errorf("destroy %s: %w", res.Address, err)
	}
	cap, err := provider.ResourceDestroyCapability(p, res)
	if err != nil {
		return Change{}, fmt.Errorf("destroy %s: %w", res.Address, err)
	}
	switch cap {
	case provider.DestroyDelete:
		change.Kind = KindDestroy
	case provider.DestroyRemove:
		change.Kind = KindRemove
	case provider.DestroyUnsupported:
		change.Kind = KindUnsupported
	case provider.DestroyProviderOwned:
		change.Kind = KindProviderOwned
	default:
		return Change{}, fmt.Errorf("destroy %s: provider returned invalid destroy capability %q", res.Address, cap)
	}
	return change, nil
}

func preservedAddresses(desired []resource.Resource, ids Identities) ([]resource.Address, error) {
	inManifest := make(map[string]struct{}, len(desired))
	for _, res := range desired {
		inManifest[res.Address.String()] = struct{}{}
	}
	all, err := ids.Addresses()
	if err != nil {
		return nil, fmt.Errorf("destroy: %w", err)
	}
	var preserved []resource.Address
	for _, addr := range all {
		if _, ok := inManifest[addr.String()]; ok {
			continue
		}
		preserved = append(preserved, addr)
	}
	sort.Slice(preserved, func(i, j int) bool {
		return preserved[i].String() < preserved[j].String()
	})
	return preserved, nil
}

func desiredByAddress(desired []resource.Resource) (map[string]resource.Resource, error) {
	out := make(map[string]resource.Resource, len(desired))
	for _, res := range desired {
		key := res.Address.String()
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("destroy: duplicate desired resource %s", res.Address)
		}
		out[key] = res
	}
	return out, nil
}

// HasMutations reports whether any supported destructive operation is planned.
func (p *Plan) HasMutations() bool {
	if p == nil {
		return false
	}
	for _, c := range p.Changes {
		if c.Kind == KindDestroy || c.Kind == KindRemove {
			return true
		}
	}
	return len(p.Finalizations) > 0
}

// MutationCount is the number of supported destroy/remove operations.
func (p *Plan) MutationCount() int {
	if p == nil {
		return 0
	}
	n := 0
	for _, c := range p.Changes {
		if c.Kind == KindDestroy || c.Kind == KindRemove {
			n++
		}
	}
	return n
}

// RemainingCount is the number of state-bound resources that will stay in
// state because destroy is unsupported or provider-owned.
func (p *Plan) RemainingCount() int {
	if p == nil {
		return 0
	}
	n := 0
	for _, c := range p.Changes {
		if c.Kind == KindUnsupported || c.Kind == KindProviderOwned {
			n++
		}
	}
	return n
}

func (p *Plan) remainingAddresses() []resource.Address {
	if p == nil {
		return nil
	}
	var out []resource.Address
	for _, c := range p.Changes {
		if c.Kind == KindUnsupported || c.Kind == KindProviderOwned {
			out = append(out, c.Address)
		}
	}
	return out
}
