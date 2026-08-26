package client_test

import (
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/providers/googleads/client"
)

func TestConfigStringRedactsSecrets(t *testing.T) {
	t.Parallel()

	cfg := client.Config{
		DeveloperToken: testDeveloperToken,
		ClientID:       testClientID,
		ClientSecret:   testClientSecret,
		RefreshToken:   testRefreshToken,
		CustomerID:     testCustomerID,
		BaseURL:        "https://user:" + testClientSecret + "@googleads.googleapis.com/v25",
		TokenURL:       "https://oauth.example.com/token?client_secret=" + testClientSecret,
	}

	redacted := cfg.Redacted()
	assertNoSecret(t, cfg.String())
	assertNoSecret(t, redacted.DeveloperToken)
	assertNoSecret(t, redacted.ClientID)
	assertNoSecret(t, redacted.ClientSecret)
	assertNoSecret(t, redacted.RefreshToken)
	assertNoSecret(t, redacted.BaseURL)
	assertNoSecret(t, redacted.TokenURL)
	if !strings.Contains(cfg.String(), testCustomerID) {
		t.Fatalf("String() should keep customer ID: %s", cfg.String())
	}
}

func TestRedact(t *testing.T) {
	t.Parallel()

	in := "Authorization: Bearer " + testAccessToken + " developer-token: " + testDeveloperToken + " refresh_token=" + testRefreshToken
	out := client.Redact(in, testAccessToken, testDeveloperToken, testRefreshToken)
	assertNoSecret(t, out)
	if !strings.Contains(out, "[redacted]") {
		t.Fatalf("redacted = %q, want [redacted]", out)
	}
}

func TestConfigStringHidesMalformedURL(t *testing.T) {
	t.Parallel()

	cfg := client.Config{
		DeveloperToken: testDeveloperToken,
		ClientSecret:   testClientSecret,
		RefreshToken:   testRefreshToken,
		BaseURL:        "://user:" + testClientSecret + "@googleads.googleapis.com",
	}
	got := cfg.String()
	assertNoSecret(t, got)
	if !strings.Contains(got, "[redacted-invalid-url]") {
		t.Fatalf("String() = %q, want invalid URL placeholder", got)
	}
}
