package apply_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

type acceptedCreateFailureProvider struct {
	provider.Provider
	err error
}

func (p *acceptedCreateFailureProvider) Create(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	live, err := p.Provider.Create(ctx, res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	return live, p.err
}

func TestExecutePersistsIdentityForAcceptedCreateFailure(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	p := &acceptedCreateFailureProvider{Provider: inner, err: errors.New("remote object still converging")}
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	st := mustStore(t)

	_, err := apply.Run(context.Background(), []resource.Resource{res}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Run succeeded, want recoverable partial create failure")
	}
	partial := requirePartial(t, err)
	if partial.Stage != apply.StageMutation || partial.Operation != "create" {
		t.Fatalf("partial = %+v, want create mutation failure", partial)
	}
	if partial.RemoteIdentity.IsZero() {
		t.Fatalf("partial remote identity = %+v, want accepted identity", partial.RemoteIdentity)
	}
	persisted, ok, identErr := st.Identity(res.Address)
	if identErr != nil || !ok || persisted.ID != partial.RemoteIdentity.ID {
		t.Fatalf("Identity = (%v,%v,%v), want %q", persisted, ok, identErr, partial.RemoteIdentity.ID)
	}

	// Simulate the provider having converged before the next run. Because the
	// recovery identity was persisted, plan/apply must read that object instead
	// of invoking Create a second time.
	p.err = nil
	if _, err := apply.Run(context.Background(), []resource.Resource{res}, lookupProvider(p), st, ioDiscard()); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	_, creates, _, _ := inner.Calls()
	if creates != 1 {
		t.Fatalf("creates = %d, want exactly 1 across both runs", creates)
	}
}
