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
	// AttrIDTrigger is not configurable; Matomo trigger identity is
	// stored in local state.
	AttrIDTrigger = "idTrigger"
)

func (p *Provider) validateTriggerSafe(res resource.Resource) error {
	if err := rejectManifestTriggerIdentity(res); err != nil {
		return err
	}
	return p.validateTrigger(res)
}

func (p *Provider) readTriggerSafe(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateTriggerSafe(res); err != nil {
		return resource.RemoteResource{}, err
	}
	id, bound, err := boundTriggerIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if !bound {
		return p.readTrigger(ctx, res)
	}

	live, err := p.readTriggerByID(ctx, res.Address, id)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: persisted identity %q was not found remotely; refusing to plan a replacement resource: %w", res.Address, id, state.ErrStaleIdentity)
		}
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s by identity %q: %w", res.Address, id, err)
	}
	if err := ensureImmutableTriggerType(res, live); err != nil {
		return resource.RemoteResource{}, err
	}
	return live, nil
}

func (p *Provider) createTriggerSafe(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateTriggerSafe(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundTriggerIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("matomo: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}
	return p.createTrigger(ctx, res)
}

func (p *Provider) updateTriggerSafe(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateTriggerSafe(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: missing remote identity", desired.Address)
	}
	if id, bound, err := boundTriggerIdentity(desired); err != nil {
		return resource.RemoteResource{}, err
	} else if bound && id != actual.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: persisted identity %q does not match planned remote identity %q", desired.Address, id, actual.Identity.ID)
	}
	return p.updateTrigger(ctx, desired, actual)
}

func (p *Provider) importTrigger(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	id = strings.TrimSpace(id)
	if err := validateTriggerIdentity(addr, id); err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireTagManagerConfig(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: import %s: %w", addr, err)
	}
	live, err := p.readTriggerByID(ctx, addr, id)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("matomo: import %s: remote trigger %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("matomo: import %s: %w", addr, err)
	}
	return live, nil
}

func (p *Provider) normalizeTriggerComparableSafe(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	if err := rejectManifestTriggerIdentity(desired); err != nil {
		return nil, nil, err
	}
	if live != nil {
		if _, bound, err := boundTriggerIdentity(desired); err != nil {
			return nil, nil, err
		} else if bound {
			if live.Identity.IsZero() || live.Identity.ID != desired.Identity.ID {
				return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, desired.Identity.ID, live.Identity.ID)
			}
			if err := ensureImmutableTriggerType(desired, *live); err != nil {
				return nil, nil, err
			}
		}
	}
	return p.normalizeTriggerComparable(desired, live)
}

func rejectManifestTriggerIdentity(res resource.Resource) error {
	if _, ok := res.Attributes[AttrIDTrigger]; ok {
		return fmt.Errorf("resource %s: attribute %q is not configurable; persist the Matomo trigger identity in local state (%s), not in the manifest", res.Address, AttrIDTrigger, state.DefaultFilename)
	}
	return nil
}

func boundTriggerIdentity(res resource.Resource) (string, bool, error) {
	if res.Identity.IsZero() {
		return "", false, nil
	}
	id := strings.TrimSpace(res.Identity.ID)
	if err := validateTriggerIdentity(res.Address, id); err != nil {
		return "", true, err
	}
	return id, true, nil
}

func validateTriggerIdentity(addr resource.Address, id string) error {
	if id == "" {
		return fmt.Errorf("resource %s: persisted identity is empty; a Matomo trigger id is required", addr)
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("resource %s: persisted identity %q is not a valid Matomo trigger id", addr, id)
	}
	return nil
}
