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

// Result counts fully committed mutations. It is only meaningful when
// Execute or Run returns a nil error.
type Result struct {
	Created int
	Updated int
}

// Run builds a plan with the shared reconciliation engine, then executes it.
//
// Validation, remote reads, and identity-guard checks happen inside
// plan.BuildWithState before any mutation. Successful creates and updates
// persist identities through st. On success, Run writes human-readable
// progress and Format(result) to out.
func Run(ctx context.Context, desired []resource.Resource, lookup Lookup, st Persistence, out io.Writer) (Result, error) {
	if out == nil {
		out = io.Discard
	}
	if lookup == nil && len(desired) > 0 {
		return Result{}, fmt.Errorf("apply: provider lookup is required")
	}
	if st == nil {
		return Result{}, fmt.Errorf("apply: state store is required")
	}

	planned, err := plan.BuildWithState(ctx, desired, func(addr resource.Address) (provider.Reader, error) {
		if lookup == nil {
			return nil, fmt.Errorf("provider lookup is required")
		}
		return lookup(addr)
	}, st)
	if err != nil {
		return Result{}, err
	}

	result, err := Execute(ctx, planned, desired, lookup, st, out)
	if err != nil {
		return result, err
	}
	if result.Created+result.Updated > 0 {
		fmt.Fprintln(out)
	}
	fmt.Fprint(out, Format(result))
	return result, nil
}

// Execute carries out the actions in p in deterministic address order.
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

	changes := append([]plan.Change(nil), p.Changes...)
	sort.SliceStable(changes, func(i, j int) bool {
		return changes[i].Address.String() < changes[j].Address.String()
	})

	if err := preflight(changes, byAddr, lookup); err != nil {
		return Result{}, err
	}

	var result Result
	for _, change := range changes {
		switch change.Action {
		case plan.ActionUnchanged:
			continue
		case plan.ActionCreate:
			if err := executeCreate(ctx, change, byAddr[change.Address.String()], lookup, st, out); err != nil {
				return result, err
			}
			result.Created++
		case plan.ActionUpdate:
			if err := executeUpdate(ctx, change, byAddr[change.Address.String()], lookup, st, out); err != nil {
				return result, err
			}
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

func executeCreate(ctx context.Context, change plan.Change, desired resource.Resource, lookup Lookup, st Store, out io.Writer) error {
	addr := change.Address
	fmt.Fprintf(out, "%s: creating...\n", addr)

	p, err := lookup(addr)
	if err != nil {
		return applyError(addr, "create", err)
	}
	if p == nil {
		return applyError(addr, "create", fmt.Errorf("provider is nil"))
	}

	desired.Identity = resource.Identity{}
	live, err := p.Create(ctx, desired)
	if err != nil {
		return applyError(addr, "create", err)
	}
	if live.Identity.IsZero() {
		return applyError(addr, "create", fmt.Errorf("provider returned no identity"))
	}

	fmt.Fprintf(out, "%s: created\n", addr)

	if err := st.RecordCreate(addr, live); err != nil {
		return fmt.Errorf("apply %s: create succeeded but could not persist identity: %w", addr, err)
	}
	return nil
}

func executeUpdate(ctx context.Context, change plan.Change, desired resource.Resource, lookup Lookup, st Store, out io.Writer) error {
	addr := change.Address
	fmt.Fprintf(out, "%s: updating...\n", addr)

	p, err := lookup(addr)
	if err != nil {
		return applyError(addr, "update", err)
	}
	if p == nil {
		return applyError(addr, "update", fmt.Errorf("provider is nil"))
	}
	if change.Identity.IsZero() {
		return applyError(addr, "update", fmt.Errorf("missing planned identity"))
	}

	// Refresh the complete identity-bound live resource immediately before
	// mutation. Plan.Change.Before is a comparable attribute view and must
	// not be reconstructed as Provider.Update's actual argument.
	desired.Identity = change.Identity
	actual, err := p.Read(ctx, desired)
	if err != nil {
		return applyError(addr, "update", fmt.Errorf("read live resource: %w", err))
	}
	if actual.Identity.IsZero() {
		return applyError(addr, "update", fmt.Errorf("read returned no identity"))
	}
	if actual.Identity.ID != change.Identity.ID {
		return fmt.Errorf("apply %s: update: read returned identity %q for persisted identity %q; refusing to rebind managed resource", addr, actual.Identity.ID, change.Identity.ID)
	}

	live, err := p.Update(ctx, desired, actual)
	if err != nil {
		return applyError(addr, "update", err)
	}
	if !live.Identity.IsZero() && live.Identity.ID != change.Identity.ID {
		return fmt.Errorf("apply %s: update: provider returned identity %q for persisted identity %q; refusing to rebind managed resource", addr, live.Identity.ID, change.Identity.ID)
	}

	fmt.Fprintf(out, "%s: updated\n", addr)

	if err := st.RecordUpdate(addr, live); err != nil {
		return fmt.Errorf("apply %s: update succeeded but could not persist identity: %w", addr, err)
	}
	return nil
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
	return fmt.Sprintf("Apply complete! %d created, %d updated.\n", r.Created, r.Updated)
}
