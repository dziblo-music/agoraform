package plan

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dziblo-music/agoraform/internal/graph"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
)

// Lookup resolves the read-only provider for a resource address.
//
// The function returns provider.Reader so callers cannot pass mutation
// methods into the planner.
type Lookup func(addr resource.Address) (provider.Reader, error)

// Identities looks up persisted provider-native identities.
//
// A nil Identities value is treated as empty. Implementations must treat
// identities as opaque strings and must not interpret provider-specific
// field names.
type Identities interface {
	Identity(addr resource.Address) (resource.Identity, bool, error)
}

// Build compares desired resources with provider-reported live state.
//
// Missing remote resources become creates unless the provider declares a
// provider-created lifecycle such as adoption. Configurable differences become
// updates. Computed/read-only fields are ignored. Resources are read in
// deterministic dependency order (prerequisites first) so providers can
// observe prerequisite identities while normalizing dependents. Display
// order remains address order in Format. Build never invokes Create,
// Update, or Import.
func Build(ctx context.Context, desired []resource.Resource, lookup Lookup) (*Plan, error) {
	return BuildWithState(ctx, desired, lookup, nil)
}

// BuildWithState is Build plus persisted identity bindings.
//
// Resource references are validated as a dependency graph before any
// remote read. Provider resource-set validation then checks cross-resource
// provider invariants. Reads follow the graph's prerequisite-first order.
// When identities contains a binding, that identity is attached to the
// desired resource before Validate/Read. A bound identity that is missing
// remotely is a stale-state error, not a create. A provider must also
// return the same identity it was asked to resolve; core rejects
// mismatches rather than allowing a mutable discovery field to rebind the
// logical resource.
func BuildWithState(ctx context.Context, desired []resource.Resource, lookup Lookup, identities Identities) (*Plan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	g, err := graph.Build(desired)
	if err != nil {
		return nil, fmt.Errorf("plan: %w", err)
	}
	if lookup == nil && len(desired) > 0 {
		return nil, fmt.Errorf("plan: provider lookup is required")
	}
	if err := validateProviderResourceSets(ctx, desired, lookup); err != nil {
		return nil, err
	}
	if err := provider.ValidateOutputRefs(desired, lookup); err != nil {
		return nil, fmt.Errorf("plan: %w", err)
	}

	byAddr := make(map[string]resource.Resource, len(desired))
	for _, res := range desired {
		byAddr[res.Address.String()] = res
	}

	knownOutputs := make(map[string]resource.Attributes, len(desired))
	seenExternal := make(map[string]string)
	changes := make([]Change, 0, len(desired))
	for _, addr := range g.Order() {
		res := byAddr[addr.String()]
		change, err := planResource(ctx, res, lookup, identities, knownOutputs, seenExternal)
		if err != nil {
			return nil, err
		}
		// Only outputs from unchanged or external prerequisites are safe to
		// substitute while planning dependents. An updating prerequisite may
		// produce a different output after convergence, so keeping its current
		// live value would let a dependent incorrectly plan as unchanged and
		// then skip re-resolution. External resources are never reconciled, so
		// their latest read is the output view dependents may use.
		if (change.Action == ActionUnchanged || change.Action == ActionExternal) && !change.Identity.IsZero() {
			knownOutputs[addr.String()] = change.Computed.Clone()
		}
		changes = append(changes, change)
	}

	return &Plan{Changes: changes}, nil
}

func validateProviderResourceSets(ctx context.Context, desired []resource.Resource, lookup Lookup) error {
	if len(desired) == 0 || lookup == nil {
		return nil
	}
	seen := make(map[string]struct{})
	for _, res := range desired {
		reader, err := lookup(res.Address)
		if err != nil {
			return fmt.Errorf("plan %s: %w", res.Address, err)
		}
		if reader == nil {
			return fmt.Errorf("plan %s: provider reader is nil", res.Address)
		}
		name := reader.Name()
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		validator, ok := reader.(provider.ResourceSetValidator)
		if !ok {
			continue
		}
		if err := validator.ValidateResourceSet(ctx, desired); err != nil {
			return fmt.Errorf("plan: provider %s: %w", name, err)
		}
	}
	return nil
}

func planResource(ctx context.Context, res resource.Resource, lookup Lookup, identities Identities, knownOutputs map[string]resource.Attributes, seenExternal map[string]string) (Change, error) {
	addr := res.Address
	bound, err := attachIdentity(addr, &res, identities)
	if err != nil {
		return Change{}, fmt.Errorf("plan %s: %w", addr, err)
	}
	stored, storedBound, err := storedOwnership(identities, addr)
	if err != nil {
		return Change{}, fmt.Errorf("plan %s: %w", addr, err)
	}
	if err := resource.OwnershipConflict(addr, res.Ownership, stored, storedBound); err != nil {
		return Change{}, fmt.Errorf("plan %s: %w", addr, err)
	}

	reader, err := lookup(addr)
	if err != nil {
		return Change{}, fmt.Errorf("plan %s: %w", addr, err)
	}
	if reader == nil {
		return Change{}, fmt.Errorf("plan %s: provider reader is nil", addr)
	}

	if res.IsExternal() || stored.IsExternal() {
		return planExternal(ctx, res, reader, bound, seenExternal)
	}

	if err := reader.Validate(ctx, res); err != nil {
		return Change{}, fmt.Errorf("plan %s: %w", addr, err)
	}

	live, err := reader.Read(ctx, res)
	if errors.Is(err, provider.ErrNotFound) {
		if bound {
			return Change{}, fmt.Errorf("plan %s: persisted identity %q was not found remotely; refusing to create a replacement: %w", addr, res.Identity.ID, state.ErrStaleIdentity)
		}
		want, _, err := comparableAttributes(reader, res, nil)
		if err != nil {
			return Change{}, fmt.Errorf("plan %s: %w", addr, err)
		}
		operation, err := missingResourceOperation(reader, res)
		if err != nil {
			return Change{}, fmt.Errorf("plan %s: %w", addr, err)
		}
		return Change{
			Address:   addr,
			Action:    ActionCreate,
			After:     want,
			Diffs:     diffsFromDesired(want),
			Operation: operation,
		}, nil
	}
	if err != nil {
		return Change{}, fmt.Errorf("plan %s: read live resource: %w", addr, err)
	}

	if bound {
		if live.Identity.IsZero() {
			return Change{}, fmt.Errorf("plan %s: provider returned no identity for persisted identity %q; refusing to rebind managed resource", addr, res.Identity.ID)
		}
		if live.Identity.ID != res.Identity.ID {
			return Change{}, fmt.Errorf("plan %s: provider returned identity %q for persisted identity %q; refusing to rebind managed resource", addr, live.Identity.ID, res.Identity.ID)
		}
	}

	want, got, err := comparableAttributes(reader, res, &live)
	if err != nil {
		return Change{}, fmt.Errorf("plan %s: %w", addr, err)
	}

	compareWant := substituteKnownOutputs(want, knownOutputs)
	diffs := diffAttributes("", got, compareWant)
	if len(diffs) > 0 {
		diffs = restoreOutputRefs(diffs, want)
	}
	action := ActionUnchanged
	if len(diffs) > 0 {
		action = ActionUpdate
	}

	return Change{
		Address:  addr,
		Action:   action,
		Identity: live.Identity,
		Before:   got,
		After:    want,
		Diffs:    diffs,
		Computed: live.Computed.Clone(),
	}, nil
}

func planExternal(ctx context.Context, res resource.Resource, reader provider.Reader, bound bool, seenExternal map[string]string) (Change, error) {
	addr := res.Address
	if err := provider.ValidateExternal(reader, res); err != nil {
		return Change{}, fmt.Errorf("plan %s: %w", addr, err)
	}
	external, ok := reader.(provider.ExternalReader)
	if !ok {
		return Change{}, fmt.Errorf("plan %s: provider does not support external ownership", addr)
	}
	id, err := resolveExternalID(reader, res, bound)
	if err != nil {
		return Change{}, fmt.Errorf("plan %s: %w", addr, err)
	}
	dupKey := addr.Provider + "\x00" + addr.Type + "\x00" + id
	if other, exists := seenExternal[dupKey]; exists {
		return Change{}, fmt.Errorf("plan %s: external resources %s and %s both reference remote identity %q for %s.%s; lookup is ambiguous", addr, other, addr, id, addr.Provider, addr.Type)
	}
	seenExternal[dupKey] = addr.String()

	live, err := external.ReadExternal(ctx, addr, id)
	if errors.Is(err, provider.ErrNotFound) {
		return Change{}, fmt.Errorf("plan %s: external resource %q was not found; refusing to create a replacement: %w", addr, id, err)
	}
	if err != nil {
		return Change{}, fmt.Errorf("plan %s: read external resource: %w", addr, err)
	}
	if live.Address != addr {
		return Change{}, fmt.Errorf("plan %s: provider returned logical address %s for external resource %s", addr, live.Address, addr)
	}
	if live.Identity.IsZero() {
		return Change{}, fmt.Errorf("plan %s: provider returned no identity for external resource %q", addr, id)
	}
	if live.Identity.ID != id {
		return Change{}, fmt.Errorf("plan %s: provider returned identity %q for external identity %q; refusing to bind a different remote resource", addr, live.Identity.ID, id)
	}
	return Change{
		Address:   addr,
		Action:    ActionExternal,
		Identity:  live.Identity,
		Operation: string(resource.OwnershipExternal),
		Computed:  live.Computed.Clone(),
	}, nil
}

func resolveExternalID(reader provider.Reader, res resource.Resource, bound bool) (string, error) {
	manifestID := strings.TrimSpace(res.ExternalID)
	if manifestID != "" {
		if normalizer, ok := reader.(provider.ImportIDNormalizer); ok {
			normalized, err := normalizer.NormalizeImportID(res.Address, manifestID)
			if err != nil {
				return "", err
			}
			manifestID = strings.TrimSpace(normalized)
			if manifestID == "" {
				return "", fmt.Errorf("lifecycle.id normalized to an empty identity")
			}
		}
	}
	stateID := ""
	if bound {
		stateID = strings.TrimSpace(res.Identity.ID)
	}
	if stateID != "" && manifestID != "" && stateID != manifestID {
		return "", fmt.Errorf("lifecycle.id %q does not match persisted identity %q", manifestID, stateID)
	}
	if stateID != "" {
		return stateID, nil
	}
	if manifestID != "" {
		return manifestID, nil
	}
	return "", fmt.Errorf("external resource has no identity; set lifecycle.id or run `agoraform import --external %s REMOTE-ID`", res.Address)
}

func storedOwnership(identities Identities, addr resource.Address) (resource.Ownership, bool, error) {
	if identities == nil {
		return "", false, nil
	}
	type ownershipSource interface {
		Ownership(addr resource.Address) (resource.Ownership, bool, error)
	}
	if src, ok := identities.(ownershipSource); ok {
		return src.Ownership(addr)
	}
	_, bound, err := identities.Identity(addr)
	if err != nil || !bound {
		return resource.OwnershipManaged, bound, err
	}
	return resource.OwnershipManaged, true, nil
}

func missingResourceOperation(reader provider.Reader, res resource.Resource) (string, error) {
	planner, ok := reader.(provider.MissingResourcePlanner)
	if !ok {
		return "", nil
	}
	mode, err := planner.PlanMissingResource(res)
	if err != nil {
		return "", err
	}
	switch mode {
	case "", provider.MissingResourceCreate:
		return "", nil
	case provider.MissingResourceAdopt:
		return string(mode), nil
	default:
		return "", fmt.Errorf("provider returned unsupported missing-resource mode %q", mode)
	}
}

func attachIdentity(addr resource.Address, res *resource.Resource, identities Identities) (bool, error) {
	identity := res.Identity
	if identities != nil {
		id, ok, err := identities.Identity(addr)
		if err != nil {
			return false, err
		}
		if ok {
			if !identity.IsZero() && identity.ID != id.ID {
				return true, fmt.Errorf("desired identity %q conflicts with persisted identity %q", identity.ID, id.ID)
			}
			identity = id
		}
	}
	res.Identity = identity
	return !res.Identity.IsZero(), nil
}

func comparableAttributes(reader provider.Reader, desired resource.Resource, live *resource.RemoteResource) (want, got resource.Attributes, err error) {
	if n, ok := reader.(provider.Normalizer); ok {
		want, got, err = n.NormalizeComparable(desired, live)
		if err != nil {
			return nil, nil, err
		}
		if want == nil {
			want = resource.Attributes{}
		}
		if live == nil {
			want, _ = overlayLocalAsset(desired, want, nil, nil)
			return want, nil, nil
		}
		if got == nil {
			got = resource.Attributes{}
		}
		want, got = overlayLocalAsset(desired, want, got, live)
		return want, got, nil
	}

	want = normalizeAttributes(desired.Attributes)
	if live == nil {
		want, _ = overlayLocalAsset(desired, want, nil, nil)
		return want, nil, nil
	}
	// Live comparable state is configurable attributes only. Computed
	// fields stay on RemoteResource.Computed and are never diffed.
	got = normalizeAttributes(live.Attributes)
	want, got = overlayLocalAsset(desired, want, got, live)
	return want, got, nil
}

func normalizeAttributes(in resource.Attributes) resource.Attributes {
	if in == nil {
		return resource.Attributes{}
	}
	out := make(resource.Attributes, len(in))
	for k, v := range in {
		if v == nil {
			continue
		}
		out[k] = v
	}
	return out
}

func diffsFromDesired(attrs resource.Attributes) []AttributeDiff {
	if len(attrs) == 0 {
		return nil
	}
	return diffAttributes("", nil, attrs)
}

func substituteKnownOutputs(attrs resource.Attributes, known map[string]resource.Attributes) resource.Attributes {
	if len(attrs) == 0 {
		return attrs
	}
	mapped, err := resource.MapRefs(attrs, func(_ string, ref resource.Ref) (any, error) {
		if !ref.HasOutput() {
			return ref, nil
		}
		outputs, ok := known[ref.Address.String()]
		if !ok {
			return ref, nil
		}
		v, ok := outputs[ref.Output]
		if !ok {
			return ref, nil
		}
		return resource.CloneValue(v), nil
	})
	if err != nil {
		return attrs
	}
	out, ok := mapped.(resource.Attributes)
	if !ok {
		return attrs
	}
	return out
}

func restoreOutputRefs(diffs []AttributeDiff, want resource.Attributes) []AttributeDiff {
	refs := make(map[string]resource.Ref)
	resource.WalkRefValues(want, func(path string, ref resource.Ref) {
		if ref.HasOutput() {
			refs[path] = ref
		}
	})
	if len(refs) == 0 {
		return diffs
	}
	out := append([]AttributeDiff(nil), diffs...)
	for i, d := range out {
		if ref, ok := refs[d.Path]; ok {
			out[i].After = ref
		}
	}
	return out
}
