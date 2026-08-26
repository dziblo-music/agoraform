package client

import "time"

const (
	// Version is the Google Ads REST API version used by every request.
	// Change this constant to upgrade; resource packages must not hardcode it.
	Version = "v25"

	// DefaultBaseURL is the Google Ads REST host.
	DefaultBaseURL = "https://googleads.googleapis.com"

	// DefaultTokenURL is the OAuth 2.0 token endpoint used to exchange a
	// refresh token for an access token. Interactive OAuth flows are not
	// implemented.
	DefaultTokenURL = "https://oauth2.googleapis.com/token"

	// DefaultTimeout is used when Config.Timeout is unset and no custom
	// HTTP client is provided.
	DefaultTimeout = 30 * time.Second

	// Scope is the OAuth scope required for Google Ads API access.
	Scope = "https://www.googleapis.com/auth/adwords"

	maxResponseBody = 1 << 20
	maxQueryPages   = 100
	tokenSkew       = time.Minute

	userAgent = "agoraform"
)
