package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client is a reusable Google Ads REST client.
//
// Resource implementations should call Query and Mutate instead of issuing
// raw HTTP requests. API version selection is centralized on Version.
type Client struct {
	cfg      Config
	baseURL  string
	tokenURL string
	http     *http.Client

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
}

// New constructs a Client. cfg.Validate must succeed.
func New(cfg Config) (*Client, error) {
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	customerID, err := cfg.normalizedCustomerID()
	if err != nil {
		return nil, err
	}
	loginID, err := cfg.normalizedLoginCustomerID()
	if err != nil {
		return nil, err
	}
	cfg.CustomerID = customerID
	cfg.LoginCustomerID = loginID

	baseURL, err := normalizeEndpoint(cfg.BaseURL, "API base URL")
	if err != nil {
		return nil, err
	}
	tokenURL, err := normalizeEndpoint(cfg.TokenURL, "OAuth token URL")
	if err != nil {
		return nil, err
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	return &Client{
		cfg:      cfg,
		baseURL:  baseURL,
		tokenURL: tokenURL,
		http:     httpClient,
	}, nil
}

// CustomerID returns the normalized configured customer ID.
func (c *Client) CustomerID() string {
	if c == nil {
		return ""
	}
	return c.cfg.CustomerID
}

// LoginCustomerID returns the normalized manager customer ID, if set.
func (c *Client) LoginCustomerID() string {
	if c == nil {
		return ""
	}
	return c.cfg.LoginCustomerID
}

// CheckConnection confirms OAuth credentials, the developer token, and the
// configured customer can be queried without modifying remote state.
func (c *Client) CheckConnection(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("googleads: client is nil")
	}
	results, err := c.Query(ctx, "SELECT customer.id FROM customer LIMIT 1")
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return fmt.Errorf("googleads: configured customer %s was not returned by the API", c.cfg.CustomerID)
	}
	return nil
}

func (c *Client) doJSON(ctx context.Context, operation, method, path string, payload any) (json.RawMessage, error) {
	if c == nil {
		return nil, fmt.Errorf("googleads: client is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, fmtSafe(operation, "could not encode request", err)
		}
		body = bytes.NewReader(encoded)
	}

	endpoint := c.baseURL + "/" + Version + "/" + strings.TrimPrefix(path, "/")
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, c.sanitize(operation, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("developer-token", c.cfg.DeveloperToken)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cfg.LoginCustomerID != "" {
		req.Header.Set("login-customer-id", c.cfg.LoginCustomerID)
	}

	if err := ctx.Err(); err != nil {
		return nil, c.sanitize(operation, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, c.sanitize(operation, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		return nil, responseReadError(operation, resp.StatusCode)
	}
	if len(raw) > maxResponseBody {
		return nil, responseTooLargeError(operation, resp.StatusCode)
	}

	requestID := strings.TrimSpace(resp.Header.Get("request-id"))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseAPIError(operation, resp.StatusCode, requestID, raw, append(c.secrets(), token))
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return json.RawMessage([]byte("{}")), nil
	}
	if !json.Valid(raw) {
		return nil, malformedResponseError(operation, resp.StatusCode)
	}
	return json.RawMessage(append([]byte(nil), raw...)), nil
}

func (c *Client) sanitize(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return fmtSafe(operation, "request canceled", context.Canceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmtSafe(operation, "request timed out", context.DeadlineExceeded)
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr != nil && urlErr.Timeout() {
			return fmtSafe(operation, "request timed out", err)
		}
		return fmtSafe(operation, "network error", err)
	}
	return fmtSafe(operation, Redact(err.Error(), append(c.secrets(), c.token)...), err)
}

func (c *Client) secrets() []string {
	return []string{
		c.cfg.DeveloperToken,
		c.cfg.ClientID,
		c.cfg.ClientSecret,
		c.cfg.RefreshToken,
		c.token,
	}
}

func (c *Client) resolveCustomerID(customerID string) (string, error) {
	if strings.TrimSpace(customerID) == "" {
		if c == nil {
			return "", fmt.Errorf("googleads: customer ID is required")
		}
		return c.cfg.CustomerID, nil
	}
	return NormalizeCustomerID(customerID)
}
