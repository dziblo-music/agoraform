package apply

import (
	"context"
	"fmt"
	"io"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

// ProviderSet exposes the provider registry operations required by the
// canonical apply lifecycle. *provider.Registry implements this interface.
type ProviderSet interface {
	LookupFor(addr resource.Address) (provider.Provider, error)
	Lookup(name string) (provider.Provider, bool)
	List() []provider.Provider
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

	for _, registered := range providers.List() {
		if registered == nil {
			continue
		}
		finalizer, ok := registered.(provider.Finalizer)
		if !ok {
			continue
		}
		planned, err := finalizer.PlanFinalization(ctx, pending)
		if err != nil {
			return fmt.Errorf("provider %q finalization plan: %w", registered.Name(), err)
		}
		if planned != nil {
			p.Finalizations = append(p.Finalizations, *planned)
		}
	}
	return nil
}

// ExecuteFinalizations runs provider finalizations after all resource
// mutations and required state writes have succeeded. The returned count is
// the number of finalizers that reported a provider-side state change.
func ExecuteFinalizations(ctx context.Context, providers ProviderSet, plans []provider.FinalizationPlan, out io.Writer) (int, error) {
	if len(plans) == 0 {
		return 0, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if out == nil {
		out = io.Discard
	}
	if providers == nil {
		return 0, fmt.Errorf("apply: provider registry is required for finalization")
	}

	changed := 0
	for _, planned := range plans {
		registered, ok := providers.Lookup(planned.Address.Provider)
		if !ok {
			return changed, fmt.Errorf("apply %s: provider %q is not registered", planned.Address, planned.Address.Provider)
		}
		finalizer, ok := registered.(provider.Finalizer)
		if !ok {
			return changed, fmt.Errorf("apply %s: provider %q does not support finalization", planned.Address, registered.Name())
		}

		result, err := finalizer.Finalize(ctx, planned)
		addr := result.Address
		if addr.Provider == "" {
			addr = planned.Address
		}
		for _, detail := range result.Details {
			fmt.Fprintf(out, "%s: %s\n", addr, detail)
		}
		if err != nil {
			action := planned.Action
			if action == "" {
				action = "finalize"
			}
			return changed, fmt.Errorf("apply %s: %s: %w", planned.Address, action, err)
		}
		if result.Changed {
			changed++
		}
	}
	return changed, nil
}
