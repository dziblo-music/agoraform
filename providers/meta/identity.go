package meta

import (
	"context"
	"fmt"

	"github.com/dziblo-music/agoraform/internal/provider"
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

// SetOutputMatcher supplies the provider-neutral v0.5 import catalog used to
// reconstruct relationships from declared, non-sensitive outputs.
func (p *Provider) SetOutputMatcher(m provider.OutputMatcher) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.outputs = m
}

func (p *Provider) matchManagedAddress(ctx context.Context, resourceType, output, remoteID string) (resource.Address, bool, error) {
	p.mu.Lock()
	matcher := p.outputs
	p.mu.Unlock()
	if matcher != nil {
		ref, match, err := matcher.Match(ctx, provider.OutputMatchQuery{
			Provider: Name, ResourceType: resourceType, Output: output, Value: remoteID,
		})
		if err != nil {
			return resource.Address{}, false, err
		}
		switch match {
		case provider.OutputMatchUnique:
			return ref.Address, true, nil
		case provider.OutputMatchAmbiguous:
			return resource.Address{}, false, fmt.Errorf("remote id %s ambiguously matches multiple bound meta.%s resources; refusing to guess", remoteID, resourceType)
		case provider.OutputMatchNone:
			return resource.Address{}, false, nil
		default:
			return resource.Address{}, false, fmt.Errorf("invalid output catalog match result %d", match)
		}
	}
	return p.lookupManagedAddress(resourceType, remoteID)
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
