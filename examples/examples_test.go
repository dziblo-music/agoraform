package examples_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/providers/matomo"
)

func TestManifestsRemainValid(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob(filepath.Join("*", "agoraform.yaml"))
	if err != nil {
		t.Fatalf("find example manifests: %v", err)
	}
	paths = append(paths, "agoraform.yaml")

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

			p := matomo.New(matomo.Config{
				BaseURL:     "https://matomo.example.com",
				TokenAuth:   "test-token",
				SiteID:      "1",
				ContainerID: "example-container",
			})
			if cfg, ok := m.Providers["matomo"]; ok {
				if err := p.Configure(cfg); err != nil {
					t.Fatalf("validate provider configuration: %v", err)
				}
			}
			for _, res := range m.Resources {
				if res.Address.Provider != "matomo" {
					t.Fatalf("unsupported example provider %q", res.Address.Provider)
				}
				if err := p.Validate(context.Background(), res); err != nil {
					t.Fatalf("validate %s: %v", res.Address, err)
				}
			}
		})
	}
}
