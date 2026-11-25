package destroy

import (
	"context"
	"errors"
	"testing"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

type failSecondFinalizer struct {
	*fake.Provider
	calls int
}

func (p *failSecondFinalizer) PlanFinalization(context.Context, []provider.PendingChange) (*provider.FinalizationPlan, error) {
	return nil, nil
}

func (p *failSecondFinalizer) Finalize(_ context.Context, plan provider.FinalizationPlan) (provider.FinalizationResult, error) {
	p.calls++
	if p.calls == 2 {
		return provider.FinalizationResult{Address: plan.Address}, errors.New("publish rejected")
	}
	return provider.FinalizationResult{Address: plan.Address, Changed: true}, nil
}

// A provider counts as finalized only when every one of its finalizations
// succeeded; otherwise its deferred bindings must stay in state for retry.
func TestExecuteFinalizationsPartialProviderFailureIsNotSuccess(t *testing.T) {
	t.Parallel()

	first, err := resource.ParseAddress("fake.widget.one")
	if err != nil {
		t.Fatal(err)
	}
	second, err := resource.ParseAddress("fake.widget.two")
	if err != nil {
		t.Fatal(err)
	}
	fin := &failSecondFinalizer{Provider: fake.New()}
	lookup := func(resource.Address) (provider.Provider, error) { return fin, nil }
	run := &execution{incomplete: map[string]struct{}{}, completed: []Change{{Address: first, Kind: KindDestroy}}}
	plans := []provider.FinalizationPlan{
		{Address: first, Action: "publish"},
		{Address: second, Action: "publish"},
	}

	outcome, err := executeFinalizations(context.Background(), nil, lookup, plans, nil, run)
	if err == nil {
		t.Fatal("expected the second finalization to fail")
	}
	if outcome.completed != 1 {
		t.Fatalf("completed = %d, want 1", outcome.completed)
	}
	if _, ok := outcome.succeeded[fake.Name]; ok {
		t.Fatal("provider marked succeeded although one of its finalizations failed")
	}
}
