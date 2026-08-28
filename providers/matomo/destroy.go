package matomo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

// DestroyCapability implements provider.Destroyer.
func (p *Provider) DestroyCapability(res resource.Resource) (provider.DestroyCapability, error) {
	if res.Address.Provider != Name {
		return provider.DestroyUnsupported, nil
	}
	switch res.Address.Type {
	case TypeGoal:
		// Matomo marks goals deleted rather than physically removing their
		// historical identity, so expose this as a provider-native remove.
		return provider.DestroyRemove, nil
	case TypeContainer, TypeVariable, TypeTrigger, TypeTag:
		return provider.DestroyDelete, nil
	default:
		return provider.DestroyUnsupported, nil
	}
}

// Destroy implements provider.Destroyer.
func (p *Provider) Destroy(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	switch res.Address.Type {
	case TypeGoal:
		return p.destroyGoal(ctx, res)
	case TypeContainer:
		return p.destroyContainer(ctx, res)
	case TypeVariable:
		return p.destroyVariable(ctx, res)
	case TypeTrigger:
		return p.destroyTrigger(ctx, res)
	case TypeTag:
		return p.destroyTag(ctx, res)
	default:
		return provider.DestroyResult{}, notImplemented("destroy", res.Address)
	}
}

func (p *Provider) destroyGoal(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	id := strings.TrimSpace(res.Identity.ID)
	if id == "" {
		return provider.DestroyResult{}, fmt.Errorf("matomo: destroy %s: missing identity", res.Address)
	}
	live, err := p.readGoalByID(ctx, res.Address, id)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return provider.DestroyResult{Status: provider.DestroyStatusAlreadyAbsent}, nil
		}
		return provider.DestroyResult{}, fmt.Errorf("matomo: destroy %s: %w", res.Address, err)
	}
	if isDeletedFlag(live.Computed["deleted"]) {
		return provider.DestroyResult{Status: provider.DestroyStatusAlreadyAbsent}, nil
	}
	c, err := p.Client()
	if err != nil {
		return provider.DestroyResult{}, err
	}
	if err := c.Analytics().DeleteGoal(ctx, id); err != nil {
		return provider.DestroyResult{}, fmt.Errorf("matomo: destroy %s: %w", res.Address, err)
	}
	return provider.DestroyResult{Status: provider.DestroyStatusDestroyed}, nil
}

func (p *Provider) destroyContainer(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	id := strings.TrimSpace(res.Identity.ID)
	if id == "" {
		return provider.DestroyResult{}, fmt.Errorf("matomo: destroy %s: missing identity", res.Address)
	}
	if envID := strings.TrimSpace(p.cfg.ContainerID); envID != "" && !p.hasManagedContainer() {
		return provider.DestroyResult{}, fmt.Errorf("matomo: refusing to delete container selected by %s; externally managed containers cannot be destroyed", EnvContainerID)
	}
	_, err := p.readContainerByID(ctx, res.Address, id)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return provider.DestroyResult{Status: provider.DestroyStatusAlreadyAbsent}, nil
		}
		return provider.DestroyResult{}, fmt.Errorf("matomo: destroy %s: %w", res.Address, err)
	}
	c, err := p.Client()
	if err != nil {
		return provider.DestroyResult{}, err
	}
	if err := c.TagManager().DeleteContainer(ctx, id); err != nil {
		return provider.DestroyResult{}, fmt.Errorf("matomo: destroy %s: %w", res.Address, err)
	}
	return provider.DestroyResult{Status: provider.DestroyStatusDestroyed}, nil
}

func (p *Provider) destroyVariable(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	return p.destroyDraftChild(ctx, res, "variable", p.readVariableByID, func(ctx context.Context, tm *client.TagManager, version, id string) error {
		return tm.DeleteContainerVariable(ctx, version, id)
	})
}

func (p *Provider) destroyTrigger(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	return p.destroyDraftChild(ctx, res, "trigger", p.readTriggerByID, func(ctx context.Context, tm *client.TagManager, version, id string) error {
		return tm.DeleteContainerTrigger(ctx, version, id)
	})
}

func (p *Provider) destroyTag(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	return p.destroyDraftChild(ctx, res, "tag", p.readTagByID, func(ctx context.Context, tm *client.TagManager, version, id string) error {
		return tm.DeleteContainerTag(ctx, version, id)
	})
}

type draftReader func(context.Context, resource.Resource, string) (resource.RemoteResource, error)

type draftDeleter func(context.Context, *client.TagManager, string, string) error

func (p *Provider) destroyDraftChild(ctx context.Context, res resource.Resource, kind string, read draftReader, del draftDeleter) (provider.DestroyResult, error) {
	id := strings.TrimSpace(res.Identity.ID)
	if id == "" {
		return provider.DestroyResult{}, fmt.Errorf("matomo: destroy %s: missing identity", res.Address)
	}
	_, err := read(ctx, res, id)
	if absent, wrap := p.childAbsence(res, err); wrap != nil {
		return provider.DestroyResult{}, fmt.Errorf("matomo: destroy %s: %w", res.Address, wrap)
	} else if absent {
		return provider.DestroyResult{Status: provider.DestroyStatusAlreadyAbsent}, nil
	}

	tm, err := p.tagManagerFor(res)
	if err != nil {
		return provider.DestroyResult{}, fmt.Errorf("matomo: destroy %s: %w", res.Address, err)
	}
	version, err := tm.DraftVersion(ctx)
	if err != nil {
		if absent, wrap := p.childAbsence(res, err); wrap != nil {
			return provider.DestroyResult{}, fmt.Errorf("matomo: destroy %s: %w", res.Address, wrap)
		} else if absent {
			return provider.DestroyResult{Status: provider.DestroyStatusAlreadyAbsent}, nil
		}
		return provider.DestroyResult{}, fmt.Errorf("matomo: destroy %s: %w", res.Address, err)
	}
	if err := del(ctx, tm, version, id); err != nil {
		return provider.DestroyResult{}, fmt.Errorf("matomo: destroy %s %s: %w", res.Address, kind, err)
	}
	return provider.DestroyResult{Status: provider.DestroyStatusDestroyed}, nil
}

func (p *Provider) childAbsence(res resource.Resource, err error) (bool, error) {
	if err == nil {
		return false, nil
	}
	if errors.Is(err, provider.ErrNotFound) {
		return true, nil
	}
	if errors.Is(err, client.ErrContainerNotFound) {
		if _, ok := res.Attributes[AttrContainer]; ok {
			return true, nil
		}
		return false, fmt.Errorf("Tag Manager container selected by %s was not found", EnvContainerID)
	}
	return false, err
}

func isDeletedFlag(v any) bool {
	s := strings.TrimSpace(fmt.Sprint(v))
	return s == "1" || strings.EqualFold(s, "true")
}
