package manifest_test

import (
	"testing"

	"github.com/dziblo-music/agoraform/internal/manifest"
)

func TestParseProviderConfiguration(t *testing.T) {
	t.Parallel()

	m, err := manifest.Parse([]byte(`apiVersion: agoraform.io/v1alpha1
providers:
  matomo:
    publish: true
    environment: live
resources: []
`), "test.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cfg, ok := m.Providers["matomo"]
	if !ok {
		t.Fatal("matomo provider configuration missing")
	}
	if got, ok := cfg["publish"].(bool); !ok || !got {
		t.Fatalf("publish = %#v, want true", cfg["publish"])
	}
	if got, ok := cfg["environment"].(string); !ok || got != "live" {
		t.Fatalf("environment = %#v, want live", cfg["environment"])
	}
}

func TestParseRejectsInvalidProviderName(t *testing.T) {
	t.Parallel()

	_, err := manifest.Parse([]byte(`apiVersion: agoraform.io/v1alpha1
providers:
  Bad Name:
    publish: true
`), "test.yaml")
	if err == nil {
		t.Fatal("expected invalid provider name error")
	}
}
