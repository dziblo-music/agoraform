package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Container is a Matomo Tag Manager container as returned by
// TagManager.getContainer.
type Container struct {
	IDContainer  string
	IDSite       string
	Name         string
	DraftVersion string
}

type rawContainer struct {
	IDContainer flexibleString `json:"idcontainer"`
	IDSite      flexibleString `json:"idsite"`
	Name        flexibleString `json:"name"`
	Draft       *rawDraft      `json:"draft"`
}

type rawDraft struct {
	IDContainerVersion flexibleString `json:"idcontainerversion"`
}

func (c rawContainer) container() Container {
	out := Container{
		IDContainer: string(c.IDContainer),
		IDSite:      string(c.IDSite),
		Name:        string(c.Name),
	}
	if c.Draft != nil {
		out.DraftVersion = string(c.Draft.IDContainerVersion)
	}
	return out
}

// GetContainer returns the configured Tag Manager container.
func (t *TagManager) GetContainer(ctx context.Context) (Container, error) {
	if t == nil || t.c == nil {
		return Container{}, fmt.Errorf("matomo: tag manager client is nil")
	}
	raw, err := t.Call(ctx, "getContainer", nil)
	if err != nil {
		return Container{}, err
	}
	container, err := decodeContainer(raw)
	if err != nil {
		return Container{}, malformedResponseError("TagManager.getContainer", 0)
	}
	return container, nil
}

// DraftVersion returns the draft container version id for Tag Manager
// resource operations.
func (t *TagManager) DraftVersion(ctx context.Context) (string, error) {
	container, err := t.GetContainer(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(container.DraftVersion) == "" {
		return "", fmt.Errorf("matomo: TagManager.getContainer returned no draft version")
	}
	return container.DraftVersion, nil
}

func decodeContainer(raw json.RawMessage) (Container, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "false" {
		return Container{}, fmt.Errorf("container not found")
	}
	if raw[0] != '{' {
		return Container{}, fmt.Errorf("unexpected TagManager.getContainer payload")
	}
	var item rawContainer
	if err := json.Unmarshal(raw, &item); err != nil {
		return Container{}, err
	}
	c := item.container()
	if strings.TrimSpace(c.IDContainer) == "" && strings.TrimSpace(c.DraftVersion) == "" {
		return Container{}, fmt.Errorf("unexpected TagManager.getContainer payload")
	}
	return c, nil
}
