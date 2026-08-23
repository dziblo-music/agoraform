package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

const testToken = "super-secret-token-value"

func TestCallAuthenticatedSuccess(t *testing.T) {
	t.Parallel()

	var gotURL *url.URL
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		gotBody = string(body)
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `"5.2.0"`)
	}))
	t.Cleanup(srv.Close)

	c := mustClient(t, srv.URL, testToken)
	raw, err := c.Call(context.Background(), "API.getMatomoVersion", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var version string
	if err := json.Unmarshal(raw, &version); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if version != "5.2.0" {
		t.Fatalf("version = %q, want 5.2.0", version)
	}
	if gotURL != nil && (gotURL.RawQuery != "" && strings.Contains(gotURL.RawQuery, "token")) {
		t.Fatalf("query %q leaked a token", gotURL.RawQuery)
	}
	vals, err := url.ParseQuery(gotBody)
	if err != nil {
		t.Fatalf("parse body: %v", err)
	}
	if vals.Get("token_auth") != testToken {
		t.Fatalf("token_auth = %q, want test token in POST body", vals.Get("token_auth"))
	}
	if vals.Get("module") != "API" || vals.Get("method") != "API.getMatomoVersion" || vals.Get("format") != "JSON" {
		t.Fatalf("form = %v, want module/method/format", vals)
	}
}

func TestCheckConnectionSuccess(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `"5.1.2"`)
	}))
	t.Cleanup(srv.Close)

	c := mustClient(t, srv.URL, testToken)
	if err := c.CheckConnection(context.Background()); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func TestCheckConnectionWrappedVersion(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"value":"5.0.0"}`)
	}))
	t.Cleanup(srv.Close)

	c := mustClient(t, srv.URL, testToken)
	version, err := c.Analytics().GetMatomoVersion(context.Background())
	if err != nil {
		t.Fatalf("GetMatomoVersion: %v", err)
	}
	if version != "5.0.0" {
		t.Fatalf("version = %q, want 5.0.0", version)
	}
}

func TestNewMalformedBaseURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		url  string
	}{
		{name: "empty", url: ""},
		{name: "spaces", url: "   "},
		{name: "no scheme", url: "matomo.example.com"},
		{name: "ftp", url: "ftp://matomo.example.com"},
		{name: "colon slash", url: "://example.com"},
		{name: "token in query", url: "https://matomo.example.com/?token_auth=" + testToken},
		{name: "userinfo", url: "https://user:" + testToken + "@matomo.example.com"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := client.New(client.Config{BaseURL: tc.url, TokenAuth: testToken})
			if err == nil {
				t.Fatal("expected error")
			}
			assertNoSecret(t, err.Error())
		})
	}
}

func TestNewMissingToken(t *testing.T) {
	t.Parallel()

	_, err := client.New(client.Config{BaseURL: "https://matomo.example.com"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "token") {
		t.Fatalf("error = %q, want token", err)
	}
}

func TestCallUnauthorizedHTTP(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope "+testToken, http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	c := mustClient(t, srv.URL, testToken)
	_, err := c.Call(context.Background(), "API.getMatomoVersion", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *client.Error
	if !errors.As(err, &apiErr) || !apiErr.IsUnauthorized() {
		t.Fatalf("error = %v, want unauthorized API error", err)
	}
	assertNoSecret(t, err.Error())
}

func TestCallMatomoAPIError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"result":"error","message":"Unable to authenticate with `+testToken+`"}`)
	}))
	t.Cleanup(srv.Close)

	c := mustClient(t, srv.URL, testToken)
	_, err := c.Call(context.Background(), "API.getMatomoVersion", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "authenticate") {
		t.Fatalf("error = %q, want API message", err)
	}
	assertNoSecret(t, err.Error())
}

func TestCallMalformedResponse(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `<html>login `+testToken+`</html>`)
	}))
	t.Cleanup(srv.Close)

	c := mustClient(t, srv.URL, testToken)
	_, err := c.Call(context.Background(), "API.getMatomoVersion", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("error = %q, want malformed", err)
	}
	assertNoSecret(t, err.Error())
}

func TestCallTimeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		_, _ = io.WriteString(w, `"5.2.0"`)
	}))
	t.Cleanup(srv.Close)

	c, err := client.New(client.Config{
		BaseURL:    srv.URL,
		TokenAuth:  testToken,
		HTTPClient: &http.Client{Timeout: 50 * time.Millisecond},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, callErr := c.Call(context.Background(), "API.getMatomoVersion", nil)
	if callErr == nil {
		t.Fatal("expected timeout")
	}
	if !strings.Contains(callErr.Error(), "timed out") && !strings.Contains(callErr.Error(), "network") {
		t.Fatalf("error = %q, want timeout or network", callErr)
	}
	assertNoSecret(t, callErr.Error())
}

func TestCallCanceledContext(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `"5.2.0"`)
	}))
	t.Cleanup(srv.Close)

	c := mustClient(t, srv.URL, testToken)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Call(ctx, "API.getMatomoVersion", nil)
	if err == nil {
		t.Fatal("expected cancellation")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("error = %v, want canceled", err)
	}
	assertNoSecret(t, err.Error())
}

func TestCallNetworkFailure(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	c := mustClient(t, srv.URL, testToken)
	srv.Close()

	_, err := c.Call(context.Background(), "API.getMatomoVersion", nil)
	if err == nil {
		t.Fatal("expected network error")
	}
	if !strings.Contains(err.Error(), "network") && !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %q, want network error", err)
	}
	assertNoSecret(t, err.Error())
}

func TestCallMissingMethod(t *testing.T) {
	t.Parallel()

	c := mustClient(t, "https://matomo.example.com", testToken)
	_, err := c.Call(context.Background(), "", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAnalyticsAppliesSiteID(t *testing.T) {
	t.Parallel()

	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		_, _ = io.WriteString(w, `{"id":1}`)
	}))
	t.Cleanup(srv.Close)

	c, err := client.New(client.Config{
		BaseURL:    srv.URL,
		TokenAuth:  testToken,
		SiteID:     "7",
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Analytics().Call(context.Background(), "Goals.getGoals", nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
	vals, _ := url.ParseQuery(gotBody)
	if vals.Get("idSite") != "7" {
		t.Fatalf("idSite = %q, want 7", vals.Get("idSite"))
	}
}

func TestTagManagerCallPrefixesAndDefaults(t *testing.T) {
	t.Parallel()

	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		_, _ = io.WriteString(w, `{"idcontainer":"abc"}`)
	}))
	t.Cleanup(srv.Close)

	c, err := client.New(client.Config{
		BaseURL:     srv.URL,
		TokenAuth:   testToken,
		SiteID:      "3",
		ContainerID: "abcXYZ",
		HTTPClient:  srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.TagManager().Call(context.Background(), "getContainer", nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
	vals, _ := url.ParseQuery(gotBody)
	if vals.Get("method") != "TagManager.getContainer" {
		t.Fatalf("method = %q, want TagManager.getContainer", vals.Get("method"))
	}
	if vals.Get("idSite") != "3" {
		t.Fatalf("idSite = %q, want 3", vals.Get("idSite"))
	}
	if vals.Get("idContainer") != "abcXYZ" {
		t.Fatalf("idContainer = %q, want abcXYZ", vals.Get("idContainer"))
	}
}

func TestRedact(t *testing.T) {
	t.Parallel()

	in := "GET https://matomo.example.com/index.php?token_auth=" + testToken + " Authorization: Bearer " + testToken
	out := client.Redact(in, testToken)
	assertNoSecret(t, out)
	if !strings.Contains(out, "[redacted]") {
		t.Fatalf("redacted = %q, want [redacted]", out)
	}
}

func TestConfigStringRedactsToken(t *testing.T) {
	t.Parallel()

	cfg := client.Config{
		BaseURL:   "https://matomo.example.com",
		TokenAuth: testToken,
		SiteID:    "1",
	}
	assertNoSecret(t, cfg.String())
	assertNoSecret(t, cfg.Redacted().TokenAuth)
}

func TestNormalizeIndexPHP(t *testing.T) {
	t.Parallel()

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `"ok"`)
	}))
	t.Cleanup(srv.Close)

	c := mustClient(t, srv.URL, testToken)
	if _, err := c.Call(context.Background(), "API.getMatomoVersion", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(gotPath, "/index.php") {
		t.Fatalf("path = %q, want /index.php", gotPath)
	}
}

func mustClient(t *testing.T, baseURL, token string) *client.Client {
	t.Helper()
	c, err := client.New(client.Config{
		BaseURL:    baseURL,
		TokenAuth:  token,
		HTTPClient: http.DefaultClient,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func assertNoSecret(t *testing.T, s string) {
	t.Helper()
	if strings.Contains(s, testToken) {
		t.Fatalf("secret leaked in %q", s)
	}
}
