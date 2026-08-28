package matomo

import (
	"os"
	"strings"

	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

const (
	// EnvURL is the Matomo instance base URL, for example
	// https://matomo.example.com
	EnvURL = "MATOMO_URL"

	// EnvTokenAuth is the Matomo API token. It must never be committed
	// to Git or written to manifests, plans, or logs.
	EnvTokenAuth = "MATOMO_TOKEN_AUTH"

	// EnvSiteID is an optional default Matomo site identifier.
	EnvSiteID = "MATOMO_SITE_ID"

	// EnvContainerID selects an existing Tag Manager container when no
	// matomo.container resource is declared.
	EnvContainerID = "MATOMO_CONTAINER_ID"
)

// Config is an alias for the Matomo client configuration.
type Config = client.Config

// ConfigFromEnv loads provider configuration from the process environment.
//
// Environment variables are the only supported secret source. Manifests
// must not contain tokens. Values passed to New override environment
// values because they are supplied explicitly by the caller.
func ConfigFromEnv() Config {
	return Config{
		BaseURL:     strings.TrimSpace(os.Getenv(EnvURL)),
		TokenAuth:   strings.TrimSpace(os.Getenv(EnvTokenAuth)),
		SiteID:      strings.TrimSpace(os.Getenv(EnvSiteID)),
		ContainerID: strings.TrimSpace(os.Getenv(EnvContainerID)),
		Timeout:     client.DefaultTimeout,
	}
}

// Configured reports whether a base URL and token are present.
func Configured(cfg Config) bool {
	return strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.TokenAuth) != ""
}
