package destroy

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/dziblo-music/agoraform/internal/graph"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

// Result counts confirmed teardowns and provider finalizations.
type Result struct {
	Destroyed     int
	AlreadyAbsent int
	Removed       int
	Finalized     int
	Remaining     int
}

// ProviderSet exposes registry operations required to plan and execute
// destroy finalizations. *provider.Registry implements this interface.
type ProviderSet interface {
	LookupFor(addr resource.Address) (provider.Provider, error)
	Lookup(name string) (provider.Provider, bool)
	List() []provider.Provider
}

// Approve is invoked after the destroy plan is rendered and before any
// mutation. Returning a non-nil error skips execution. A nil Approve
// auto-approves.
type Approve func(*Plan) error

// Run builds the destroy plan, attaches provider finalizations, optionally
// confirms, executes supported teardowns in reverse dependency order, then
// runs finalizations. out receives the reviewable plan, progress, and summary.
func Run(ctx context.Context, desired []resource.Resource, lookup Lookup, st Store, out io.Writer, approve Approve, providerSets ...ProviderSet) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if out == nil {
		out = io.Discard
	}
	if len(providerSets) > 1 {
		return Result{}, fmt.Errorf("destroy: at most one provider set may be supplied")
	}
	var providers ProviderSet
	if len(providerSets) == 1 {
		providers = providerSets[0]
	}
	if lookup == nil && providers == nil && len(desired) > 0 {
		return Result{}, fmt.Errorf("destroy: provider lookup is required")
	}
	if st == nil {
		return Result{}, fmt.Errorf("destroy: state store is required")
	}

	resourceLookup := lookup
	if resourceLookup == nil && providers != nil {
		resourceLookup = providers.LookupFor
	}

	planned, err := Build(ctx, desired, resourceLookup, st)
	if err != nil {
		return Result{}, err
	}
	if err := attachFinalizations(ctx, providers, resourceLookup, planned); err != nil {
		return Result{}, err
	}

	fmt.Fprint(out, Format(planned))

	if !planned.HasMutations() {
		result := Result{Remaining: planned.RemainingCount()}
		fmt.Fprint(out, FormatResult(result))
		if result.Remaining > 0 {
			return result, remainingError(planned)
		}
		return result, nil
	}

	if approve != nil {
		if err := approve(planned); err != nil {
			return Result{}, err
		}
	}

	result, err := Execute(ctx, planned, desired, resourceLookup, st, out)
	if err != nil {
		return result, err
	}
	result.Finalized, err = executeFinalizations(ctx, providers, resourceLookup, planned.Finalizations, out, result.Destroyed+result.AlreadyAbsent+result.Removed > 0)
	if err != nil {
		return result, err
	}
	if len(planned.Finalizations) > 0 {
		if err := persistCompletedChanges(planned, st); err != nil {
			return result, err
		}
	}
	result.Remaining = planned.RemainingCount()
	if result.Destroyed+result.AlreadyAbsent+result.Removed > 0 || len(planned.Finalizations) > 0 || result.Remaining > 0 {
		fmt.Fprintln(out)
	}
	fmt.Fprint(out, FormatResult(result))
	if result.Remaining > 0 {
		return result, remainingError(planned)
	}
	return result, nil
}

// Execute carries out planned destroy/remove operations in reverse dependency
// order. Provider finalizations are not executed here; callers that need the
// complete lifecycle should use Run. When the reviewed plan contains provider
// finalizations, state removal is deferred until Run confirms those
// finalizations succeeded.
func Execute(ctx context.Context, p *Plan, desired []resource.Resource, lookup Lookup, st Store, out io.Writer) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if out == nil {
		out = io.Discard
	}
	if st == nil {
		return Result{}, fmt.Errorf("destroy: state store is required")
	}
	if p == nil {
		p = &Plan{}
	}

	g, err := graph.Build(desired)
	if err != nil {
		return Result{}, fmt.Errorf("destroy: %w", err)
	}

	changes := orderChanges(p.Changes, g)
	if err := preflight(changes, lookup); err != nil {
		return Result{}, err
	}

	runtime := seedRuntime(changes)
	persistImmediately := len(p.Finalizations) == 0

	var result Result
	for _, change := range changes {
		switch change.Kind {
		case KindNotManaged, KindUnsupported, KindProviderOwned:
			continue
		case KindDestroy, KindRemove:
			desiredRes, err := resolveDesired(change.Resource, runtime)
			if err != nil {
				return result, destroyError(change.Address, string(change.Kind), err)
			}
			desiredRes.Identity = change.Identity
			status, err := executeDestroy(ctx, change, desiredRes, lookup, st, out, persistImmediately)
			if err != nil {
				return result, err
			}
			switch status {
			case provider.DestroyStatusAlreadyAbsent:
				result.AlreadyAbsent++
			case provider.DestroyStatusRemoved:
				result.Removed++
			case provider.DestroyStatusDestroyed:
				result.Destroyed++
			default:
				return result, invalidDestroyStatusError(change, status)
			}
		default:
			return result, fmt.Errorf("destroy %s: unsupported kind %q", change.Address, change.Kind)
		}
	}
	return result, nil
}

func orderChanges(changes []Change, g *graph.Graph) []Change {
	byAddr := make(map[string]Change, len(changes))
	for _, change := range changes {
		byAddr[change.Address.String()] = change
	}

	ordered := make([]Change, 0, len(changes))
	for _, addr := range g.ReverseOrder() {
		key := addr.String()
		change, ok := byAddr[key]
		if !ok {
			continue
		}
		ordered = append(ordered, change)
		delete(byAddr, key)
	}
	if len(byAddr) == 0 {
		return ordered
	}

	leftovers := make([]Change, 0, len(byAddr))
	for _, change := range byAddr {
		leftovers = append(leftovers, change)
	}
	sort.Slice(leftovers, func(i, j int) bool {
		return leftovers[i].Address.String() < leftovers[j].Address.String()
	})
	return append(ordered, leftovers...)
}

func seedRuntime(changes []Change) map[string]resource.Resolved {
	runtime := make(map[string]resource.Resolved, len(changes))
	for _, change := range changes {
		if change.Identity.IsZero() {
			continue
		}
		runtime[change.Address.String()] = resource.Resolved{
			Address:  change.Address,
			Identity: change.Identity,
		}
	}
	return runtime
}

func resolveDesired(res resource.Resource, runtime map[string]resource.Resolved) (resource.Resource, error) {
	mapped, err := resource.MapRefs(res.Attributes, func(path string, ref resource.Ref) (any, error) {
		got, ok := runtime[ref.Address.String()]
		if !ok || got.Identity.IsZero() {
			return nil, fmt.Errorf("attribute %q: dependency %s has no provider-native identity", displayPath(path), ref.Address)
		}
		if !ref.HasOutput() {
			return resource.Resolved{
				Address:  got.Address,
				Identity: got.Identity,
				Outputs:  got.Outputs.Clone(),
			}, nil
		}
		if val, ok := got.Select(ref.Output); ok {
			return val, nil
		}
		// Destroy uses identity, not output values. Keep the logical
		// selector when the named output is unavailable.
		return ref, nil
	})
	if err != nil {
		return resource.Resource{}, err
	}
	switch attrs := mapped.(type) {
	case nil:
		res.Attributes = nil
	case resource.Attributes:
		res.Attributes = attrs
	default:
		return resource.Resource{}, fmt.Errorf("internal: resolved attributes have type %T", mapped)
	}
	return res, nil
}

func displayPath(path string) string {
	if path == "" {
		return "(attributes)"
	}
	return path
}

func preflight(changes []Change, lookup Lookup) error {
	mutating := 0
	for _, change := range changes {
		switch change.Kind {
		case KindNotManaged, KindUnsupported, KindProviderOwned, "":
			continue
		case KindDestroy, KindRemove:
			if change.Identity.IsZero() {
				return fmt.Errorf("destroy %s: %s: missing planned identity", change.Address, change.Kind)
			}
			mutating++
		default:
			return fmt.Errorf("destroy %s: unsupported kind %q", change.Address, change.Kind)
		}
	}
	if mutating > 0 && lookup == nil {
		return fmt.Errorf("destroy: provider lookup is required")
	}
	return nil
}

func executeDestroy(ctx context.Context, change Change, desired resource.Resource, lookup Lookup, st Store, out io.Writer, persist bool) (provider.DestroyStatus, error) {
	addr := change.Address
	op := string(change.Kind)
	progress := "destroying"
	if change.Kind == KindRemove {
		progress = "removing"
	}
	fmt.Fprintf(out, "%s: %s...\n", addr, progress)

	p, err := lookup(addr)
	if err != nil {
		return "", destroyError(addr, op, err)
	}
	if p == nil {
		return "", destroyError(addr, op, fmt.Errorf("provider is nil"))
	}
	d, ok := p.(provider.Destroyer)
	if !ok {
		return "", destroyError(addr, op, fmt.Errorf("provider %q does not support destroy", p.Name()))
	}

	result, err := d.Destroy(ctx, desired)
	if err != nil {
		return "", destroyError(addr, op, err)
	}
	status := result.Status
	switch status {
	case provider.DestroyStatusAlreadyAbsent:
		fmt.Fprintf(out, "%s: already absent\n", addr)
	case provider.DestroyStatusRemoved:
		fmt.Fprintf(out, "%s: removed\n", addr)
	case provider.DestroyStatusDestroyed:
		fmt.Fprintf(out, "%s: destroyed\n", addr)
	default:
		return "", invalidDestroyStatusError(change, status)
	}

	if persist {
		if err := st.Remove(addr); err != nil {
			return "", persistDestroyError(addr, change.Identity, err)
		}
	}
	return status, nil
}

func persistCompletedChanges(p *Plan, st Store) error {
	if p == nil {
		return nil
	}
	for _, change := range p.Changes {
		switch change.Kind {
		case KindDestroy, KindRemove:
			if err := st.Remove(change.Address); err != nil {
				return persistDestroyError(change.Address, change.Identity, err)
			}
		}
	}
	return nil
}

func invalidDestroyStatusError(change Change, status provider.DestroyStatus) error {
	return &PartialError{
		Address:         change.Address,
		Operation:       string(change.Kind),
		RemoteIdentity:  change.Identity,
		Stage:           StageMutation,
		ResourceChanges: true,
		Err: fmt.Errorf(
			"destroy %s: %s: provider returned invalid destroy status %q; local state was preserved",
			change.Address,
			change.Kind,
			status,
		),
	}
}

func destroyError(addr resource.Address, op string, err error) error {
	return fmt.Errorf("destroy %s: %s: %w", addr, op, err)
}
