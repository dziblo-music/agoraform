package matomo

import (
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

var _ provider.IdentityBinder = (*Provider)(nil)

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

// BindIdentity implements provider.IdentityBinder. Destroy uses it to supply
// the container identity that publication needs without reading the
// container, including a reference-only (external) container.
func (p *Provider) BindIdentity(addr resource.Address, id resource.Identity) {
	if p == nil || addr.Provider != Name {
		return
	}
	// Keep a name learned from an earlier read when the identity is unchanged.
	value := strings.TrimSpace(id.ID)
	name := ""
	if p.lookupID(addr) == value {
		name = p.lookupName(addr)
	}
	p.rememberBinding(addr, value, name)
}

func (p *Provider) lookupID(addr resource.Address) string {
	if p == nil || addr.IsZero() {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.known[addr.String()].id
}

func (p *Provider) lookupName(addr resource.Address) string {
	if p == nil || addr.IsZero() {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.known[addr.String()].name
}
