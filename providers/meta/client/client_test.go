package client_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dziblo-music/agoraform/providers/meta/client"
)

const (
	testToken     = "EAAB-secret-test-token"
	testAccountID = "123456789012345"
)

func TestNewNormalizesConfiguration(t *testing.T) {
	t.Parallel()
	c, err := client.New(client.Config{AccessToken: testToken, AdAccountID: testAccountID})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := c.AdAccountID(), "act_"+testAccountID; got != want {
		t.Fatalf("AdAccountID = %q, want %q", got, want)
	}
}

func TestNormalizeAdAccountID(t *testing.T) {
	t.Parallel()
	for _, input := range []string{testAccountID, "act_" + testAccountID, "  act_" + testAccountID + "  "} {
		got, err := client.NormalizeAdAccountID(input)
		if err != nil {
			t.Fatalf("NormalizeAdAccountID(%q): %v", input, err)
		}
		if got != "act_"+testAccountID {
			t.Fatalf("NormalizeAdAccountID(%q) = %q", input, got)
		}
	}
	for _, input := range []string{"", "act_", "act_abc", "123-456", "act_act_123", "0"} {
		if _, err := client.NormalizeAdAccountID(input); err == nil {
			t.Fatalf("NormalizeAdAccountID(%q) succeeded", input)
		}
	}
}

func TestVersionedAuthenticatedMethods(t *testing.T) {
	t.Parallel()
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/"+client.Version+"/") {
			t.Errorf("path = %q, want version %s", r.URL.Path, client.Version)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
			t.Errorf("Authorization = %q", got)
		}
		methods = append(methods, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()
	c := newClient(t, server.URL, server.Client(), time.Second)

	var out map[string]bool
	if err := c.Get(context.Background(), "me", url.Values{"fields": {"id"}}, &out); err != nil {
		t.Fatal(err)
	}
	if err := c.Post(context.Background(), c.AdAccountID()+"/campaigns", url.Values{"name": {"test"}}, &out); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(context.Background(), "123", nil, &out); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(methods, ","), "GET,POST,DELETE"; got != want {
		t.Fatalf("methods = %s, want %s", got, want)
	}
}

func TestListUsesCursorWithoutFollowingNextURL(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if got := r.URL.Query().Get("access_token"); got != "" {
			t.Fatalf("pagination copied access_token from next URL")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("after") == "cursor-2" {
			_, _ = io.WriteString(w, `{"data":[{"id":"2"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"1"}],"paging":{"cursors":{"after":"cursor-2"},"next":"https://evil.example/next?access_token=leak"}}`)
	}))
	defer server.Close()
	c := newClient(t, server.URL, server.Client(), time.Second)
	items, err := c.List(context.Background(), "objects", url.Values{"fields": {"id"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || calls.Load() != 2 {
		t.Fatalf("items=%d calls=%d, want 2/2", len(items), calls.Load())
	}
}

func TestAPIErrorMappingAndRedaction(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-fb-request-id", "request-123")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"slow down `+testToken+`","type":"OAuthException","code":613,"error_subcode":99,"is_transient":true,"fbtrace_id":"trace-456"}}`)
	}))
	defer server.Close()
	c := newClient(t, server.URL, server.Client(), time.Second)
	err := c.Get(context.Background(), "me", nil, &struct{}{})
	var apiErr *client.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T %v, want *client.Error", err, err)
	}
	if apiErr.Code != 613 || apiErr.Subcode != 99 || apiErr.RequestID != "request-123" || apiErr.TraceID != "trace-456" || !apiErr.IsTransient() {
		t.Fatalf("unexpected mapped error: %#v", apiErr)
	}
	assertNoToken(t, err.Error())
}

func TestAuthenticationAndPermissionClassification(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		status     int
		code       int
		auth       bool
		permission bool
	}{
		{name: "authentication", status: 401, code: 190, auth: true},
		{name: "permission", status: 403, code: 200, permission: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, `{"error":{"message":"denied","code":`+strconv.Itoa(tc.code)+`}}`)
			}))
			defer server.Close()
			c := newClient(t, server.URL, server.Client(), time.Second)
			err := c.Get(context.Background(), "me", nil, &struct{}{})
			var apiErr *client.Error
			if !errors.As(err, &apiErr) || apiErr.IsAuthentication() != tc.auth || apiErr.IsPermission() != tc.permission {
				t.Fatalf("classification: %#v", apiErr)
			}
		})
	}
}

func TestIsNotFound(t *testing.T) {
	t.Parallel()
	if !(&client.Error{StatusCode: 404}).IsNotFound() || !(&client.Error{Code: 803}).IsNotFound() {
		t.Fatal("404 and API code 803 must be absence")
	}
	if (&client.Error{StatusCode: 400, Code: 100}).IsNotFound() {
		t.Fatal("generic invalid parameter must not be treated as absence")
	}
}

func TestMalformedResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{not-json`)
	}))
	defer server.Close()
	c := newClient(t, server.URL, server.Client(), time.Second)
	err := c.Get(context.Background(), "me", nil, &struct{}{})
	if err == nil || !strings.Contains(err.Error(), "malformed JSON response") {
		t.Fatalf("error = %v", err)
	}
}

func TestRequestTimeout(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	c := newClient(t, server.URL, server.Client(), 20*time.Millisecond)
	err := c.Get(context.Background(), "me", nil, &struct{}{})
	if err == nil || !strings.Contains(err.Error(), "request timed out") {
		t.Fatalf("error = %v, want timeout", err)
	}
	assertNoToken(t, err.Error())
}

func TestNetworkFailureRedactsToken(t *testing.T) {
	t.Parallel()
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("Authorization: Bearer " + testToken)
	})}
	c := newClient(t, "https://graph.example.com", httpClient, time.Second)
	err := c.Get(context.Background(), testToken, nil, &struct{}{})
	if err == nil || !strings.Contains(err.Error(), "network error") {
		t.Fatalf("error = %v", err)
	}
	assertNoToken(t, err.Error())
}

func TestRedactAndConfigString(t *testing.T) {
	t.Parallel()
	cfg := client.Config{AccessToken: testToken, AdAccountID: testAccountID, BaseURL: "https://graph.facebook.com?access_token=" + testToken}
	assertNoToken(t, cfg.String())
	out := client.Redact("Authorization: Bearer "+testToken+" access_token="+testToken, testToken)
	assertNoToken(t, out)
	if !strings.Contains(out, "[redacted]") {
		t.Fatalf("Redact = %q", out)
	}
}

func TestListRejectsMalformedPagination(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":[],"paging":{"next":"https://graph.facebook.com/next"}}`)
	}))
	defer server.Close()
	c := newClient(t, server.URL, server.Client(), time.Second)
	_, err := c.List(context.Background(), "objects", nil)
	if err == nil || !strings.Contains(err.Error(), "no after cursor") {
		t.Fatalf("error = %v", err)
	}
}

func newClient(t *testing.T, baseURL string, httpClient *http.Client, timeout time.Duration) *client.Client {
	t.Helper()
	c, err := client.New(client.Config{AccessToken: testToken, AdAccountID: testAccountID, BaseURL: baseURL, HTTPClient: httpClient, Timeout: timeout})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func assertNoToken(t *testing.T, value string) {
	t.Helper()
	if strings.Contains(value, testToken) {
		t.Fatalf("access token leaked in %q", value)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
