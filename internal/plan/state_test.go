package plan_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
)

func TestBuildWithStateUsesPersistedIdentity(t *testing.T) {
	t.Parallel()

	p := fake.New()
	managed := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Old"})
	other := widget(t, "other", resource.Attributes{fake.AttrTitle: "New"})
	seed(t, p, managed, resource.Attributes{fake.AttrSerial: 1})
	seed(t, p, other, resource.Attributes{fake.AttrSerial: 2})

	st := mustStore(t)
	if err := st.Bind(managed.Address, resource.Identity{ID: "id-homepage"}); err != nil {
		t.Fatal(err)
	}

	desired := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "New"})
	got, err := plan.BuildWithState(context.Background(), []resource.Resource{desired}, lookupProvider(p), st)
	if err != nil {
		t.Fatalf("BuildWithState: %v", err)
	}
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want update of bound identity", got.Changes)
	}
	if got.Changes[0].Identity.ID != "id-homepage" {
		t.Fatalf("identity = %q, want id-homepage", got.Changes[0].Identity.ID)
	}
	assertNoMutations(t, p, 1)
}

func TestBuildWithStateStaleIdentityIsNotCreate(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	st := mustStore(t)
	if err := st.Bind(res.Address, resource.Identity{ID: "missing"}); err != nil {
		t.Fatal(err)
	}

	_, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, lookupProvider(p), st)
	if err == nil {
		t.Fatal("BuildWithState succeeded, want stale identity error")
	}
	if !errors.Is(err, state.ErrStaleIdentity) {
		t.Fatalf("error = %v, want ErrStaleIdentity", err)
	}
	if !strings.Contains(err.Error(), "persisted identity") {
		t.Fatalf("error = %q, want persisted identity diagnostic", err)
	}
	assertNoMutations(t, p, 1)
}

func TestRecordCreateThenPlanIsUnchanged(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	st := mustStore(t)

	first := mustBuild(t, []resource.Resource{res}, p)
	if first.Changes[0].Action != plan.ActionCreate {
		t.Fatalf("first plan = %+v, want create", first.Changes)
	}

	live, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RecordCreate(res.Address, live); err != nil {
		t.Fatal(err)
	}

	got, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, lookupProvider(p), st)
	if err != nil {
		t.Fatalf("second plan: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("after create persistence, plan has changes: %+v", got.Changes)
	}
	if got.Changes[0].Identity.ID != live.Identity.ID {
		t.Fatalf("identity = %q, want %q", got.Changes[0].Identity.ID, live.Identity.ID)
	}
}

func TestRecordImportThenPlanIsUnchanged(t *testing.T) {
	t.Parallel()

	p := fake.New()
	original := widget(t, "old", resource.Attributes{fake.AttrTitle: "Imported"})
	seed(t, p, original, resource.Attributes{fake.AttrSerial: 4})
	desired := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Imported"})

	live, err := p.Import(context.Background(), desired.Address, "id-old")
	if err != nil {
		t.Fatal(err)
	}
	st := mustStore(t)
	if err := st.RecordImport(desired.Address, live.Identity.ID); err != nil {
		t.Fatal(err)
	}

	got, err := plan.BuildWithState(context.Background(), []resource.Resource{desired}, lookupProvider(p), st)
	if err != nil {
		t.Fatalf("plan after import: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("after import persistence, plan has changes: %+v", got.Changes)
	}
	if got.Changes[0].Identity.ID != "id-old" {
		t.Fatalf("identity = %q, want id-old", got.Changes[0].Identity.ID)
	}
}

func TestEmptyStateMatchesNoStatePlan(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	without := mustBuild(t, []resource.Resource{res}, p)
	got, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, lookupProvider(p), mustStore(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.Changes[0].Action != without.Changes[0].Action {
		t.Fatalf("empty state action = %s, want %s", got.Changes[0].Action, without.Changes[0].Action)
	}
}

func mustStore(t *testing.T) *state.Store {
	t.Helper()
	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}
	return st
}
