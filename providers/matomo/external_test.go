package matomo_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/destroy"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestExternalContainerPlanApplyDestroyDoesNotMutate(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	srv.seed(apiContainer{
		ID:          testManagedContainerID,
		Name:        "Main Website",
		Context:     "web",
		Description: "primary",
	})
	p := testContainerProvider(t, srv)
	if !p.SupportsExternal(matomo.TypeContainer) || p.SupportsExternal(matomo.TypeGoal) {
		t.Fatalf("SupportsExternal container=%v goal=%v", p.SupportsExternal(matomo.TypeContainer), p.SupportsExternal(matomo.TypeGoal))
	}

	res := resource.Resource{
		Address:    mustContainerAddress(t, "main"),
		Ownership:  resource.OwnershipExternal,
		ExternalID: testManagedContainerID,
	}
	st := mustExternalState(t)
	lookup := func(resource.Address) (provider.Provider, error) { return p, nil }

	planned, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(addr resource.Address) (provider.Reader, error) {
		return lookup(addr)
	}, st)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if planned.HasChanges() || planned.ExternalCount() != 1 || planned.Changes[0].Action != plan.ActionExternal {
		t.Fatalf("plan = %+v", planned.Changes)
	}
	if planned.Changes[0].Identity.ID != testManagedContainerID {
		t.Fatalf("identity = %s", planned.Changes[0].Identity.ID)
	}

	result, err := apply.Run(context.Background(), []resource.Resource{res}, lookup, st, nil)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.Created != 0 || result.Updated != 0 || result.External != 1 {
		t.Fatalf("apply result = %+v", result)
	}
	ownership, ok, err := st.Ownership(res.Address)
	if err != nil || !ok || ownership != resource.OwnershipExternal {
		t.Fatalf("ownership = (%s, %v, %v)", ownership, ok, err)
	}

	destroyed, err := destroy.Run(context.Background(), []resource.Resource{res}, lookup, st, nil, nil)
	if err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if destroyed.Destroyed != 0 || destroyed.ExternalReleased != 1 {
		t.Fatalf("destroy result = %+v", destroyed)
	}
	if srv.createCount() != 0 || srv.updateCount() != 0 || srv.deleteCount() != 0 {
		t.Fatalf("container mutated creates=%d updates=%d deletes=%d", srv.createCount(), srv.updateCount(), srv.deleteCount())
	}
	if _, err := p.Import(context.Background(), res.Address, testManagedContainerID); err != nil {
		t.Fatalf("container missing after destroy: %v", err)
	}
}

func TestExternalContainerMissingIsNotCreated(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	p := testContainerProvider(t, srv)
	res := resource.Resource{
		Address:    mustContainerAddress(t, "main"),
		Ownership:  resource.OwnershipExternal,
		ExternalID: "Missing1",
	}
	_, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, nil)
	if err == nil || !errors.Is(err, provider.ErrNotFound) || !strings.Contains(err.Error(), "refusing to create") {
		t.Fatalf("error = %v", err)
	}
	if srv.createCount() != 0 || srv.updateCount() != 0 || srv.deleteCount() != 0 {
		t.Fatal("missing container was mutated")
	}
}

func TestExternalContainerChildReferenceIsValid(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{BaseURL: "https://matomo.example.com", TokenAuth: "token", SiteID: "3"})
	container := resource.Resource{
		Address:    mustContainerAddress(t, "main"),
		Ownership:  resource.OwnershipExternal,
		ExternalID: testManagedContainerID,
	}
	triggerAddr, err := resource.ParseAddress("matomo.trigger.trial_started")
	if err != nil {
		t.Fatal(err)
	}
	trigger := resource.Resource{
		Address: triggerAddr,
		Attributes: resource.Attributes{
			matomo.AttrContainer: resource.Ref{Address: container.Address},
			"type":               "customEvent",
			"event":              "trialStarted",
			"name":               "Trial Started",
		},
	}
	if err := p.ValidateResourceSet(context.Background(), []resource.Resource{container, trigger}); err != nil {
		t.Fatalf("ValidateResourceSet: %v", err)
	}
}

func mustExternalState(t *testing.T) *state.Store {
	t.Helper()
	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}
	return st
}
