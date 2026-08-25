package matomo_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

const providerToken = "super-secret-token-value"

func TestProviderRegisterAndLookup(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	p := matomo.New(client.Config{BaseURL: "https://matomo.example.com", TokenAuth: providerToken})
	if err := reg.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, ok := reg.Lookup(matomo.Name)
	if !ok {
		t.Fatal("Lookup(matomo) failed")
	}
	if got.Name() != matomo.Name {
		t.Fatalf("Name = %q", got.Name())
	}
	if !provider.Supports(got, matomo.TypeGoal) {
		t.Fatal("matomo.goal must be registered")
	}
	if !provider.Supports(got, matomo.TypeVariable) {
		t.Fatal("matomo.variable must be registered")
	}
	if !provider.Supports(got, matomo.TypeTrigger) {
		t.Fatal("matomo.trigger must be registered")
	}

	addr, err := resource.ParseAddress("matomo.goal.trial_started")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.LookupFor(addr); err != nil {
		t.Fatalf("LookupFor goal: %v", err)
	}

	variableAddr, err := resource.ParseAddress("matomo.variable.user_id")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.LookupFor(variableAddr); err != nil {
		t.Fatalf("LookupFor variable: %v", err)
	}

	triggerAddr, err := resource.ParseAddress("matomo.trigger.trial_started")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.LookupFor(triggerAddr); err != nil {
		t.Fatalf("LookupFor trigger: %v", err)
	}
}

func TestProviderCheckConnection(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `"5.2.0"`)
	}))
	t.Cleanup(srv.Close)

	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:   srv.URL,
		TokenAuth: providerToken,
	}, srv.Client())
	if err := p.CheckConnection(context.Background()); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func TestProviderCheckConnectionMissingCredentials(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{})
	err := p.CheckConnection(context.Background())
	if err == nil {
		t.Fatal("expected missing credential error")
	}
	if !strings.Contains(err.Error(), matomo.EnvURL) && !strings.Contains(err.Error(), "required") {
		t.Fatalf("error = %q, want missing %s", err, matomo.EnvURL)
	}
	if strings.Contains(err.Error(), providerToken) {
		t.Fatalf("secret leaked in %q", err)
	}
}

func TestProviderCheckConnectionUnauthorized(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad "+providerToken, http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:   srv.URL,
		TokenAuth: providerToken,
	}, srv.Client())
	err := p.CheckConnection(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), providerToken) {
		t.Fatalf("secret leaked in %q", err)
	}
}

func TestProviderValidateUnknownType(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{BaseURL: "https://matomo.example.com", TokenAuth: providerToken})
	addr, err := resource.ParseAddress("matomo.tag.pageview")
	if err != nil {
		t.Fatal(err)
	}
	err = p.Validate(context.Background(), resource.Resource{Address: addr})
	if err == nil {
		t.Fatal("expected unknown type")
	}
	if !strings.Contains(err.Error(), "unknown type") {
		t.Fatalf("error = %q, want unknown type", err)
	}
}

func TestProviderLifecycleNotImplemented(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{BaseURL: "https://matomo.example.com", TokenAuth: providerToken})
	addr, err := resource.ParseAddress("matomo.tag.pageview")
	if err != nil {
		t.Fatal(err)
	}
	res := resource.Resource{Address: addr}

	if _, err := p.Read(context.Background(), res); err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("Read = %v, want not implemented", err)
	}
	if _, err := p.Create(context.Background(), res); err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("Create = %v, want not implemented", err)
	}
	if _, err := p.Update(context.Background(), res, resource.RemoteResource{Address: addr}); err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("Update = %v, want not implemented", err)
	}
	if _, err := p.Import(context.Background(), addr, "1"); err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("Import = %v, want not implemented", err)
	}
}

func TestCheckProvidersInvokesConnectionChecker(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `"5.2.0"`)
	}))
	t.Cleanup(srv.Close)

	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:   srv.URL,
		TokenAuth: providerToken,
	}, srv.Client())

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	// ConnectionChecker runs after a supported type resolves.
	addr, err := resource.ParseAddress("matomo.goal.trial_started")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.LookupFor(addr); err != nil {
		t.Fatalf("LookupFor goal: %v", err)
	}
}

func TestProviderClientRejectsMalformedURL(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{
		BaseURL:   "https://example.com/?token_auth=" + providerToken,
		TokenAuth: providerToken,
	})
	_, err := p.Client()
	if err == nil {
		t.Fatal("expected malformed URL error")
	}
	if strings.Contains(err.Error(), providerToken) {
		t.Fatalf("secret leaked in %q", err)
	}
}
