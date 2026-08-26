package googleads_test

import (
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/providers/googleads"
	"github.com/dziblo-music/agoraform/providers/googleads/client"
)

func TestConfigFromEnv(t *testing.T) {
	t.Setenv(googleads.EnvDeveloperToken, " "+testDeveloperToken+" ")
	t.Setenv(googleads.EnvClientID, " "+testClientID+" ")
	t.Setenv(googleads.EnvClientSecret, " "+testClientSecret+" ")
	t.Setenv(googleads.EnvRefreshToken, " "+testRefreshToken+" ")
	t.Setenv(googleads.EnvCustomerID, " 123-456-7890 ")
	t.Setenv(googleads.EnvLoginCustomerID, " 098-765-4321 ")

	cfg := googleads.ConfigFromEnv()
	if cfg.DeveloperToken != testDeveloperToken {
		t.Fatalf("DeveloperToken = %q", cfg.DeveloperToken)
	}
	if cfg.ClientID != testClientID {
		t.Fatalf("ClientID = %q", cfg.ClientID)
	}
	if cfg.ClientSecret != testClientSecret {
		t.Fatalf("ClientSecret = %q", cfg.ClientSecret)
	}
	if cfg.RefreshToken != testRefreshToken {
		t.Fatalf("RefreshToken = %q", cfg.RefreshToken)
	}
	if cfg.CustomerID != "123-456-7890" {
		t.Fatalf("CustomerID = %q", cfg.CustomerID)
	}
	if cfg.LoginCustomerID != "098-765-4321" {
		t.Fatalf("LoginCustomerID = %q", cfg.LoginCustomerID)
	}
	if cfg.Timeout != client.DefaultTimeout {
		t.Fatalf("Timeout = %s, want default", cfg.Timeout)
	}
	if !googleads.Configured(cfg) {
		t.Fatal("expected Configured")
	}
}

func TestConfiguredFalseWhenIncomplete(t *testing.T) {
	if googleads.Configured(googleads.Config{}) {
		t.Fatal("empty config should not be configured")
	}
	if googleads.Configured(googleads.Config{
		DeveloperToken: testDeveloperToken,
		ClientID:       testClientID,
		ClientSecret:   testClientSecret,
		RefreshToken:   testRefreshToken,
	}) {
		t.Fatal("customer-less config should not be configured")
	}
}

func TestConfigFromEnvEmpty(t *testing.T) {
	t.Setenv(googleads.EnvDeveloperToken, "")
	t.Setenv(googleads.EnvClientID, "")
	t.Setenv(googleads.EnvClientSecret, "")
	t.Setenv(googleads.EnvRefreshToken, "")
	t.Setenv(googleads.EnvCustomerID, "")
	t.Setenv(googleads.EnvLoginCustomerID, "")

	cfg := googleads.ConfigFromEnv()
	if googleads.Configured(cfg) {
		t.Fatal("empty env should not be configured")
	}
	if strings.Contains(cfg.String(), testDeveloperToken) {
		t.Fatalf("String leaked a token: %s", cfg.String())
	}
}

func TestNormalizeCustomerID(t *testing.T) {
	got, err := googleads.NormalizeCustomerID("123-456-7890")
	if err != nil {
		t.Fatal(err)
	}
	if got != "1234567890" {
		t.Fatalf("got %q", got)
	}
}
