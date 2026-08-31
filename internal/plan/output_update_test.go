package plan_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestBuildOutputDependentUpdatesWhenPrerequisiteUpdates(t *testing.T) {
	t.Parallel()

	widgets := fake.New()
	notes := fake.NewAlt()

	liveParent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, widgets, liveParent, resource.Attributes{
		fake.AttrSerial:  4,
		fake.OutputToken: "tok-homepage",
	})
	desiredParent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage updated"})

	child := noteResource(t, "banner", resource.Ref{Address: desiredParent.Address, Output: fake.OutputToken})
	if err := notes.Seed(resource.RemoteResource{
		Address:    child.Address,
		Identity:   resource.Identity{ID: "id-banner"},
		Attributes: resource.Attributes{fake.AttrText: "tok-homepage"},
		Computed:   resource.Attributes{},
	}); err != nil {
		t.Fatal(err)
	}

	st := mustOutputStore(t)
	if err := st.Bind(desiredParent.Address, resource.Identity{ID: "id-homepage"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(child.Address, resource.Identity{ID: "id-banner"}); err != nil {
		t.Fatal(err)
	}

	got, err := plan.BuildWithState(context.Background(), []resource.Resource{child, desiredParent}, lookupOutputs(widgets, notes), st)
	if err != nil {
		t.Fatalf("BuildWithState: %v", err)
	}

	actions := make(map[string]plan.Action, len(got.Changes))
	for _, change := range got.Changes {
		actions[change.Address.String()] = change.Action
	}
	if actions[desiredParent.Address.String()] != plan.ActionUpdate {
		t.Fatalf("parent action = %s, want update", actions[desiredParent.Address.String()])
	}
	if actions[child.Address.String()] != plan.ActionUpdate {
		t.Fatalf("dependent action = %s, want update so its output reference is re-resolved after prerequisite convergence", actions[child.Address.String()])
	}

	out := plan.Format(got)
	if !strings.Contains(out, `{$ref: "fake.widget.homepage", output: "token"}`) {
		t.Fatalf("plan did not render the logical output selector:\n%s", out)
	}
}
