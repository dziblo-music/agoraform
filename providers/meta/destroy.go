package meta

import (
	"context"
	"errors"
	"fmt"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/meta/client"
)

const pixelDestroyGuidance = "Pixel/Dataset event sources are owned in Events Manager / Business Manager; Agoraform imports or adopts them and never deletes the remote object"

// DestroyCapability implements provider.Destroyer.
func (p *Provider) DestroyCapability(res resource.Resource) (provider.DestroyCapability, error) {
	spec, ok := Lifecycle(res.Address.Type)
	if !ok {
		return provider.DestroyUnsupported, nil
	}
	return spec.Destroy, nil
}

// Destroy implements provider.Destroyer.
func (p *Provider) Destroy(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	spec, ok := Lifecycle(res.Address.Type)
	if !ok {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: unsupported resource type", res.Address)
	}
	if spec.Destroy == provider.DestroyProviderOwned {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: %s", res.Address, spec.ServingSafety)
	}
	switch res.Address.Type {
	case TypeCustomConversion:
		return p.destroyCustomConversion(ctx, res)
	case TypeCampaign:
		return p.destroyCampaign(ctx, res)
	case TypeAdSet:
		return p.destroyAdSet(ctx, res)
	case TypeAdCreative:
		return p.destroyAdCreative(ctx, res)
	case TypeAd:
		return p.destroyAd(ctx, res)
	default:
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: unsupported resource type", res.Address)
	}
}

func (p *Provider) destroyCustomConversion(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	id, bound, err := boundIdentity(res)
	if err != nil {
		return provider.DestroyResult{}, err
	}
	if !bound {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: missing persisted identity", res.Address)
	}

	live, err := p.readCustomConversionByID(ctx, res, id)
	if errors.Is(err, provider.ErrNotFound) {
		return provider.DestroyResult{Status: provider.DestroyStatusAlreadyAbsent}, nil
	}
	if err != nil {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: %w", res.Address, err)
	}

	c, err := p.Client()
	if err != nil {
		return provider.DestroyResult{}, err
	}
	var result map[string]any
	if err := c.Delete(ctx, live.Identity.ID, nil, &result); err != nil {
		if client.IsNotFound(err) {
			return provider.DestroyResult{Status: provider.DestroyStatusAlreadyAbsent}, nil
		}
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: %w", res.Address, err)
	}
	if success, ok := result["success"].(bool); ok && !success {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: API did not report success", res.Address)
	}

	_, err = p.readCustomConversionByID(ctx, res, id)
	if errors.Is(err, provider.ErrNotFound) {
		return provider.DestroyResult{Status: provider.DestroyStatusRemoved}, nil
	}
	if err != nil {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: DELETE succeeded but confirming terminal state failed: %w", res.Address, err)
	}
	return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: custom conversion %s is still active after DELETE", res.Address, id)
}
