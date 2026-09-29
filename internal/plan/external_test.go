package plan_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
)

func TestPlanExternalReferenceFeedsDependentAndDoesNotMutate(t *testing.T) {
	t.Parallel()

	widgets := fake.New()
	notes := fake.NewAlt()
	parent := externalWidget(t, "homepage", "widget-1")
	if err := widgets.Seed(resource.RemoteResource{
		Address:    parent.Address,
		Identity:   resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage"},
		Computed:   resource.Attributes{fake.AttrSerial: 4, fake.OutputToken: "tok-homepage"},
	}); err != nil {
		t.Fatal(err)
	}
	child := resource.Resource{
		Address: mustPlanAddress(t, "alt.note.banner"),
		Attributes: resource.Attributes{
			fake.AttrText: resource.Ref{Address: parent.Address, Output: fake.OutputToken},
		},
	}

	st := mustPlanStore(t)
	got, err := plan.BuildWithState(context.Background(), []resource.Resource{child, parent}, planLookup(widgets, notes), st)
	if err != nil {
		t.Fatalf("BuildWithState: %v", err)
	}
	if len(got.Changes) != 2 {
		t.Fatalf("changes = %+v", got.Changes)
	}
	if got.Changes[0].Address != parent.Address || got.Changes[0].Action != plan.ActionExternal {
		t.Fatalf("first change = %+v, want external parent first", got.Changes[0])
	}
	if got.Changes[1].Address != child.Address || got.Changes[1].Action != plan.ActionCreate {
		t.Fatalf("second change = %+v, want note create", got.Changes[1])
	}
	if got.HasChanges() != true || got.ExternalCount() != 1 {
		t.Fatalf("plan counts = changes %v external %d", got.HasChanges(), got.ExternalCount())
	}
	formatted := plan.Format(got)
	if !strings.Contains(formatted, "= fake.widget.homepage") || !strings.Contains(formatted, "1 external") {
		t.Fatalf("plan output missing external marker:\n%s", formatted)
	}
	reads, creates, updates, imports := widgets.Calls()
	if creates != 0 || updates != 0 || imports != 0 || reads == 0 {
		t.Fatalf("widget calls reads=%d creates=%d updates=%d imports=%d", reads, creates, updates, imports)
	}
	if _, noteCreates, noteUpdates, _ := notes.Calls(); noteCreates != 0 || noteUpdates != 0 {
		t.Fatalf("note mutated during plan: creates=%d updates=%d", noteCreates, noteUpdates)
	}
}

func TestPlanExternalMissingDoesNotCreate(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := externalWidget(t, "homepage", "missing")
	_, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, planLookup(p, nil), nil)
	if err == nil || !errors.Is(err, provider.ErrNotFound) || !strings.Contains(err.Error(), "refusing to create") {
		t.Fatalf("error = %v, want missing external", err)
	}
	_, creates, updates, _ := p.Calls()
	if creates != 0 || updates != 0 {
		t.Fatalf("missing external mutated provider: creates=%d updates=%d", creates, updates)
	}
}

func TestPlanExternalAmbiguousLookup(t *testing.T) {
	t.Parallel()

	p := fake.New()
	p.MarkAmbiguousExternal("widget-1")
	if err := p.Seed(resource.RemoteResource{
		Address:  mustPlanAddress(t, "fake.widget.other"),
		Identity: resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{
			fake.AttrTitle: "Other",
		},
	}); err != nil {
		t.Fatal(err)
	}
	res := externalWidget(t, "homepage", "widget-1")
	_, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, planLookup(p, nil), nil)
	if err == nil || !strings.Contains(err.Error(), "multiple remote widgets") {
		t.Fatalf("error = %v, want ambiguous lookup", err)
	}
}

func TestPlanExternalDriftRefreshesReadWithoutUpdate(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := externalWidget(t, "homepage", "widget-1")
	remote := resource.RemoteResource{
		Address:    res.Address,
		Identity:   resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage"},
		Computed:   resource.Attributes{fake.OutputToken: "tok-old"},
	}
	if err := p.Seed(remote); err != nil {
		t.Fatal(err)
	}
	first, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, planLookup(p, nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	remote.Attributes[fake.AttrTitle] = "Renamed elsewhere"
	remote.Computed[fake.OutputToken] = "tok-new"
	if err := p.Seed(remote); err != nil {
		t.Fatal(err)
	}
	second, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, planLookup(p, nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.HasChanges() || second.Changes[0].Action != plan.ActionExternal {
		t.Fatalf("drift plan = %+v", second.Changes)
	}
	if second.Changes[0].Computed[fake.OutputToken] != "tok-new" {
		t.Fatalf("computed = %v, want refreshed token", second.Changes[0].Computed)
	}
	if first.Changes[0].Computed[fake.OutputToken] != "tok-old" {
		t.Fatalf("first computed = %v", first.Changes[0].Computed)
	}
	_, creates, updates, _ := p.Calls()
	if creates != 0 || updates != 0 {
		t.Fatalf("drift mutated provider: creates=%d updates=%d", creates, updates)
	}
}

func TestPlanRejectsSilentOwnershipTransitions(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustPlanAddress(t, "fake.widget.homepage")
	if err := p.Seed(resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage"},
	}); err != nil {
		t.Fatal(err)
	}
	st := mustPlanStore(t)
	if err := st.SetOwnership(addr, resource.Identity{ID: "widget-1"}, resource.OwnershipExternal); err != nil {
		t.Fatal(err)
	}
	managed := resource.Resource{
		Address:    addr,
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage"},
	}
	_, err := plan.BuildWithState(context.Background(), []resource.Resource{managed}, planLookup(p, nil), st)
	if err == nil || !strings.Contains(err.Error(), "import --adopt") {
		t.Fatalf("managed manifest over external state = %v", err)
	}

	if err := st.SetOwnership(addr, resource.Identity{ID: "widget-1"}, resource.OwnershipManaged); err != nil {
		t.Fatal(err)
	}
	external := externalWidget(t, "homepage", "widget-1")
	_, err = plan.BuildWithState(context.Background(), []resource.Resource{external}, planLookup(p, nil), st)
	if err == nil || !strings.Contains(err.Error(), "--release") {
		t.Fatalf("external manifest over managed state = %v", err)
	}
	_, creates, updates, _ := p.Calls()
	if creates != 0 || updates != 0 {
		t.Fatalf("transition check mutated provider: creates=%d updates=%d", creates, updates)
	}
}

func TestPlanRejectsUnsupportedExternalType(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := resource.Resource{
		Address:    mustPlanAddress(t, "fake.widget.homepage"),
		Ownership:  resource.OwnershipExternal,
		ExternalID: "widget-1",
	}
	res.Address.Type = "gadget"
	_, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, planLookup(p, nil), nil)
	if err == nil || !strings.Contains(err.Error(), "does not support external ownership") {
		t.Fatalf("error = %v", err)
	}
}

func TestPlanDuplicateExternalIdentityIsAmbiguous(t *testing.T) {
	t.Parallel()

	p := fake.New()
	first := externalWidget(t, "homepage", "widget-1")
	second := externalWidget(t, "banner", "widget-1")
	if err := p.Seed(resource.RemoteResource{
		Address:    first.Address,
		Identity:   resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage"},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := plan.BuildWithState(context.Background(), []resource.Resource{first, second}, planLookup(p, nil), nil)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("error = %v, want duplicate external identity", err)
	}
}

func externalWidget(t *testing.T, name, id string) resource.Resource {
	t.Helper()
	return resource.Resource{
		Address:    mustPlanAddress(t, "fake.widget."+name),
		Ownership:  resource.OwnershipExternal,
		ExternalID: id,
	}
}

func mustPlanAddress(t *testing.T, raw string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress(raw)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func mustPlanStore(t *testing.T) *state.Store {
	t.Helper()
	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func planLookup(widgets *fake.Provider, notes *fake.Alt) plan.Lookup {
	return func(addr resource.Address) (provider.Reader, error) {
		switch addr.Provider {
		case fake.Name:
			return widgets, nil
		case fake.AltName:
			if notes == nil {
				return nil, errors.New("alt provider is not registered")
			}
			return notes, nil
		default:
			return nil, errors.New("unknown provider")
		}
	}
}
