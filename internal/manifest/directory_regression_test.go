package manifest_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/graph"
	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestDirectoryMultiProviderParityWithSingleFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "media"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "media", "hero.png"), []byte("sample asset"), 0o600); err != nil {
		t.Fatal(err)
	}
	const providers = `apiVersion: agoraform.io/v1alpha1
providers:
  matomo: {}
  googleads: {}
  meta: {}
assets:
  root: media
`
	const matomoFile = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.trigger.trial_started
    attributes:
      type: customEvent
      event: trialStarted
`
	const googleFile = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: googleads.conversion_action.trial_started
    attributes:
      name: Trial Started
      category: SIGNUP
`
	const metaFile = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: meta.pixel.main
    attributes:
      name: Main pixel
  - address: meta.image.hero
    attributes:
      source:
        file: hero.png
`
	const eventsFile = `apiVersion: agoraform.io/v1alpha1
applicationEvents:
  trial_started:
    matomo:
      trigger:
        $ref: matomo.trigger.trial_started
    googleAds:
      conversion:
        $ref: googleads.conversion_action.trial_started
    meta:
      eventSource:
        $ref: meta.pixel.main
      eventName: StartTrial
      delivery: both
`
	parts := map[string]string{
		"providers.agoraform.yaml": providers,
		"matomo.agoraform.yaml": matomoFile,
		"googleads.agoraform.yaml": googleFile,
		"meta.agoraform.yaml": metaFile,
		"events.agoraform.yaml": eventsFile,
	}
	for name, contents := range parts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Collect resource entries under one resources key, not repeated YAML keys.
	full := providers + "resources:\n" +
		strings.TrimPrefix(strings.TrimPrefix(matomoFile, "apiVersion: agoraform.io/v1alpha1\n"), "resources:\n") +
		strings.TrimPrefix(strings.TrimPrefix(googleFile, "apiVersion: agoraform.io/v1alpha1\n"), "resources:\n") +
		strings.TrimPrefix(strings.TrimPrefix(metaFile, "apiVersion: agoraform.io/v1alpha1\n"), "resources:\n") +
		strings.TrimPrefix(eventsFile, "apiVersion: agoraform.io/v1alpha1\n")
	file := filepath.Join(dir, "agoraform.yaml")
	if err := os.WriteFile(file, []byte(full), 0o600); err != nil {
		t.Fatal(err)
	}
	single, err := manifest.Load(file)
	if err != nil {
		t.Fatalf("single-file load: %v", err)
	}
	merged, err := manifest.Load(dir)
	if err != nil {
		t.Fatalf("directory load: %v", err)
	}
	if len(merged.Files) != len(parts) {
		t.Fatalf("merged %d files, want %d", len(merged.Files), len(parts))
	}
	if !reflect.DeepEqual(single.Providers, merged.Providers) || single.Assets != merged.Assets {
		t.Fatalf("provider or asset root mismatch")
	}
	if err := manifest.CheckApplicationEvents(single); err != nil {
		t.Fatalf("single-file contract: %v", err)
	}
	if err := manifest.CheckApplicationEvents(merged); err != nil {
		t.Fatalf("directory contract: %v", err)
	}
	if !reflect.DeepEqual(single.ApplicationEvents, merged.ApplicationEvents) {
		t.Fatalf("application event contract differs across layouts")
	}
	byAddress := make(map[string]resource.Resource, len(single.Resources))
	for _, res := range single.Resources {
		byAddress[res.Address.String()] = res
	}
	if len(byAddress) != len(merged.Resources) {
		t.Fatalf("resource count mismatch: %d versus %d", len(byAddress), len(merged.Resources))
	}
	for _, res := range merged.Resources {
		old, ok := byAddress[res.Address.String()]
		if !ok || !reflect.DeepEqual(old.Attributes, res.Attributes) {
			t.Fatalf("resource %s changed after split", res.Address)
		}
		if (old.LocalAsset == nil) != (res.LocalAsset == nil) {
			t.Fatalf("local asset presence changed for %s", res.Address)
		}
		if old.LocalAsset != nil && (old.LocalAsset.Path != res.LocalAsset.Path || old.LocalAsset.Digest != res.LocalAsset.Digest) {
			t.Fatalf("local asset fingerprint changed for %s", res.Address)
		}
	}
	originalGraph, err := graph.Build(single.Resources)
	if err != nil {
		t.Fatal(err)
	}
	mergedGraph, err := graph.Build(merged.Resources)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(originalGraph.Order(), mergedGraph.Order()) {
		t.Fatalf("execution order changed when splitting files")
	}
}
