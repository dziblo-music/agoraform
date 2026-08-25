package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

type finalizingFake struct {
	*fake.Provider
	finalized  bool
	failCreate bool
}

func (p *finalizingFake) Configure(resource.Attributes) error { return nil }

func (p *finalizingFake) Create(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if p.failCreate {
		return resource.RemoteResource{}, errors.New("synthetic create failure")
	}
	return p.Provider.Create(ctx, res)
}

func (p *finalizingFake) PlanFinalization(_ context.Context, pending []provider.PendingChange) (*provider.FinalizationPlan, error) {
	if len(pending) == 0 {
		return nil, nil
	}
	return &provider.FinalizationPlan{
		Address: resource.Address{Provider: fake.Name, Type: "deployment", Name: "main"},
		Action:  "activate",
		Target:  "test",
	}, nil
}

func (p *finalizingFake) Finalize(_ context.Context, planned provider.FinalizationPlan) (provider.FinalizationResult, error) {
	p.finalized = true
	return provider.FinalizationResult{
		Address: planned.Address,
		Changed: true,
		Details: []string{"activated test"},
	}, nil
}

func TestPlanSurfacesProviderFinalization(t *testing.T) {
	t.Parallel()

	path := writeFinalizationManifest(t)
	p := &finalizingFake{Provider: fake.New()}
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := ExecuteWithRegistry(IOStreams{Out: &stdout, ErrOut: &stderr}, []string{"plan", "-f", path}, reg)
	if code != ExitChanges {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, ExitChanges, stderr.String())
	}
	if !strings.Contains(stdout.String(), "> fake.deployment.main: activate -> test") {
		t.Fatalf("plan output missing provider finalization:\n%s", stdout.String())
	}
	if p.finalized {
		t.Fatal("plan executed provider finalization")
	}
}

func TestApplyUsesCanonicalFinalizationLifecycle(t *testing.T) {
	t.Parallel()

	path := writeFinalizationManifest(t)
	p := &finalizingFake{Provider: fake.New()}
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := ExecuteWithRegistry(IOStreams{Out: &stdout, ErrOut: &stderr}, []string{"apply", "-f", path}, reg)
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", code, ExitOK, stdout.String(), stderr.String())
	}
	if !p.finalized {
		t.Fatal("apply did not execute provider finalization")
	}
	if !strings.Contains(stdout.String(), "fake.deployment.main: activated test") {
		t.Fatalf("apply output missing finalization detail:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Apply complete! 1 created, 0 updated, 1 provider action completed.") {
		t.Fatalf("apply summary missing provider action:\n%s", stdout.String())
	}
}

func TestApplyResourceFailurePreventsFinalization(t *testing.T) {
	t.Parallel()

	path := writeFinalizationManifest(t)
	p := &finalizingFake{Provider: fake.New(), failCreate: true}
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := ExecuteWithRegistry(IOStreams{Out: &stdout, ErrOut: &stderr}, []string{"apply", "-f", path}, reg)
	if code != ExitError {
		t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", code, ExitError, stdout.String(), stderr.String())
	}
	if p.finalized {
		t.Fatal("provider finalization ran after resource mutation failed")
	}
	if strings.Contains(stdout.String(), "activated test") {
		t.Fatalf("finalization output appeared after failure: %q", stdout.String())
	}
}

func TestProviderSpecificPublishCommandIsNotExposed(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := ExecuteWithRegistry(IOStreams{Out: &stdout, ErrOut: &stderr}, []string{"publish"}, reg)
	if code != ExitUsage {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, ExitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("stderr = %q, want unknown command", stderr.String())
	}
}

func writeFinalizationManifest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "agoraform.yaml")
	content := `apiVersion: agoraform.io/v1alpha1
providers:
  fake: {}
resources:
  - address: fake.widget.one
    attributes:
      title: One
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
