package manifest_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
)

func TestValidateApplicationEvents_ValidRefs(t *testing.T) {
	t.Parallel()

	yaml := baseManifest + `
applicationEvents:
  trial_started:
    googleAds:
      conversion:
        $ref: googleads.conversion_action.trial_started
    meta:
      eventSource:
        $ref: meta.pixel.main
      eventName: StartTrial
`
	m, err := manifest.Parse([]byte(yaml), "test")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	reg := provider.NewRegistry()
	if err := reg.Register(&fake.Provider{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	// CheckProviders will invoke validateApplicationEvents; we skip provider
	// connection checks by using an empty registry for just the event validation.
	if err := manifest.CheckProviders(context.Background(), m, nil); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidateApplicationEvents_InvalidRef_Unknown(t *testing.T) {
	t.Parallel()

	yaml := `apiVersion: agoraform.io/v1alpha1
resources: []
applicationEvents:
  trial_started:
    googleAds:
      conversion:
        $ref: googleads.conversion_action.nonexistent
`
	m, err := manifest.Parse([]byte(yaml), "test")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	err = manifest.CheckProviders(context.Background(), m, nil)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown resource") {
		t.Errorf("error = %q, want substring 'unknown resource'", err.Error())
	}
}

func TestValidateApplicationEvents_InvalidRef_WrongType(t *testing.T) {
	t.Parallel()

	// Reference meta.pixel.* from googleAds.conversion → wrong resource type.
	yaml := baseManifest + `
applicationEvents:
  trial_started:
    googleAds:
      conversion:
        $ref: meta.pixel.main
`
	m, err := manifest.Parse([]byte(yaml), "test")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	err = manifest.CheckProviders(context.Background(), m, nil)
	if err == nil {
		t.Fatal("expected validation error for wrong resource type, got nil")
	}
	if !strings.Contains(err.Error(), "googleads.conversion_action") {
		t.Errorf("error = %q, want mention of googleads.conversion_action", err.Error())
	}
}

func TestValidateApplicationEvents_MetaWrongType(t *testing.T) {
	t.Parallel()

	// Reference googleads resource from meta.eventSource → wrong provider.
	yaml := baseManifest + `
applicationEvents:
  trial_started:
    meta:
      eventSource:
        $ref: googleads.conversion_action.trial_started
      eventName: StartTrial
`
	m, err := manifest.Parse([]byte(yaml), "test")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	err = manifest.CheckProviders(context.Background(), m, nil)
	if err == nil {
		t.Fatal("expected validation error for wrong provider, got nil")
	}
	if !strings.Contains(err.Error(), "meta.pixel") {
		t.Errorf("error = %q, want mention of meta.pixel", err.Error())
	}
}

func TestValidateApplicationEvents_NoEvents_NoError(t *testing.T) {
	t.Parallel()

	m, err := manifest.Parse([]byte(baseManifest), "test")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if err := manifest.CheckProviders(context.Background(), m, nil); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
