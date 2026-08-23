package client_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestConfigStringRedactsCredentialBearingBaseURL(t *testing.T) {
	t.Parallel()

	cfg := client.Config{
		BaseURL:   "https://user:password@matomo.example.com/path?token_auth=" + testToken + "&other=sensitive#fragment",
		TokenAuth: testToken,
		SiteID:    "1",
	}

	redacted := cfg.Redacted()
	for _, secret := range []string{"user", "password", testToken, "token_auth", "sensitive", "fragment"} {
		if strings.Contains(redacted.BaseURL, secret) {
			t.Fatalf("Redacted().BaseURL leaked %q in %q", secret, redacted.BaseURL)
		}
		if strings.Contains(cfg.String(), secret) {
			t.Fatalf("String() leaked %q in %q", secret, cfg.String())
		}
	}
	if redacted.BaseURL != "https://matomo.example.com/path" {
		t.Fatalf("Redacted().BaseURL = %q, want sanitized URL", redacted.BaseURL)
	}
}

func TestConfigStringHidesMalformedBaseURL(t *testing.T) {
	t.Parallel()

	cfg := client.Config{
		BaseURL:   "://user:password@matomo.example.com/?token_auth=" + testToken,
		TokenAuth: testToken,
	}

	got := cfg.String()
	for _, secret := range []string{"user", "password", testToken, "token_auth"} {
		if strings.Contains(got, secret) {
			t.Fatalf("String() leaked %q in %q", secret, got)
		}
	}
	if !strings.Contains(got, "[redacted-invalid-url]") {
		t.Fatalf("String() = %q, want invalid URL placeholder", got)
	}
}

func TestCallReservedParamsCannotBeOverridden(t *testing.T) {
	t.Parallel()

	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		gotBody = string(body)
		_, _ = io.WriteString(w, `"ok"`)
	}))
	t.Cleanup(srv.Close)

	c, err := client.New(client.Config{
		BaseURL:    srv.URL,
		TokenAuth:  testToken,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}

	params := url.Values{
		"module":     {"EvilModule"},
		"method":     {"Evil.method"},
		"format":     {"XML"},
		"token_auth": {"attacker-token"},
		"idSite":     {"42"},
	}
	const method = "API.getMatomoVersion"
	if _, err := c.Call(context.Background(), method, params); err != nil {
		t.Fatalf("Call: %v", err)
	}

	vals, err := url.ParseQuery(gotBody)
	if err != nil {
		t.Fatalf("parse body: %v", err)
	}

	want := map[string]string{
		"module":     "API",
		"method":     method,
		"format":     "JSON",
		"token_auth": testToken,
		"idSite":     "42",
	}
	for key, expected := range want {
		if got := vals.Get(key); got != expected {
			t.Fatalf("%s = %q, want %q", key, got, expected)
		}
	}
	for _, key := range []string{"module", "method", "format", "token_auth"} {
		if len(vals[key]) != 1 {
			t.Fatalf("%s values = %v, want exactly one client-owned value", key, vals[key])
		}
	}
}
