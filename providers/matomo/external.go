package matomo

import (
	"context"
	"fmt"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

// SupportsExternal implements provider.ExternalReader.
//
// Only matomo.container can be referenced without Agoraform lifecycle
// ownership. Tags, triggers, variables, and goals stay managed resources.
func (p *Provider) SupportsExternal(resourceType string) bool {
	return resourceType == TypeContainer
}

// ReadExternal implements provider.ExternalReader.
//
// It reads the existing container by provider-native id and does not create,
// update, or delete it.
func (p *Provider) ReadExternal(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	if !p.SupportsExternal(addr.Type) {
		return resource.RemoteResource{}, fmt.Errorf("matomo: %s does not support external ownership", addr)
	}
	return p.importContainer(ctx, addr, id)
}

var _ provider.ExternalReader = (*Provider)(nil)
