package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
)

func attachFinalizations(ctx context.Context, reg *provider.Registry, p *plan.Plan) error {
	if p == nil || reg == nil {
		return nil
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

	for _, registered := range reg.List() {
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

func executeFinalizations(ctx context.Context, reg *provider.Registry, plans []provider.FinalizationPlan, out io.Writer) error {
	if len(plans) == 0 {
		return nil
	}
	if out == nil {
		out = io.Discard
	}
	if reg == nil {
		return fmt.Errorf("apply: provider registry is required for finalization")
	}

	for _, planned := range plans {
		registered, ok := reg.Lookup(planned.Address.Provider)
		if !ok {
			return fmt.Errorf("apply %s: provider %q is not registered", planned.Address, planned.Address.Provider)
		}
		finalizer, ok := registered.(provider.Finalizer)
		if !ok {
			return fmt.Errorf("apply %s: provider %q does not support finalization", planned.Address, registered.Name())
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
			return fmt.Errorf("apply %s: %s: %w", planned.Address, action, err)
		}
	}
	return nil
}
