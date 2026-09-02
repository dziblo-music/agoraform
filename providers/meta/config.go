package meta

import (
	"os"
	"strings"

	"github.com/dziblo-music/agoraform/providers/meta/client"
)

const (
	// EnvAccessToken is the Meta access token. Automation should prefer a
	// system-user token supplied through a secret manager.
	EnvAccessToken = "META_ACCESS_TOKEN"

	// EnvAdAccountID is the Meta ad account, in numeric or act_<numeric> form.
	EnvAdAccountID = "META_AD_ACCOUNT_ID"
)

// Config is an alias for Meta client configuration.
type Config = client.Config

// ConfigFromEnv loads Meta provider configuration from environment variables.
func ConfigFromEnv() Config {
	return Config{
		AccessToken: strings.TrimSpace(os.Getenv(EnvAccessToken)),
		AdAccountID: strings.TrimSpace(os.Getenv(EnvAdAccountID)),
		Timeout:     client.DefaultTimeout,
	}
}

// Configured reports whether the required runtime values are present.
func Configured(cfg Config) bool {
	return strings.TrimSpace(cfg.AccessToken) != "" && strings.TrimSpace(cfg.AdAccountID) != ""
}

// NormalizeAdAccountID returns the canonical act_<numeric> account ID.
func NormalizeAdAccountID(raw string) (string, error) {
	return client.NormalizeAdAccountID(raw)
}
