package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultTimeout is used when Config.Timeout is unset and no custom
	// HTTP client is provided.
	DefaultTimeout = 30 * time.Second

	maxResponseBody = 1 << 20

	userAgent = "agoraform"
)

// Config holds connection settings for a Matomo instance.
//
// TokenAuth is a secret and must never be written to manifests, plan
// output, logs, or persisted state.
type Config struct {
	BaseURL     string
	TokenAuth   string
	SiteID      string
	ContainerID string
	Timeout     time.Duration
	HTTPClient  *http.Client
}

// WithDefaults returns a copy with a timeout applied when unset.
func (c Config) WithDefaults() Config {
	if c.Timeout <= 0 {
		c.Timeout = DefaultTimeout
	}
	return c
}

// Validate reports missing or malformed connection settings.
//
// The returned error does not include the token or a credential-bearing URL.
func (c Config) Validate() error {
	if strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("matomo: base URL is required")
	}
	if strings.TrimSpace(c.TokenAuth) == "" {
		return fmt.Errorf("matomo: authentication token is required")
	}
	_, err := normalizeEndpoint(c.BaseURL)
	return err
}

// Redacted returns a copy safe for diagnostics.
func (c Config) Redacted() Config {
	out := c
	out.BaseURL = redactedBaseURL(out.BaseURL, out.TokenAuth)
	if out.TokenAuth != "" {
		out.TokenAuth = redacted
	}
	out.HTTPClient = nil
	return out
}

func (c Config) String() string {
	r := c.Redacted()
	return fmt.Sprintf("matomo config url=%s site_id=%s container_id=%s", r.BaseURL, r.SiteID, r.ContainerID)
}

// Client is a reusable Matomo HTTP API client.
//
// Resource implementations should call Call or the Analytics / TagManager
// helpers instead of issuing raw HTTP requests.
type Client struct {
	cfg      Config
	endpoint string
	http     *http.Client
}

// New constructs a Client. cfg.Validate must succeed.
func New(cfg Config) (*Client, error) {
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	endpoint, err := normalizeEndpoint(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	return &Client{
		cfg:      cfg,
		endpoint: endpoint,
		http:     httpClient,
	}, nil
}

// CheckConnection confirms the base URL and token can query Matomo
// without modifying remote state.
func (c *Client) CheckConnection(ctx context.Context) error {
	version, err := c.Analytics().GetMatomoVersion(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(version) == "" {
		return fmt.Errorf("matomo: API.getMatomoVersion returned an empty version")
	}
	return nil
}

// Call invokes a Matomo API method using POST form parameters.
//
// token_auth is sent in the request body, never on the query string, so
// transport errors that echo the URL cannot leak the token. Reserved Matomo
// request fields are owned by the client and cannot be overridden by callers.
func (c *Client) Call(ctx context.Context, method string, params url.Values) (json.RawMessage, error) {
	if c == nil {
		return nil, fmt.Errorf("matomo: client is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(method) == "" {
		return nil, fmt.Errorf("matomo: API method is required")
	}

	form := cloneValues(params)
	form.Set("module", "API")
	form.Set("method", method)
	form.Set("format", "JSON")
	form.Set("token_auth", c.cfg.TokenAuth)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, c.sanitize(method, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, c.sanitize(method, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		return nil, c.sanitize(method, fmt.Errorf("read response: %w", err))
	}
	if len(body) > maxResponseBody {
		return nil, apiError(method, resp.StatusCode, "response exceeded size limit")
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, unauthorizedError(method, resp.StatusCode)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, unexpectedStatusError(method, resp.StatusCode)
	}

	var apiErr struct {
		Result  string `json:"result"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &apiErr); err == nil && strings.EqualFold(apiErr.Result, "error") {
		msg := Redact(strings.TrimSpace(apiErr.Message), c.cfg.TokenAuth)
		if msg == "" {
			msg = "API error"
		}
		return nil, apiError(method, resp.StatusCode, msg)
	}

	if !json.Valid(body) {
		return nil, malformedResponseError(method, resp.StatusCode)
	}
	return json.RawMessage(append([]byte(nil), body...)), nil
}

func (c *Client) sanitize(method string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return fmtSafe(method, "request canceled", context.Canceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmtSafe(method, "request timed out", context.DeadlineExceeded)
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr != nil && urlErr.Timeout() {
			return fmtSafe(method, "request timed out", err)
		}
		return fmtSafe(method, "network error", err)
	}
	return fmtSafe(method, Redact(err.Error(), c.cfg.TokenAuth), err)
}

func normalizeEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("matomo: base URL is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("matomo: malformed base URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("matomo: base URL must use http or https")
	}
	if u.Host == "" {
		return "", fmt.Errorf("matomo: malformed base URL")
	}
	if u.User != nil || u.Query().Get("token_auth") != "" || u.Query().Get("token") != "" {
		return "", fmt.Errorf("matomo: do not put credentials in the base URL")
	}

	u.RawQuery = ""
	u.Fragment = ""
	path := strings.TrimSuffix(u.Path, "/")
	if !strings.HasSuffix(strings.ToLower(path), "index.php") {
		if path == "" {
			path = "/index.php"
		} else {
			path = path + "/index.php"
		}
	}
	u.Path = path
	return u.String(), nil
}

func redactedBaseURL(raw string, secrets ...string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "[redacted-invalid-url]"
	}

	// User info, query parameters, and fragments are unnecessary for
	// diagnostics and may contain credentials or other sensitive values.
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return Redact(u.String(), secrets...)
}

func cloneValues(params url.Values) url.Values {
	out := url.Values{}
	for key, values := range params {
		for _, value := range values {
			out.Add(key, value)
		}
	}
	return out
}
