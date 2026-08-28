package destroy

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func attachFinalizations(ctx context.Context, providers ProviderSet, lookup Lookup, p *Plan) error {
	if p == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	pending := pendingChanges(p)
	seen := make(map[string]struct{})
	var list []provider.Provider
	if providers != nil {
		list = providers.List()
	} else if lookup != nil {
		for _, change := range p.Changes {
			prov, err := lookup(change.Address)
			if err != nil || prov == nil {
				continue
			}
			if _, ok := seen[prov.Name()]; ok {
				continue
			}
			seen[prov.Name()] = struct{}{}
			list = append(list, prov)
		}
		sort.Slice(list, func(i, j int) bool {
			return list[i].Name() < list[j].Name()
		})
	}

	for _, registered := range list {
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

func pendingChanges(p *Plan) []provider.PendingChange {
	pending := make([]provider.PendingChange, 0, len(p.Changes))
	for _, change := range p.Changes {
		switch change.Kind {
		case KindDestroy, KindRemove:
			pending = append(pending, provider.PendingChange{
				Address: change.Address,
				Action:  string(change.Kind),
			})
		}
	}
	return pending
}

func executeFinalizations(ctx context.Context, providers ProviderSet, lookup Lookup, plans []provider.FinalizationPlan, out io.Writer, resourceChanges bool) (int, error) {
	if len(plans) == 0 {
		return 0, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if out == nil {
		out = io.Discard
	}

	completed := 0
	remoteChanged := resourceChanges
	for _, planned := range plans {
		action := planned.Action
		if action == "" {
			action = "finalize"
		}

		registered, err := lookupFinalizer(providers, lookup, planned.Address)
		if err != nil {
			if remoteChanged {
				return completed, finalizeError(planned.Address, action, nil, resourceChanges, err)
			}
			return completed, fmt.Errorf("destroy %s: %s: %w", planned.Address, action, err)
		}
		finalizer, ok := registered.(provider.Finalizer)
		if !ok {
			err := fmt.Errorf("provider %q does not support finalization", registered.Name())
			if remoteChanged {
				return completed, finalizeError(planned.Address, action, nil, resourceChanges, err)
			}
			return completed, fmt.Errorf("destroy %s: %s: %w", planned.Address, action, err)
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
			return completed, fmt.Errorf("destroy %s: %s: %w", planned.Address, action, err)
		}
		completed++
	}
	return completed, nil
}

func lookupFinalizer(providers ProviderSet, lookup Lookup, addr resource.Address) (provider.Provider, error) {
	if providers != nil {
		p, ok := providers.Lookup(addr.Provider)
		if ok {
			return p, nil
		}
		return nil, fmt.Errorf("provider %q is not registered", addr.Provider)
	}
	if lookup == nil {
		return nil, fmt.Errorf("provider lookup is required")
	}
	p, err := lookup(addr)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, fmt.Errorf("provider %q is not registered", addr.Provider)
	}
	return p, nil
}
