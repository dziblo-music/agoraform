package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

type finalizingFake struct {
	*fake.Provider
	finalized bool
}

func (p *finalizingFake) Configure(resource.Attributes) error { return nil }

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

func TestFinalizationsArePlannedAndExecutedSeparately(t *testing.T) {
	t.Parallel()

	p := &finalizingFake{Provider: fake.New()}
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	planned := &plan.Plan{Changes: []plan.Change{{
		Address: resource.Address{Provider: fake.Name, Type: fake.TypeWidget, Name: "one"},
		Action:  plan.ActionUpdate,
	}}}
	if err := attachFinalizations(context.Background(), reg, planned); err != nil {
		t.Fatalf("attachFinalizations: %v", err)
	}
	if len(planned.Finalizations) != 1 {
		t.Fatalf("finalizations = %v, want one", planned.Finalizations)
	}

	var out bytes.Buffer
	if err := executeFinalizations(context.Background(), reg, planned.Finalizations, &out); err != nil {
		t.Fatalf("executeFinalizations: %v", err)
	}
	if !p.finalized {
		t.Fatal("finalizer was not executed")
	}
	if got := out.String(); got != "fake.deployment.main: activated test\n" {
		t.Fatalf("output = %q", got)
	}
}
