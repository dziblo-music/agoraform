package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// TagManager is the Matomo Tag Manager API surface.
//
// Tags, variables, and triggers call through this helper rather than
// constructing raw HTTP requests. Versions remain follow-up work.
type TagManager struct {
	c *Client
}

// TagManager returns the Tag Manager API helper.
func (c *Client) TagManager() *TagManager {
	return &TagManager{c: c}
}

// Call invokes a Tag Manager method.
//
// method may be "TagManager.getContainer" or the short name "getContainer".
// Configured idSite and idContainer values are applied when unset in params.
func (t *TagManager) Call(ctx context.Context, method string, params url.Values) (json.RawMessage, error) {
	if t == nil || t.c == nil {
		return nil, fmt.Errorf("matomo: tag manager client is nil")
	}
	method = strings.TrimSpace(method)
	if method == "" {
		return nil, fmt.Errorf("matomo: Tag Manager method is required")
	}
	if !strings.HasPrefix(method, "TagManager.") {
		method = "TagManager." + method
	}
	return t.c.Call(ctx, method, t.c.withTagManagerDefaults(params))
}

func (c *Client) withTagManagerDefaults(params url.Values) url.Values {
	out := c.withSiteID(params)
	if c.cfg.ContainerID != "" && out.Get("idContainer") == "" {
		out.Set("idContainer", c.cfg.ContainerID)
	}
	return out
}
