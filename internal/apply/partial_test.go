package apply_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
)

func TestExecuteCreateAtomicStateWriteFailurePreservesFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, state.DefaultFilename)
	st, err := state.New(path)
	if err != nil {
		t.Fatal(err)
	}
	existing := widget(t, "existing", resource.Attributes{fake.AttrTitle: "Existing"})
	if err := st.Bind(existing.Address, resource.Identity{ID: "widget-keep"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	planned := &plan.Plan{
		Changes: []plan.Change{{Address: res.Address, Action: plan.ActionCreate, After: res.Attributes}},
	}

	restore := blockStateFileReplace(t, path)
	t.Cleanup(restore)
	_, err = apply.Execute(context.Background(), planned, []resource.Resource{res}, lookupProvider(p), st, ioDiscard())
	restore()
	if err == nil {
		t.Fatal("Execute succeeded, want atomic state write failure")
	}
	partial := requirePartial(t, err)
	if partial.Operation != "create" || partial.RemoteIdentity.ID == "" {
		t.Fatalf("partial = %+v, want create with remote identity", partial)
	}
	if !strings.Contains(err.Error(), "agoraform import "+res.Address.String()+" "+partial.RemoteIdentity.ID) {
		t.Fatalf("error = %q, want import recovery after state-file problem", err)
	}
	if strings.Contains(err.Error(), "already bound") {
		t.Fatalf("error = %q, filesystem failure should not look like an ownership conflict", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("previous state changed after failed atomic write:\n got %s\nwant %s", got, original)
	}

	reloaded, err := state.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := reloaded.Identity(res.Address); ok {
		t.Fatal("failed persist wrote a binding for the created resource")
	}
	id, ok, identErr := reloaded.Identity(existing.Address)
	if identErr != nil || !ok || id.ID != "widget-keep" {
		t.Fatalf("preserved identity = (%v,%v,%v), want widget-keep", id, ok, identErr)
	}

	_, creates, _, _ := p.Calls()
	if creates != 1 {
		t.Fatalf("creates = %d, want 1", creates)
	}
}

func TestExecuteCreateIdentityConflictDoesNotSuggestImport(t *testing.T) {
	t.Parallel()

	st := mustStore(t)
	owner := widget(t, "other", resource.Attributes{fake.AttrTitle: "Other"})
	if err := st.Bind(owner.Address, resource.Identity{ID: "widget-1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	planned := &plan.Plan{
		Changes: []plan.Change{{Address: res.Address, Action: plan.ActionCreate, After: res.Attributes}},
	}

	_, err := apply.Execute(context.Background(), planned, []resource.Resource{res}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Execute succeeded, want identity conflict")
	}
	partial := requirePartial(t, err)
	if partial.RemoteIdentity.ID != "widget-1" {
		t.Fatalf("remote identity = %q, want widget-1", partial.RemoteIdentity.ID)
	}
	if !strings.Contains(err.Error(), "already bound") || !strings.Contains(err.Error(), owner.Address.String()) {
		t.Fatalf("error = %q, want ownership conflict with %s", err, owner.Address)
	}
	if strings.Contains(err.Error(), "agoraform import") {
		t.Fatalf("error = %q, must not suggest import for an ownership conflict", err)
	}

	if _, ok, _ := st.Identity(res.Address); ok {
		t.Fatal("conflicting create wrote a binding")
	}
	id, ok, identErr := st.Identity(owner.Address)
	if identErr != nil || !ok || id.ID != "widget-1" {
		t.Fatalf("owner identity = (%v,%v,%v), want widget-1", id, ok, identErr)
	}
}

func TestRunResourceSuccessFinalizationFailureBeforeMutationIsPartial(t *testing.T) {
	t.Parallel()

	p := &finalizingProvider{Provider: fake.New(), finalizeErr: errors.New("activation rejected")}
	reg := registerProvider(t, p)
	st := newStateStore(t)
	res := resource.Resource{
		Address:    resource.Address{Provider: fake.Name, Type: fake.TypeWidget, Name: "one"},
		Attributes: resource.Attributes{fake.AttrTitle: "One"},
	}
	var out bytes.Buffer

	_, err := apply.Run(context.Background(), []resource.Resource{res}, nil, st, &out, reg)
	if err == nil {
		t.Fatal("Run succeeded, want finalization failure")
	}
	partial := requirePartial(t, err)
	if partial.Stage != apply.StageFinalize || !partial.ResourceChanges {
		t.Fatalf("partial = %+v, want finalize after resource changes", partial)
	}
	if !strings.Contains(err.Error(), "Earlier resource changes remain applied") {
		t.Fatalf("error = %q, want prior resource mutations remain applied", err)
	}
	if strings.Contains(err.Error(), "rolled back") && !strings.Contains(err.Error(), "were not rolled back") {
		t.Fatalf("error = %q, must not imply rollback", err)
	}
	if !strings.Contains(err.Error(), "agoraform plan") || !strings.Contains(err.Error(), "agoraform apply") {
		t.Fatalf("error = %q, want plan/apply retry guidance", err)
	}
	if strings.Contains(out.String(), "Apply complete!") {
		t.Fatalf("failed apply claimed complete:\n%s", out.String())
	}

	id, ok, identErr := st.Identity(res.Address)
	if identErr != nil || !ok || id.ID == "" {
		t.Fatalf("resource identity = (%v,%v,%v), want persisted after resource success", id, ok, identErr)
	}
}

func TestRunFinalizationOnlyFailureBeforeMutationIsNotPartial(t *testing.T) {
	t.Parallel()

	p := &finalizingProvider{
		Provider:    fake.New(),
		alwaysPlan:  true,
		finalizeErr: errors.New("activation rejected"),
	}
	reg := registerProvider(t, p)
	st := newStateStore(t)

	_, err := apply.Run(context.Background(), nil, nil, st, &bytes.Buffer{}, reg)
	if err == nil {
		t.Fatal("Run succeeded, want finalization failure")
	}
	if apply.IsPartial(err) {
		t.Fatalf("pre-mutation finalization-only failure classified as partial: %v", err)
	}
	if !strings.Contains(err.Error(), "activation rejected") {
		t.Fatalf("error = %q, want provider message", err)
	}
}

func TestRunValidationFailureIsNotPartial(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrColor: "blue"})
	st := mustStore(t)

	_, err := apply.Run(context.Background(), []resource.Resource{res}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Run succeeded, want validation error")
	}
	if apply.IsPartial(err) {
		t.Fatalf("pre-mutation validation failure classified as partial: %v", err)
	}
}

func TestRunUnsupportedActionIsNotPartial(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	st := mustStore(t)
	planned := &plan.Plan{
		Changes: []plan.Change{{Address: res.Address, Action: plan.Action("delete")}},
	}

	_, err := apply.Execute(context.Background(), planned, []resource.Resource{res}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Execute succeeded, want unsupported action")
	}
	if apply.IsPartial(err) {
		t.Fatalf("pre-mutation unsupported action classified as partial: %v", err)
	}
	_, creates, updates, imports := p.Calls()
	if creates != 0 || updates != 0 || imports != 0 {
		t.Fatalf("pre-mutation failure mutated provider: creates=%d updates=%d imports=%d", creates, updates, imports)
	}
}
