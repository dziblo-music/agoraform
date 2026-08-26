package client_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/providers/googleads/client"
)

const (
	testDeveloperToken = "test-developer-token-value"
	testClientID       = "test-oauth-client-id-value"
	testClientSecret   = "test-oauth-client-secret-value"
	testRefreshToken   = "test-oauth-refresh-token-value"
	testAccessToken    = "test-oauth-access-token-value"
	testCustomerID     = "1234567890"
	testLoginCustomer  = "0987654321"
)

type fakeAds struct {
	mu sync.Mutex

	tokenStatus  int
	tokenBody    string
	searchStatus int
	searchBody   string
	searchPages  []string
	mutateStatus int
	mutateBody   string

	tokenHits  int
	searchHits int
	mutateHits int

	lastTokenForm     url.Values
	lastPath          string
	lastMethod        string
	lastAuth          string
	lastDevToken      string
	lastLoginCustomer string
	lastJSON          string
}

func (f *fakeAds) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	body, _ := io.ReadAll(r.Body)
	f.lastPath = r.URL.Path
	f.lastMethod = r.Method
	f.lastAuth = r.Header.Get("Authorization")
	f.lastDevToken = r.Header.Get("developer-token")
	f.lastLoginCustomer = r.Header.Get("login-customer-id")

	if r.URL.Path == "/oauth/token" {
		f.tokenHits++
		vals, _ := url.ParseQuery(string(body))
		f.lastTokenForm = vals
		status := f.tokenStatus
		if status == 0 {
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if f.tokenBody != "" {
			_, _ = io.WriteString(w, f.tokenBody)
			return
		}
		if status >= 400 {
			_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"bad `+testRefreshToken+`"}`)
			return
		}
		_, _ = io.WriteString(w, `{"access_token":"`+testAccessToken+`","expires_in":3600,"token_type":"Bearer"}`)
		return
	}

	if strings.Contains(r.URL.Path, "/googleAds:search") {
		f.searchHits++
		f.lastJSON = string(body)
		status := f.searchStatus
		if status == 0 {
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("request-id", "req-search-1")
		w.WriteHeader(status)
		if status >= 400 {
			if f.searchBody != "" {
				_, _ = io.WriteString(w, f.searchBody)
				return
			}
			_, _ = io.WriteString(w, googleAdsErrorJSON(status, "query failed "+testAccessToken))
			return
		}
		if len(f.searchPages) > 0 {
			idx := f.searchHits - 1
			if idx >= len(f.searchPages) {
				idx = len(f.searchPages) - 1
			}
			_, _ = io.WriteString(w, f.searchPages[idx])
			return
		}
		if f.searchBody != "" {
			_, _ = io.WriteString(w, f.searchBody)
			return
		}
		_, _ = io.WriteString(w, `{"results":[{"customer":{"id":"`+testCustomerID+`"}}]}`)
		return
	}

	if strings.Contains(r.URL.Path, ":mutate") {
		f.mutateHits++
		f.lastJSON = string(body)
		status := f.mutateStatus
		if status == 0 {
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("request-id", "req-mutate-1")
		w.WriteHeader(status)
		if status >= 400 {
			if f.mutateBody != "" {
				_, _ = io.WriteString(w, f.mutateBody)
				return
			}
			_, _ = io.WriteString(w, googleAdsErrorJSON(status, "mutate failed "+testDeveloperToken))
			return
		}
		if f.mutateBody != "" {
			_, _ = io.WriteString(w, f.mutateBody)
			return
		}
		_, _ = io.WriteString(w, `{"results":[{"resourceName":"customers/`+testCustomerID+`/conversionActions/1"}]}`)
		return
	}

	http.NotFound(w, r)
}

func googleAdsErrorJSON(status int, message string) string {
	rpcStatus := "INVALID_ARGUMENT"
	if status == 401 {
		rpcStatus = "UNAUTHENTICATED"
	}
	if status == 403 {
		rpcStatus = "PERMISSION_DENIED"
	}
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(map[string]any{
		"error": map[string]any{
			"code":    status,
			"message": message,
			"status":  rpcStatus,
			"details": []any{
				map[string]any{
					"@type": "type.googleapis.com/google.ads.googleads.v25.errors.GoogleAdsFailure",
					"errors": []any{
						map[string]any{
							"errorCode": map[string]string{"requestError": "BAD_RESOURCE_ID"},
							"message":   message,
						},
					},
					"requestId": "failure-id-1",
				},
			},
		},
	})
	return buf.String()
}

func startFakeAds(t *testing.T, f *fakeAds) *httptest.Server {
	t.Helper()
	if f == nil {
		f = &fakeAds{}
	}
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(srv.Close)
	return srv
}

func testConfig(srv *httptest.Server) client.Config {
	return client.Config{
		DeveloperToken: testDeveloperToken,
		ClientID:       testClientID,
		ClientSecret:   testClientSecret,
		RefreshToken:   testRefreshToken,
		CustomerID:     testCustomerID,
		BaseURL:        srv.URL,
		TokenURL:       srv.URL + "/oauth/token",
		HTTPClient:     srv.Client(),
	}
}

func mustClient(t *testing.T, srv *httptest.Server) *client.Client {
	t.Helper()
	c, err := client.New(testConfig(srv))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func assertNoSecret(t *testing.T, s string) {
	t.Helper()
	for _, secret := range []string{testDeveloperToken, testClientID, testClientSecret, testRefreshToken, testAccessToken} {
		if strings.Contains(s, secret) {
			t.Fatalf("secret leaked in %q", s)
		}
	}
}
