package matomo

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
)

const (
	// AttrIDTag is not configurable; Matomo tag identity is stored in
	// local state.
	AttrIDTag = "idTag"
)

func (p *Provider) validateTagSafe(res resource.Resource) error {
	if err := rejectManifestTagIdentity(res); err != nil {
		return err
	}
	if err := p.validateTag(res); err != nil {
		return err
	}
	return optionalEventValue(res)
}

func (p *Provider) readTagSafe(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateTagSafe(res); err != nil {
		return resource.RemoteResource{}, err
	}
	id, bound, err := boundTagIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if !bound {
		live, err := p.readTag(ctx, res)
		if err != nil {
			return resource.RemoteResource{}, err
		}
		return p.reconcileTagVariableRefs(ctx, res, live)
	}

	live, err := p.readTagByID(ctx, res.Address, id, res.Attributes)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: persisted identity %q was not found remotely; refusing to plan a replacement resource: %w", res.Address, id, state.ErrStaleIdentity)
		}
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s by identity %q: %w", res.Address, id, err)
	}
	if err := ensureImmutableTagType(res, live); err != nil {
		return resource.RemoteResource{}, err
	}
	return p.reconcileTagVariableRefs(ctx, res, live)
}

func (p *Provider) createTagSafe(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateTagSafe(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundTagIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("matomo: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}
	return p.createTag(ctx, res)
}

func (p *Provider) updateTagSafe(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateTagSafe(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: missing remote identity", desired.Address)
	}
	if id, bound, err := boundTagIdentity(desired); err != nil {
		return resource.RemoteResource{}, err
	} else if bound && id != actual.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: persisted identity %q does not match planned remote identity %q", desired.Address, id, actual.Identity.ID)
	}
	return p.updateTag(ctx, desired, actual)
}

func (p *Provider) importTag(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	id = strings.TrimSpace(id)
	if err := validateTagIdentity(addr, id); err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireTagManagerConfig(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: import %s: %w", addr, err)
	}
	live, err := p.readTagByID(ctx, addr, id, nil)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("matomo: import %s: remote tag %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("matomo: import %s: %w", addr, err)
	}
	return live, nil
}

func (p *Provider) normalizeTagComparableSafe(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	if err := rejectManifestTagIdentity(desired); err != nil {
		return nil, nil, err
	}
	if live != nil {
		if _, bound, err := boundTagIdentity(desired); err != nil {
			return nil, nil, err
		} else if bound {
			if live.Identity.IsZero() || live.Identity.ID != desired.Identity.ID {
				return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, desired.Identity.ID, live.Identity.ID)
			}
			if err := ensureImmutableTagType(desired, *live); err != nil {
				return nil, nil, err
			}
		}
	}
	return p.normalizeTagComparable(desired, live)
}

func rejectManifestTagIdentity(res resource.Resource) error {
	if _, ok := res.Attributes[AttrIDTag]; ok {
		return fmt.Errorf("resource %s: attribute %q is not configurable; persist the Matomo tag identity in local state (%s), not in the manifest", res.Address, AttrIDTag, state.DefaultFilename)
	}
	return nil
}

func boundTagIdentity(res resource.Resource) (string, bool, error) {
	if res.Identity.IsZero() {
		return "", false, nil
	}
	id := strings.TrimSpace(res.Identity.ID)
	if err := validateTagIdentity(res.Address, id); err != nil {
		return "", true, err
	}
	return id, true, nil
}

func validateTagIdentity(addr resource.Address, id string) error {
	if id == "" {
		return fmt.Errorf("resource %s: persisted identity is empty; a Matomo tag id is required", addr)
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("resource %s: persisted identity %q is not a valid Matomo tag id", addr, id)
	}
	return nil
}
