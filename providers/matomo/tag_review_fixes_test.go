package matomo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo"
)

func TestValidateTagEventValueMustBeNumericOrVariableRef(t *testing.T) {
	t.Parallel()

	p := testTagProvider(t, newTagServer(t))
	cases := []struct {
		name    string
		value   any
		wantErr bool
	}{
		{name: "numeric string", value: "12.5"},
		{name: "integer", value: 12},
		{name: "float", value: 12.5},
		{name: "negative", value: -3},
		{name: "scientific notation", value: "1.5e3"},
		{name: "variable ref", value: variableRef(t, "user_id")},
		{name: "non numeric string", value: "not-a-number", wantErr: true},
		{name: "nan", value: "NaN", wantErr: true},
		{name: "infinity", value: "Inf", wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			attrs := validTagAttrs(t)
			attrs[matomo.AttrEventValue] = tc.value
			err := p.Validate(context.Background(), tagResource(t, "trial_started", attrs))
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), "numeric") {
				t.Fatalf("error = %q, want numeric validation error", err)
			}
		})
	}
}

func TestCreateTagRejectsNonNumericEventValueBeforeMutation(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	p := testTagProvider(t, srv)
	attrs := validTagAttrs(t)
	attrs[matomo.AttrTrigger] = resolvedTrigger(t, "trial_started", "4")
	attrs[matomo.AttrEventValue] = "not-a-number"

	_, err := p.Create(context.Background(), tagResource(t, "trial_started", attrs))
	if err == nil {
		t.Fatal("expected validation error")
	}
	if srv.createCount() != 0 {
		t.Fatalf("creates = %d, want 0", srv.createCount())
	}
}

func TestPlanTagVariableReferenceRequiresWrappedRemoteReference(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		remoteName string
		wantAction plan.Action
	}{
		{name: "missing remote value", remoteName: "", wantAction: plan.ActionUpdate},
		{name: "literal variable name", remoteName: "userId", wantAction: plan.ActionUpdate},
		{name: "wrapped variable reference", remoteName: "{{userId}}", wantAction: plan.ActionUnchanged},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := newTagServer(t)
			srv.seedVariable(apiTagVariable{ID: 2, Name: "userId", Type: "DataLayer", Key: "userId"})
			srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
			srv.seedTag(apiTag{
				ID:            7,
				Name:          "trialStarted",
				Type:          "Matomo",
				FireTriggerID: 4,
				Category:      "signup",
				Action:        "trialStarted",
				EventName:     tc.remoteName,
			})
			p := testTagProvider(t, srv)

			variable := resource.Resource{
				Address: mustVariableAddress(t, "user_id"),
				Attributes: resource.Attributes{
					matomo.AttrType: "dataLayer",
					matomo.AttrKey:  "userId",
				},
			}
			tagAttrs := validTagAttrs(t)
			tagAttrs[matomo.AttrEventName] = variableRef(t, "user_id")
			tag := tagResource(t, "trial_started", tagAttrs)

			got := mustPlanTag(t, p, trialStartedTrigger(t), variable, tag)
			change := changeByAddr(t, got, "matomo.tag.trial_started")
			if change.Action != tc.wantAction {
				t.Fatalf("action = %s, want %s; diffs=%+v", change.Action, tc.wantAction, change.Diffs)
			}
		})
	}
}
