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

// Provider is the Agoraform Matomo provider.
//
// It registers the v0.1 matomo.goal resource type and shares a single
// HTTP client with later Tag Manager resource types.
type Provider struct {
	cfg    Config
	once   sync.Once
	client *client.Client
	err    error
}

var (
	_ provider.Provider          = (*Provider)(nil)
	_ provider.ConnectionChecker = (*Provider)(nil)
	_ provider.Normalizer        = (*Provider)(nil)
)

// New returns a Matomo provider using cfg.
//
// Construction does not contact Matomo. Missing or invalid settings are
// reported by Client or CheckConnection.
func New(cfg Config) *Provider {
	return &Provider{cfg: cfg.WithDefaults()}
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

// Name implements provider.Provider.
func (p *Provider) Name() string { return Name }

// ResourceTypes implements provider.Provider.
func (p *Provider) ResourceTypes() []string { return []string{TypeGoal} }

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
	default:
		return nil
	}
}

// Read implements provider.Provider.
func (p *Provider) Read(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	switch res.Address.Type {
	case TypeGoal:
		return p.readGoalSafe(ctx, res)
	default:
		return resource.RemoteResource{}, notImplemented("read", res.Address)
	}
}

// Create implements provider.Provider.
func (p *Provider) Create(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	switch res.Address.Type {
	case TypeGoal:
		return p.createGoalSafe(ctx, res)
	default:
		return resource.RemoteResource{}, notImplemented("create", res.Address)
	}
}

// Update implements provider.Provider.
func (p *Provider) Update(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	switch desired.Address.Type {
	case TypeGoal:
		return p.updateGoalSafe(ctx, desired, actual)
	default:
		return resource.RemoteResource{}, notImplemented("update", desired.Address)
	}
}

// Import implements provider.Provider.
func (p *Provider) Import(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	switch addr.Type {
	case TypeGoal:
		return p.importGoal(ctx, addr, id)
	default:
		return resource.RemoteResource{}, notImplemented("import", addr)
	}
}

// NormalizeComparable implements provider.Normalizer.
func (p *Provider) NormalizeComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	switch desired.Address.Type {
	case TypeGoal:
		return p.normalizeGoalComparableSafe(desired, live)
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
