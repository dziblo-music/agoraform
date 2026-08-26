package client

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config holds connection settings for the Google Ads API.
//
// DeveloperToken, ClientSecret, RefreshToken, and any derived access
// token are secrets and must never be written to manifests, plan output,
// logs, or persisted state. Customer IDs are not secrets.
type Config struct {
	DeveloperToken  string
	ClientID        string
	ClientSecret    string
	RefreshToken    string
	CustomerID      string
	LoginCustomerID string
	BaseURL         string
	TokenURL        string
	Timeout         time.Duration
	HTTPClient      *http.Client
}

// WithDefaults returns a copy with timeout and endpoint defaults applied.
func (c Config) WithDefaults() Config {
	c.DeveloperToken = strings.TrimSpace(c.DeveloperToken)
	c.ClientID = strings.TrimSpace(c.ClientID)
	c.ClientSecret = strings.TrimSpace(c.ClientSecret)
	c.RefreshToken = strings.TrimSpace(c.RefreshToken)
	c.CustomerID = strings.TrimSpace(c.CustomerID)
	c.LoginCustomerID = strings.TrimSpace(c.LoginCustomerID)
	c.BaseURL = strings.TrimSpace(c.BaseURL)
	c.TokenURL = strings.TrimSpace(c.TokenURL)
	if c.Timeout <= 0 {
		c.Timeout = DefaultTimeout
	}
	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	if c.TokenURL == "" {
		c.TokenURL = DefaultTokenURL
	}
	return c
}

// Validate reports missing or malformed connection settings.
//
// The returned error does not include credentials or credential-bearing URLs.
func (c Config) Validate() error {
	c = c.WithDefaults()
	if c.DeveloperToken == "" {
		return fmt.Errorf("googleads: developer token is required")
	}
	if c.ClientID == "" {
		return fmt.Errorf("googleads: OAuth client ID is required")
	}
	if c.ClientSecret == "" {
		return fmt.Errorf("googleads: OAuth client secret is required")
	}
	if c.RefreshToken == "" {
		return fmt.Errorf("googleads: OAuth refresh token is required")
	}
	if _, err := NormalizeCustomerID(c.CustomerID); err != nil {
		return err
	}
	if c.LoginCustomerID != "" {
		if _, err := NormalizeCustomerID(c.LoginCustomerID); err != nil {
			return fmt.Errorf("googleads: login customer ID must be a 10-digit identifier")
		}
	}
	if _, err := normalizeEndpoint(c.BaseURL, "API base URL"); err != nil {
		return err
	}
	if _, err := normalizeEndpoint(c.TokenURL, "OAuth token URL"); err != nil {
		return err
	}
	return nil
}

// Redacted returns a copy safe for diagnostics.
func (c Config) Redacted() Config {
	out := c
	out.BaseURL = redactedURL(out.BaseURL, out.secrets()...)
	out.TokenURL = redactedURL(out.TokenURL, out.secrets()...)
	if out.DeveloperToken != "" {
		out.DeveloperToken = redacted
	}
	if out.ClientSecret != "" {
		out.ClientSecret = redacted
	}
	if out.RefreshToken != "" {
		out.RefreshToken = redacted
	}
	if out.ClientID != "" {
		out.ClientID = redacted
	}
	out.HTTPClient = nil
	return out
}

func (c Config) String() string {
	r := c.Redacted()
	return fmt.Sprintf("googleads config customer_id=%s login_customer_id=%s api=%s", r.CustomerID, r.LoginCustomerID, r.BaseURL)
}

func (c Config) secrets() []string {
	return []string{c.DeveloperToken, c.ClientID, c.ClientSecret, c.RefreshToken}
}

func (c Config) normalizedCustomerID() (string, error) {
	return NormalizeCustomerID(c.CustomerID)
}

func (c Config) normalizedLoginCustomerID() (string, error) {
	if strings.TrimSpace(c.LoginCustomerID) == "" {
		return "", nil
	}
	return NormalizeCustomerID(c.LoginCustomerID)
}

func normalizeEndpoint(raw, label string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("googleads: %s is required", label)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("googleads: malformed %s", label)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("googleads: %s must use http or https", label)
	}
	if u.Host == "" {
		return "", fmt.Errorf("googleads: malformed %s", label)
	}
	if u.User != nil {
		return "", fmt.Errorf("googleads: do not put credentials in the %s", label)
	}
	u.Fragment = ""
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
