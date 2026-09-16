package manifest_test

import (
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/manifest"
)

func TestValidateApplicationEvents_MatomoTriggerRequiredType(t *testing.T) {
	t.Parallel()
	yaml := `apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.tag.trial_started
    attributes:
      type: matomo
applicationEvents:
  trial_started:
    matomo:
      trigger:
        $ref: matomo.tag.trial_started
`
	m, err := manifest.Parse([]byte(yaml), "test")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	err = manifest.CheckApplicationEvents(m)
	if err == nil || !strings.Contains(err.Error(), "matomo.trigger") {
		t.Fatalf("error = %v, want matomo.trigger type error", err)
	}
}

func TestValidateApplicationEvents_MatomoRejectsPageViewTrigger(t *testing.T) {
	t.Parallel()
	yaml := `apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.trigger.pageview
    attributes:
      type: pageView
applicationEvents:
  pageview:
    matomo:
      trigger:
        $ref: matomo.trigger.pageview
`
	m, err := manifest.Parse([]byte(yaml), "test")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	err = manifest.CheckApplicationEvents(m)
	if err == nil || !strings.Contains(err.Error(), "customEvent") {
		t.Fatalf("error = %v, want customEvent requirement for applicationEvents", err)
	}
}

func TestValidateApplicationEvents_MatomoDerivesManagedEvent(t *testing.T) {
	t.Parallel()
	yaml := `apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.trigger.trial_started
    attributes:
      type: customEvent
      event: trialStartedV2
applicationEvents:
  trial_started:
    matomo:
      trigger:
        $ref: matomo.trigger.trial_started
`
	m, err := manifest.Parse([]byte(yaml), "test")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := manifest.CheckApplicationEvents(m); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if got := m.ApplicationEvents["trial_started"].Matomo.Event; got != "trialStartedV2" {
		t.Fatalf("derived event = %q, want trialStartedV2", got)
	}
}
