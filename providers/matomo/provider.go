package matomo

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

const (
	// Name is the provider identifier used in resource addresses.
	Name = "matomo"
)

// IdentityCatalog looks up logical addresses for provider-native identities
// already bound in local state. Import uses it to reconstruct `$ref`
// relationships without embedding Matomo-native ids in configuration.
type IdentityCatalog interface {
	AddressByRemoteID(provider, resourceType, remoteID string) (resource.Address, bool, error)
}

// Provider is the Agoraform Matomo provider.
//
// It registers matomo.goal, matomo.variable, matomo.trigger, and
// matomo.tag and shares a single HTTP client across analytics and Tag
// Manager resource types.
type Provider struct {
	cfg    Config
	once   sync.Once
	client *client.Client
	err    error

	mu                 sync.Mutex
	known              map[string]remoteBinding
	identities         IdentityCatalog
	publishEnabled     bool
	publishEnvironment string
}

var (
	_ provider.Provider          = (*Provider)(nil)
	_ provider.ConnectionChecker = (*Provider)(nil)
	_ provider.Normalizer        = (*Provider)(nil)
	_ provider.Configurator      = (*Provider)(nil)
	_ provider.Finalizer         = (*Provider)(nil)
)

// New returns a Matomo provider using cfg.
//
// Construction does not contact Matomo. Missing or invalid settings are
// reported by Client or CheckConnection.
func New(cfg Config) *Provider {
	return &Provider{
		cfg:                cfg.WithDefaults(),
		publishEnvironment: client.DefaultEnvironment,
	}
}

// NewFromEnv returns a provider configured from MATOMO_* environment
// variables.
func NewFromEnv() *Provider {
	return New(ConfigFromEnv())
}

// NewWithHTTPClient returns a provider that uses httpClient for tests.
func NewWithHTTPClient(cfg Config, httpClient *http.Client) *Provider {
	cfg.HTTPClient = httpClient
	p := New(cfg)
	if httpClient != nil {
		c, err := client.New(cfg)
		p.client = c
		p.err = err
	}
	return p
}

// SetIdentityCatalog supplies local-state reverse lookups for import
// reference reconstruction. Passing nil clears the catalog.
func (p *Provider) SetIdentityCatalog(c IdentityCatalog) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.identities = c
}

// Name implements provider.Provider.
func (p *Provider) Name() string { return Name }

// ResourceTypes implements provider.Provider.
func (p *Provider) ResourceTypes() []string {
	return []string{TypeGoal, TypeVariable, TypeTrigger, TypeTag}
}

// Client returns the reusable Matomo HTTP client, creating it on first use.
func (p *Provider) Client() (*client.Client, error) {
	if p == nil {
		return nil, fmt.Errorf("matomo: provider is nil")
	}
	p.once.Do(func() {
		if p.client != nil || p.err != nil {
			return
		}
		p.client, p.err = client.New(p.cfg)
	})
	return p.client, p.err
}

// CheckConnection implements provider.ConnectionChecker.
func (p *Provider) CheckConnection(ctx context.Context) error {
	if err := missingConfigError(p.cfg); err != nil {
		return err
	}
	c, err := p.Client()
	if err != nil {
		return err
	}
	return c.CheckConnection(ctx)
}

// Validate implements provider.Provider.
func (p *Provider) Validate(_ context.Context, res resource.Resource) error {
	if res.Address.Provider != Name {
		return fmt.Errorf("resource %s: unsupported provider %q", res.Address, res.Address.Provider)
	}
	if !provider.Supports(p, res.Address.Type) {
		return fmt.Errorf("resource %s: unknown type %q for provider %q", res.Address, res.Address.Type, Name)
	}
	switch res.Address.Type {
	case TypeGoal:
		return p.validateGoalSafe(res)
	case TypeVariable:
		return p.validateVariableSafe(res)
	case TypeTrigger:
		return p.validateTriggerSafe(res)
	case TypeTag:
		return p.validateTagSafe(res)
	default:
		return nil
	}
}

// Read implements provider.Provider.
func (p *Provider) Read(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	switch res.Address.Type {
	case TypeGoal:
		return p.readGoalSafe(ctx, res)
	case TypeVariable:
		return p.readVariableSafe(ctx, res)
	case TypeTrigger:
		return p.readTriggerSafe(ctx, res)
	case TypeTag:
		return p.readTagSafe(ctx, res)
	default:
		return resource.RemoteResource{}, notImplemented("read", res.Address)
	}
}

// Create implements provider.Provider.
func (p *Provider) Create(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	switch res.Address.Type {
	case TypeGoal:
		return p.createGoalSafe(ctx, res)
	case TypeVariable:
		return p.createVariableSafe(ctx, res)
	case TypeTrigger:
		return p.createTriggerSafe(ctx, res)
	case TypeTag:
		return p.createTagSafe(ctx, res)
	default:
		return resource.RemoteResource{}, notImplemented("create", res.Address)
	}
}

// Update implements provider.Provider.
func (p *Provider) Update(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	switch desired.Address.Type {
	case TypeGoal:
		return p.updateGoalSafe(ctx, desired, actual)
	case TypeVariable:
		return p.updateVariableSafe(ctx, desired, actual)
	case TypeTrigger:
		return p.updateTriggerSafe(ctx, desired, actual)
	case TypeTag:
		return p.updateTagSafe(ctx, desired, actual)
	default:
		return resource.RemoteResource{}, notImplemented("update", desired.Address)
	}
}

// Import implements provider.Provider.
func (p *Provider) Import(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	switch addr.Type {
	case TypeGoal:
		return p.importGoal(ctx, addr, id)
	case TypeVariable:
		return p.importVariable(ctx, addr, id)
	case TypeTrigger:
		return p.importTrigger(ctx, addr, id)
	case TypeTag:
		return p.importTag(ctx, addr, id)
	default:
		return resource.RemoteResource{}, notImplemented("import", addr)
	}
}

// NormalizeComparable implements provider.Normalizer.
func (p *Provider) NormalizeComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	switch desired.Address.Type {
	case TypeGoal:
		return p.normalizeGoalComparableSafe(desired, live)
	case TypeVariable:
		return p.normalizeVariableComparableSafe(desired, live)
	case TypeTrigger:
		return p.normalizeTriggerComparableSafe(desired, live)
	case TypeTag:
		return p.normalizeTagComparableSafe(desired, live)
	default:
		want := desired.Attributes.Clone()
		if live == nil {
			return want, nil, nil
		}
		return want, live.Attributes.Clone(), nil
	}
}

func notImplemented(op string, addr resource.Address) error {
	return fmt.Errorf("matomo: %s %s: resource type %q is not implemented", op, addr, addr.Type)
}

func missingConfigError(cfg Config) error {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return fmt.Errorf("matomo: %s is required", EnvURL)
	}
	if strings.TrimSpace(cfg.TokenAuth) == "" {
		return fmt.Errorf("matomo: %s is required", EnvTokenAuth)
	}
	return nil
}
