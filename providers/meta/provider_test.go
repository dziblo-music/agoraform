package meta_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/meta"
)

const (
	providerToken   = "provider-secret-token"
	providerAccount = "123456789012345"
)

func TestProviderRegisterAndLookup(t *testing.T) {
	t.Parallel()
	reg := provider.NewRegistry()
	p := meta.New(meta.Config{AccessToken: providerToken, AdAccountID: providerAccount})
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	got, ok := reg.Lookup(meta.Name)
	if !ok || got.Name() != meta.Name {
		t.Fatalf("Lookup(meta) = %v, %v", got, ok)
	}
	if len(got.ResourceTypes()) != 0 {
		t.Fatalf("foundation resource types = %v, want none", got.ResourceTypes())
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv(meta.EnvAccessToken, providerToken)
	t.Setenv(meta.EnvAdAccountID, "act_"+providerAccount)
	cfg := meta.ConfigFromEnv()
	if cfg.AccessToken != providerToken || cfg.AdAccountID != "act_"+providerAccount {
		t.Fatalf("ConfigFromEnv = %#v", cfg.Redacted())
	}
}

func TestProviderConfigureRejectsRuntimeValues(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: providerToken, AdAccountID: providerAccount})
	for _, attrs := range []resource.Attributes{
		{"accessToken": providerToken},
		{"adAccountId": providerAccount},
	} {
		err := p.Configure(attrs)
		if err == nil || !strings.Contains(err.Error(), "environment") {
			t.Fatalf("Configure(%v) = %v", attrs, err)
		}
		if strings.Contains(err.Error(), providerToken) {
			t.Fatalf("token leaked in %q", err)
		}
	}
	if err := p.Configure(resource.Attributes{}); err != nil {
		t.Fatal(err)
	}
}

func TestProviderCheckConnectionMissingConfiguration(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		cfg  meta.Config
		want string
	}{
		{cfg: meta.Config{}, want: meta.EnvAccessToken},
		{cfg: meta.Config{AccessToken: providerToken}, want: meta.EnvAdAccountID},
	} {
		err := meta.New(tc.cfg).CheckConnection(context.Background())
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("error = %v, want %s", err, tc.want)
		}
	}
}

func TestProviderCheckConnection(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/me/permissions"):
			_, _ = io.WriteString(w, `{"data":[{"permission":"ads_management","status":"granted"}]}`)
		case strings.HasSuffix(r.URL.Path, "/act_"+providerAccount):
			_, _ = io.WriteString(w, `{"id":"`+providerAccount+`"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	p := meta.NewWithHTTPClient(meta.Config{AccessToken: providerToken, AdAccountID: providerAccount, BaseURL: server.URL, Timeout: time.Second}, server.Client())
	if err := p.CheckConnection(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestProviderRejectsResourceTypesUntilImplemented(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: providerToken, AdAccountID: providerAccount})
	addr, err := resource.ParseAddress("meta.campaign.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(context.Background(), resource.Resource{Address: addr}); err == nil || !strings.Contains(err.Error(), "unknown type") {
		t.Fatalf("Validate = %v", err)
	}
}
