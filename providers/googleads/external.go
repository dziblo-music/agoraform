package googleads

import (
	"context"
	"errors"
	"fmt"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

// SupportsExternal implements provider.ExternalReader.
//
// Search campaigns can be referenced by id. Other Google Ads types stay
// managed because their reads depend on Agoraform-owned parent bindings.
func (p *Provider) SupportsExternal(resourceType string) bool {
	return resourceType == TypeCampaign
}

// ReadExternal implements provider.ExternalReader.
//
// The campaign is resolved by its canonical id. A missing budget binding does
// not block the read: an external campaign is not reconciled, so its budget
// does not have to be an Agoraform resource. Multiple rows for one id fail
// instead of guessing.
func (p *Provider) ReadExternal(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	if !p.SupportsExternal(addr.Type) {
		return resource.RemoteResource{}, fmt.Errorf("googleads: %s does not support external ownership", addr)
	}
	canonical, err := p.canonicalCampaignImportID(addr, id)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: external %s: %w", addr, err)
	}
	if err := p.requireCustomerID(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: external %s: %w", addr, err)
	}
	live, err := p.readCampaignByID(ctx, addr, canonical, nil)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("googleads: external %s: remote campaign %q was not found: %w", addr, canonical, provider.ErrNotFound)
		}
		return resource.RemoteResource{}, fmt.Errorf("googleads: external %s: %w", addr, err)
	}
	if live.Identity.ID != canonical {
		return resource.RemoteResource{}, fmt.Errorf("googleads: external %s: provider returned identity %q for %q", addr, live.Identity.ID, canonical)
	}
	return live, nil
}

var _ provider.ExternalReader = (*Provider)(nil)
