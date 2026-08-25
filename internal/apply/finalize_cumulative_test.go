package apply_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

type sequencedFinalizer struct {
	*fake.Provider
	name        string
	changed     bool
	finalizeErr error
}

func (p *sequencedFinalizer) Name() string {
	return p.name
}

func (p *sequencedFinalizer) PlanFinalization(context.Context, []provider.PendingChange) (*provider.FinalizationPlan, error) {
	return &provider.FinalizationPlan{
		Address: resource.Address{Provider: p.name, Type: "deployment", Name: "main"},
		Action:  "activate",
		Target:  "test",
	}, nil
}

func (p *sequencedFinalizer) Finalize(_ context.Context, planned provider.FinalizationPlan) (provider.FinalizationResult, error) {
	result := provider.FinalizationResult{
		Address: planned.Address,
		Changed: p.changed,
	}
	if p.changed {
		result.Details = []string{"activated test"}
	}
	return result, p.finalizeErr
}

func TestRunLaterFinalizationFailureIsPartialAfterEarlierFinalizationMutation(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	first := &sequencedFinalizer{Provider: fake.New(), name: "alpha", changed: true}
	second := &sequencedFinalizer{Provider: fake.New(), name: "beta", finalizeErr: errors.New("activation rejected")}
	if err := reg.Register(first); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(second); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	_, err := apply.Run(context.Background(), nil, nil, newStateStore(t), &out, reg)
	if err == nil {
		t.Fatal("Run succeeded, want second finalization failure")
	}
	partial := requirePartial(t, err)
	if partial.Stage != apply.StageFinalize || partial.ResourceChanges {
		t.Fatalf("partial = %+v, want provider-only cumulative finalization failure", partial)
	}
	if !strings.Contains(err.Error(), "Remote provider state may already have changed") {
		t.Fatalf("error = %q, want prior finalization mutation guidance", err)
	}
	if !strings.Contains(out.String(), "alpha.deployment.main: activated test") {
		t.Fatalf("first finalization progress missing:\n%s", out.String())
	}
}
