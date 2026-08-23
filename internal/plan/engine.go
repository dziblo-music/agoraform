package plan

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

// Lookup resolves the read-only provider for a resource address.
//
// The function returns provider.Reader so callers cannot pass mutation
// methods into the planner.
type Lookup func(addr resource.Address) (provider.Reader, error)

// Build compares desired resources with provider-reported live state.
//
// Missing remote resources become creates. Configurable differences become
// updates. Computed/read-only fields are ignored. Resources are planned in
// address order. Build never invokes Create, Update, or Import.
func Build(ctx context.Context, desired []resource.Resource, lookup Lookup) (*Plan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if lookup == nil && len(desired) > 0 {
		return nil, fmt.Errorf("plan: provider lookup is required")
	}

	resources := append([]resource.Resource(nil), desired...)
	sort.SliceStable(resources, func(i, j int) bool {
		return resources[i].Address.String() < resources[j].Address.String()
	})

	changes := make([]Change, 0, len(resources))
	for _, res := range resources {
		change, err := planResource(ctx, res, lookup)
		if err != nil {
			return nil, err
		}
		changes = append(changes, change)
	}

	return &Plan{Changes: changes}, nil
}

func planResource(ctx context.Context, res resource.Resource, lookup Lookup) (Change, error) {
	addr := res.Address
	reader, err := lookup(addr)
	if err != nil {
		return Change{}, fmt.Errorf("plan %s: %w", addr, err)
	}
	if reader == nil {
		return Change{}, fmt.Errorf("plan %s: provider reader is nil", addr)
	}

	if err := reader.Validate(ctx, res); err != nil {
		return Change{}, fmt.Errorf("plan %s: %w", addr, err)
	}

	live, err := reader.Read(ctx, res)
	if errors.Is(err, provider.ErrNotFound) {
		want, _, err := comparableAttributes(reader, res, nil)
		if err != nil {
			return Change{}, fmt.Errorf("plan %s: %w", addr, err)
		}
		return Change{
			Address: addr,
			Action:  ActionCreate,
			After:   want,
			Diffs:   diffsFromDesired(want),
		}, nil
	}
	if err != nil {
		return Change{}, fmt.Errorf("plan %s: read live resource: %w", addr, err)
	}

	want, got, err := comparableAttributes(reader, res, &live)
	if err != nil {
		return Change{}, fmt.Errorf("plan %s: %w", addr, err)
	}

	diffs := diffAttributes("", got, want)
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
	}, nil
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
			return want, nil, nil
		}
		if got == nil {
			got = resource.Attributes{}
		}
		return want, got, nil
	}

	want = normalizeAttributes(desired.Attributes)
	if live == nil {
		return want, nil, nil
	}
	// Live comparable state is configurable attributes only. Computed
	// fields stay on RemoteResource.Computed and are never diffed.
	got = normalizeAttributes(live.Attributes)
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
