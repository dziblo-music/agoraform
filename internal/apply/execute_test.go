package apply_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
)

const plantedSecret = "super-secret-token-value"

func TestRunCreatePersistsIdentity(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	st := mustStore(t)
	var out bytes.Buffer

	result, err := apply.Run(context.Background(), []resource.Resource{res}, lookupProvider(p), st, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Created != 1 || result.Updated != 0 {
		t.Fatalf("result = %+v, want 1 created", result)
	}

	_, creates, updates, imports := p.Calls()
	if creates != 1 || updates != 0 || imports != 0 {
		t.Fatalf("Calls() creates=%d updates=%d imports=%d, want 1 0 0", creates, updates, imports)
	}

	id, ok, err := st.Identity(res.Address)
	if err != nil || !ok || id.ID == "" {
		t.Fatalf("Identity = (%v,%v,%v), want persisted id", id, ok, err)
	}

	got := out.String()
	if !strings.Contains(got, "fake.widget.homepage: creating...") || !strings.Contains(got, "fake.widget.homepage: created") {
		t.Fatalf("progress output:\n%s", got)
	}
	if !strings.Contains(got, "Apply complete! 1 created, 0 updated.") {
		t.Fatalf("summary missing:\n%s", got)
	}
	assertNoSecret(t, got)
	assertStateHasNoSecret(t, st)
}

func TestRunUpdateRetainsIdentity(t *testing.T) {
	t.Parallel()

	p := fake.New()
	live := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, p, live, resource.Attributes{fake.AttrSerial: 3})
	st := mustStore(t)
	if err := st.Bind(live.Address, resource.Identity{ID: "id-homepage"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	desired := widget(t, "homepage", resource.Attributes{
		fake.AttrTitle: "Homepage",
		fake.AttrColor: "blue",
	})
	var out bytes.Buffer
	result, err := apply.Run(context.Background(), []resource.Resource{desired}, lookupProvider(p), st, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Created != 0 || result.Updated != 1 {
		t.Fatalf("result = %+v, want 1 updated", result)
	}

	id, ok, err := st.Identity(desired.Address)
	if err != nil || !ok || id.ID != "id-homepage" {
		t.Fatalf("Identity = (%v,%v,%v), want id-homepage", id, ok, err)
	}

	got := out.String()
	if !strings.Contains(got, "fake.widget.homepage: updating...") || !strings.Contains(got, "fake.widget.homepage: updated") {
		t.Fatalf("progress output:\n%s", got)
	}
	if !strings.Contains(got, "Apply complete! 0 created, 1 updated.") {
		t.Fatalf("summary missing:\n%s", got)
	}
}

const liveETag = "live-etag"

func TestExecuteUpdatePassesFullLiveResourceFromRead(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	live := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	if err := inner.Seed(resource.RemoteResource{
		Address:    live.Address,
		Identity:   resource.Identity{ID: "id-homepage"},
		Attributes: live.Attributes.Clone(),
		Computed: resource.Attributes{
			fake.AttrSerial: 9,
			"etag":          liveETag,
		},
	}); err != nil {
		t.Fatal(err)
	}
	p := &computedAwareProvider{Provider: inner}

	desired := widget(t, "homepage", resource.Attributes{
		fake.AttrTitle: "Homepage",
		fake.AttrColor: "blue",
	})
	st := mustStore(t)
	planned := updatePlan(desired, live.Attributes, "id-homepage")

	var out bytes.Buffer
	result, err := apply.Execute(context.Background(), planned, []resource.Resource{desired}, lookupProvider(p), st, &out)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Updated != 1 {
		t.Fatalf("result = %+v, want 1 updated", result)
	}
	if p.updates != 1 {
		t.Fatalf("updates = %d, want 1", p.updates)
	}
	if p.lastActual.Computed["etag"] != liveETag {
		t.Fatalf("Update actual.Computed = %+v, want etag %q from Read", p.lastActual.Computed, liveETag)
	}
	if p.lastRead.Identity.ID != "id-homepage" {
		t.Fatalf("pre-update Read identity = %q, want id-homepage", p.lastRead.Identity.ID)
	}
	if !strings.Contains(out.String(), "fake.widget.homepage: updated") {
		t.Fatalf("output missing updated progress:\n%s", out.String())
	}

	id, ok, err := st.Identity(desired.Address)
	if err != nil || !ok || id.ID != "id-homepage" {
		t.Fatalf("Identity = (%v,%v,%v), want id-homepage", id, ok, err)
	}
}

func TestExecuteUpdateReadFailureDoesNotUpdate(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	live := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, inner, live, resource.Attributes{fake.AttrSerial: 1})
	p := &scriptedProvider{Provider: inner, readErr: errors.New("remote read failed")}
	st := mustStore(t)
	if err := st.Bind(live.Address, resource.Identity{ID: "id-homepage"}); err != nil {
		t.Fatal(err)
	}

	desired := widget(t, "homepage", resource.Attributes{
		fake.AttrTitle: "Homepage",
		fake.AttrColor: "red",
	})
	_, err := apply.Execute(context.Background(), updatePlan(desired, live.Attributes, "id-homepage"), []resource.Resource{desired}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Execute succeeded, want pre-update read failure")
	}
	if !strings.Contains(err.Error(), "fake.widget.homepage") || !strings.Contains(err.Error(), "update") {
		t.Fatalf("error = %q, want address and update", err)
	}
	if !strings.Contains(err.Error(), "read live resource") || !strings.Contains(err.Error(), "remote read failed") {
		t.Fatalf("error = %q, want read failure diagnostic", err)
	}
	if p.updates != 0 {
		t.Fatalf("updates = %d, want 0", p.updates)
	}
	_, _, updates, _ := inner.Calls()
	if updates != 0 {
		t.Fatalf("inner updates = %d, want 0", updates)
	}
	id, ok, identErr := st.Identity(desired.Address)
	if identErr != nil || !ok || id.ID != "id-homepage" {
		t.Fatalf("Identity = (%v,%v,%v), want unchanged binding", id, ok, identErr)
	}
}

func TestExecuteUpdateReadWrongIdentityDoesNotUpdate(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	live := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, inner, live, resource.Attributes{fake.AttrSerial: 1})
	p := &scriptedProvider{Provider: inner, readIdentity: resource.Identity{ID: "other-id"}}
	st := mustStore(t)
	if err := st.Bind(live.Address, resource.Identity{ID: "id-homepage"}); err != nil {
		t.Fatal(err)
	}

	desired := widget(t, "homepage", resource.Attributes{
		fake.AttrTitle: "Homepage",
		fake.AttrColor: "red",
	})
	_, err := apply.Execute(context.Background(), updatePlan(desired, live.Attributes, "id-homepage"), []resource.Resource{desired}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Execute succeeded, want identity mismatch")
	}
	if !strings.Contains(err.Error(), "refusing to rebind") || !strings.Contains(err.Error(), "other-id") {
		t.Fatalf("error = %q, want rebind refusal", err)
	}
	if p.updates != 0 {
		t.Fatalf("updates = %d, want 0", p.updates)
	}
	id, ok, identErr := st.Identity(desired.Address)
	if identErr != nil || !ok || id.ID != "id-homepage" {
		t.Fatalf("Identity = (%v,%v,%v), want unchanged binding", id, ok, identErr)
	}
}

func TestExecuteUpdateReadEmptyIdentityDoesNotUpdate(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	live := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, inner, live, resource.Attributes{fake.AttrSerial: 1})
	p := &scriptedProvider{Provider: inner, stripReadIdentity: true}
	st := mustStore(t)
	if err := st.Bind(live.Address, resource.Identity{ID: "id-homepage"}); err != nil {
		t.Fatal(err)
	}

	desired := widget(t, "homepage", resource.Attributes{
		fake.AttrTitle: "Homepage",
		fake.AttrColor: "red",
	})
	_, err := apply.Execute(context.Background(), updatePlan(desired, live.Attributes, "id-homepage"), []resource.Resource{desired}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Execute succeeded, want empty identity")
	}
	if !strings.Contains(err.Error(), "fake.widget.homepage") || !strings.Contains(err.Error(), "update") {
		t.Fatalf("error = %q, want address and update", err)
	}
	if !strings.Contains(err.Error(), "read returned no identity") {
		t.Fatalf("error = %q, want empty identity diagnostic", err)
	}
	if p.updates != 0 {
		t.Fatalf("updates = %d, want 0", p.updates)
	}
	id, ok, identErr := st.Identity(desired.Address)
	if identErr != nil || !ok || id.ID != "id-homepage" {
		t.Fatalf("Identity = (%v,%v,%v), want unchanged binding", id, ok, identErr)
	}
}

func TestRunZeroChange(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, p, res, resource.Attributes{fake.AttrSerial: 4})
	st := mustStore(t)
	if err := st.Bind(res.Address, resource.Identity{ID: "id-homepage"}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	result, err := apply.Run(context.Background(), []resource.Resource{res}, lookupProvider(p), st, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Created != 0 || result.Updated != 0 {
		t.Fatalf("result = %+v, want zero changes", result)
	}

	_, creates, updates, imports := p.Calls()
	if creates != 0 || updates != 0 || imports != 0 {
		t.Fatalf("zero-change apply mutated provider: creates=%d updates=%d imports=%d", creates, updates, imports)
	}
	if strings.Contains(out.String(), "creating") || strings.Contains(out.String(), "updating") {
		t.Fatalf("zero-change output still reported mutations:\n%s", out.String())
	}
	if out.String() != apply.Format(result) {
		t.Fatalf("zero-change output = %q, want summary only", out.String())
	}
}

func TestRunDeterministicOrdering(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	rec := &recordingProvider{Provider: inner}
	desired := []resource.Resource{
		widget(t, "zeta", resource.Attributes{fake.AttrTitle: "Z"}),
		widget(t, "alpha", resource.Attributes{fake.AttrTitle: "A"}),
		widget(t, "mu", resource.Attributes{fake.AttrTitle: "M"}),
	}
	st := mustStore(t)

	if _, err := apply.Run(context.Background(), desired, lookupProvider(rec), st, ioDiscard()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []string{
		"create:fake.widget.alpha",
		"create:fake.widget.mu",
		"create:fake.widget.zeta",
	}
	if got := rec.ops; len(got) != len(want) {
		t.Fatalf("ops = %v, want %v", got, want)
	}
	for i, op := range rec.ops {
		if op != want[i] {
			t.Fatalf("ops = %v, want %v", rec.ops, want)
		}
	}
}

func TestRunValidationFailureDoesNotMutate(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrColor: "blue"})
	st := mustStore(t)

	_, err := apply.Run(context.Background(), []resource.Resource{res}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Run succeeded, want validation error")
	}
	if !strings.Contains(err.Error(), "title") {
		t.Fatalf("error = %q, want title validation", err)
	}

	_, creates, updates, imports := p.Calls()
	if creates != 0 || updates != 0 || imports != 0 {
		t.Fatalf("validation failure mutated provider: creates=%d updates=%d imports=%d", creates, updates, imports)
	}
	if _, ok, _ := st.Identity(res.Address); ok {
		t.Fatal("validation failure wrote identity")
	}
}

func TestRunProviderCreateFailureDoesNotWriteState(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	p := &scriptedProvider{Provider: inner, createErr: errors.New("remote create rejected")}
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	st := mustStore(t)

	_, err := apply.Run(context.Background(), []resource.Resource{res}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Run succeeded, want create failure")
	}
	if !strings.Contains(err.Error(), "fake.widget.homepage") || !strings.Contains(err.Error(), "create") {
		t.Fatalf("error = %q, want address and create", err)
	}
	if !strings.Contains(err.Error(), "remote create rejected") {
		t.Fatalf("error = %q, want provider message", err)
	}
	if apply.IsPartial(err) {
		t.Fatalf("pre-mutation create failure classified as partial: %v", err)
	}
	assertNoSecret(t, err.Error())

	if _, ok, _ := st.Identity(res.Address); ok {
		t.Fatal("failed create wrote identity")
	}
	_, creates, updates, _ := inner.Calls()
	if creates != 0 || updates != 0 {
		t.Fatalf("inner mutated: creates=%d updates=%d", creates, updates)
	}
}

func TestRunProviderUpdateFailureDoesNotWriteState(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	live := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, inner, live, resource.Attributes{fake.AttrSerial: 1})
	st := mustStore(t)
	if err := st.Bind(live.Address, resource.Identity{ID: "id-homepage"}); err != nil {
		t.Fatal(err)
	}

	p := &scriptedProvider{Provider: inner, updateErr: errors.New("remote update rejected")}
	desired := widget(t, "homepage", resource.Attributes{
		fake.AttrTitle: "Homepage",
		fake.AttrColor: "red",
	})
	_, err := apply.Run(context.Background(), []resource.Resource{desired}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Run succeeded, want update failure")
	}
	if !strings.Contains(err.Error(), "fake.widget.homepage") || !strings.Contains(err.Error(), "update") {
		t.Fatalf("error = %q, want address and update", err)
	}
	if apply.IsPartial(err) {
		t.Fatalf("pre-mutation update failure classified as partial: %v", err)
	}

	id, ok, err := st.Identity(desired.Address)
	if err != nil || !ok || id.ID != "id-homepage" {
		t.Fatalf("Identity after failed update = (%v,%v,%v)", id, ok, err)
	}
	_, _, updates, _ := inner.Calls()
	if updates != 0 {
		t.Fatalf("inner updates = %d, want 0", updates)
	}
}

func TestExecuteUnsupportedActionProtectsBeforeMutation(t *testing.T) {
	t.Parallel()

	p := fake.New()
	alpha := widget(t, "alpha", resource.Attributes{fake.AttrTitle: "A"})
	beta := widget(t, "beta", resource.Attributes{fake.AttrTitle: "B"})
	st := mustStore(t)

	planned := &plan.Plan{
		Changes: []plan.Change{
			{Address: alpha.Address, Action: plan.ActionCreate, After: alpha.Attributes},
			{Address: beta.Address, Action: plan.Action("delete")},
		},
	}

	_, err := apply.Execute(context.Background(), planned, []resource.Resource{alpha, beta}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Execute succeeded, want unsupported action")
	}
	if !strings.Contains(err.Error(), "unsupported action") || !strings.Contains(err.Error(), "delete") {
		t.Fatalf("error = %q, want unsupported delete", err)
	}

	_, creates, updates, imports := p.Calls()
	if creates != 0 || updates != 0 || imports != 0 {
		t.Fatalf("unsupported action mutated provider: creates=%d updates=%d imports=%d", creates, updates, imports)
	}
}

func TestExecuteCreateEmptyIdentityDoesNotWriteState(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	p := &scriptedProvider{Provider: inner, stripCreateIdentity: true}
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	st := mustStore(t)
	planned := &plan.Plan{
		Changes: []plan.Change{{Address: res.Address, Action: plan.ActionCreate, After: res.Attributes}},
	}

	_, err := apply.Execute(context.Background(), planned, []resource.Resource{res}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Execute succeeded, want missing identity error")
	}
	if !strings.Contains(err.Error(), "create") || !strings.Contains(err.Error(), "no identity") {
		t.Fatalf("error = %q, want missing identity", err)
	}
	if _, ok, _ := st.Identity(res.Address); ok {
		t.Fatal("empty identity was persisted")
	}
}

func TestExecuteStateWriteFailureAfterCreate(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage " + plantedSecret})
	st := &failingStore{err: errors.New("disk full")}
	planned := &plan.Plan{
		Changes: []plan.Change{{Address: res.Address, Action: plan.ActionCreate, After: res.Attributes}},
	}

	var out bytes.Buffer
	_, err := apply.Execute(context.Background(), planned, []resource.Resource{res}, lookupProvider(p), st, &out)
	if err == nil {
		t.Fatal("Execute succeeded, want state write failure")
	}
	partial := requirePartial(t, err)
	if partial.Operation != "create" || partial.Stage != apply.StagePersist || partial.RemoteIdentity.ID == "" {
		t.Fatalf("partial = %+v, want create persist with remote identity", partial)
	}
	if !strings.Contains(err.Error(), "created remotely") || !strings.Contains(err.Error(), partial.RemoteIdentity.ID) {
		t.Fatalf("error = %q, want remote create identity", err)
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("error = %q, want underlying write error", err)
	}
	if !strings.Contains(err.Error(), "agoraform import "+res.Address.String()+" "+partial.RemoteIdentity.ID) {
		t.Fatalf("error = %q, want import recovery", err)
	}
	if strings.Contains(out.String(), "Apply complete!") {
		t.Fatalf("state write failure claimed apply complete:\n%s", out.String())
	}
	assertNoSecret(t, err.Error())
	assertNoSecret(t, out.String())

	_, creates, _, _ := p.Calls()
	if creates != 1 {
		t.Fatalf("creates = %d, want 1 (mutation already happened)", creates)
	}
}

func TestExecuteStateWriteFailureAfterUpdate(t *testing.T) {
	t.Parallel()

	p := fake.New()
	live := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, p, live, resource.Attributes{fake.AttrSerial: 2})
	inner := mustStore(t)
	if err := inner.Bind(live.Address, resource.Identity{ID: "id-homepage"}); err != nil {
		t.Fatal(err)
	}
	if err := inner.Save(); err != nil {
		t.Fatal(err)
	}
	st := &failingRecordStore{Store: inner, err: errors.New("permission denied")}
	desired := widget(t, "homepage", resource.Attributes{
		fake.AttrTitle: "Homepage",
		fake.AttrColor: "green",
	})
	planned := &plan.Plan{
		Changes: []plan.Change{
			{
				Address:  desired.Address,
				Action:   plan.ActionUpdate,
				Identity: resource.Identity{ID: "id-homepage"},
				Before:   live.Attributes,
				After:    desired.Attributes,
			},
		},
	}

	_, err := apply.Execute(context.Background(), planned, []resource.Resource{desired}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Execute succeeded, want state write failure")
	}
	partial := requirePartial(t, err)
	if partial.Operation != "update" || partial.Stage != apply.StagePersist {
		t.Fatalf("partial = %+v, want update persist", partial)
	}
	if !strings.Contains(err.Error(), "updated remotely") {
		t.Fatalf("error = %q, want remote update succeeded", err)
	}
	if strings.Contains(err.Error(), "agoraform import") {
		t.Fatalf("error = %q, must not require import after update persist failure", err)
	}
	if !strings.Contains(err.Error(), "existing identity binding remains valid") {
		t.Fatalf("error = %q, want identity remains valid", err)
	}

	_, _, updates, _ := p.Calls()
	if updates != 1 {
		t.Fatalf("updates = %d, want 1 (not repeated)", updates)
	}
	if st.recordUpdates != 1 {
		t.Fatalf("RecordUpdate calls = %d, want 1", st.recordUpdates)
	}
	id, ok, identErr := inner.Identity(desired.Address)
	if identErr != nil || !ok || id.ID != "id-homepage" {
		t.Fatalf("Identity = (%v,%v,%v), want original binding intact", id, ok, identErr)
	}
}

func TestRunStateBoundResourceIsNotRebound(t *testing.T) {
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
	if _, err := apply.Run(context.Background(), []resource.Resource{desired}, lookupProvider(p), st, ioDiscard()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	id, ok, err := st.Identity(managed.Address)
	if err != nil || !ok || id.ID != "id-homepage" {
		t.Fatalf("Identity = (%v,%v,%v), want id-homepage", id, ok, err)
	}
	if _, ok, _ := st.Identity(other.Address); ok {
		t.Fatal("apply rebound managed resource onto the other remote object")
	}

	got, err := p.Read(context.Background(), resource.Resource{
		Address:  managed.Address,
		Identity: resource.Identity{ID: "id-homepage"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Attributes[fake.AttrTitle] != "New" {
		t.Fatalf("managed title = %v, want New", got.Attributes[fake.AttrTitle])
	}
	otherLive, err := p.Read(context.Background(), resource.Resource{
		Address:  other.Address,
		Identity: resource.Identity{ID: "id-other"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if otherLive.Attributes[fake.AttrTitle] != "New" {
		t.Fatalf("unmanaged other title = %v, want unchanged New", otherLive.Attributes[fake.AttrTitle])
	}
}

func TestExecuteRefusesUpdateIdentityRebind(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	live := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, inner, live, resource.Attributes{fake.AttrSerial: 1})
	p := &scriptedProvider{Provider: inner, updateIdentity: resource.Identity{ID: "other-id"}}
	st := mustStore(t)
	if err := st.Bind(live.Address, resource.Identity{ID: "id-homepage"}); err != nil {
		t.Fatal(err)
	}

	desired := widget(t, "homepage", resource.Attributes{
		fake.AttrTitle: "Homepage",
		fake.AttrColor: "blue",
	})
	planned := &plan.Plan{
		Changes: []plan.Change{
			{
				Address:  desired.Address,
				Action:   plan.ActionUpdate,
				Identity: resource.Identity{ID: "id-homepage"},
				Before:   live.Attributes,
				After:    desired.Attributes,
			},
		},
	}

	_, err := apply.Execute(context.Background(), planned, []resource.Resource{desired}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Execute succeeded, want rebind refusal")
	}
	if !strings.Contains(err.Error(), "refusing to rebind") {
		t.Fatalf("error = %q, want rebind refusal", err)
	}

	id, ok, identErr := st.Identity(desired.Address)
	if identErr != nil || !ok || id.ID != "id-homepage" {
		t.Fatalf("Identity = (%v,%v,%v), want original binding", id, ok, identErr)
	}
}

func TestRunThenPlanIsUnchanged(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	st := mustStore(t)

	if _, err := apply.Run(context.Background(), []resource.Resource{res}, lookupProvider(p), st, ioDiscard()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("plan after apply: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("plan after apply has changes: %+v", got.Changes)
	}
	id, ok, err := st.Identity(res.Address)
	if err != nil || !ok {
		t.Fatalf("Identity = (%v,%v,%v)", id, ok, err)
	}
	if got.Changes[0].Identity.ID != id.ID {
		t.Fatalf("plan identity = %q, want persisted %q", got.Changes[0].Identity.ID, id.ID)
	}
}

func TestRunRequiresStateStore(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	_, err := apply.Run(context.Background(), []resource.Resource{res}, lookupProvider(p), nil, ioDiscard())
	if err == nil || !strings.Contains(err.Error(), "state store is required") {
		t.Fatalf("error = %v, want state store required", err)
	}
	_, creates, _, _ := p.Calls()
	if creates != 0 {
		t.Fatalf("creates = %d, want 0", creates)
	}
}

func TestFormat(t *testing.T) {
	t.Parallel()

	got := apply.Format(apply.Result{Created: 2, Updated: 1})
	want := "Apply complete! 2 created, 1 updated.\n"
	if got != want {
		t.Fatalf("Format = %q, want %q", got, want)
	}
}

func widget(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{
		Address:    mustAddress(t, "fake.widget."+name),
		Attributes: attrs,
	}
}

func seed(t *testing.T, p *fake.Provider, res resource.Resource, computed resource.Attributes) {
	t.Helper()
	if err := p.Seed(resource.RemoteResource{
		Address:    res.Address,
		Identity:   resource.Identity{ID: "id-" + res.Address.Name},
		Attributes: res.Attributes.Clone(),
		Computed:   computed.Clone(),
	}); err != nil {
		t.Fatalf("Seed: %v", err)
	}
}

func lookupProvider(p provider.Provider) apply.Lookup {
	return func(resource.Address) (provider.Provider, error) {
		return p, nil
	}
}

func updatePlan(desired resource.Resource, before resource.Attributes, identity string) *plan.Plan {
	return &plan.Plan{
		Changes: []plan.Change{
			{
				Address:  desired.Address,
				Action:   plan.ActionUpdate,
				Identity: resource.Identity{ID: identity},
				Before:   before.Clone(),
				After:    desired.Attributes.Clone(),
			},
		},
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

func mustAddress(t *testing.T, s string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress(s)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func ioDiscard() *bytes.Buffer {
	return &bytes.Buffer{}
}

func assertNoSecret(t *testing.T, s string) {
	t.Helper()
	if strings.Contains(s, plantedSecret) {
		t.Fatalf("secret leaked in %q", s)
	}
}

func assertStateHasNoSecret(t *testing.T, st *state.Store) {
	t.Helper()
	raw, err := os.ReadFile(st.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), plantedSecret) {
		t.Fatal("state file contained a secret")
	}
	if strings.Contains(string(raw), "token") {
		t.Fatalf("state file contained credential-like data: %s", raw)
	}
}

type recordingProvider struct {
	provider.Provider
	mu  sync.Mutex
	ops []string
}

func (p *recordingProvider) Create(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	p.mu.Lock()
	p.ops = append(p.ops, "create:"+res.Address.String())
	p.mu.Unlock()
	return p.Provider.Create(ctx, res)
}

func (p *recordingProvider) Update(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	p.mu.Lock()
	p.ops = append(p.ops, "update:"+desired.Address.String())
	p.mu.Unlock()
	return p.Provider.Update(ctx, desired, actual)
}

type scriptedProvider struct {
	provider.Provider
	createErr           error
	updateErr           error
	readErr             error
	stripCreateIdentity bool
	stripReadIdentity   bool
	readIdentity        resource.Identity
	updateIdentity      resource.Identity
	updates             int
}

func (p *scriptedProvider) Create(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if p.createErr != nil {
		return resource.RemoteResource{}, p.createErr
	}
	live, err := p.Provider.Create(ctx, res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if p.stripCreateIdentity {
		live.Identity = resource.Identity{}
	}
	return live, nil
}

func (p *scriptedProvider) Read(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if p.readErr != nil {
		return resource.RemoteResource{}, p.readErr
	}
	live, err := p.Provider.Read(ctx, res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if p.stripReadIdentity {
		live.Identity = resource.Identity{}
	}
	if !p.readIdentity.IsZero() {
		live.Identity = p.readIdentity
	}
	return live, nil
}

func (p *scriptedProvider) Update(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	p.updates++
	if p.updateErr != nil {
		return resource.RemoteResource{}, p.updateErr
	}
	live, err := p.Provider.Update(ctx, desired, actual)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if !p.updateIdentity.IsZero() {
		live.Identity = p.updateIdentity
	}
	return live, nil
}

type computedAwareProvider struct {
	provider.Provider
	updates    int
	lastRead   resource.Resource
	lastActual resource.RemoteResource
}

func (p *computedAwareProvider) Read(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	p.lastRead = res
	return p.Provider.Read(ctx, res)
}

func (p *computedAwareProvider) Update(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	p.updates++
	p.lastActual = actual
	if actual.Computed == nil || actual.Computed["etag"] != liveETag {
		return resource.RemoteResource{}, fmt.Errorf("update requires live etag %q; actual.Computed=%v", liveETag, actual.Computed)
	}
	if _, ok := actual.Computed[fake.AttrSerial]; !ok {
		return resource.RemoteResource{}, fmt.Errorf("update requires live computed %s", fake.AttrSerial)
	}
	return p.Provider.Update(ctx, desired, actual)
}

type failingStore struct {
	err error
}

func (s *failingStore) RecordCreate(resource.Address, resource.RemoteResource) error {
	return s.err
}

func (s *failingStore) RecordUpdate(resource.Address, resource.RemoteResource) error {
	return s.err
}

type failingRecordStore struct {
	*state.Store
	err           error
	recordUpdates int
}

func (s *failingRecordStore) RecordCreate(resource.Address, resource.RemoteResource) error {
	return s.err
}

func (s *failingRecordStore) RecordUpdate(resource.Address, resource.RemoteResource) error {
	s.recordUpdates++
	return s.err
}

func requirePartial(t *testing.T, err error) *apply.PartialApplyError {
	t.Helper()
	var partial *apply.PartialApplyError
	if !errors.As(err, &partial) {
		t.Fatalf("error = %v (%T), want PartialApplyError", err, err)
	}
	if !partial.RemoteMutation {
		t.Fatalf("partial.RemoteMutation = false, want true")
	}
	return partial
}

var _ apply.Store = (*failingStore)(nil)
var _ apply.Store = (*failingRecordStore)(nil)
var _ apply.Persistence = (*state.Store)(nil)
