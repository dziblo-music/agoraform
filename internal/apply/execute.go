package apply

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/dziblo-music/agoraform/internal/graph"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

// Lookup resolves the mutating provider for a resource address.
type Lookup func(addr resource.Address) (provider.Provider, error)

// Store persists provider-native identities after successful mutations.
//
// Implementations must not write a new identity for a failed mutation.
type Store interface {
	RecordCreate(addr resource.Address, live resource.RemoteResource) error
	RecordUpdate(addr resource.Address, live resource.RemoteResource) error
}

// Persistence is local identity state used both to plan and to record
// successful mutations. *state.Store implements this interface.
type Persistence interface {
	plan.Identities
	Store
}

// Result counts fully committed resource mutations and provider finalizations.
// It is only meaningful when Execute or Run returns a nil error. Execute is a
// resource-CRUD primitive and therefore leaves Finalized at zero.
type Result struct {
	Created   int
	Updated   int
	Finalized int
}

// Run is the canonical high-level apply lifecycle. It builds the resource
// plan, attaches provider finalizations, executes resource mutations, then runs
// those finalizations only after every resource mutation and state write
// succeeds.
//
// Validation, remote reads, identity-guard checks, and dependency-graph
// checks happen inside plan.BuildWithState before any mutation. Successful
// creates and updates persist identities through st. Provider finalization
// planning is non-mutating and happens before resource execution. On success,
// Run writes human-readable progress and Format(result) to out.
func Run(ctx context.Context, desired []resource.Resource, providers ProviderSet, st Persistence, out io.Writer) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if out == nil {
		out = io.Discard
	}
	if providers == nil && len(desired) > 0 {
		return Result{}, fmt.Errorf("apply: provider registry is required")
	}
	if st == nil {
		return Result{}, fmt.Errorf("apply: state store is required")
	}

	var lookup Lookup
	if providers != nil {
		lookup = providers.LookupFor
	}

	planned, err := plan.BuildWithState(ctx, desired, func(addr resource.Address) (provider.Reader, error) {
		if providers == nil {
			return nil, fmt.Errorf("provider registry is required")
		}
		return providers.LookupFor(addr)
	}, st)
	if err != nil {
		return Result{}, err
	}
	if err := AttachFinalizations(ctx, providers, planned); err != nil {
		return Result{}, err
	}

	result, err := Execute(ctx, planned, desired, lookup, st, out)
	if err != nil {
		return result, err
	}
	result.Finalized, err = ExecuteFinalizations(ctx, providers, planned.Finalizations, out)
	if err != nil {
		return result, err
	}
	if result.Created+result.Updated > 0 || len(planned.Finalizations) > 0 {
		fmt.Fprintln(out)
	}
	fmt.Fprint(out, Format(result))
	return result, nil
}

// Execute carries out resource actions in p in deterministic dependency order.
// Provider finalizations are intentionally not executed here; callers that
// need the complete apply lifecycle should use Run.
//
// It builds the resource dependency graph from desired and fails before any
// mutation when the graph is invalid. Create and update operations run
// sequentially: prerequisites first, with address order as the tie-breaker
// among unrelated resources. Immediately before each mutation, explicit
// resource.Ref values are replaced with resource.Resolved bindings from
// earlier operations, or from the planned identity and computed outputs
// for unchanged prerequisites. Providers translate those bindings into
// native API values.
//
// It does not recompute diffs. For updates, it re-reads the identity-bound
// live resource so Provider.Update receives the complete RemoteResource,
// including computed fields, rather than the plan's comparable Before view.
// Unsupported actions are rejected before any mutation. Execution stops at
// the first provider or state-write failure. Progress lines are written to
// out; the Apply complete summary is not.
func Execute(ctx context.Context, p *plan.Plan, desired []resource.Resource, lookup Lookup, st Store, out io.Writer) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if out == nil {
		out = io.Discard
	}
	if st == nil {
		return Result{}, fmt.Errorf("apply: state store is required")
	}
	if p == nil {
		p = &plan.Plan{}
	}

	byAddr, err := desiredByAddress(desired)
	if err != nil {
		return Result{}, err
	}

	g, err := graph.Build(desired)
	if err != nil {
		return Result{}, fmt.Errorf("apply: %w", err)
	}

	changes := orderChanges(p.Changes, g)
	if err := preflight(changes, byAddr, lookup); err != nil {
		return Result{}, err
	}

	runtime := seedRuntime(changes)

	var result Result
	for _, change := range changes {
		switch change.Action {
		case plan.ActionUnchanged:
			continue
		case plan.ActionCreate:
			desiredRes, err := resolveDesired(byAddr[change.Address.String()], runtime)
			if err != nil {
				return result, applyError(change.Address, "create", err)
			}
			live, err := executeCreate(ctx, change, desiredRes, lookup, st, out)
			if err != nil {
				return result, err
			}
			remember(runtime, change.Address, live, resource.Identity{})
			result.Created++
		case plan.ActionUpdate:
			desiredRes, err := resolveDesired(byAddr[change.Address.String()], runtime)
			if err != nil {
				return result, applyError(change.Address, "update", err)
			}
			live, err := executeUpdate(ctx, change, desiredRes, lookup, st, out)
			if err != nil {
				return result, err
			}
			remember(runtime, change.Address, live, change.Identity)
			result.Updated++
		default:
			return result, unsupportedActionError(change)
		}
	}
	return result, nil
}

func desiredByAddress(desired []resource.Resource) (map[string]resource.Resource, error) {
	out := make(map[string]resource.Resource, len(desired))
	for _, res := range desired {
		key := res.Address.String()
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("apply: duplicate desired resource %s", res.Address)
		}
		out[key] = res
	}
	return out, nil
}

func orderChanges(changes []plan.Change, g *graph.Graph) []plan.Change {
	byAddr := make(map[string]plan.Change, len(changes))
	for _, change := range changes {
		byAddr[change.Address.String()] = change
	}

	ordered := make([]plan.Change, 0, len(changes))
	for _, addr := range g.Order() {
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

	leftovers := make([]plan.Change, 0, len(byAddr))
	for _, change := range byAddr {
		leftovers = append(leftovers, change)
	}
	sort.Slice(leftovers, func(i, j int) bool {
		return leftovers[i].Address.String() < leftovers[j].Address.String()
	})
	return append(ordered, leftovers...)
}

func seedRuntime(changes []plan.Change) map[string]resource.Resolved {
	runtime := make(map[string]resource.Resolved, len(changes))
	for _, change := range changes {
		if change.Identity.IsZero() {
			continue
		}
		runtime[change.Address.String()] = resource.Resolved{
			Address:  change.Address,
			Identity: change.Identity,
			Outputs:  change.Computed.Clone(),
		}
	}
	return runtime
}

func remember(runtime map[string]resource.Resolved, addr resource.Address, live resource.RemoteResource, fallback resource.Identity) {
	id := live.Identity
	if id.IsZero() {
		id = fallback
	}
	if id.IsZero() {
		return
	}
	runtime[addr.String()] = resource.Resolved{
		Address:  addr,
		Identity: id,
		Outputs:  live.Computed.Clone(),
	}
}

func resolveDesired(res resource.Resource, runtime map[string]resource.Resolved) (resource.Resource, error) {
	mapped, err := resource.MapRefs(res.Attributes, func(path string, ref resource.Ref) (any, error) {
		got, ok := runtime[ref.Address.String()]
		if !ok || got.Identity.IsZero() {
			return nil, fmt.Errorf("attribute %q: dependency %s has no provider-native identity", displayPath(path), ref.Address)
		}
		return resource.Resolved{
			Address:  got.Address,
			Identity: got.Identity,
			Outputs:  got.Outputs.Clone(),
		}, nil
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

func preflight(changes []plan.Change, byAddr map[string]resource.Resource, lookup Lookup) error {
	mutating := 0
	for _, change := range changes {
		switch change.Action {
		case plan.ActionUnchanged:
			continue
		case plan.ActionCreate, plan.ActionUpdate:
			if _, ok := byAddr[change.Address.String()]; !ok {
				return fmt.Errorf("apply %s: %s: desired resource is missing", change.Address, change.Action)
			}
			mutating++
		default:
			return unsupportedActionError(change)
		}
	}
	if mutating > 0 && lookup == nil {
		return fmt.Errorf("apply: provider lookup is required")
	}
	return nil
}

func executeCreate(ctx context.Context, change plan.Change, desired resource.Resource, lookup Lookup, st Store, out io.Writer) (resource.RemoteResource, error) {
	addr := change.Address
	fmt.Fprintf(out, "%s: creating...\n", addr)

	p, err := lookup(addr)
	if err != nil {
		return resource.RemoteResource{}, applyError(addr, "create", err)
	}
	if p == nil {
		return resource.RemoteResource{}, applyError(addr, "create", fmt.Errorf("provider is nil"))
	}

	desired.Identity = resource.Identity{}
	live, err := p.Create(ctx, desired)
	if err != nil {
		return resource.RemoteResource{}, applyError(addr, "create", err)
	}
	if live.Identity.IsZero() {
		return resource.RemoteResource{}, applyError(addr, "create", fmt.Errorf("provider returned no identity"))
	}

	fmt.Fprintf(out, "%s: created\n", addr)

	if err := st.RecordCreate(addr, live); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("apply %s: create succeeded but could not persist identity: %w", addr, err)
	}
	return live, nil
}

func executeUpdate(ctx context.Context, change plan.Change, desired resource.Resource, lookup Lookup, st Store, out io.Writer) (resource.RemoteResource, error) {
	addr := change.Address
	fmt.Fprintf(out, "%s: updating...\n", addr)

	p, err := lookup(addr)
	if err != nil {
		return resource.RemoteResource{}, applyError(addr, "update", err)
	}
	if p == nil {
		return resource.RemoteResource{}, applyError(addr, "update", fmt.Errorf("provider is nil"))
	}
	if change.Identity.IsZero() {
		return resource.RemoteResource{}, applyError(addr, "update", fmt.Errorf("missing planned identity"))
	}

	// Refresh the complete identity-bound live resource immediately before
	// mutation. Plan.Change.Before is a comparable attribute view and must
	// not be reconstructed as Provider.Update's actual argument.
	desired.Identity = change.Identity
	actual, err := p.Read(ctx, desired)
	if err != nil {
		return resource.RemoteResource{}, applyError(addr, "update", fmt.Errorf("read live resource: %w", err))
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, applyError(addr, "update", fmt.Errorf("read returned no identity"))
	}
	if actual.Identity.ID != change.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("apply %s: update: read returned identity %q for persisted identity %q; refusing to rebind managed resource", addr, actual.Identity.ID, change.Identity.ID)
	}

	live, err := p.Update(ctx, desired, actual)
	if err != nil {
		return resource.RemoteResource{}, applyError(addr, "update", err)
	}
	if !live.Identity.IsZero() && live.Identity.ID != change.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("apply %s: update: provider returned identity %q for persisted identity %q; refusing to rebind managed resource", addr, live.Identity.ID, change.Identity.ID)
	}

	fmt.Fprintf(out, "%s: updated\n", addr)

	if err := st.RecordUpdate(addr, live); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("apply %s: update succeeded but could not persist identity: %w", addr, err)
	}
	return live, nil
}

func unsupportedActionError(change plan.Change) error {
	action := string(change.Action)
	if action == "" {
		action = "(empty)"
	}
	return fmt.Errorf("apply %s: unsupported action %q", change.Address, action)
}

func applyError(addr resource.Address, op string, err error) error {
	return fmt.Errorf("apply %s: %s: %w", addr, op, err)
}

// Format renders a successful apply result as deterministic terminal text.
func Format(r Result) string {
	if r.Finalized == 0 {
		return fmt.Sprintf("Apply complete! %d created, %d updated.\n", r.Created, r.Updated)
	}
	actionLabel := "provider actions"
	if r.Finalized == 1 {
		actionLabel = "provider action"
	}
	return fmt.Sprintf("Apply complete! %d created, %d updated, %d %s completed.\n", r.Created, r.Updated, r.Finalized, actionLabel)
}
