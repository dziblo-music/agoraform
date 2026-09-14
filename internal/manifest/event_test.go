package manifest_test

import (
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/manifest"
)

const baseManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.trigger.trial_started
    attributes:
      type: customEvent
      event: trialStarted
      name: Trial started
  - address: googleads.conversion_action.trial_started
    attributes:
      name: Trial Started
      category: SIGNUP
      value: 0
      count: ONE
      primaryForGoal: true
  - address: meta.pixel.main
    attributes:
      name: Website
`

func TestParseApplicationEvents_Valid(t *testing.T) {
	t.Parallel()
	yaml := baseManifest + `
applicationEvents:
  trial_started:
    matomo:
      trigger:
        $ref: matomo.trigger.trial_started
      fields:
        - userId
    googleAds:
      conversion:
        $ref: googleads.conversion_action.trial_started
    meta:
      eventSource:
        $ref: meta.pixel.main
      eventName: StartTrial
      delivery: both
`
	m, err := manifest.Parse([]byte(yaml), "test")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if err := manifest.CheckApplicationEvents(m); err != nil {
		t.Fatalf("unexpected event validation error: %v", err)
	}
	evt := m.ApplicationEvents["trial_started"]
	if evt.Name != "trial_started" {
		t.Errorf("Name = %q, want trial_started", evt.Name)
	}
	if evt.Matomo == nil || evt.Matomo.Trigger.Address.String() != "matomo.trigger.trial_started" {
		t.Fatalf("unexpected Matomo binding: %#v", evt.Matomo)
	}
	if evt.Matomo.Event != "trialStarted" {
		t.Errorf("derived Matomo.Event = %q, want trialStarted", evt.Matomo.Event)
	}
	if len(evt.Matomo.Fields) != 1 || evt.Matomo.Fields[0] != "userId" {
		t.Errorf("Matomo.Fields = %v, want [userId]", evt.Matomo.Fields)
	}
	if evt.GoogleAds == nil || evt.GoogleAds.Conversion.Address.String() != "googleads.conversion_action.trial_started" {
		t.Fatalf("unexpected Google Ads binding: %#v", evt.GoogleAds)
	}
	if evt.Meta == nil || evt.Meta.EventSource.Address.String() != "meta.pixel.main" {
		t.Fatalf("unexpected Meta binding: %#v", evt.Meta)
	}
	if evt.Meta.EventName != "StartTrial" || evt.Meta.Delivery != manifest.DeliveryBoth {
		t.Errorf("unexpected Meta contract: %#v", evt.Meta)
	}
}

func TestParseApplicationEvents_MatomoOnly(t *testing.T) {
	t.Parallel()
	yaml := baseManifest + `
applicationEvents:
  page_view:
    matomo:
      trigger:
        $ref: matomo.trigger.trial_started
`
	m, err := manifest.Parse([]byte(yaml), "test")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	evt := m.ApplicationEvents["page_view"]
	if evt.Matomo == nil || evt.GoogleAds != nil || evt.Meta != nil {
		t.Fatalf("unexpected bindings: %#v", evt)
	}
}

func TestParseApplicationEvents_DefaultDelivery(t *testing.T) {
	t.Parallel()
	yaml := baseManifest + `
applicationEvents:
  checkout:
    meta:
      eventSource:
        $ref: meta.pixel.main
      eventName: InitiateCheckout
`
	m, err := manifest.Parse([]byte(yaml), "test")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if got := m.ApplicationEvents["checkout"].Meta.Delivery; got != manifest.DeliveryBrowser {
		t.Errorf("default Delivery = %q, want browser", got)
	}
}

func TestParseApplicationEvents_Empty(t *testing.T) {
	t.Parallel()
	m, err := manifest.Parse([]byte(baseManifest), "test")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(m.ApplicationEvents) != 0 {
		t.Errorf("applicationEvents = %d, want 0", len(m.ApplicationEvents))
	}
}

func TestParseApplicationEvents_MultipleEvents(t *testing.T) {
	t.Parallel()
	yaml := baseManifest + `
applicationEvents:
  trial_started:
    matomo:
      trigger:
        $ref: matomo.trigger.trial_started
  purchase:
    meta:
      eventSource:
        $ref: meta.pixel.main
      eventName: Purchase
      delivery: server
`
	m, err := manifest.Parse([]byte(yaml), "test")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(m.ApplicationEvents) != 2 {
		t.Fatalf("applicationEvents count = %d, want 2", len(m.ApplicationEvents))
	}
}

func TestParseApplicationEvents_Errors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		yaml    string
		wantSub string
	}{
		{"no provider bindings", baseManifest + "\napplicationEvents:\n  trial_started: {}\n", "at least one provider binding"},
		{"matomo missing trigger", baseManifest + "\napplicationEvents:\n  trial_started:\n    matomo:\n      fields: [userId]\n", "trigger is required"},
		{"matomo trigger not ref", baseManifest + "\napplicationEvents:\n  trial_started:\n    matomo:\n      trigger: nope\n", "trigger must be a resource reference"},
		{"matomo copied event rejected", baseManifest + "\napplicationEvents:\n  trial_started:\n    matomo:\n      event: trialStarted\n", "unknown field"},
		{"googleAds missing conversion", baseManifest + "\napplicationEvents:\n  trial_started:\n    googleAds: {}\n", "conversion is required"},
		{"googleAds conversion not ref", baseManifest + "\napplicationEvents:\n  trial_started:\n    googleAds:\n      conversion: nope\n", "conversion must be a resource reference"},
		{"meta missing eventSource", baseManifest + "\napplicationEvents:\n  trial_started:\n    meta:\n      eventName: StartTrial\n", "eventSource is required"},
		{"meta missing eventName", baseManifest + "\napplicationEvents:\n  trial_started:\n    meta:\n      eventSource:\n        $ref: meta.pixel.main\n", "eventName is required"},
		{"meta invalid delivery", baseManifest + "\napplicationEvents:\n  trial_started:\n    meta:\n      eventSource:\n        $ref: meta.pixel.main\n      eventName: StartTrial\n      delivery: api\n", "delivery must be browser, server, or both"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := manifest.Parse([]byte(tc.yaml), "test")
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantSub)
			}
		})
	}
}

func TestApplicationEventNames_Sorted(t *testing.T) {
	t.Parallel()
	events := map[string]manifest.ApplicationEvent{
		"z_event": {Name: "z_event"},
		"a_event": {Name: "a_event"},
		"m_event": {Name: "m_event"},
	}
	got := manifest.ApplicationEventNames(events)
	want := []string{"a_event", "m_event", "z_event"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
