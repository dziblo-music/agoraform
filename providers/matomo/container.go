package matomo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

const (
	// TypeContainer is the Matomo Tag Manager container type used in
	// addresses such as matomo.container.main.
	TypeContainer = "container"

	// AttrContext is the Tag Manager container context, for example web.
	AttrContext = "context"
	// AttrDescription is an optional container description.
	AttrDescription = "description"

	containerContextWeb     = "web"
	containerContextAndroid = "android"
	containerContextIOS     = "ios"

	// MaxContainerNameLen is Matomo's maximum length for a container name.
	MaxContainerNameLen = 255
)

var (
	supportedContainerAttrs = map[string]struct{}{
		AttrName:        {},
		AttrContext:     {},
		AttrDescription: {},
	}

	computedContainerAttrs = map[string]struct{}{
		"idcontainer":                        {},
		"idContainer":                        {},
		"idsite":                             {},
		"status":                             {},
		"draft":                              {},
		"idcontainerversion":                 {},
		"idContainerVersion":                 {},
		"versions":                           {},
		"releases":                           {},
		"created_date":                       {},
		"updated_date":                       {},
		"ignoreGtmDataLayer":                 {},
		"isTagFireLimitAllowedInPreviewMode": {},
		"activelySyncGtmDataLayer":           {},
		"disablePreview":                     {},
	}

	supportedContainerContexts = map[string]struct{}{
		containerContextWeb:     {},
		containerContextAndroid: {},
		containerContextIOS:     {},
	}
)

func (p *Provider) validateContainer(res resource.Resource) error {
	if err := p.requireContainerSiteID(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}

	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedContainerAttrs[key]; ok {
			continue
		}
		if _, computed := computedContainerAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; matomo.container supports %s, %s, and optional %s", res.Address, key, AttrName, AttrContext, AttrDescription)
	}

	name, err := requiredString(res, AttrName)
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("resource %s: attribute %q must be a non-empty string", res.Address, AttrName)
	}
	if err := rejectEdgeWhitespace(res.Address, AttrName, name); err != nil {
		return err
	}
	if utf8.RuneCountInString(name) > MaxContainerNameLen {
		return fmt.Errorf("resource %s: attribute %q must be at most %d characters", res.Address, AttrName, MaxContainerNameLen)
	}

	context, err := requiredString(res, AttrContext)
	if err != nil {
		return err
	}
	if _, ok := supportedContainerContexts[context]; !ok {
		return fmt.Errorf("resource %s: attribute %q must be one of %s", res.Address, AttrContext, joinSorted(keys(supportedContainerContexts)))
	}

	description, set, err := optionalString(res, AttrDescription)
	if err != nil {
		return err
	}
	if set {
		if err := rejectEdgeWhitespace(res.Address, AttrDescription, description); err != nil {
			return err
		}
	}

	return nil
}

func (p *Provider) requireContainerSiteID() error {
	if p == nil || strings.TrimSpace(p.cfg.SiteID) == "" {
		return fmt.Errorf("%s is required to manage Tag Manager containers", EnvSiteID)
	}
	return nil
}

func (p *Provider) readContainer(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateContainer(res); err != nil {
		return resource.RemoteResource{}, err
	}

	name := stringAttr(res.Attributes, AttrName)
	containers, err := p.listContainers(ctx)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: %w", res.Address, err)
	}

	matches := findContainersByName(containers, name)
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: %w", res.Address, provider.ErrNotFound)
	case 1:
		live, err := p.readContainerByID(ctx, res.Address, matches[0].IDContainer)
		if err == nil {
			p.rememberContainer(res.Address, live.Identity.ID, name)
		}
		return live, err
	default:
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: multiple remote containers named %q (ids %s); names must be unique", res.Address, name, joinContainerIDs(matches))
	}
}

func (p *Provider) createContainer(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateContainer(res); err != nil {
		return resource.RemoteResource{}, err
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	id, err := c.TagManager().AddContainer(ctx, containerInput(res.Attributes))
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: create %s: %w", res.Address, err)
	}

	live, err := p.readContainerByID(ctx, res.Address, id)
	if err == nil {
		p.rememberContainer(res.Address, live.Identity.ID, stringAttr(res.Attributes, AttrName))
		return live, nil
	}
	p.rememberContainer(res.Address, id, stringAttr(res.Attributes, AttrName))
	return remoteContainer(res.Address, client.Container{
		IDContainer: id,
		IDSite:      strings.TrimSpace(p.cfg.SiteID),
		Name:        stringAttr(res.Attributes, AttrName),
		Context:     stringAttr(res.Attributes, AttrContext),
		Description: stringAttr(res.Attributes, AttrDescription),
	}), nil
}

func (p *Provider) updateContainer(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateContainer(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: missing remote identity", desired.Address)
	}

	current, err := p.readContainerByID(ctx, desired.Address, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: refresh remote container %q: %w", desired.Address, actual.Identity.ID, err)
	}
	if err := ensureImmutableContainerContext(desired, current); err != nil {
		return resource.RemoteResource{}, err
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	tm := c.TagManager().ForContainer(actual.Identity.ID)
	if err := tm.UpdateContainer(ctx, actual.Identity.ID, containerInput(desired.Attributes), containerPreserved(current)); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: %w", desired.Address, err)
	}

	live, err := p.readContainerByID(ctx, desired.Address, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s succeeded but refreshing container %q failed: %w", desired.Address, actual.Identity.ID, err)
	}
	return live, nil
}

func (p *Provider) readContainerByID(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	container, err := c.TagManager().ForContainer(id).GetContainer(ctx)
	if err != nil {
		if errors.Is(err, client.ErrContainerNotFound) {
			return resource.RemoteResource{}, provider.ErrNotFound
		}
		return resource.RemoteResource{}, err
	}
	if strings.TrimSpace(container.IDContainer) != "" && container.IDContainer != id {
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: requested container %q but Matomo returned %q", addr, id, container.IDContainer)
	}
	live := remoteContainer(addr, container)
	p.rememberContainer(addr, live.Identity.ID, container.Name)
	return live, nil
}

func (p *Provider) listContainers(ctx context.Context) ([]client.Container, error) {
	if err := p.requireContainerSiteID(); err != nil {
		return nil, err
	}
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	return c.TagManager().GetContainers(ctx)
}

func (p *Provider) normalizeContainerComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	want, err := comparableContainer(desired.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if live == nil {
		return want, nil, nil
	}
	got, err := comparableContainer(live.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	return want, got, nil
}

func comparableContainer(attrs resource.Attributes) (resource.Attributes, error) {
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	name, err := coerceString(attrs[AttrName])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrName, err)
	}
	context, err := coerceString(attrs[AttrContext])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrContext, err)
	}
	description, err := coerceString(attrs[AttrDescription])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrDescription, err)
	}
	return resource.Attributes{
		AttrName:        name,
		AttrContext:     context,
		AttrDescription: description,
	}, nil
}

func remoteContainer(addr resource.Address, c client.Container) resource.RemoteResource {
	attrs := resource.Attributes{
		AttrName:        c.Name,
		AttrContext:     c.Context,
		AttrDescription: c.Description,
	}
	computed := resource.Attributes{}
	setComputed(computed, "idcontainer", c.IDContainer)
	setComputed(computed, "idsite", c.IDSite)
	setComputed(computed, "status", c.Status)
	setComputed(computed, "idcontainerversion", c.DraftVersion)
	setComputed(computed, "ignoreGtmDataLayer", c.IgnoreGtmDataLayer)
	setComputed(computed, "isTagFireLimitAllowedInPreviewMode", c.IsTagFireLimitAllowedInPreviewMode)
	setComputed(computed, "activelySyncGtmDataLayer", c.ActivelySyncGtmDataLayer)
	return resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: c.IDContainer},
		Attributes: attrs,
		Computed:   computed,
	}
}

func containerInput(attrs resource.Attributes) client.ContainerInput {
	return client.ContainerInput{
		Context:     stringAttr(attrs, AttrContext),
		Name:        stringAttr(attrs, AttrName),
		Description: stringAttr(attrs, AttrDescription),
	}
}

func containerPreserved(live resource.RemoteResource) client.ContainerPreservedFields {
	return client.ContainerPreservedFields{
		IgnoreGtmDataLayer:                 computedString(live.Computed, "ignoreGtmDataLayer"),
		IsTagFireLimitAllowedInPreviewMode: computedString(live.Computed, "isTagFireLimitAllowedInPreviewMode"),
		ActivelySyncGtmDataLayer:           computedString(live.Computed, "activelySyncGtmDataLayer"),
	}
}

func findContainersByName(containers []client.Container, name string) []client.Container {
	var matches []client.Container
	for _, c := range containers {
		if strings.EqualFold(c.Status, "deleted") {
			continue
		}
		if c.Name == name {
			matches = append(matches, c)
		}
	}
	return matches
}

func joinContainerIDs(containers []client.Container) string {
	ids := make([]string, 0, len(containers))
	for _, c := range containers {
		ids = append(ids, c.IDContainer)
	}
	return joinSorted(ids)
}

func ensureImmutableContainerContext(desired resource.Resource, live resource.RemoteResource) error {
	want := stringAttr(desired.Attributes, AttrContext)
	got := stringAttr(live.Attributes, AttrContext)
	if want == got || got == "" {
		return nil
	}
	return fmt.Errorf("resource %s: attribute %q is immutable for Matomo container %q; remote context is %q and configuration requests %q", desired.Address, AttrContext, live.Identity.ID, got, want)
}

func (p *Provider) rememberContainer(addr resource.Address, id, name string) {
	p.rememberBinding(addr, id, name)
	p.setManagedContainer(addr)
}
