package destroy_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/destroy"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestBuildPlansProviderNativeRemove(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, inner, res)

	st := mustStore(t)
	bind(t, st, res, "id-homepage")

	p := &removeDestroyer{Provider: inner}
	planned, err := destroy.Build(context.Background(), []resource.Resource{res}, lookupProvider(p), st)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(planned.Changes) != 1 || planned.Changes[0].Kind != destroy.KindRemove {
		t.Fatalf("changes = %+v, want one remove", planned.Changes)
	}
	if got := destroy.Format(planned); !strings.Contains(got, "fake.widget.homepage (remove)") {
		t.Fatalf("remove operation not visible in plan:\n%s", got)
	}
}

func TestRunFinalizationFailureKeepsStateForRetry(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, inner, res)

	st := mustStore(t)
	bind(t, st, res, "id-homepage")

	p := &retryFinalizingDestroyer{Provider: inner, failFinalize: true}
	result, err := destroy.Run(context.Background(), []resource.Resource{res}, lookupProvider(p), st, nil, nil)
	if err == nil {
		t.Fatal("Run succeeded, want finalization failure")
	}
	if !destroy.IsPartial(err) {
		t.Fatalf("error = %v, want partial destroy error", err)
	}
	if result.Destroyed != 1 || result.Finalized != 0 {
		t.Fatalf("result = %+v, want remote destroy before failed finalization", result)
	}
	id, ok, identErr := st.Identity(res.Address)
	if identErr != nil || !ok || id.ID != "id-homepage" {
		t.Fatalf("identity after failed finalization = (%v, %v, %v), want preserved binding", id, ok, identErr)
	}
	if _, readErr := inner.Read(context.Background(), resource.Resource{Address: res.Address, Identity: resource.Identity{ID: "id-homepage"}}); !errors.Is(readErr, provider.ErrNotFound) {
		t.Fatalf("remote resource should already be terminal, Read err = %v", readErr)
	}

	p.failFinalize = false
	result, err = destroy.Run(context.Background(), []resource.Resource{res}, lookupProvider(p), st, nil, nil)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if result.AlreadyAbsent != 1 || result.Finalized != 1 {
		t.Fatalf("retry result = %+v, want already-absent convergence plus finalization", result)
	}
	assertMissing(t, st, res.Address)
	if p.finalizeCalls != 2 {
		t.Fatalf("finalizeCalls = %d, want 2", p.finalizeCalls)
	}
}

func TestRunInvalidDestroyStatusPreservesState(t *testing.T) {
	t.Parallel()

	for _, status := range []provider.DestroyStatus{"", "pending"} {
		status := status
		name := string(status)
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			inner := fake.New()
			res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
			seed(t, inner, res)

			st := mustStore(t)
			bind(t, st, res, "id-homepage")

			p := &invalidStatusDestroyer{Provider: inner, status: status}
			_, err := destroy.Run(context.Background(), []resource.Resource{res}, lookupProvider(p), st, nil, nil)
			if err == nil {
				t.Fatal("Run succeeded, want invalid status failure")
			}
			if !destroy.IsPartial(err) {
				t.Fatalf("error = %v, want partial destroy error", err)
			}
			if !strings.Contains(err.Error(), "invalid destroy status") {
				t.Fatalf("error = %q, want invalid status diagnostic", err.Error())
			}
			id, ok, identErr := st.Identity(res.Address)
			if identErr != nil || !ok || id.ID != "id-homepage" {
				t.Fatalf("identity after invalid status = (%v, %v, %v), want preserved binding", id, ok, identErr)
			}
		})
	}
}

type removeDestroyer struct {
	provider.Provider
}

func (p *removeDestroyer) DestroyCapability(resource.Resource) (provider.DestroyCapability, error) {
	return provider.DestroyRemove, nil
}

func (p *removeDestroyer) Destroy(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	return p.Provider.(provider.Destroyer).Destroy(ctx, res)
}

type invalidStatusDestroyer struct {
	provider.Provider
	status provider.DestroyStatus
}

func (p *invalidStatusDestroyer) DestroyCapability(resource.Resource) (provider.DestroyCapability, error) {
	return provider.DestroyDelete, nil
}

func (p *invalidStatusDestroyer) Destroy(context.Context, resource.Resource) (provider.DestroyResult, error) {
	return provider.DestroyResult{Status: p.status}, nil
}

type retryFinalizingDestroyer struct {
	provider.Provider
	failFinalize bool
	finalizeCalls int
}

func (p *retryFinalizingDestroyer) DestroyCapability(resource.Resource) (provider.DestroyCapability, error) {
	return provider.DestroyDelete, nil
}

func (p *retryFinalizingDestroyer) Destroy(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	return p.Provider.(provider.Destroyer).Destroy(ctx, res)
}

func (p *retryFinalizingDestroyer) PlanFinalization(_ context.Context, pending []provider.PendingChange) (*provider.FinalizationPlan, error) {
	if len(pending) == 0 {
		return nil, nil
	}
	return &provider.FinalizationPlan{
		Address: pending[0].Address,
		Action:  "publish",
		Target:  "live",
	}, nil
}

func (p *retryFinalizingDestroyer) Finalize(_ context.Context, planned provider.FinalizationPlan) (provider.FinalizationResult, error) {
	p.finalizeCalls++
	result := provider.FinalizationResult{Address: planned.Address}
	if p.failFinalize {
		return result, errors.New("publication rejected")
	}
	result.Changed = true
	result.Details = []string{"published to live"}
	return result, nil
}
