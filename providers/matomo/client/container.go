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
	Releases     []ContainerRelease
}

// ContainerRelease is a published container version in one environment.
type ContainerRelease struct {
	IDContainerVersion string
	Environment        string
}

type rawContainer struct {
	IDContainer flexibleString  `json:"idcontainer"`
	IDSite      flexibleString  `json:"idsite"`
	Name        flexibleString  `json:"name"`
	Draft       *rawDraft       `json:"draft"`
	Releases    json.RawMessage `json:"releases"`
}

type rawDraft struct {
	IDContainerVersion flexibleString `json:"idcontainerversion"`
}

type rawRelease struct {
	IDContainerVersion flexibleString `json:"idcontainerversion"`
	Environment        flexibleString `json:"environment"`
}

func (c rawContainer) container() (Container, error) {
	out := Container{
		IDContainer: string(c.IDContainer),
		IDSite:      string(c.IDSite),
		Name:        string(c.Name),
	}
	if c.Draft != nil {
		out.DraftVersion = string(c.Draft.IDContainerVersion)
	}
	releases, err := decodeReleases(c.Releases)
	if err != nil {
		return Container{}, err
	}
	out.Releases = releases
	return out, nil
}

// ReleaseFor returns the published version for environment, if any.
func (c Container) ReleaseFor(environment string) (ContainerRelease, bool) {
	environment = strings.TrimSpace(environment)
	for _, rel := range c.Releases {
		if rel.Environment == environment && strings.TrimSpace(rel.IDContainerVersion) != "" {
			return rel, true
		}
	}
	return ContainerRelease{}, false
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
	c, err := item.container()
	if err != nil {
		return Container{}, err
	}
	if strings.TrimSpace(c.IDContainer) == "" && strings.TrimSpace(c.DraftVersion) == "" {
		return Container{}, fmt.Errorf("unexpected TagManager.getContainer payload")
	}
	return c, nil
}

func decodeReleases(raw json.RawMessage) ([]ContainerRelease, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if raw[0] != '[' {
		return nil, fmt.Errorf("unexpected TagManager.getContainer releases payload")
	}
	var items []rawRelease
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	out := make([]ContainerRelease, 0, len(items))
	for _, item := range items {
		rel := ContainerRelease{
			IDContainerVersion: strings.TrimSpace(string(item.IDContainerVersion)),
			Environment:        strings.TrimSpace(string(item.Environment)),
		}
		if rel.IDContainerVersion == "" && rel.Environment == "" {
			continue
		}
		out = append(out, rel)
	}
	return out, nil
}
