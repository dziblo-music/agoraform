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
// Tags, variables, triggers, containers, and publication call through this
// helper rather than constructing raw HTTP requests. Container selection is
// per helper value so one operation cannot leak into another.
type TagManager struct {
	c           *Client
	containerID string
}

// TagManager returns the Tag Manager API helper.
//
// Operations that omit idContainer use Config.ContainerID when set. Prefer
// ForContainer when the caller has an explicit container identity.
func (c *Client) TagManager() *TagManager {
	return &TagManager{c: c}
}

// ForContainer returns a Tag Manager helper bound to idContainer.
//
// The original helper is not mutated. An empty idContainer falls back to
// Config.ContainerID the same way TagManager does.
func (t *TagManager) ForContainer(idContainer string) *TagManager {
	if t == nil {
		return nil
	}
	out := *t
	out.containerID = strings.TrimSpace(idContainer)
	return &out
}

// ContainerID is the container this helper will send as idContainer when
// callers do not set it themselves.
func (t *TagManager) ContainerID() string {
	if t == nil || t.c == nil {
		return ""
	}
	if id := strings.TrimSpace(t.containerID); id != "" {
		return id
	}
	return strings.TrimSpace(t.c.cfg.ContainerID)
}

// Call invokes a Tag Manager method.
//
// method may be "TagManager.getContainer" or the short name "getContainer".
// Configured idSite and the helper's container id are applied when unset in
// params.
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
	return t.c.Call(ctx, method, t.c.withTagManagerDefaults(params, t.containerID))
}

func (c *Client) withTagManagerDefaults(params url.Values, containerID string) url.Values {
	out := c.withSiteID(params)
	id := strings.TrimSpace(containerID)
	if id == "" {
		id = strings.TrimSpace(c.cfg.ContainerID)
	}
	if id != "" && out.Get("idContainer") == "" {
		out.Set("idContainer", id)
	}
	return out
}
