package googleads_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

const (
	testDeveloperToken = "test-developer-token-value"
	testClientID       = "test-oauth-client-id-value"
	testClientSecret   = "test-oauth-client-secret-value"
	testRefreshToken   = "test-oauth-refresh-token-value"
	testAccessToken    = "test-oauth-access-token-value"
	testCustomerID     = "1234567890"
)

func TestProviderRegisterAndLookup(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	p := googleads.New(validConfig("https://googleads.example.com"))
	if err := reg.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, ok := reg.Lookup(googleads.Name)
	if !ok {
		t.Fatal("Lookup(googleads) failed")
	}
	if got.Name() != googleads.Name {
		t.Fatalf("Name = %q", got.Name())
	}
	if len(got.ResourceTypes()) != 10 || got.ResourceTypes()[0] != googleads.TypeConversionAction || got.ResourceTypes()[1] != googleads.TypeCustomerConversionGoal || got.ResourceTypes()[2] != googleads.TypeCampaignBudget || got.ResourceTypes()[3] != googleads.TypeCampaign || got.ResourceTypes()[4] != googleads.TypeCampaignConversionGoal || got.ResourceTypes()[5] != googleads.TypeAdGroup || got.ResourceTypes()[6] != googleads.TypeKeyword || got.ResourceTypes()[7] != googleads.TypeResponsiveSearchAd || got.ResourceTypes()[8] != googleads.TypeCampaignLocation || got.ResourceTypes()[9] != googleads.TypeCampaignLanguage {
		t.Fatalf("ResourceTypes = %v, want [%s %s %s %s %s %s %s %s %s %s]", got.ResourceTypes(), googleads.TypeConversionAction, googleads.TypeCustomerConversionGoal, googleads.TypeCampaignBudget, googleads.TypeCampaign, googleads.TypeCampaignConversionGoal, googleads.TypeAdGroup, googleads.TypeKeyword, googleads.TypeResponsiveSearchAd, googleads.TypeCampaignLocation, googleads.TypeCampaignLanguage)
	}

	addr, err := resource.ParseAddress("googleads.conversion_action.trial_started")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.LookupFor(addr); err != nil {
		t.Fatalf("LookupFor conversion_action: %v", err)
	}

	goalAddr, err := resource.ParseAddress("googleads.customer_conversion_goal.signup")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.LookupFor(goalAddr); err != nil {
		t.Fatalf("LookupFor customer_conversion_goal: %v", err)
	}

	budgetAddr, err := resource.ParseAddress("googleads.campaign_budget.brand")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.LookupFor(budgetAddr); err != nil {
		t.Fatalf("LookupFor campaign_budget: %v", err)
	}

	campaignAddr, err := resource.ParseAddress("googleads.campaign.brand")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.LookupFor(campaignAddr); err != nil {
		t.Fatalf("LookupFor campaign: %v", err)
	}

	campaignGoalAddr, err := resource.ParseAddress("googleads.campaign_conversion_goal.trial_signup")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.LookupFor(campaignGoalAddr); err != nil {
		t.Fatalf("LookupFor campaign_conversion_goal: %v", err)
	}

	adGroupAddr, err := resource.ParseAddress("googleads.ad_group.brand")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.LookupFor(adGroupAddr); err != nil {
		t.Fatalf("LookupFor ad_group: %v", err)
	}

	keywordAddr, err := resource.ParseAddress("googleads.keyword.brand_exact")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.LookupFor(keywordAddr); err != nil {
		t.Fatalf("LookupFor keyword: %v", err)
	}

	rsaAddr, err := resource.ParseAddress("googleads.responsive_search_ad.brand")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.LookupFor(rsaAddr); err != nil {
		t.Fatalf("LookupFor responsive_search_ad: %v", err)
	}

	locationAddr, err := resource.ParseAddress("googleads.campaign_location.united_states")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.LookupFor(locationAddr); err != nil {
		t.Fatalf("LookupFor campaign_location: %v", err)
	}

	languageAddr, err := resource.ParseAddress("googleads.campaign_language.english")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.LookupFor(languageAddr); err != nil {
		t.Fatalf("LookupFor campaign_language: %v", err)
	}

	unknown, err := resource.ParseAddress("googleads.ad.brand")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.LookupFor(unknown); err == nil {
		t.Fatal("LookupFor ad succeeded, want unknown type")
	}
}

func TestProviderCheckConnection(t *testing.T) {
	t.Parallel()

	p, _ := testProvider(t, nil)
	if err := p.CheckConnection(context.Background()); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func TestProviderCheckConnectionMissingCredentials(t *testing.T) {
	t.Parallel()

	p := googleads.New(googleads.Config{})
	err := p.CheckConnection(context.Background())
	if err == nil {
		t.Fatal("expected missing credential error")
	}
	if !strings.Contains(err.Error(), googleads.EnvDeveloperToken) && !strings.Contains(err.Error(), "required") {
		t.Fatalf("error = %q, want missing %s", err, googleads.EnvDeveloperToken)
	}
	if strings.Contains(err.Error(), testDeveloperToken) {
		t.Fatalf("secret leaked in %q", err)
	}
}

func TestProviderCheckConnectionUnauthorized(t *testing.T) {
	t.Parallel()

	p, _ := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/oauth/token") {
			writeToken(w)
			return
		}
		http.Error(w, "bad "+testAccessToken, http.StatusForbidden)
	})
	err := p.CheckConnection(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), testAccessToken) || strings.Contains(err.Error(), testDeveloperToken) {
		t.Fatalf("secret leaked in %q", err)
	}
}

func TestProviderValidateUnknownType(t *testing.T) {
	t.Parallel()

	p := googleads.New(validConfig("https://googleads.example.com"))
	addr, err := resource.ParseAddress("googleads.ad.brand")
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

	p := googleads.New(validConfig("https://googleads.example.com"))
	addr, err := resource.ParseAddress("googleads.ad.brand")
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
	if _, err := p.Destroy(context.Background(), res); err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("Destroy = %v, want not implemented", err)
	}
}

func TestProviderConfigureRejectsSecrets(t *testing.T) {
	t.Parallel()

	p := googleads.New(validConfig("https://googleads.example.com"))
	err := p.Configure(resource.Attributes{"developerToken": testDeveloperToken})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "environment") {
		t.Fatalf("error = %q, want environment-variable guidance", err)
	}
	if strings.Contains(err.Error(), testDeveloperToken) {
		t.Fatalf("secret leaked in %q", err)
	}
}

func TestProviderConfigureRejectsUnknownField(t *testing.T) {
	t.Parallel()

	p := googleads.New(validConfig("https://googleads.example.com"))
	err := p.Configure(resource.Attributes{"campaigns": true})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown provider configuration field") {
		t.Fatalf("error = %q", err)
	}
}

func TestProviderConfigureEmpty(t *testing.T) {
	t.Parallel()

	p := googleads.New(validConfig("https://googleads.example.com"))
	if err := p.Configure(nil); err != nil {
		t.Fatalf("Configure(nil): %v", err)
	}
	if err := p.Configure(resource.Attributes{}); err != nil {
		t.Fatalf("Configure empty: %v", err)
	}
}

func TestExplicitConfigOverridesEnv(t *testing.T) {
	t.Setenv(googleads.EnvDeveloperToken, "env-developer-token")
	t.Setenv(googleads.EnvClientID, "env-client-id")
	t.Setenv(googleads.EnvClientSecret, "env-client-secret")
	t.Setenv(googleads.EnvRefreshToken, "env-refresh-token")
	t.Setenv(googleads.EnvCustomerID, "0000000000")

	p, srv := testProvider(t, nil)
	if err := p.CheckConnection(context.Background()); err != nil {
		t.Fatalf("CheckConnection with explicit config: %v", err)
	}
	fromEnv := googleads.NewFromEnv()
	c, err := fromEnv.Client()
	if err != nil {
		t.Fatalf("NewFromEnv Client: %v", err)
	}
	if c.CustomerID() != "0000000000" {
		t.Fatalf("env customer ID = %q", c.CustomerID())
	}
	_ = srv
}

func TestProviderClientRejectsMalformedURL(t *testing.T) {
	t.Parallel()

	p := googleads.New(googleads.Config{
		DeveloperToken: testDeveloperToken,
		ClientID:       testClientID,
		ClientSecret:   testClientSecret,
		RefreshToken:   testRefreshToken,
		CustomerID:     testCustomerID,
		BaseURL:        "https://user:" + testClientSecret + "@googleads.googleapis.com",
	})
	_, err := p.Client()
	if err == nil {
		t.Fatal("expected malformed URL error")
	}
	if strings.Contains(err.Error(), testClientSecret) {
		t.Fatalf("secret leaked in %q", err)
	}
}

func validConfig(baseURL string) googleads.Config {
	return googleads.Config{
		DeveloperToken: testDeveloperToken,
		ClientID:       testClientID,
		ClientSecret:   testClientSecret,
		RefreshToken:   testRefreshToken,
		CustomerID:     testCustomerID,
		BaseURL:        baseURL,
	}
}

func testProvider(t *testing.T, handler http.HandlerFunc) (*googleads.Provider, *httptest.Server) {
	t.Helper()
	if handler == nil {
		handler = func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/oauth/token") {
				writeToken(w)
				return
			}
			_, _ = io.WriteString(w, `{"results":[{"customer":{"id":"`+testCustomerID+`"}}]}`)
		}
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cfg := validConfig(srv.URL)
	cfg.TokenURL = srv.URL + "/oauth/token"
	p := googleads.NewWithHTTPClient(cfg, srv.Client())
	return p, srv
}

func writeToken(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"access_token":"`+testAccessToken+`","expires_in":3600,"token_type":"Bearer"}`)
}
