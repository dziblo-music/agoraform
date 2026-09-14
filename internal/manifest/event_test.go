package manifest_test

import (
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/manifest"
)

const baseManifest = `apiVersion: agoraform.io/v1alpha1
resources:
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
      event: trialStarted
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
	if len(m.ApplicationEvents) != 1 {
		t.Fatalf("applicationEvents count = %d, want 1", len(m.ApplicationEvents))
	}
	evt, ok := m.ApplicationEvents["trial_started"]
	if !ok {
		t.Fatal("expected event trial_started not found")
	}

	if evt.Name != "trial_started" {
		t.Errorf("Name = %q, want trial_started", evt.Name)
	}

	// Matomo
	if evt.Matomo == nil {
		t.Fatal("Matomo binding is nil")
	}
	if evt.Matomo.Event != "trialStarted" {
		t.Errorf("Matomo.Event = %q, want trialStarted", evt.Matomo.Event)
	}
	if len(evt.Matomo.Fields) != 1 || evt.Matomo.Fields[0] != "userId" {
		t.Errorf("Matomo.Fields = %v, want [userId]", evt.Matomo.Fields)
	}

	// Google Ads
	if evt.GoogleAds == nil {
		t.Fatal("GoogleAds binding is nil")
	}
	if evt.GoogleAds.Conversion.Address.String() != "googleads.conversion_action.trial_started" {
		t.Errorf("GoogleAds.Conversion = %s", evt.GoogleAds.Conversion.Address)
	}

	// Meta
	if evt.Meta == nil {
		t.Fatal("Meta binding is nil")
	}
	if evt.Meta.EventSource.Address.String() != "meta.pixel.main" {
		t.Errorf("Meta.EventSource = %s", evt.Meta.EventSource.Address)
	}
	if evt.Meta.EventName != "StartTrial" {
		t.Errorf("Meta.EventName = %q, want StartTrial", evt.Meta.EventName)
	}
	if evt.Meta.Delivery != manifest.DeliveryBoth {
		t.Errorf("Meta.Delivery = %q, want both", evt.Meta.Delivery)
	}
}

func TestParseApplicationEvents_MatomoOnly(t *testing.T) {
	t.Parallel()

	yaml := baseManifest + `
applicationEvents:
  page_view:
    matomo:
      event: pageView
`
	m, err := manifest.Parse([]byte(yaml), "test")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	evt := m.ApplicationEvents["page_view"]
	if evt.Matomo == nil {
		t.Fatal("Matomo binding is nil")
	}
	if evt.GoogleAds != nil {
		t.Error("GoogleAds should be nil for matomo-only event")
	}
	if evt.Meta != nil {
		t.Error("Meta should be nil for matomo-only event")
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
	evt := m.ApplicationEvents["checkout"]
	if evt.Meta == nil {
		t.Fatal("Meta binding is nil")
	}
	if evt.Meta.Delivery != manifest.DeliveryBrowser {
		t.Errorf("default Delivery = %q, want browser", evt.Meta.Delivery)
	}
}

func TestParseApplicationEvents_Empty(t *testing.T) {
	t.Parallel()

	yaml := baseManifest
	m, err := manifest.Parse([]byte(yaml), "test")
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
      event: trialStarted
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
		{
			name: "no provider bindings",
			yaml: baseManifest + `
applicationEvents:
  trial_started: {}
`,
			wantSub: "at least one provider binding",
		},
		{
			name: "matomo missing event",
			yaml: baseManifest + `
applicationEvents:
  trial_started:
    matomo:
      fields:
        - userId
`,
			wantSub: "event is required",
		},
		{
			name: "matomo empty event",
			yaml: baseManifest + `
applicationEvents:
  trial_started:
    matomo:
      event: ""
`,
			wantSub: "event must not be empty",
		},
		{
			name: "googleAds missing conversion",
			yaml: baseManifest + `
applicationEvents:
  trial_started:
    googleAds: {}
`,
			wantSub: "conversion is required",
		},
		{
			name: "googleAds conversion not a ref",
			yaml: baseManifest + `
applicationEvents:
  trial_started:
    googleAds:
      conversion: "not-a-ref"
`,
			wantSub: "conversion must be a resource reference",
		},
		{
			name: "meta missing eventSource",
			yaml: baseManifest + `
applicationEvents:
  trial_started:
    meta:
      eventName: StartTrial
`,
			wantSub: "eventSource is required",
		},
		{
			name: "meta missing eventName",
			yaml: baseManifest + `
applicationEvents:
  trial_started:
    meta:
      eventSource:
        $ref: meta.pixel.main
`,
			wantSub: "eventName is required",
		},
		{
			name: "meta invalid delivery",
			yaml: baseManifest + `
applicationEvents:
  trial_started:
    meta:
      eventSource:
        $ref: meta.pixel.main
      eventName: StartTrial
      delivery: api
`,
			wantSub: "delivery must be browser, server, or both",
		},
		{
			name: "unknown field",
			yaml: baseManifest + `
applicationEvents:
  trial_started:
    matomo:
      event: trialStarted
    unknown: {}
`,
			wantSub: "unknown field",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := manifest.Parse([]byte(tc.yaml), "test")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.wantSub)
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
	names := manifest.ApplicationEventNames(events)
	want := []string{"a_event", "m_event", "z_event"}
	if len(names) != len(want) {
		t.Fatalf("len = %d, want %d", len(names), len(want))
	}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, n, want[i])
		}
	}
}
