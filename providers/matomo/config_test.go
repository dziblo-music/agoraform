package matomo_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestConfigFromEnv(t *testing.T) {
	t.Setenv(matomo.EnvURL, " https://matomo.example.com ")
	t.Setenv(matomo.EnvTokenAuth, " env-token ")
	t.Setenv(matomo.EnvSiteID, " 4 ")
	t.Setenv(matomo.EnvContainerID, " containerA ")
	t.Setenv(matomo.EnvEnvironment, " staging ")

	cfg := matomo.ConfigFromEnv()
	if cfg.BaseURL != "https://matomo.example.com" {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.TokenAuth != "env-token" {
		t.Fatalf("TokenAuth = %q", cfg.TokenAuth)
	}
	if cfg.SiteID != "4" {
		t.Fatalf("SiteID = %q", cfg.SiteID)
	}
	if cfg.ContainerID != "containerA" {
		t.Fatalf("ContainerID = %q", cfg.ContainerID)
	}
	if cfg.Environment != "staging" {
		t.Fatalf("Environment = %q", cfg.Environment)
	}
	if cfg.Timeout != client.DefaultTimeout {
		t.Fatalf("Timeout = %s, want default", cfg.Timeout)
	}
	if !matomo.Configured(cfg) {
		t.Fatal("expected Configured")
	}
}

func TestExplicitConfigOverridesEnv(t *testing.T) {
	t.Setenv(matomo.EnvURL, "https://from-env.example")
	t.Setenv(matomo.EnvTokenAuth, "env-token")

	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = io.WriteString(w, `"5.2.0"`)
	}))
	t.Cleanup(srv.Close)

	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:   srv.URL,
		TokenAuth: "explicit-token",
	}, srv.Client())
	if err := p.CheckConnection(context.Background()); err != nil {
		t.Fatalf("CheckConnection with explicit config: %v", err)
	}
	if hits != 1 {
		t.Fatalf("hits = %d, want 1 against the explicit URL", hits)
	}

	fromEnv := matomo.NewFromEnv()
	c, err := fromEnv.Client()
	if err != nil {
		t.Fatalf("NewFromEnv Client: %v", err)
	}
	if c == nil {
		t.Fatal("expected env-configured client")
	}
}

func TestConfiguredFalseWhenIncomplete(t *testing.T) {
	if matomo.Configured(matomo.Config{}) {
		t.Fatal("empty config should not be configured")
	}
	if matomo.Configured(matomo.Config{BaseURL: "https://matomo.example.com"}) {
		t.Fatal("token-less config should not be configured")
	}
}

func TestConfigFromEnvEmpty(t *testing.T) {
	t.Setenv(matomo.EnvURL, "")
	t.Setenv(matomo.EnvTokenAuth, "")
	t.Setenv(matomo.EnvSiteID, "")
	t.Setenv(matomo.EnvContainerID, "")
	t.Setenv(matomo.EnvEnvironment, "")

	cfg := matomo.ConfigFromEnv()
	if cfg.BaseURL != "" || cfg.TokenAuth != "" {
		t.Fatalf("cfg = %+v, want empty secrets", cfg.Redacted())
	}
	if strings.Contains(cfg.String(), "env-token") {
		t.Fatalf("String leaked a token: %s", cfg.String())
	}
}
