package client

import "time"

const (
	// Version is the Meta Graph and Marketing API version used by every
	// request. Version upgrades are deliberate code changes so they can be
	// reviewed and tested with the provider resources that depend on them.
	Version = "v26.0"

	// DefaultBaseURL is the Meta Graph API host.
	DefaultBaseURL = "https://graph.facebook.com"

	// DefaultTimeout bounds each API request when Config.Timeout is unset.
	DefaultTimeout = 30 * time.Second

	maxResponseBody = 4 << 20
	maxPages        = 100
	userAgent       = "agoraform"
)
