package client

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var adAccountIDPattern = regexp.MustCompile(`^[1-9][0-9]*$`)

// Config holds connection settings for the Meta Graph and Marketing APIs.
// AccessToken is a secret and must never be written to manifests, plans,
// logs, diagnostics, or state. AdAccountID is not a secret.
type Config struct {
	AccessToken string
	AdAccountID string
	BaseURL     string
	Timeout     time.Duration
	HTTPClient  *http.Client
}

// WithDefaults returns a normalized copy with endpoint and timeout defaults.
func (c Config) WithDefaults() Config {
	c.AccessToken = strings.TrimSpace(c.AccessToken)
	c.AdAccountID = strings.TrimSpace(c.AdAccountID)
	c.BaseURL = strings.TrimSpace(c.BaseURL)
	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	if c.Timeout <= 0 {
		c.Timeout = DefaultTimeout
	}
	return c
}

// Validate reports missing or malformed connection settings without
// including the access token in its error.
func (c Config) Validate() error {
	c = c.WithDefaults()
	if c.AccessToken == "" {
		return fmt.Errorf("meta: access token is required")
	}
	if _, err := NormalizeAdAccountID(c.AdAccountID); err != nil {
		return err
	}
	if _, err := normalizeBaseURL(c.BaseURL); err != nil {
		return err
	}
	return nil
}

// Redacted returns a copy safe for user-visible diagnostics.
func (c Config) Redacted() Config {
	out := c
	if out.AccessToken != "" {
		out.AccessToken = redacted
	}
	out.BaseURL = redactedURL(out.BaseURL, c.AccessToken)
	out.HTTPClient = nil
	return out
}

func (c Config) String() string {
	r := c.Redacted()
	return fmt.Sprintf("meta config ad_account_id=%s api=%s version=%s", r.AdAccountID, r.BaseURL, Version)
}

// NormalizeAdAccountID accepts numeric and act_<numeric> forms and returns
// the canonical act_<numeric> form used in Graph API paths.
func NormalizeAdAccountID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if strings.HasPrefix(id, "act_") {
		id = strings.TrimPrefix(id, "act_")
	}
	if !adAccountIDPattern.MatchString(id) {
		return "", fmt.Errorf("meta: ad account ID must be a numeric identifier, optionally prefixed with act_")
	}
	return "act_" + id, nil
}

func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("meta: malformed API base URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("meta: API base URL must use http or https")
	}
	if u.User != nil {
		return "", fmt.Errorf("meta: do not put credentials in the API base URL")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("meta: API base URL must not include a query or fragment")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func redactedURL(raw string, secrets ...string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "[redacted-invalid-url]"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return Redact(u.String(), secrets...)
}
