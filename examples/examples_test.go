package examples_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/providers/googleads"
	"github.com/dziblo-music/agoraform/providers/matomo"
)

func TestManifestsRemainValid(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob(filepath.Join("*", "agoraform.yaml"))
	if err != nil {
		t.Fatalf("find example manifests: %v", err)
	}
	paths = append(paths, "agoraform.yaml")

	foundGoogleAds := false
	for _, path := range paths {
		if filepath.ToSlash(path) == "googleads-conversion/agoraform.yaml" {
			foundGoogleAds = true
			break
		}
	}
	if !foundGoogleAds {
		t.Fatal("missing googleads-conversion/agoraform.yaml")
	}

	for _, path := range paths {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read example: %v", err)
			}
			m, err := manifest.Parse(data, path)
			if err != nil {
				t.Fatalf("parse example: %v", err)
			}

			providers := exampleProviders()
			for name, cfg := range m.Providers {
				p, ok := providers[name]
				if !ok {
					t.Fatalf("unsupported example provider %q", name)
				}
				if configurator, ok := p.(provider.Configurator); ok {
					if err := configurator.Configure(cfg); err != nil {
						t.Fatalf("validate provider configuration: %v", err)
					}
				}
			}

			seen := make(map[string]provider.Provider)
			for _, res := range m.Resources {
				p, ok := providers[res.Address.Provider]
				if !ok {
					t.Fatalf("unsupported example provider %q", res.Address.Provider)
				}
				if err := p.Validate(context.Background(), res); err != nil {
					t.Fatalf("validate %s: %v", res.Address, err)
				}
				seen[p.Name()] = p
			}
			for _, p := range seen {
				if validator, ok := p.(provider.ResourceSetValidator); ok {
					if err := validator.ValidateResourceSet(context.Background(), m.Resources); err != nil {
						t.Fatalf("validate resource set for %s: %v", p.Name(), err)
					}
				}
			}
		})
	}
}

func exampleProviders() map[string]provider.Provider {
	return map[string]provider.Provider{
		matomo.Name: matomo.New(matomo.Config{
			BaseURL:     "https://matomo.example.com",
			TokenAuth:   "test-token",
			SiteID:      "1",
			ContainerID: "example-container",
		}),
		googleads.Name: googleads.New(googleads.Config{
			CustomerID: "1234567890",
		}),
	}
}
