package apply_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestExecuteCreateSuccessWithoutIdentityIsPartial(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	p := &scriptedProvider{Provider: inner, stripCreateIdentity: true}
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	st := mustStore(t)
	planned := &plan.Plan{
		Changes: []plan.Change{{Address: res.Address, Action: plan.ActionCreate, After: res.Attributes}},
	}

	_, err := apply.Execute(context.Background(), planned, []resource.Resource{res}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Execute succeeded, want provider result failure")
	}
	partial := requirePartial(t, err)
	if partial.Stage != apply.StageMutation || partial.Operation != "create" {
		t.Fatalf("partial = %+v, want create mutation failure", partial)
	}
	if !partial.RemoteIdentity.IsZero() {
		t.Fatalf("remote identity = %+v, want unavailable identity", partial.RemoteIdentity)
	}
	if !strings.Contains(err.Error(), "created remotely") || !strings.Contains(err.Error(), "provider returned no identity") {
		t.Fatalf("error = %q, want post-create provider result guidance", err)
	}
	if !strings.Contains(err.Error(), "Identify the created remote resource") {
		t.Fatalf("error = %q, want recovery guidance for unknown identity", err)
	}
	if _, ok, _ := st.Identity(res.Address); ok {
		t.Fatal("invalid provider result wrote an identity binding")
	}
	_, creates, _, _ := inner.Calls()
	if creates != 1 {
		t.Fatalf("creates = %d, want 1", creates)
	}
}

func TestExecuteUpdateSuccessWithDifferentIdentityIsPartial(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	live := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, inner, live, resource.Attributes{fake.AttrSerial: 1})
	p := &scriptedProvider{Provider: inner, updateIdentity: resource.Identity{ID: "other-id"}}
	st := mustStore(t)
	if err := st.Bind(live.Address, resource.Identity{ID: "id-homepage"}); err != nil {
		t.Fatal(err)
	}

	desired := widget(t, "homepage", resource.Attributes{
		fake.AttrTitle: "Homepage",
		fake.AttrColor: "blue",
	})

	_, err := apply.Execute(context.Background(), updatePlan(desired, live.Attributes, "id-homepage"), []resource.Resource{desired}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Execute succeeded, want identity rebind refusal")
	}
	partial := requirePartial(t, err)
	if partial.Stage != apply.StageMutation || partial.Operation != "update" || partial.RemoteIdentity.ID != "other-id" {
		t.Fatalf("partial = %+v, want update mutation failure with returned identity", partial)
	}
	if !strings.Contains(err.Error(), "updated remotely") || !strings.Contains(err.Error(), "refusing to rebind") {
		t.Fatalf("error = %q, want post-update identity mismatch", err)
	}

	id, ok, identErr := st.Identity(desired.Address)
	if identErr != nil || !ok || id.ID != "id-homepage" {
		t.Fatalf("Identity = (%v,%v,%v), want original binding", id, ok, identErr)
	}
	if p.updates != 1 {
		t.Fatalf("updates = %d, want 1", p.updates)
	}
}
