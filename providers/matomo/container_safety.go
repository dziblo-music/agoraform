package matomo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
)

const (
	// AttrIDContainer is not configurable; Matomo container identity is
	// stored in local state.
	AttrIDContainer = "idContainer"
)

func (p *Provider) validateContainerSafe(res resource.Resource) error {
	if err := rejectManifestContainerIdentity(res); err != nil {
		return err
	}
	return p.validateContainer(res)
}

func (p *Provider) readContainerSafe(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateContainerSafe(res); err != nil {
		return resource.RemoteResource{}, err
	}
	id, bound, err := boundContainerIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if !bound {
		return p.readContainer(ctx, res)
	}

	live, err := p.readContainerByID(ctx, res.Address, id)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: persisted identity %q was not found remotely; refusing to plan a replacement resource: %w", res.Address, id, state.ErrStaleIdentity)
		}
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s by identity %q: %w", res.Address, id, err)
	}
	if err := ensureImmutableContainerContext(res, live); err != nil {
		return resource.RemoteResource{}, err
	}
	return live, nil
}

func (p *Provider) createContainerSafe(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateContainerSafe(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundContainerIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("matomo: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}
	return p.createContainer(ctx, res)
}

func (p *Provider) updateContainerSafe(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateContainerSafe(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: missing remote identity", desired.Address)
	}
	if id, bound, err := boundContainerIdentity(desired); err != nil {
		return resource.RemoteResource{}, err
	} else if bound && id != actual.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: persisted identity %q does not match planned remote identity %q", desired.Address, id, actual.Identity.ID)
	}
	return p.updateContainer(ctx, desired, actual)
}

func (p *Provider) importContainer(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	id = strings.TrimSpace(id)
	if err := validateContainerIdentity(addr, id); err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireContainerSiteID(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: import %s: %w", addr, err)
	}
	live, err := p.readContainerByID(ctx, addr, id)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("matomo: import %s: remote container %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("matomo: import %s: %w", addr, err)
	}
	return live, nil
}

func (p *Provider) normalizeContainerComparableSafe(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	if err := rejectManifestContainerIdentity(desired); err != nil {
		return nil, nil, err
	}
	if live != nil {
		if _, bound, err := boundContainerIdentity(desired); err != nil {
			return nil, nil, err
		} else if bound {
			if live.Identity.IsZero() || live.Identity.ID != desired.Identity.ID {
				return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, desired.Identity.ID, live.Identity.ID)
			}
			if err := ensureImmutableContainerContext(desired, *live); err != nil {
				return nil, nil, err
			}
		}
	}
	return p.normalizeContainerComparable(desired, live)
}

func rejectManifestContainerIdentity(res resource.Resource) error {
	if _, ok := res.Attributes[AttrIDContainer]; ok {
		return fmt.Errorf("resource %s: attribute %q is not configurable; persist the Matomo container identity in local state (%s), not in the manifest", res.Address, AttrIDContainer, state.DefaultFilename)
	}
	return nil
}

func boundContainerIdentity(res resource.Resource) (string, bool, error) {
	if res.Identity.IsZero() {
		return "", false, nil
	}
	id := strings.TrimSpace(res.Identity.ID)
	if err := validateContainerIdentity(res.Address, id); err != nil {
		return "", true, err
	}
	return id, true, nil
}

func validateContainerIdentity(addr resource.Address, id string) error {
	if id == "" {
		return fmt.Errorf("resource %s: persisted identity is empty; a Matomo container id is required", addr)
	}
	if id != strings.TrimSpace(id) {
		return fmt.Errorf("resource %s: persisted identity %q is not a valid Matomo container id", addr, id)
	}
	for _, r := range id {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return fmt.Errorf("resource %s: persisted identity %q is not a valid Matomo container id", addr, id)
		}
	}
	return nil
}
