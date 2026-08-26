package plan_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

type adoptReader struct {
	setErr     error
	readCalled bool
}

func (r *adoptReader) Name() string { return "fake" }
func (r *adoptReader) ResourceTypes() []string { return []string{"widget"} }
func (r *adoptReader) Validate(context.Context, resource.Resource) error { return nil }
func (r *adoptReader) Read(context.Context, resource.Resource) (resource.RemoteResource, error) {
	r.readCalled = true
	return resource.RemoteResource{}, provider.ErrNotFound
}
func (r *adoptReader) PlanMissingResource(resource.Resource) (provider.MissingResourceMode, error) {
	return provider.MissingResourceAdopt, nil
}
func (r *adoptReader) ValidateResourceSet(context.Context, []resource.Resource) error {
	return r.setErr
}

func TestBuildRepresentsProviderCreatedMissingResourceAsAdopt(t *testing.T) {
	addr, err := resource.ParseAddress("fake.widget.example")
	if err != nil {
		t.Fatal(err)
	}
	reader := &adoptReader{}
	got, err := plan.Build(context.Background(), []resource.Resource{{
		Address:    addr,
		Attributes: resource.Attributes{"enabled": true},
	}}, func(resource.Address) (provider.Reader, error) {
		return reader, nil
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got.Changes) != 1 {
		t.Fatalf("changes = %d, want 1", len(got.Changes))
	}
	change := got.Changes[0]
	if change.Action != plan.ActionCreate {
		t.Fatalf("core action = %q, want create for new local binding", change.Action)
	}
	if change.Operation != string(provider.MissingResourceAdopt) {
		t.Fatalf("operation = %q, want adopt", change.Operation)
	}
	create, update, destroy := got.Counts()
	if create != 0 || update != 0 || destroy != 0 || got.AdoptionCount() != 1 {
		t.Fatalf("counts = create:%d adopt:%d update:%d destroy:%d", create, got.AdoptionCount(), update, destroy)
	}
	out := plan.Format(got)
	if !strings.Contains(out, "* fake.widget.example (adopt)") {
		t.Fatalf("plan output missing adopt action:\n%s", out)
	}
	if !strings.Contains(out, "Plan: 0 to create, 1 to adopt, 0 to update, 0 to destroy.") {
		t.Fatalf("plan output missing adopt count:\n%s", out)
	}
}

func TestBuildRunsResourceSetValidationBeforeRead(t *testing.T) {
	addr, err := resource.ParseAddress("fake.widget.example")
	if err != nil {
		t.Fatal(err)
	}
	reader := &adoptReader{setErr: errors.New("cross-resource mismatch")}
	_, err = plan.Build(context.Background(), []resource.Resource{{Address: addr}}, func(resource.Address) (provider.Reader, error) {
		return reader, nil
	})
	if err == nil || !strings.Contains(err.Error(), "cross-resource mismatch") {
		t.Fatalf("Build error = %v, want cross-resource mismatch", err)
	}
	if reader.readCalled {
		t.Fatal("Read was called before resource-set validation failed")
	}
}
