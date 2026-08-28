package matomo

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

const (
	// AttrContainer is a $ref to a managed matomo.container. It is required
	// for Tag Manager child resources in managed-container mode and omitted
	// when MATOMO_CONTAINER_ID selects an externally managed container.
	AttrContainer = "container"
)

var errContainerIdentityUnavailable = errors.New("Tag Manager container identity is unavailable")

type catalogBindings interface {
	Bindings(provider, resourceType string) ([]state.Binding, error)
}

func (p *Provider) setManagedContainer(addr resource.Address) {
	if p == nil || addr.IsZero() {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.managedContainer = addr
}

func (p *Provider) clearManagedContainer() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.managedContainer = resource.Address{}
}

func (p *Provider) managedContainerAddress() resource.Address {
	if p == nil {
		return resource.Address{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.managedContainer
}

func (p *Provider) hasManagedContainer() bool {
	return !p.managedContainerAddress().IsZero()
}

func (p *Provider) publicationAddress() resource.Address {
	if addr := p.managedContainerAddress(); !addr.IsZero() {
		return addr
	}
	return resource.Address{Provider: Name, Type: TypeContainer, Name: "external"}
}

func (p *Provider) requireTagManagerConfig(res resource.Resource) error {
	if err := p.requireSiteID(); err != nil {
		return fmt.Errorf("%s is required to manage Tag Manager resources", EnvSiteID)
	}
	if _, ok := res.Attributes[AttrContainer]; ok {
		return nil
	}
	if p == nil || strings.TrimSpace(p.cfg.ContainerID) == "" {
		return fmt.Errorf("%s is required to manage Tag Manager resources", EnvContainerID)
	}
	return nil
}

func (p *Provider) containerIDFor(res resource.Resource) (string, error) {
	if v, ok := res.Attributes[AttrContainer]; ok {
		if resolved, ok := resource.AsResolved(v); ok && resolved.Identity.ID != "" {
			return resolved.Identity.ID, nil
		}
		ref, err := containerRefValue(v)
		if err != nil {
			return "", fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrContainer, err)
		}
		if id := p.lookupID(ref.Address); id != "" {
			return id, nil
		}
		return "", fmt.Errorf("resource %s: %w", res.Address, errContainerIdentityUnavailable)
	}
	if p != nil {
		if id := strings.TrimSpace(p.cfg.ContainerID); id != "" {
			return id, nil
		}
	}
	return "", fmt.Errorf("%s is required to manage Tag Manager resources", EnvContainerID)
}

func (p *Provider) tagManagerFor(res resource.Resource) (*client.TagManager, error) {
	id, err := p.containerIDFor(res)
	if err != nil {
		return nil, err
	}
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	return c.TagManager().ForContainer(id), nil
}

func (p *Provider) importContainerID() (string, error) {
	if err := p.requireSiteID(); err != nil {
		return "", fmt.Errorf("%s is required to manage Tag Manager resources", EnvSiteID)
	}
	envID := ""
	if p != nil {
		envID = strings.TrimSpace(p.cfg.ContainerID)
	}
	bindings, err := p.boundContainerIdentities()
	if err != nil {
		return "", err
	}
	switch {
	case len(bindings) > 1:
		return "", fmt.Errorf("matomo: at most one matomo.container identity may be bound in local state")
	case len(bindings) == 1:
		id := strings.TrimSpace(bindings[0].RemoteID)
		if envID != "" && envID != id {
			return "", fmt.Errorf("matomo: %s %q does not match managed container %s bound as %s", EnvContainerID, envID, bindings[0].Address, id)
		}
		return id, nil
	case envID != "":
		return envID, nil
	default:
		return "", fmt.Errorf("%s is required to import Tag Manager resources unless a matomo.container resource is already bound in local state", EnvContainerID)
	}
}

func (p *Provider) importTagManagerResource(addr resource.Address) (resource.Resource, string, error) {
	containerID, err := p.importContainerID()
	if err != nil {
		return resource.Resource{}, "", err
	}
	res := resource.Resource{Address: addr, Attributes: resource.Attributes{}}
	if managed, ok, err := p.lookupManagedAddress(TypeContainer, containerID); err != nil {
		return resource.Resource{}, "", err
	} else if ok {
		res.Attributes[AttrContainer] = resource.Ref{Address: managed}
		p.rememberBinding(managed, containerID, "")
	}
	return res, containerID, nil
}

func (p *Provider) tagManagerForImport() (*client.TagManager, error) {
	id, err := p.importContainerID()
	if err != nil {
		return nil, err
	}
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	return c.TagManager().ForContainer(id), nil
}

func (p *Provider) boundContainerIdentities() ([]state.Binding, error) {
	if p == nil {
		return nil, nil
	}
	p.mu.Lock()
	catalog := p.identities
	p.mu.Unlock()
	lister, ok := catalog.(catalogBindings)
	if !ok || lister == nil {
		return nil, nil
	}
	return lister.Bindings(Name, TypeContainer)
}

func (p *Provider) attachImportedContainerRef(live resource.RemoteResource, containerID string) resource.RemoteResource {
	addr, ok, err := p.lookupManagedAddress(TypeContainer, containerID)
	if err != nil || !ok {
		return live
	}
	live.Attributes = live.Attributes.Clone()
	live.Attributes[AttrContainer] = resource.Ref{Address: addr}
	return live
}

func optionalContainerRef(res resource.Resource) (resource.Ref, bool, error) {
	v, ok := res.Attributes[AttrContainer]
	if !ok {
		return resource.Ref{}, false, nil
	}
	ref, err := containerRefValue(v)
	if err != nil {
		return resource.Ref{}, true, fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrContainer, err)
	}
	if ref.Address.Provider != Name || ref.Address.Type != TypeContainer {
		return resource.Ref{}, true, fmt.Errorf("resource %s: attribute %q must reference a %s.%s resource", res.Address, AttrContainer, Name, TypeContainer)
	}
	return ref, true, nil
}

func containerRefValue(v any) (resource.Ref, error) {
	if resolved, ok := resource.AsResolved(v); ok {
		return resource.Ref{Address: resolved.Address}, nil
	}
	ref, ok := resource.AsRef(v)
	if !ok {
		return resource.Ref{}, fmt.Errorf("must be a resource reference ($ref) to a %s.%s resource", Name, TypeContainer)
	}
	return ref, nil
}

func withComparableContainer(out resource.Attributes, attrs resource.Attributes) (resource.Attributes, error) {
	v, ok := attrs[AttrContainer]
	if !ok {
		return out, nil
	}
	ref, err := containerRefValue(v)
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrContainer, err)
	}
	out[AttrContainer] = ref
	return out, nil
}

func attachContainerRef(live resource.RemoteResource, desired resource.Attributes) resource.RemoteResource {
	ref := logicalRef(desired[AttrContainer])
	if ref.IsZero() {
		return live
	}
	live.Attributes = live.Attributes.Clone()
	live.Attributes[AttrContainer] = ref
	return live
}

func ensureImmutableContainerRef(desired resource.Resource, live resource.RemoteResource) error {
	want := logicalRef(desired.Attributes[AttrContainer])
	got := logicalRef(live.Attributes[AttrContainer])
	if want.Address == got.Address {
		return nil
	}
	return fmt.Errorf("resource %s: attribute %q is immutable and cannot be changed from %s to %s", desired.Address, AttrContainer, displayContainerRef(got), displayContainerRef(want))
}

func displayContainerRef(ref resource.Ref) string {
	if ref.IsZero() {
		return "none"
	}
	return ref.Address.String()
}

func mapUnavailableContainer(addr resource.Address, err error) error {
	if errors.Is(err, errContainerIdentityUnavailable) {
		return fmt.Errorf("matomo: read %s: %w", addr, provider.ErrNotFound)
	}
	return err
}
