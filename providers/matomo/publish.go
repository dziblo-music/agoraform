package matomo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

const maxVersionNameLen = 50

var publishClock = time.Now

// Configure implements provider.Configurator. Publication is desired state,
// not a credential or connection setting, so it belongs in the manifest.
func (p *Provider) Configure(config resource.Attributes) error {
	if p == nil {
		return fmt.Errorf("matomo: provider is nil")
	}

	enabled := false
	environment := client.DefaultEnvironment
	for key := range config {
		switch key {
		case "publish", "environment":
		default:
			return fmt.Errorf("matomo: unknown provider configuration field %q", key)
		}
	}
	if raw, ok := config["publish"]; ok {
		value, ok := raw.(bool)
		if !ok {
			return fmt.Errorf("matomo: provider configuration publish must be true or false")
		}
		enabled = value
	}
	if raw, ok := config["environment"]; ok {
		value, ok := raw.(string)
		if !ok {
			return fmt.Errorf("matomo: provider configuration environment must be a string")
		}
		environment = strings.TrimSpace(value)
		if environment == "" {
			return fmt.Errorf("matomo: provider configuration environment must not be empty")
		}
	}

	p.mu.Lock()
	p.publishEnabled = enabled
	p.publishEnvironment = environment
	p.mu.Unlock()
	return nil
}

// PlanFinalization implements provider.Finalizer. It never mutates Matomo.
// When draft resource changes are already planned, publication is visible in
// the same plan even if the current pre-apply draft still matches live.
func (p *Provider) PlanFinalization(ctx context.Context, pending []provider.PendingChange) (*provider.FinalizationPlan, error) {
	enabled, environment := p.publicationSettings()
	if !enabled {
		return nil, nil
	}
	if err := p.requirePublishConfig(); err != nil {
		return nil, err
	}
	if err := p.CheckConnection(ctx); err != nil {
		return nil, err
	}

	tm, containerID, pendingCreate, err := p.publicationTagManager(pending)
	if err != nil {
		return nil, err
	}
	if err := p.ensurePublishEnvironment(ctx, tm, environment); err != nil {
		return nil, err
	}

	addr := p.publicationAddress()
	conditional := hasPendingTagManagerChange(pending)
	if pendingCreate {
		return &provider.FinalizationPlan{
			Address:     addr,
			Action:      "publish",
			Target:      environment,
			Conditional: true,
		}, nil
	}

	container, err := tm.GetContainer(ctx)
	if err != nil {
		return nil, fmt.Errorf("matomo: read container for publication plan: %w", err)
	}
	if strings.TrimSpace(container.DraftVersion) == "" {
		return nil, fmt.Errorf("matomo: Tag Manager container %s has no draft version", containerID)
	}

	needed := conditional
	if !conditional {
		needed, err = p.publicationNeeded(ctx, tm, container, environment)
		if err != nil {
			return nil, err
		}
	}
	if !needed {
		return nil, nil
	}

	return &provider.FinalizationPlan{
		Address:     addr,
		Action:      "publish",
		Target:      environment,
		Conditional: conditional,
	}, nil
}

// Finalize implements provider.Finalizer. It runs only after all planned
// resource mutations have succeeded. It rechecks idempotency before creating a
// version so a stale or externally converged plan does not create duplicates.
func (p *Provider) Finalize(ctx context.Context, planned provider.FinalizationPlan) (provider.FinalizationResult, error) {
	result := provider.FinalizationResult{Address: p.publicationAddress()}
	if planned.Address.Provider != "" {
		result.Address = planned.Address
	}
	enabled, environment := p.publicationSettings()
	if !enabled {
		return result, fmt.Errorf("matomo: publication is disabled by provider configuration")
	}
	if strings.TrimSpace(planned.Action) != "publish" {
		return result, fmt.Errorf("matomo: unsupported finalization action %q", planned.Action)
	}
	if target := strings.TrimSpace(planned.Target); target != "" && target != environment {
		return result, fmt.Errorf("matomo: planned environment %q does not match configured environment %q", target, environment)
	}
	if err := p.requirePublishConfig(); err != nil {
		return result, err
	}
	if err := p.CheckConnection(ctx); err != nil {
		return result, err
	}

	tm, containerID, _, err := p.publicationTagManager(nil)
	if err != nil {
		return result, err
	}
	if err := p.ensurePublishEnvironment(ctx, tm, environment); err != nil {
		return result, err
	}

	container, err := tm.GetContainer(ctx)
	if err != nil {
		return result, fmt.Errorf("matomo: read container for publication: %w", err)
	}
	if strings.TrimSpace(container.DraftVersion) == "" {
		return result, fmt.Errorf("matomo: Tag Manager container %s has no draft version", containerID)
	}
	needed, err := p.publicationNeeded(ctx, tm, container, environment)
	if err != nil {
		return result, err
	}
	if !needed {
		result.Details = append(result.Details, "no publication required")
		return result, nil
	}

	versionName := newContainerVersionName(publishClock())
	versionID, err := tm.CreateContainerVersion(ctx, versionName, "")
	if err != nil {
		return result, fmt.Errorf("matomo: create container version: %w", err)
	}
	result.Changed = true
	result.Details = append(result.Details, fmt.Sprintf("version %s created", versionID))
	if err := tm.PublishContainerVersion(ctx, versionID, environment); err != nil {
		return result, fmt.Errorf("matomo: publish container version %s to %s: %w", versionID, environment, err)
	}
	result.Details = append(result.Details, fmt.Sprintf("published to %s", environment))
	return result, nil
}

func (p *Provider) publicationSettings() (bool, string) {
	if p == nil {
		return false, client.DefaultEnvironment
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	environment := strings.TrimSpace(p.publishEnvironment)
	if environment == "" {
		environment = client.DefaultEnvironment
	}
	return p.publishEnabled, environment
}

func (p *Provider) requirePublishConnection() error {
	if p == nil {
		return fmt.Errorf("matomo: provider is nil")
	}
	if err := missingConfigError(p.cfg); err != nil {
		return err
	}
	if strings.TrimSpace(p.cfg.SiteID) == "" {
		return fmt.Errorf("matomo: %s is required when provider publication is enabled", EnvSiteID)
	}
	return nil
}

func (p *Provider) requirePublishConfig() error {
	if err := p.requirePublishConnection(); err != nil {
		return err
	}
	if p.hasManagedContainer() {
		return nil
	}
	if strings.TrimSpace(p.cfg.ContainerID) == "" {
		return fmt.Errorf("matomo: %s is required when provider publication is enabled without a managed matomo.container resource", EnvContainerID)
	}
	return nil
}

func (p *Provider) publicationTagManager(pending []provider.PendingChange) (*client.TagManager, string, bool, error) {
	c, err := p.Client()
	if err != nil {
		return nil, "", false, err
	}
	if pendingContainerCreate(pending) {
		return c.TagManager(), "", true, nil
	}
	id, err := p.publicationContainerID()
	if err != nil {
		return nil, "", false, err
	}
	return c.TagManager().ForContainer(id), id, false, nil
}

func (p *Provider) publicationContainerID() (string, error) {
	if addr := p.managedContainerAddress(); !addr.IsZero() {
		if id := p.lookupID(addr); id != "" {
			return id, nil
		}
		return "", fmt.Errorf("matomo: managed container %s has no provider-native identity", addr)
	}
	if id := strings.TrimSpace(p.cfg.ContainerID); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("matomo: %s is required when provider publication is enabled without a managed matomo.container resource", EnvContainerID)
}

func pendingContainerCreate(pending []provider.PendingChange) bool {
	for _, change := range pending {
		if change.Address.Provider == Name && change.Address.Type == TypeContainer && change.Action == "create" {
			return true
		}
	}
	return false
}

func (p *Provider) ensurePublishEnvironment(ctx context.Context, tm *client.TagManager, environment string) error {
	publishable, err := tm.GetAvailableEnvironmentsWithPublishCapability(ctx)
	if err != nil {
		return fmt.Errorf("matomo: list publishable environments: %w", err)
	}
	for _, env := range publishable {
		if env.ID == environment {
			return nil
		}
	}

	available, err := tm.GetAvailableEnvironments(ctx)
	if err != nil {
		return fmt.Errorf("matomo: list Tag Manager environments: %w", err)
	}
	for _, env := range available {
		if env.ID == environment {
			return fmt.Errorf("matomo: current credentials cannot publish to Tag Manager environment %q", environment)
		}
	}
	return fmt.Errorf("matomo: environment %q is not a valid Tag Manager publish target", environment)
}

func hasPendingTagManagerChange(pending []provider.PendingChange) bool {
	for _, change := range pending {
		if change.Address.Provider != Name {
			continue
		}
		switch change.Address.Type {
		case TypeContainer, TypeVariable, TypeTrigger, TypeTag:
			return true
		}
	}
	return false
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
