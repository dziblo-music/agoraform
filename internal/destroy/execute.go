package destroy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/dziblo-music/agoraform/internal/graph"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

// Result counts confirmed teardowns, provider finalizations, and resources
// that stayed in state because they failed or were blocked by a failure.
type Result struct {
	Destroyed        int
	AlreadyAbsent    int
	Removed          int
	ExternalReleased int
	Finalized        int
	Remaining        int
	Failed           int
	Skipped          int
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
//
// A provider failure does not stop the run. The failed resource and every
// resource it depends on keep their state bindings, and teardown continues
// for unrelated resources. A provider's finalization runs only when all of
// that provider's planned changes completed.
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
		if planned.ExternalCount() > 0 {
			released, err := Execute(ctx, planned, desired, resourceLookup, st, out)
			if err != nil {
				return released, err
			}
			result.ExternalReleased = released.ExternalReleased
		}
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

	run, err := execute(ctx, planned, desired, resourceLookup, st, out)
	result := run.result
	if err != nil {
		// Keep the failures recorded before the run stopped.
		return result, errors.Join(run.incompleteError(nil), err)
	}

	finalized, err := executeFinalizations(ctx, providers, resourceLookup, planned.Finalizations, out, run)
	result.Finalized = finalized.completed
	if persistErr := persistCompletedChanges(run, finalized.succeeded, st); persistErr != nil {
		return result, errors.Join(err, persistErr)
	}
	if err != nil {
		return result, errors.Join(run.incompleteError(finalized.skipped), err)
	}

	result.Remaining = planned.RemainingCount()
	if result.Destroyed+result.AlreadyAbsent+result.Removed+result.Failed+result.Skipped > 0 || len(planned.Finalizations) > 0 || result.Remaining > 0 {
		fmt.Fprintln(out)
	}
	fmt.Fprint(out, FormatResult(result))
	if incomplete := run.incompleteError(finalized.skipped); incomplete != nil {
		return result, incomplete
	}
	if result.Remaining > 0 {
		return result, remainingError(planned)
	}
	return result, nil
}

// Execute carries out planned destroy/remove operations in reverse dependency
// order. Provider finalizations are not executed here; callers that need the
// complete lifecycle should use Run. When the reviewed plan contains a
// provider finalization, state removal for that provider's resources is
// deferred until Run confirms the finalization succeeded.
//
// A provider failure is recorded and execution continues with resources that
// do not depend on the failed one. The returned error is an *IncompleteError
// when any resource failed or was skipped. A local state write failure or
// context cancellation stops execution immediately; the returned error then
// joins that cause with any failures already recorded.
func Execute(ctx context.Context, p *Plan, desired []resource.Resource, lookup Lookup, st Store, out io.Writer) (Result, error) {
	run, err := execute(ctx, p, desired, lookup, st, out)
	if err != nil {
		return run.result, errors.Join(run.incompleteError(nil), err)
	}
	if incomplete := run.incompleteError(nil); incomplete != nil {
		return run.result, incomplete
	}
	return run.result, nil
}

// execution records per-resource outcomes so Run can decide which provider
// finalizations may run and which deferred bindings may be removed.
type execution struct {
	result    Result
	completed []Change
	failures  []Failure
	skipped   []Skipped
	// incomplete holds provider names with a failed or skipped change.
	incomplete map[string]struct{}
}

func (e *execution) providerComplete(name string) bool {
	_, bad := e.incomplete[name]
	return !bad
}

func (e *execution) incompleteError(skippedFinalizations []provider.FinalizationPlan) error {
	if len(e.failures) == 0 && len(e.skipped) == 0 && len(skippedFinalizations) == 0 {
		return nil
	}
	return &IncompleteError{
		Failures:             append([]Failure(nil), e.failures...),
		Skipped:              append([]Skipped(nil), e.skipped...),
		SkippedFinalizations: append([]provider.FinalizationPlan(nil), skippedFinalizations...),
	}
}

func execute(ctx context.Context, p *Plan, desired []resource.Resource, lookup Lookup, st Store, out io.Writer) (*execution, error) {
	run := &execution{incomplete: map[string]struct{}{}}
	if ctx == nil {
		ctx = context.Background()
	}
	if out == nil {
		out = io.Discard
	}
	if st == nil {
		return run, fmt.Errorf("destroy: state store is required")
	}
	if p == nil {
		p = &Plan{}
	}

	g, err := graph.Build(desired)
	if err != nil {
		return run, fmt.Errorf("destroy: %w", err)
	}

	changes := orderChanges(p.Changes, g)
	if err := preflight(changes, lookup); err != nil {
		return run, err
	}

	runtime := seedRuntime(changes)
	deferred := finalizingProviders(p.Finalizations)

	// blockedBy maps a prerequisite to the dependent whose failure keeps it
	// in place. Reverse dependency order guarantees every dependent is
	// visited before its prerequisites, so the map is final on arrival.
	blockedBy := map[string]resource.Address{}
	block := func(failed resource.Address) {
		var walk func(resource.Address)
		walk = func(addr resource.Address) {
			for _, dep := range g.Dependencies(addr) {
				key := dep.String()
				if _, seen := blockedBy[key]; seen {
					continue
				}
				blockedBy[key] = failed
				walk(dep)
			}
		}
		walk(failed)
	}

	for _, change := range changes {
		if err := ctx.Err(); err != nil {
			return run, err
		}
		switch change.Kind {
		case KindNotManaged, KindUnsupported, KindProviderOwned:
			continue
		case KindExternal, KindDestroy, KindRemove:
		default:
			return run, fmt.Errorf("destroy %s: unsupported kind %q", change.Address, change.Kind)
		}

		if cause, ok := blockedBy[change.Address.String()]; ok {
			fmt.Fprintf(out, "%s: skipped; %s depends on it and was not destroyed\n", change.Address, cause)
			run.skipped = append(run.skipped, Skipped{Address: change.Address, Cause: cause})
			run.incomplete[change.Address.Provider] = struct{}{}
			run.result.Skipped++
			continue
		}

		if change.Kind == KindExternal {
			if err := releaseExternal(change, st, out); err != nil {
				return run, err
			}
			run.result.ExternalReleased++
			continue
		}

		status, err := destroyOne(ctx, change, runtime, lookup, out)
		if err != nil {
			fmt.Fprintf(out, "%s: failed: %v\n", change.Address, err)
			run.failures = append(run.failures, Failure{Address: change.Address, Err: err})
			run.incomplete[change.Address.Provider] = struct{}{}
			run.result.Failed++
			block(change.Address)
			continue
		}
		if _, isDeferred := deferred[change.Address.Provider]; !isDeferred {
			if err := st.Remove(change.Address); err != nil {
				return run, persistDestroyError(change.Address, change.Identity, err)
			}
		}
		run.completed = append(run.completed, change)
		switch status {
		case provider.DestroyStatusAlreadyAbsent:
			run.result.AlreadyAbsent++
		case provider.DestroyStatusRemoved:
			run.result.Removed++
		case provider.DestroyStatusDestroyed:
			run.result.Destroyed++
		}
	}
	return run, nil
}

func finalizingProviders(plans []provider.FinalizationPlan) map[string]struct{} {
	out := make(map[string]struct{}, len(plans))
	for _, f := range plans {
		out[f.Address.Provider] = struct{}{}
	}
	return out
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
			// Destroy is identity-driven. A referenced resource may be
			// intentionally unmanaged or unsupported while the current
			// resource is still safe to tear down. Preserve the logical
			// reference and let the provider decide whether that attribute is
			// actually required for destruction.
			return ref, nil
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
		case KindNotManaged, KindUnsupported, KindProviderOwned, KindExternal, "":
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

func releaseExternal(change Change, st Store, out io.Writer) error {
	fmt.Fprintf(out, "%s: releasing local external binding (remote object unchanged)...\n", change.Address)
	if err := st.Remove(change.Address); err != nil {
		return destroyError(change.Address, string(KindExternal), err)
	}
	return nil
}

// destroyOne asks the provider to tear down one resource. It never touches
// local state; the caller decides whether the binding is removed now or
// after provider finalization.
func destroyOne(ctx context.Context, change Change, runtime map[string]resource.Resolved, lookup Lookup, out io.Writer) (provider.DestroyStatus, error) {
	addr := change.Address
	op := string(change.Kind)
	progress := "destroying"
	if change.Kind == KindRemove {
		progress = "removing"
	}
	fmt.Fprintf(out, "%s: %s...\n", addr, progress)

	desired, err := resolveDesired(change.Resource, runtime)
	if err != nil {
		return "", destroyError(addr, op, err)
	}
	desired.Identity = change.Identity

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
	return status, nil
}

// persistCompletedChanges removes deferred bindings for providers whose
// finalization succeeded. Bindings for providers whose finalization was
// skipped or failed stay in state so retry can confirm them as already absent
// and plan the provider action again.
func persistCompletedChanges(run *execution, finalized map[string]struct{}, st Store) error {
	for _, change := range run.completed {
		if _, ok := finalized[change.Address.Provider]; !ok {
			continue
		}
		if err := st.Remove(change.Address); err != nil {
			return persistDestroyError(change.Address, change.Identity, err)
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
