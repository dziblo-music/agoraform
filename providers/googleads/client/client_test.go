package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dziblo-music/agoraform/providers/googleads/client"
)

func TestNormalizeCustomerID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "123-456-7890", want: "1234567890"},
		{in: " 1234567890 ", want: "1234567890"},
		{in: "customers/1234567890", want: "1234567890"},
		{in: "customers/123-456-7890", want: "1234567890"},
		{in: "", wantErr: true},
		{in: "12345", wantErr: true},
		{in: "12345678901", wantErr: true},
		{in: "123456789a", wantErr: true},
		{in: "customers/", wantErr: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, err := client.NormalizeCustomerID(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeCustomerID: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCustomerResourceName(t *testing.T) {
	t.Parallel()

	got, err := client.CustomerResourceName("123-456-7890")
	if err != nil {
		t.Fatal(err)
	}
	if got != "customers/1234567890" {
		t.Fatalf("got %q", got)
	}
}

func TestNewNormalizesCustomerIDs(t *testing.T) {
	t.Parallel()

	f := &fakeAds{}
	srv := startFakeAds(t, f)
	cfg := testConfig(srv)
	cfg.CustomerID = "123-456-7890"
	cfg.LoginCustomerID = "customers/0987654321"
	c, err := client.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c.CustomerID() != testCustomerID {
		t.Fatalf("CustomerID = %q", c.CustomerID())
	}
	if c.LoginCustomerID() != testLoginCustomer {
		t.Fatalf("LoginCustomerID = %q", c.LoginCustomerID())
	}
}

func TestNewInvalidLoginCustomerID(t *testing.T) {
	t.Parallel()

	_, err := client.New(client.Config{
		DeveloperToken:  testDeveloperToken,
		ClientID:        testClientID,
		ClientSecret:    testClientSecret,
		RefreshToken:    testRefreshToken,
		CustomerID:      testCustomerID,
		LoginCustomerID: "not-a-customer-id",
		BaseURL:         "https://googleads.example.com",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "login customer ID") {
		t.Fatalf("error = %q, want login customer ID", err)
	}
	assertNoSecret(t, err.Error())
}

func TestNewMissingCredentials(t *testing.T) {
	t.Parallel()

	_, err := client.New(client.Config{CustomerID: testCustomerID})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "developer token") {
		t.Fatalf("error = %q, want developer token", err)
	}
	assertNoSecret(t, err.Error())
}

func TestNewMalformedBaseURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		url  string
	}{
		{name: "no scheme", url: "googleads.googleapis.com"},
		{name: "ftp", url: "ftp://googleads.googleapis.com"},
		{name: "userinfo", url: "https://user:" + testClientSecret + "@googleads.googleapis.com"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := client.New(client.Config{
				DeveloperToken: testDeveloperToken,
				ClientID:       testClientID,
				ClientSecret:   testClientSecret,
				RefreshToken:   testRefreshToken,
				CustomerID:     testCustomerID,
				BaseURL:        tc.url,
			})
			if err == nil {
				t.Fatal("expected error")
			}
			assertNoSecret(t, err.Error())
		})
	}
}

func TestQueryAuthenticatedSuccess(t *testing.T) {
	t.Parallel()

	f := &fakeAds{}
	srv := startFakeAds(t, f)
	c := mustClient(t, srv)

	results, err := c.Query(context.Background(), "SELECT customer.id FROM customer")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if f.tokenHits != 1 {
		t.Fatalf("tokenHits = %d, want 1", f.tokenHits)
	}
	if f.searchHits != 1 {
		t.Fatalf("searchHits = %d, want 1", f.searchHits)
	}
	if f.lastMethod != http.MethodPost {
		t.Fatalf("method = %s, want POST", f.lastMethod)
	}
	if !strings.HasSuffix(f.lastPath, "/"+client.Version+"/customers/"+testCustomerID+"/googleAds:search") {
		t.Fatalf("path = %q, want versioned search path", f.lastPath)
	}
	if f.lastAuth != "Bearer "+testAccessToken {
		t.Fatalf("Authorization = %q", f.lastAuth)
	}
	if f.lastDevToken != testDeveloperToken {
		t.Fatalf("developer-token = %q", f.lastDevToken)
	}
	if f.lastTokenForm.Get("refresh_token") != testRefreshToken {
		t.Fatalf("refresh_token not posted to token endpoint")
	}
	if f.lastTokenForm.Get("client_secret") != testClientSecret {
		t.Fatalf("client_secret not posted to token endpoint")
	}

	var row struct {
		Customer struct {
			ID string `json:"id"`
		} `json:"customer"`
	}
	if err := json.Unmarshal(results[0], &row); err != nil {
		t.Fatal(err)
	}
	if row.Customer.ID != testCustomerID {
		t.Fatalf("customer.id = %q", row.Customer.ID)
	}
}

func TestQueryPaginates(t *testing.T) {
	t.Parallel()

	f := &fakeAds{
		searchPages: []string{
			`{"results":[{"customer":{"id":"1"}}],"nextPageToken":"page-2"}`,
			`{"results":[{"customer":{"id":"2"}}]}`,
		},
	}
	srv := startFakeAds(t, f)
	c := mustClient(t, srv)

	results, err := c.Query(context.Background(), "SELECT customer.id FROM customer")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if f.searchHits != 2 {
		t.Fatalf("searchHits = %d, want 2", f.searchHits)
	}
	if !strings.Contains(f.lastJSON, `"pageToken":"page-2"`) {
		t.Fatalf("second page missing pageToken: %s", f.lastJSON)
	}
}

func TestQueryCustomerOverride(t *testing.T) {
	t.Parallel()

	f := &fakeAds{}
	srv := startFakeAds(t, f)
	c := mustClient(t, srv)
	if _, err := c.QueryCustomer(context.Background(), "111-111-1111", "SELECT customer.id FROM customer"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.lastPath, "/customers/1111111111/") {
		t.Fatalf("path = %q, want overridden customer ID", f.lastPath)
	}
}

func TestQuerySendsLoginCustomerID(t *testing.T) {
	t.Parallel()

	f := &fakeAds{}
	srv := startFakeAds(t, f)
	cfg := testConfig(srv)
	cfg.LoginCustomerID = "098-765-4321"
	c, err := client.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Query(context.Background(), "SELECT customer.id FROM customer"); err != nil {
		t.Fatal(err)
	}
	if f.lastLoginCustomer != testLoginCustomer {
		t.Fatalf("login-customer-id = %q, want %s", f.lastLoginCustomer, testLoginCustomer)
	}
}

func TestQueryCachesAccessToken(t *testing.T) {
	t.Parallel()

	f := &fakeAds{}
	srv := startFakeAds(t, f)
	c := mustClient(t, srv)
	if _, err := c.Query(context.Background(), "SELECT customer.id FROM customer"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Query(context.Background(), "SELECT customer.id FROM customer"); err != nil {
		t.Fatal(err)
	}
	if f.tokenHits != 1 {
		t.Fatalf("tokenHits = %d, want 1 cached token", f.tokenHits)
	}
}

func TestCheckConnectionSuccess(t *testing.T) {
	t.Parallel()

	f := &fakeAds{}
	srv := startFakeAds(t, f)
	c := mustClient(t, srv)
	if err := c.CheckConnection(context.Background()); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func TestCheckConnectionEmptyCustomer(t *testing.T) {
	t.Parallel()

	f := &fakeAds{searchBody: `{"results":[]}`}
	srv := startFakeAds(t, f)
	c := mustClient(t, srv)
	err := c.CheckConnection(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), testCustomerID) {
		t.Fatalf("error = %q, want customer ID", err)
	}
	assertNoSecret(t, err.Error())
}

func TestMutateSuccess(t *testing.T) {
	t.Parallel()

	f := &fakeAds{}
	srv := startFakeAds(t, f)
	c := mustClient(t, srv)

	raw, err := c.Mutate(context.Background(), "conversionActions", []map[string]any{
		{"create": map[string]any{"name": "Trial Started"}},
	})
	if err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if !strings.Contains(string(raw), "conversionActions/1") {
		t.Fatalf("response = %s", raw)
	}
	if !strings.HasSuffix(f.lastPath, "/"+client.Version+"/customers/"+testCustomerID+"/conversionActions:mutate") {
		t.Fatalf("path = %q", f.lastPath)
	}
	if !strings.Contains(f.lastJSON, `"operations"`) || !strings.Contains(f.lastJSON, "Trial Started") {
		t.Fatalf("body = %s", f.lastJSON)
	}
}

func TestSuggestGeoTargetConstants(t *testing.T) {
	t.Parallel()

	f := &fakeAds{}
	srv := startFakeAds(t, f)
	c := mustClient(t, srv)

	got, err := c.SuggestGeoTargetConstants(context.Background(), []string{"United States"})
	if err != nil {
		t.Fatalf("SuggestGeoTargetConstants: %v", err)
	}
	if len(got) != 1 || got[0].Constant.ID != "2840" || got[0].Constant.CanonicalName != "United States" {
		t.Fatalf("suggestions = %#v", got)
	}
	if !strings.HasSuffix(f.lastPath, "/"+client.Version+"/geoTargetConstants:suggest") {
		t.Fatalf("path = %q", f.lastPath)
	}
	if !strings.Contains(f.lastJSON, "United States") {
		t.Fatalf("body = %s", f.lastJSON)
	}
}

func TestSuggestGeoTargetConstantsRequiresName(t *testing.T) {
	t.Parallel()

	c := mustClient(t, startFakeAds(t, &fakeAds{}))
	_, err := c.SuggestGeoTargetConstants(context.Background(), []string{"  "})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMutateRejectsInvalidCollection(t *testing.T) {
	t.Parallel()

	c := mustClient(t, startFakeAds(t, &fakeAds{}))
	_, err := c.Mutate(context.Background(), "../evil", []map[string]any{{"create": map[string]any{}}})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "collection") {
		t.Fatalf("error = %q", err)
	}
}

func TestQueryMissingQuery(t *testing.T) {
	t.Parallel()

	c := mustClient(t, startFakeAds(t, &fakeAds{}))
	_, err := c.Query(context.Background(), "  ")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOAuthFailure(t *testing.T) {
	t.Parallel()

	f := &fakeAds{tokenStatus: http.StatusBadRequest}
	srv := startFakeAds(t, f)
	c := mustClient(t, srv)
	_, err := c.Query(context.Background(), "SELECT customer.id FROM customer")
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *client.Error
	if !errors.As(err, &apiErr) || !apiErr.IsUnauthorized() {
		t.Fatalf("error = %v, want unauthorized", err)
	}
	assertNoSecret(t, err.Error())
}

func TestQueryUnauthorized(t *testing.T) {
	t.Parallel()

	f := &fakeAds{searchStatus: http.StatusUnauthorized}
	srv := startFakeAds(t, f)
	c := mustClient(t, srv)
	_, err := c.Query(context.Background(), "SELECT customer.id FROM customer")
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *client.Error
	if !errors.As(err, &apiErr) || !apiErr.IsUnauthorized() {
		t.Fatalf("error = %v, want unauthorized", err)
	}
	assertNoSecret(t, err.Error())
}

func TestQueryAPIError(t *testing.T) {
	t.Parallel()

	f := &fakeAds{searchStatus: http.StatusBadRequest}
	srv := startFakeAds(t, f)
	c := mustClient(t, srv)
	_, err := c.Query(context.Background(), "SELECT bogus FROM customer")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "BAD_RESOURCE_ID") && !strings.Contains(err.Error(), "INVALID_ARGUMENT") {
		t.Fatalf("error = %q, want Google Ads failure details", err)
	}
	assertNoSecret(t, err.Error())
}

func TestMutateAPIError(t *testing.T) {
	t.Parallel()

	f := &fakeAds{mutateStatus: http.StatusBadRequest}
	srv := startFakeAds(t, f)
	c := mustClient(t, srv)
	_, err := c.Mutate(context.Background(), "conversionActions", []map[string]any{
		{"create": map[string]any{"name": "x"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "request-id") && !strings.Contains(err.Error(), "INVALID_ARGUMENT") {
		t.Fatalf("error = %q, want actionable mutate diagnostic", err)
	}
	assertNoSecret(t, err.Error())
}

func TestQueryMalformedResponse(t *testing.T) {
	t.Parallel()

	f := &fakeAds{searchBody: `<html>login ` + testAccessToken + `</html>`}
	srv := startFakeAds(t, f)
	c := mustClient(t, srv)
	_, err := c.Query(context.Background(), "SELECT customer.id FROM customer")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("error = %q, want malformed", err)
	}
	assertNoSecret(t, err.Error())
}

func TestQueryCanceledContext(t *testing.T) {
	t.Parallel()

	c := mustClient(t, startFakeAds(t, &fakeAds{}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Query(ctx, "SELECT customer.id FROM customer")
	if err == nil {
		t.Fatal("expected cancellation")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("error = %v, want canceled", err)
	}
	assertNoSecret(t, err.Error())
}

func TestQueryNetworkFailure(t *testing.T) {
	t.Parallel()

	srv := startFakeAds(t, &fakeAds{})
	c := mustClient(t, srv)
	srv.Close()

	_, err := c.Query(context.Background(), "SELECT customer.id FROM customer")
	if err == nil {
		t.Fatal("expected network error")
	}
	if !strings.Contains(err.Error(), "network") && !strings.Contains(err.Error(), "timed out") && !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("error = %q, want network error", err)
	}
	assertNoSecret(t, err.Error())
}

func TestQueryTimeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		_, _ = io.WriteString(w, `{"access_token":"`+testAccessToken+`","expires_in":3600}`)
	}))
	t.Cleanup(srv.Close)

	c, err := client.New(client.Config{
		DeveloperToken: testDeveloperToken,
		ClientID:       testClientID,
		ClientSecret:   testClientSecret,
		RefreshToken:   testRefreshToken,
		CustomerID:     testCustomerID,
		BaseURL:        srv.URL,
		TokenURL:       srv.URL + "/oauth/token",
		HTTPClient:     &http.Client{Timeout: 50 * time.Millisecond},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, callErr := c.Query(context.Background(), "SELECT customer.id FROM customer")
	if callErr == nil {
		t.Fatal("expected timeout")
	}
	if !strings.Contains(callErr.Error(), "timed out") && !strings.Contains(callErr.Error(), "network") {
		t.Fatalf("error = %q, want timeout or network", callErr)
	}
	assertNoSecret(t, callErr.Error())
}

func TestMutateNilOperations(t *testing.T) {
	t.Parallel()

	c := mustClient(t, startFakeAds(t, &fakeAds{}))
	_, err := c.Mutate(context.Background(), "conversionActions", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
