package meta

import (
	"github.com/dziblo-music/agoraform/internal/resource"
)

// IdentityCatalog looks up logical addresses for provider-native identities
// already bound in local state. Import uses it to reconstruct `$ref`
// relationships without embedding Meta object IDs in configuration.
type IdentityCatalog interface {
	AddressByRemoteID(provider, resourceType, remoteID string) (resource.Address, bool, error)
}

type remoteBinding struct {
	id   string
	name string
}

func (p *Provider) rememberBinding(addr resource.Address, id, name string) {
	if p == nil || addr.IsZero() || id == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.known == nil {
		p.known = make(map[string]remoteBinding)
	}
	p.known[addr.String()] = remoteBinding{id: id, name: name}
}

func (p *Provider) rememberLive(live resource.RemoteResource) resource.RemoteResource {
	if live.Identity.IsZero() {
		return live
	}
	name, _ := coerceString(live.Attributes[AttrName])
	p.rememberBinding(live.Address, live.Identity.ID, name)
	return live
}

func (p *Provider) lookupID(addr resource.Address) string {
	if p == nil || addr.IsZero() {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.known[addr.String()].id
}

// SetIdentityCatalog supplies local-state reverse lookups for import
// reconstruction of logical references.
func (p *Provider) SetIdentityCatalog(c IdentityCatalog) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.identities = c
}

func (p *Provider) lookupManagedAddress(resourceType, remoteID string) (resource.Address, bool, error) {
	if p == nil {
		return resource.Address{}, false, nil
	}
	p.mu.Lock()
	catalog := p.identities
	snapshot := make(map[string]remoteBinding, len(p.known))
	for k, v := range p.known {
		snapshot[k] = v
	}
	p.mu.Unlock()

	if catalog != nil {
		addr, ok, err := catalog.AddressByRemoteID(Name, resourceType, remoteID)
		if err != nil || ok {
			return addr, ok, err
		}
	}
	matches := make([]resource.Address, 0, 1)
	for key, binding := range snapshot {
		if binding.id != remoteID {
			continue
		}
		addr, err := resource.ParseAddress(key)
		if err != nil || addr.Type != resourceType {
			continue
		}
		matches = append(matches, addr)
	}
	switch len(matches) {
	case 1:
		return matches[0], true, nil
	default:
		return resource.Address{}, false, nil
	}
}
