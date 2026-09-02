package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client is a reusable, version-pinned Meta Graph and Marketing API client.
// It does not automatically retry requests; callers must make any retry of a
// mutation explicit and safe for that resource operation.
type Client struct {
	cfg         Config
	baseURL     string
	http        *http.Client
	adAccountID string
}

// New constructs a Client after validating and normalizing cfg.
func New(cfg Config) (*Client, error) {
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	baseURL, err := normalizeBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	accountID, err := NormalizeAdAccountID(cfg.AdAccountID)
	if err != nil {
		return nil, err
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	return &Client{cfg: cfg, baseURL: baseURL, http: httpClient, adAccountID: accountID}, nil
}

// AdAccountID returns the normalized act_<numeric> account identifier.
func (c *Client) AdAccountID() string {
	if c == nil {
		return ""
	}
	return c.adAccountID
}

// Get performs a versioned GET and decodes the JSON response into out.
func (c *Client) Get(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, query, nil, out)
}

// Post performs a versioned form-encoded POST and decodes its JSON response.
func (c *Client) Post(ctx context.Context, path string, form url.Values, out any) error {
	return c.do(ctx, http.MethodPost, path, nil, form, out)
}

// Delete performs a versioned form-encoded DELETE and decodes its JSON response.
func (c *Client) Delete(ctx context.Context, path string, form url.Values, out any) error {
	return c.do(ctx, http.MethodDelete, path, nil, form, out)
}

// List follows Meta cursor pagination and returns each element of every data
// page as raw JSON for resource-specific decoding. Pagination follows only
// the opaque after cursor, never the server-provided next URL.
func (c *Client) List(ctx context.Context, path string, query url.Values) ([]json.RawMessage, error) {
	q := cloneValues(query)
	var all []json.RawMessage
	for pageNumber := 0; pageNumber < maxPages; pageNumber++ {
		var page struct {
			Data   []json.RawMessage `json:"data"`
			Paging struct {
				Cursors struct {
					After string `json:"after"`
				} `json:"cursors"`
				Next string `json:"next"`
			} `json:"paging"`
		}
		if err := c.Get(ctx, path, q, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Data...)
		after := strings.TrimSpace(page.Paging.Cursors.After)
		if page.Paging.Next == "" {
			return all, nil
		}
		if after == "" {
			return nil, &Error{Operation: "GET " + safePath(path), Message: "malformed pagination response: next page has no after cursor"}
		}
		q.Set("after", after)
	}
	return nil, &Error{Operation: "GET " + safePath(path), Message: fmt.Sprintf("pagination exceeded %d pages", maxPages)}
}

func (c *Client) do(ctx context.Context, method, path string, query, form url.Values, out any) error {
	if c == nil {
		return fmt.Errorf("meta: client is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	cleanPath, err := normalizePath(path)
	if err != nil {
		return err
	}
	endpoint := c.baseURL + "/" + Version + "/" + cleanPath
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	operation := Redact(method+" "+cleanPath, c.cfg.AccessToken)
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return transportError(operation, err, c.cfg.AccessToken)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	req.Header.Set("User-Agent", userAgent)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return transportError(operation, err, c.cfg.AccessToken)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		return &Error{Operation: operation, StatusCode: resp.StatusCode, Message: "response could not be read", Transient: true}
	}
	if len(raw) > maxResponseBody {
		return &Error{Operation: operation, StatusCode: resp.StatusCode, Message: "response exceeded size limit"}
	}
	requestID := firstHeader(resp.Header, "x-fb-request-id")
	traceID := firstHeader(resp.Header, "x-fb-trace-id")
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseAPIError(operation, resp.StatusCode, requestID, traceID, raw, c.cfg.AccessToken)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte("{}")
	}
	if !json.Valid(raw) {
		return &Error{Operation: operation, StatusCode: resp.StatusCode, Message: "malformed JSON response"}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &Error{Operation: operation, StatusCode: resp.StatusCode, Message: "could not decode JSON response"}
	}
	return nil
}

func normalizePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return "", fmt.Errorf("meta: API path is required")
	}
	if strings.Contains(path, "?") || strings.Contains(path, "#") || strings.Contains(path, "://") {
		return "", fmt.Errorf("meta: API path must be relative and must not include a query or fragment")
	}
	return path, nil
}

func safePath(path string) string {
	clean, err := normalizePath(path)
	if err != nil {
		return "request"
	}
	return clean
}

func cloneValues(in url.Values) url.Values {
	out := make(url.Values, len(in))
	for key, values := range in {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func firstHeader(header http.Header, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(header.Get(name)); value != "" {
			return value
		}
	}
	return ""
}
