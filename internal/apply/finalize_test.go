package apply_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
)

type finalizingProvider struct {
	*fake.Provider
	alwaysPlan   bool
	failCreate   error
	finalizeErr  error
	finalized    bool
	planned      provider.FinalizationPlan
	finalDetails []string
	finalChanged bool
}

func (p *finalizingProvider) Create(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if p.failCreate != nil {
		return resource.RemoteResource{}, p.failCreate
	}
	return p.Provider.Create(ctx, res)
}

func (p *finalizingProvider) PlanFinalization(_ context.Context, pending []provider.PendingChange) (*provider.FinalizationPlan, error) {
	if len(pending) == 0 && !p.alwaysPlan {
		return nil, nil
	}
	planned := provider.FinalizationPlan{
		Address:     resource.Address{Provider: fake.Name, Type: "deployment", Name: "main"},
		Action:      "activate",
		Target:      "test",
		Conditional: len(pending) > 0,
	}
	return &planned, nil
}

func (p *finalizingProvider) Finalize(_ context.Context, planned provider.FinalizationPlan) (provider.FinalizationResult, error) {
	p.finalized = true
	p.planned = planned
	details := p.finalDetails
	if details == nil && p.finalizeErr == nil {
		details = []string{"activated test"}
	}
	return provider.FinalizationResult{
		Address: planned.Address,
		Details: details,
		Changed: p.finalizeErr == nil || p.finalChanged,
	}, p.finalizeErr
}

func TestRunPlansAndExecutesProviderFinalization(t *testing.T) {
	t.Parallel()

	p := &finalizingProvider{Provider: fake.New()}
	st := newStateStore(t)
	res := resource.Resource{
		Address:    resource.Address{Provider: fake.Name, Type: fake.TypeWidget, Name: "one"},
		Attributes: resource.Attributes{fake.AttrTitle: "One"},
	}
	var out bytes.Buffer

	result, err := apply.Run(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Provider, error) {
		return p, nil
	}, st, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Created != 1 || result.Updated != 0 || result.Finalized != 1 {
		t.Fatalf("result = %+v, want 1 created and 1 finalization", result)
	}
	if !p.finalized {
		t.Fatal("provider finalization did not run")
	}
	if !p.planned.Conditional {
		t.Fatalf("planned finalization = %+v, want conditional metadata preserved", p.planned)
	}
	got := out.String()
	if !strings.Contains(got, "fake.widget.one: created") || !strings.Contains(got, "fake.deployment.main: activated test") {
		t.Fatalf("output missing resource/finalization progress:\n%s", got)
	}
	if !strings.Contains(got, "Apply complete! 1 created, 0 updated, 1 provider action completed.") {
		t.Fatalf("output missing finalization summary:\n%s", got)
	}
}

func TestRunSupportsFinalizationOnlyApply(t *testing.T) {
	t.Parallel()

	p := &finalizingProvider{Provider: fake.New(), alwaysPlan: true}
	reg := registerProvider(t, p)
	st := newStateStore(t)
	var out bytes.Buffer

	result, err := apply.Run(context.Background(), nil, nil, st, &out, reg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Created != 0 || result.Updated != 0 || result.Finalized != 1 {
		t.Fatalf("result = %+v, want finalization-only success", result)
	}
	if !p.finalized {
		t.Fatal("finalization-only apply did not run finalizer")
	}
	if !strings.Contains(out.String(), "Apply complete! 0 created, 0 updated, 1 provider action completed.") {
		t.Fatalf("summary did not report provider action:\n%s", out.String())
	}
}

func TestRunResourceFailurePreventsProviderFinalization(t *testing.T) {
	t.Parallel()

	p := &finalizingProvider{Provider: fake.New(), failCreate: errors.New("synthetic create failure")}
	reg := registerProvider(t, p)
	st := newStateStore(t)
	res := resource.Resource{
		Address:    resource.Address{Provider: fake.Name, Type: fake.TypeWidget, Name: "one"},
		Attributes: resource.Attributes{fake.AttrTitle: "One"},
	}

	_, err := apply.Run(context.Background(), []resource.Resource{res}, nil, st, &bytes.Buffer{}, reg)
	if err == nil {
		t.Fatal("Run succeeded, want create failure")
	}
	if apply.IsPartial(err) {
		t.Fatalf("pre-mutation create failure classified as partial: %v", err)
	}
	if p.finalized {
		t.Fatal("provider finalization ran after resource mutation failure")
	}
}

func TestRunStateWriteFailurePreventsProviderFinalization(t *testing.T) {
	t.Parallel()

	p := &finalizingProvider{Provider: fake.New()}
	reg := registerProvider(t, p)
	st := &failingPersistence{err: errors.New("disk full")}
	res := resource.Resource{
		Address:    resource.Address{Provider: fake.Name, Type: fake.TypeWidget, Name: "one"},
		Attributes: resource.Attributes{fake.AttrTitle: "One"},
	}

	_, err := apply.Run(context.Background(), []resource.Resource{res}, nil, st, &bytes.Buffer{}, reg)
	if err == nil {
		t.Fatal("Run succeeded, want state write failure")
	}
	if !apply.IsPartial(err) {
		t.Fatalf("create persist failure was not partial: %v", err)
	}
	if p.finalized {
		t.Fatal("provider finalization ran after state persistence failure")
	}
}

func TestRunFinalizationFailurePreservesDetailsAndDoesNotClaimSuccess(t *testing.T) {
	t.Parallel()

	p := &finalizingProvider{
		Provider:     fake.New(),
		alwaysPlan:   true,
		finalizeErr:  errors.New("activation rejected"),
		finalDetails: []string{"prepared deployment"},
		finalChanged: true,
	}
	reg := registerProvider(t, p)
	st := newStateStore(t)
	var out bytes.Buffer

	_, err := apply.Run(context.Background(), nil, nil, st, &out, reg)
	if err == nil {
		t.Fatal("Run succeeded, want finalization failure")
	}
	partial := requirePartial(t, err)
	if partial.Stage != apply.StageFinalize || partial.ResourceChanges {
		t.Fatalf("partial = %+v, want finalize without resource CRUD", partial)
	}
	if !strings.Contains(err.Error(), "fake.deployment.main") || !strings.Contains(err.Error(), "activate") {
		t.Fatalf("error = %q, want finalization address/action", err)
	}
	if !strings.Contains(err.Error(), "Remote provider state may already have changed") {
		t.Fatalf("error = %q, want partial remote mutation guidance", err)
	}
	if !strings.Contains(out.String(), "fake.deployment.main: prepared deployment") {
		t.Fatalf("finalization detail missing:\n%s", out.String())
	}
	if strings.Contains(out.String(), "Apply complete!") {
		t.Fatalf("failed finalization claimed apply complete:\n%s", out.String())
	}
}

type uncertainFinalizeError struct{}

func (uncertainFinalizeError) Error() string {
	return "publication outcome is uncertain: empty response"
}

func (uncertainFinalizeError) UncertainOutcome() {}

func TestRunFinalizationUncertainOutcomeDoesNotEncourageRetry(t *testing.T) {
	t.Parallel()

	p := &finalizingProvider{
		Provider:     fake.New(),
		alwaysPlan:   true,
		finalizeErr:  uncertainFinalizeError{},
		finalDetails: []string{"version 10 created"},
		finalChanged: true,
	}
	reg := registerProvider(t, p)
	st := newStateStore(t)
	var out bytes.Buffer

	_, err := apply.Run(context.Background(), nil, nil, st, &out, reg)
	if err == nil {
		t.Fatal("Run succeeded, want uncertain finalization failure")
	}
	partial := requirePartial(t, err)
	if partial.Stage != apply.StageFinalize {
		t.Fatalf("partial = %+v, want finalize stage", partial)
	}
	if !strings.Contains(err.Error(), "do not create another version") {
		t.Fatalf("error = %q, want guidance against creating another version", err)
	}
	if strings.Contains(err.Error(), "rerun agoraform plan and agoraform apply") {
		t.Fatalf("error = %q, uncertain outcome should not encourage blind retry", err)
	}
	if !strings.Contains(out.String(), "version 10 created") {
		t.Fatalf("created-version detail missing:\n%s", out.String())
	}
}

func TestRunFinalizationDetailsWithoutMutationAreNotPartial(t *testing.T) {
	t.Parallel()

	p := &finalizingProvider{
		Provider:     fake.New(),
		alwaysPlan:   true,
		finalizeErr:  errors.New("activation rejected"),
		finalDetails: []string{"validated deployment"},
	}
	reg := registerProvider(t, p)
	st := newStateStore(t)
	var out bytes.Buffer

	_, err := apply.Run(context.Background(), nil, nil, st, &out, reg)
	if err == nil {
		t.Fatal("Run succeeded, want finalization failure")
	}
	if apply.IsPartial(err) {
		t.Fatalf("non-mutating finalization failure classified as partial: %v", err)
	}
	if !strings.Contains(out.String(), "fake.deployment.main: validated deployment") {
		t.Fatalf("finalization detail missing:\n%s", out.String())
	}
}

func registerProvider(t *testing.T, p provider.Provider) *provider.Registry {
	t.Helper()
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	return reg
}

func newStateStore(t *testing.T) *state.Store {
	t.Helper()
	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

type failingPersistence struct {
	err error
}

func (p *failingPersistence) Identity(resource.Address) (resource.Identity, bool, error) {
	return resource.Identity{}, false, nil
}

func (p *failingPersistence) RecordCreate(resource.Address, resource.RemoteResource) error {
	return p.err
}

func (p *failingPersistence) RecordUpdate(resource.Address, resource.RemoteResource) error {
	return p.err
}

var _ apply.Persistence = (*failingPersistence)(nil)
