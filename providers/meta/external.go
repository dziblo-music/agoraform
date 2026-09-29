package meta

import (
	"context"
	"fmt"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

// SupportsExternal implements provider.ExternalReader.
//
// Campaigns can be read by id. Other Meta types stay managed.
func (p *Provider) SupportsExternal(resourceType string) bool {
	return resourceType == TypeCampaign
}

// ReadExternal implements provider.ExternalReader.
func (p *Provider) ReadExternal(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	if !p.SupportsExternal(addr.Type) {
		return resource.RemoteResource{}, fmt.Errorf("meta: %s does not support external ownership", addr)
	}
	return p.importCampaign(ctx, addr, id)
}

var _ provider.ExternalReader = (*Provider)(nil)
