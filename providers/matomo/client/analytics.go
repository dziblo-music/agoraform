package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Analytics is the Matomo analytics and management API surface.
//
// Goal resources call through this helper rather than constructing raw
// HTTP requests.
type Analytics struct {
	c *Client
}

// Analytics returns the analytics/management API helper.
func (c *Client) Analytics() *Analytics {
	return &Analytics{c: c}
}

// Call invokes a Matomo analytics/management method.
//
// If Config.SiteID is set and params does not already include idSite,
// the configured site is added.
func (a *Analytics) Call(ctx context.Context, method string, params url.Values) (json.RawMessage, error) {
	if a == nil || a.c == nil {
		return nil, fmt.Errorf("matomo: analytics client is nil")
	}
	return a.c.Call(ctx, method, a.c.withSiteID(params))
}

// GetMatomoVersion performs the non-mutating API.getMatomoVersion call
// used for connection and authentication checks.
func (a *Analytics) GetMatomoVersion(ctx context.Context) (string, error) {
	raw, err := a.Call(ctx, "API.getMatomoVersion", nil)
	if err != nil {
		return "", err
	}

	var version string
	if err := json.Unmarshal(raw, &version); err == nil {
		return strings.TrimSpace(version), nil
	}

	var wrapped struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && strings.TrimSpace(wrapped.Value) != "" {
		return strings.TrimSpace(wrapped.Value), nil
	}

	return "", fmt.Errorf("matomo: unexpected API.getMatomoVersion payload")
}

func (c *Client) withSiteID(params url.Values) url.Values {
	out := cloneValues(params)
	if c.cfg.SiteID != "" && out.Get("idSite") == "" {
		out.Set("idSite", c.cfg.SiteID)
	}
	return out
}
