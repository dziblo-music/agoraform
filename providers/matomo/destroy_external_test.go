package matomo_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/destroy"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
)

// externalContainerGraph is managedGraph with a reference-only container.
func externalContainerGraph(t *testing.T) []resource.Resource {
	t.Helper()
	desired := managedGraph(t)
	desired[0] = resource.Resource{
		Address:    desired[0].Address,
		Ownership:  resource.OwnershipExternal,
		ExternalID: testManagedContainerID,
	}
	return desired
}

func TestDestroyRunPublishesExternalContainer(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		stateBound bool
	}{
		{name: "bound in state", stateBound: true},
		{name: "manifest lifecycle id only", stateBound: false},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := newLifecycleServer(t)
			s.seedManagedGraph()
			p := newLifecycleProvider(t, s, "")
			if err := p.Configure(resource.Attributes{"publish": true, "environment": "live"}); err != nil {
				t.Fatal(err)
			}
			desired := externalContainerGraph(t)
			if err := p.ValidateResourceSet(context.Background(), desired); err != nil {
				t.Fatalf("ValidateResourceSet: %v", err)
			}
			st := mustDestroyStore(t)
			bindGraph(t, st, map[string]string{
				"matomo.variable.user_id":      "2",
				"matomo.trigger.trial_started": "5",
				"matomo.tag.trial_started":     "7",
			})
			if tc.stateBound {
				bindExternal(t, st, desired[0].Address, testManagedContainerID)
			}

			var out bytes.Buffer
			result, err := destroy.Run(context.Background(), desired, lookupMatomo(p), st, &out, nil)
			if err != nil {
				t.Fatalf("Run: %v\n%s", err, out.String())
			}
			if result.Destroyed != 3 || result.ExternalReleased != 1 || result.Finalized != 1 {
				t.Fatalf("result = %+v, want 3 destroyed, container released, one publication", result)
			}
			if s.createCount() != 1 || s.publishCount() != 1 {
				t.Fatalf("create=%d publish=%d, want 1/1", s.createCount(), s.publishCount())
			}
			if containsMethod(s.calledMethods(), "TagManager.deleteContainer") {
				t.Fatal("external container was deleted")
			}
			if !strings.Contains(out.String(), "matomo.container.main: publish -> live") {
				t.Fatalf("plan missing publication of the external container:\n%s", out.String())
			}
		})
	}
}

func TestDestroyRunVariableInUseDoesNotStopOtherDeletes(t *testing.T) {
	t.Parallel()

	s := newLifecycleServer(t)
	s.seedManagedGraph()
	s.fail("TagManager.deleteContainerVariable", `{"result":"error","message":"This variable is still used by tag Matomo Analytics"}`)
	p := newLifecycleProvider(t, s, "")
	if err := p.Configure(resource.Attributes{"publish": true, "environment": "live"}); err != nil {
		t.Fatal(err)
	}
	desired := externalContainerGraph(t)
	if err := p.ValidateResourceSet(context.Background(), desired); err != nil {
		t.Fatalf("ValidateResourceSet: %v", err)
	}
	st := mustDestroyStore(t)
	bindGraph(t, st, map[string]string{
		"matomo.variable.user_id":      "2",
		"matomo.trigger.trial_started": "5",
		"matomo.tag.trial_started":     "7",
	})
	bindExternal(t, st, desired[0].Address, testManagedContainerID)

	var out bytes.Buffer
	result, err := destroy.Run(context.Background(), desired, lookupMatomo(p), st, &out, nil)
	var incomplete *destroy.IncompleteError
	if !errors.As(err, &incomplete) {
		t.Fatalf("error = %v, want IncompleteError\n%s", err, out.String())
	}
	if len(incomplete.Failures) != 1 || incomplete.Failures[0].Address.String() != "matomo.variable.user_id" {
		t.Fatalf("failures = %+v, want only the in-use variable", incomplete.Failures)
	}
	if !strings.Contains(err.Error(), "still used by tag") {
		t.Fatalf("error does not name the block:\n%s", err)
	}
	if !containsMethod(s.calledMethods(), "TagManager.deleteContainerTag") || !containsMethod(s.calledMethods(), "TagManager.deleteContainerTrigger") {
		t.Fatalf("tag and trigger deletes did not run: %v", s.calledMethods())
	}
	if result.Destroyed != 2 || result.Failed != 1 || result.Skipped != 1 {
		t.Fatalf("result = %+v, want tag+trigger destroyed, variable failed, container skipped", result)
	}
	if s.publishCount() != 0 {
		t.Fatal("published a container whose teardown is incomplete")
	}
	for _, addr := range []string{"matomo.container.main", "matomo.variable.user_id", "matomo.tag.trial_started", "matomo.trigger.trial_started"} {
		if _, ok, _ := st.Identity(mustAddressString(t, addr)); !ok {
			t.Fatalf("%s binding was removed before publication", addr)
		}
	}

	s.mu.Lock()
	delete(s.fails, "TagManager.deleteContainerVariable")
	s.mu.Unlock()
	result, err = destroy.Run(context.Background(), desired, lookupMatomo(p), st, nil, nil)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if result.Destroyed != 1 || result.AlreadyAbsent != 2 || result.Finalized != 1 {
		t.Fatalf("retry result = %+v, want variable destroyed, children confirmed absent, one publication", result)
	}
	if s.publishCount() != 1 {
		t.Fatalf("publishCount = %d, want 1", s.publishCount())
	}
}

func bindExternal(t *testing.T, st *state.Store, addr resource.Address, id string) {
	t.Helper()
	if err := st.SetOwnership(addr, resource.Identity{ID: id}, resource.OwnershipExternal); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
}
