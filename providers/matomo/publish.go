package matomo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

const (
	// ContainerAddress is the logical label used in publish output for
	// the single configured v0.2.0 Tag Manager container.
	ContainerAddress = "matomo.container.main"

	maxVersionNameLen = 50
)

var publishClock = time.Now

// PublishContainer implements provider.ContainerPublisher.
//
// It creates a Matomo Tag Manager container version from the configured
// draft and publishes that version to the configured environment. When
// the draft is already represented by the currently published version,
// it does not create or publish a duplicate. apply never calls this
// method.
func (p *Provider) PublishContainer(ctx context.Context) (provider.PublishResult, error) {
	result := provider.PublishResult{Address: ContainerAddress}
	if err := p.requirePublishConfig(); err != nil {
		return result, err
	}
	if err := p.CheckConnection(ctx); err != nil {
		return result, err
	}

	c, err := p.Client()
	if err != nil {
		return result, err
	}
	tm := c.TagManager()
	environment := strings.TrimSpace(p.cfg.Environment)
	if environment == "" {
		environment = client.DefaultEnvironment
	}

	if err := p.ensurePublishEnvironment(ctx, tm, environment); err != nil {
		return result, err
	}

	container, err := tm.GetContainer(ctx)
	if err != nil {
		return result, fmt.Errorf("matomo: read container for publish: %w", err)
	}
	if strings.TrimSpace(container.DraftVersion) == "" {
		return result, fmt.Errorf("matomo: Tag Manager container %s has no draft version", p.cfg.ContainerID)
	}

	needed, err := p.publicationNeeded(ctx, tm, container, environment)
	if err != nil {
		return result, err
	}
	if !needed {
		return result, nil
	}

	versionName := newContainerVersionName(publishClock())
	versionID, err := tm.CreateContainerVersion(ctx, versionName, "")
	if err != nil {
		return result, fmt.Errorf("matomo: create container version: %w", err)
	}
	if err := tm.PublishContainerVersion(ctx, versionID, environment); err != nil {
		return result, fmt.Errorf("matomo: publish container version %s to %s: %w", versionID, environment, err)
	}
	result.Created = true
	return result, nil
}

func (p *Provider) requirePublishConfig() error {
	if p == nil {
		return fmt.Errorf("matomo: provider is nil")
	}
	if err := missingConfigError(p.cfg); err != nil {
		return err
	}
	if strings.TrimSpace(p.cfg.SiteID) == "" {
		return fmt.Errorf("matomo: %s is required to publish a Tag Manager container", EnvSiteID)
	}
	if strings.TrimSpace(p.cfg.ContainerID) == "" {
		return fmt.Errorf("matomo: %s is required to publish a Tag Manager container", EnvContainerID)
	}
	return nil
}

func (p *Provider) ensurePublishEnvironment(ctx context.Context, tm *client.TagManager, environment string) error {
	envs, err := tm.GetAvailableEnvironments(ctx)
	if err != nil {
		return fmt.Errorf("matomo: list publish environments: %w", err)
	}
	if len(envs) == 0 {
		return fmt.Errorf("matomo: TagManager.getAvailableEnvironments returned no environments")
	}
	for _, env := range envs {
		if env.ID == environment {
			return nil
		}
	}
	return fmt.Errorf("matomo: environment %q is not a valid Tag Manager publish target", environment)
}

func (p *Provider) publicationNeeded(ctx context.Context, tm *client.TagManager, container client.Container, environment string) (bool, error) {
	release, ok := container.ReleaseFor(environment)
	if !ok {
		return true, nil
	}
	if release.IDContainerVersion == container.DraftVersion {
		return false, nil
	}

	draftFP, err := p.containerFingerprint(ctx, tm, container.DraftVersion)
	if err != nil {
		return false, fmt.Errorf("matomo: fingerprint draft container: %w", err)
	}
	liveFP, err := p.containerFingerprint(ctx, tm, release.IDContainerVersion)
	if err != nil {
		return false, fmt.Errorf("matomo: fingerprint published container: %w", err)
	}
	return draftFP != liveFP, nil
}

func (p *Provider) containerFingerprint(ctx context.Context, tm *client.TagManager, versionID string) (string, error) {
	vars, err := tm.GetContainerVariables(ctx, versionID)
	if err != nil {
		return "", err
	}
	triggers, err := tm.GetContainerTriggers(ctx, versionID)
	if err != nil {
		return "", err
	}
	tags, err := tm.GetContainerTags(ctx, versionID)
	if err != nil {
		return "", err
	}
	return fingerprintContainer(vars, triggers, tags)
}

func newContainerVersionName(now time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}
	name := "agoraform-" + now.UTC().Format("20060102T150405Z")
	if len(name) > maxVersionNameLen {
		return name[:maxVersionNameLen]
	}
	return name
}
