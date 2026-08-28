package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ErrContainerNotFound reports that a Tag Manager container is absent.
var ErrContainerNotFound = errors.New("container not found")

// Container is a Matomo Tag Manager container as returned by
// TagManager.getContainer.
type Container struct {
	IDContainer                        string
	IDSite                             string
	Name                               string
	Context                            string
	Description                        string
	Status                             string
	DraftVersion                       string
	Releases                           []ContainerRelease
	IgnoreGtmDataLayer                 string
	IsTagFireLimitAllowedInPreviewMode string
	ActivelySyncGtmDataLayer           string
}

// ContainerRelease is a published container version in one environment.
type ContainerRelease struct {
	IDContainerVersion string
	Environment        string
}

// ContainerInput is the configurable subset sent to TagManager.addContainer
// and TagManager.updateContainer.
type ContainerInput struct {
	Context     string
	Name        string
	Description string
}

// ContainerPreservedFields is the unmanaged portion of a Tag Manager
// container that must be carried forward on update. Matomo's update API
// replaces omitted flag parameters with defaults.
type ContainerPreservedFields struct {
	IgnoreGtmDataLayer                 string
	IsTagFireLimitAllowedInPreviewMode string
	ActivelySyncGtmDataLayer           string
}

type rawContainer struct {
	IDContainer                        flexibleString  `json:"idcontainer"`
	IDSite                             flexibleString  `json:"idsite"`
	Name                               flexibleString  `json:"name"`
	Context                            flexibleString  `json:"context"`
	Description                        flexibleString  `json:"description"`
	Status                             flexibleString  `json:"status"`
	Draft                              *rawDraft       `json:"draft"`
	Releases                           json.RawMessage `json:"releases"`
	IgnoreGtmDataLayer                 flexibleString  `json:"ignoreGtmDataLayer"`
	IsTagFireLimitAllowedInPreviewMode flexibleString  `json:"isTagFireLimitAllowedInPreviewMode"`
	ActivelySyncGtmDataLayer           flexibleString  `json:"activelySyncGtmDataLayer"`
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
		IDContainer:                        string(c.IDContainer),
		IDSite:                             string(c.IDSite),
		Name:                               string(c.Name),
		Context:                            string(c.Context),
		Description:                        string(c.Description),
		Status:                             string(c.Status),
		IgnoreGtmDataLayer:                 string(c.IgnoreGtmDataLayer),
		IsTagFireLimitAllowedInPreviewMode: string(c.IsTagFireLimitAllowedInPreviewMode),
		ActivelySyncGtmDataLayer:           string(c.ActivelySyncGtmDataLayer),
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

// GetContainers lists Tag Manager containers for the configured site.
//
// This is a site-level listing and does not send idContainer.
func (t *TagManager) GetContainers(ctx context.Context) ([]Container, error) {
	if t == nil || t.c == nil {
		return nil, fmt.Errorf("matomo: tag manager client is nil")
	}
	raw, err := t.c.Call(ctx, "TagManager.getContainers", t.c.withSiteID(nil))
	if err != nil {
		return nil, err
	}
	containers, err := decodeContainers(raw)
	if err != nil {
		return nil, malformedResponseError("TagManager.getContainers", 0)
	}
	return containers, nil
}

// GetContainer returns the Tag Manager container selected by this helper.
func (t *TagManager) GetContainer(ctx context.Context) (Container, error) {
	if t == nil || t.c == nil {
		return Container{}, fmt.Errorf("matomo: tag manager client is nil")
	}
	raw, err := t.Call(ctx, "getContainer", nil)
	if err != nil {
		if isContainerNotFoundAPIError(err) {
			return Container{}, ErrContainerNotFound
		}
		return Container{}, err
	}
	container, err := decodeContainer(raw)
	if err != nil {
		if errors.Is(err, ErrContainerNotFound) {
			return Container{}, ErrContainerNotFound
		}
		return Container{}, malformedResponseError("TagManager.getContainer", 0)
	}
	return container, nil
}

// AddContainer creates a Tag Manager container and returns its
// provider-native id.
func (t *TagManager) AddContainer(ctx context.Context, in ContainerInput) (string, error) {
	if t == nil || t.c == nil {
		return "", fmt.Errorf("matomo: tag manager client is nil")
	}
	in.Context = strings.TrimSpace(in.Context)
	in.Name = strings.TrimSpace(in.Name)
	if in.Context == "" {
		return "", fmt.Errorf("matomo: container context is required")
	}
	if in.Name == "" {
		return "", fmt.Errorf("matomo: container name is required")
	}
	params := url.Values{}
	params.Set("context", in.Context)
	params.Set("name", in.Name)
	if strings.TrimSpace(in.Description) != "" {
		params.Set("description", in.Description)
	}
	raw, err := t.c.Call(ctx, "TagManager.addContainer", t.c.withSiteID(params))
	if err != nil {
		return "", err
	}
	id, err := decodeContainerID(raw)
	if err != nil {
		return "", err
	}
	return id, nil
}

// UpdateContainer updates a Tag Manager container's supported fields.
func (t *TagManager) UpdateContainer(ctx context.Context, idContainer string, in ContainerInput, preserved ContainerPreservedFields) error {
	if t == nil || t.c == nil {
		return fmt.Errorf("matomo: tag manager client is nil")
	}
	idContainer = strings.TrimSpace(idContainer)
	if idContainer == "" {
		return fmt.Errorf("matomo: idContainer is required")
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return fmt.Errorf("matomo: container name is required")
	}
	params := url.Values{}
	params.Set("idContainer", idContainer)
	params.Set("name", in.Name)
	params.Set("description", in.Description)
	params.Set("ignoreGtmDataLayer", flagParam(preserved.IgnoreGtmDataLayer))
	params.Set("isTagFireLimitAllowedInPreviewMode", flagParam(preserved.IsTagFireLimitAllowedInPreviewMode))
	params.Set("activelySyncGtmDataLayer", flagParam(preserved.ActivelySyncGtmDataLayer))
	_, err := t.Call(ctx, "updateContainer", params)
	return err
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
		return Container{}, ErrContainerNotFound
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

func decodeContainers(raw json.RawMessage) ([]Container, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	switch raw[0] {
	case '[':
		var items []rawContainer
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		out := make([]Container, 0, len(items))
		for _, item := range items {
			c, err := item.container()
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(c.IDContainer) == "" {
				continue
			}
			out = append(out, c)
		}
		return out, nil
	case '{':
		var keyed map[string]json.RawMessage
		if err := json.Unmarshal(raw, &keyed); err != nil {
			return nil, err
		}
		if _, hasID := keyed["idcontainer"]; hasID || keyed["name"] != nil {
			c, err := decodeContainer(raw)
			if err != nil {
				return nil, err
			}
			return []Container{c}, nil
		}
		out := make([]Container, 0, len(keyed))
		for key, value := range keyed {
			value = bytes.TrimSpace(value)
			if len(value) == 0 || value[0] != '{' {
				return nil, fmt.Errorf("unexpected TagManager.getContainers payload")
			}
			c, err := decodeContainer(value)
			if err != nil {
				return nil, err
			}
			if c.IDContainer == "" {
				c.IDContainer = strings.TrimSpace(key)
			}
			out = append(out, c)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unexpected TagManager.getContainers payload")
	}
}

func decodeContainerID(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", fmt.Errorf("matomo: TagManager.addContainer returned no idContainer")
	}

	var direct flexibleString
	if err := json.Unmarshal(raw, &direct); err == nil {
		id := strings.TrimSpace(string(direct))
		if id != "" {
			return id, nil
		}
	}

	var wrapped struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && len(bytes.TrimSpace(wrapped.Value)) > 0 {
		var inner flexibleString
		if err := json.Unmarshal(wrapped.Value, &inner); err == nil {
			id := strings.TrimSpace(string(inner))
			if id != "" {
				return id, nil
			}
		}
	}

	return "", fmt.Errorf("matomo: unexpected TagManager.addContainer payload")
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

func isContainerNotFoundAPIError(err error) bool {
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr == nil {
		return false
	}
	msg := strings.ToLower(apiErr.Message)
	return strings.Contains(msg, "does not exist") || strings.Contains(msg, "not found")
}

func flagParam(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true":
		return "1"
	default:
		return "0"
	}
}
