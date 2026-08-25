package apply

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

// ProviderSet exposes the registry operations required to discover and execute
// provider finalizations, including provider-only actions with no resource CRUD.
// *provider.Registry implements this interface.
type ProviderSet interface {
	LookupFor(addr resource.Address) (provider.Provider, error)
	Lookup(name string) (provider.Provider, bool)
	List() []provider.Provider
}

type providerCatalog struct {
	lookup    Lookup
	providers ProviderSet
	byName    map[string]provider.Provider
}

func newProviderCatalog(lookup Lookup, providers ProviderSet) *providerCatalog {
	catalog := &providerCatalog{
		lookup:    lookup,
		providers: providers,
		byName:    make(map[string]provider.Provider),
	}
	catalog.refresh()
	return catalog
}

func (c *providerCatalog) LookupFor(addr resource.Address) (provider.Provider, error) {
	if c == nil {
		return nil, fmt.Errorf("provider lookup is required")
	}

	var (
		p   provider.Provider
		err error
	)
	if c.providers != nil {
		p, err = c.providers.LookupFor(addr)
	} else if c.lookup != nil {
		p, err = c.lookup(addr)
	} else {
		return nil, fmt.Errorf("provider lookup is required")
	}
	if err != nil {
		return nil, err
	}
	c.remember(p)
	return p, nil
}

func (c *providerCatalog) Lookup(name string) (provider.Provider, bool) {
	if c == nil {
		return nil, false
	}
	if p, ok := c.byName[name]; ok {
		return p, true
	}
	if c.providers != nil {
		p, found := c.providers.Lookup(name)
		if found {
			c.remember(p)
		}
		return p, found
	}
	return nil, false
}

func (c *providerCatalog) List() []provider.Provider {
	if c == nil {
		return nil
	}
	c.refresh()
	names := make([]string, 0, len(c.byName))
	for name := range c.byName {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]provider.Provider, 0, len(names))
	for _, name := range names {
		out = append(out, c.byName[name])
	}
	return out
}

func (c *providerCatalog) refresh() {
	if c == nil || c.providers == nil {
		return
	}
	for _, p := range c.providers.List() {
		c.remember(p)
	}
}

func (c *providerCatalog) remember(p provider.Provider) {
	if c == nil || p == nil {
		return
	}
	c.byName[p.Name()] = p
}

// AttachFinalizations asks registered provider finalizers to append any
// provider-level actions implied by the resource plan. It is non-mutating with
// respect to remote provider state and must run before resource mutations.
func AttachFinalizations(ctx context.Context, providers ProviderSet, p *plan.Plan) error {
	if p == nil || providers == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	pending := pendingChanges(p)
	for _, registered := range providers.List() {
		if err := attachProviderFinalization(ctx, registered, pending, p); err != nil {
			return err
		}
	}
	return nil
}

func attachCatalogFinalizations(ctx context.Context, catalog *providerCatalog, p *plan.Plan) error {
	if p == nil || catalog == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	pending := pendingChanges(p)
	for _, registered := range catalog.List() {
		if err := attachProviderFinalization(ctx, registered, pending, p); err != nil {
			return err
		}
	}
	return nil
}

func pendingChanges(p *plan.Plan) []provider.PendingChange {
	pending := make([]provider.PendingChange, 0, len(p.Changes))
	for _, change := range p.Changes {
		if change.Action == plan.ActionUnchanged {
			continue
		}
		pending = append(pending, provider.PendingChange{
			Address: change.Address,
			Action:  string(change.Action),
		})
	}
	return pending
}

func attachProviderFinalization(ctx context.Context, registered provider.Provider, pending []provider.PendingChange, p *plan.Plan) error {
	if registered == nil {
		return nil
	}
	finalizer, ok := registered.(provider.Finalizer)
	if !ok {
		return nil
	}
	planned, err := finalizer.PlanFinalization(ctx, pending)
	if err != nil {
		return fmt.Errorf("provider %q finalization plan: %w", registered.Name(), err)
	}
	if planned != nil {
		p.Finalizations = append(p.Finalizations, *planned)
	}
	return nil
}

// ExecuteFinalizations runs provider finalizations after all resource
// mutations and required state writes have succeeded. The returned count is
// the number of planned finalization actions that completed successfully,
// including conditional actions that rechecked converged state and became
// no-ops.
func ExecuteFinalizations(ctx context.Context, providers ProviderSet, plans []provider.FinalizationPlan, out io.Writer) (int, error) {
	if len(plans) == 0 {
		return 0, nil
	}
	if providers == nil {
		return 0, fmt.Errorf("apply: provider registry is required for finalization")
	}
	catalog := newProviderCatalog(nil, providers)
	return executeCatalogFinalizations(ctx, catalog, plans, out, false)
}

func executeCatalogFinalizations(ctx context.Context, catalog *providerCatalog, plans []provider.FinalizationPlan, out io.Writer, resourceChanges bool) (int, error) {
	if len(plans) == 0 {
		return 0, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if out == nil {
		out = io.Discard
	}
	if catalog == nil {
		return 0, fmt.Errorf("apply: provider catalog is required for finalization")
	}

	completed := 0
	remoteChanged := resourceChanges
	for _, planned := range plans {
		action := planned.Action
		if action == "" {
			action = "finalize"
		}

		registered, ok := catalog.Lookup(planned.Address.Provider)
		if !ok {
			err := fmt.Errorf("provider %q is not registered", planned.Address.Provider)
			if remoteChanged {
				return completed, finalizeError(planned.Address, action, nil, resourceChanges, err)
			}
			return completed, fmt.Errorf("apply %s: %s: %w", planned.Address, action, err)
		}
		finalizer, ok := registered.(provider.Finalizer)
		if !ok {
			err := fmt.Errorf("provider %q does not support finalization", registered.Name())
			if remoteChanged {
				return completed, finalizeError(planned.Address, action, nil, resourceChanges, err)
			}
			return completed, fmt.Errorf("apply %s: %s: %w", planned.Address, action, err)
		}

		result, err := finalizer.Finalize(ctx, planned)
		if result.Changed {
			remoteChanged = true
		}
		addr := result.Address
		if addr.Provider == "" {
			addr = planned.Address
		}
		for _, detail := range result.Details {
			fmt.Fprintf(out, "%s: %s\n", addr, detail)
		}
		if err != nil {
			if remoteChanged {
				return completed, finalizeError(planned.Address, action, result.Details, resourceChanges, err)
			}
			return completed, fmt.Errorf("apply %s: %s: %w", planned.Address, action, err)
		}
		completed++
	}
	return completed, nil
}
