package googleads

import (
	"os"
	"strings"

	"github.com/dziblo-music/agoraform/providers/googleads/client"
)

const (
	// EnvDeveloperToken is the Google Ads API developer token.
	EnvDeveloperToken = "GOOGLE_ADS_DEVELOPER_TOKEN"

	// EnvClientID is the OAuth 2.0 client ID.
	EnvClientID = "GOOGLE_ADS_CLIENT_ID"

	// EnvClientSecret is the OAuth 2.0 client secret. It must never be
	// committed to Git or written to manifests, plans, or logs.
	EnvClientSecret = "GOOGLE_ADS_CLIENT_SECRET"

	// EnvRefreshToken is the OAuth 2.0 refresh token. Interactive OAuth
	// flows are not implemented; supply a previously issued token.
	EnvRefreshToken = "GOOGLE_ADS_REFRESH_TOKEN"

	// EnvCustomerID is the Google Ads customer ID to operate on.
	EnvCustomerID = "GOOGLE_ADS_CUSTOMER_ID"

	// EnvLoginCustomerID is an optional manager-account customer ID sent
	// as the login-customer-id header.
	EnvLoginCustomerID = "GOOGLE_ADS_LOGIN_CUSTOMER_ID"
)

// Config is an alias for the Google Ads client configuration.
type Config = client.Config

// ConfigFromEnv loads provider configuration from the process environment.
//
// Environment variables are the only supported secret source. Manifests
// must not contain tokens. Values passed to New override environment
// values because they are supplied explicitly by the caller.
func ConfigFromEnv() Config {
	return Config{
		DeveloperToken:  strings.TrimSpace(os.Getenv(EnvDeveloperToken)),
		ClientID:        strings.TrimSpace(os.Getenv(EnvClientID)),
		ClientSecret:    strings.TrimSpace(os.Getenv(EnvClientSecret)),
		RefreshToken:    strings.TrimSpace(os.Getenv(EnvRefreshToken)),
		CustomerID:      strings.TrimSpace(os.Getenv(EnvCustomerID)),
		LoginCustomerID: strings.TrimSpace(os.Getenv(EnvLoginCustomerID)),
		Timeout:         client.DefaultTimeout,
	}
}

// Configured reports whether the credentials required for API access are present.
func Configured(cfg Config) bool {
	return strings.TrimSpace(cfg.DeveloperToken) != "" &&
		strings.TrimSpace(cfg.ClientID) != "" &&
		strings.TrimSpace(cfg.ClientSecret) != "" &&
		strings.TrimSpace(cfg.RefreshToken) != "" &&
		strings.TrimSpace(cfg.CustomerID) != ""
}

// NormalizeCustomerID returns the canonical 10-digit Google Ads customer ID.
func NormalizeCustomerID(raw string) (string, error) {
	return client.NormalizeCustomerID(raw)
}
