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
// Resource CRUD is intentionally unimplemented in this foundation. The
// provider exists so the CLI can register Matomo, load credentials, and
// share a single HTTP client with later resource types.
type Provider struct {
	cfg    Config
	once   sync.Once
	client *client.Client
	err    error
}

var (
	_ provider.Provider          = (*Provider)(nil)
	_ provider.ConnectionChecker = (*Provider)(nil)
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
//
// v0.1 resource types (goal, Tag Manager objects) are registered in
// follow-up issues once their CRUD is implemented.
func (p *Provider) ResourceTypes() []string { return nil }

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
	return nil
}

// Read implements provider.Provider.
func (p *Provider) Read(_ context.Context, res resource.Resource) (resource.RemoteResource, error) {
	return resource.RemoteResource{}, notImplemented("read", res.Address)
}

// Create implements provider.Provider.
func (p *Provider) Create(_ context.Context, res resource.Resource) (resource.RemoteResource, error) {
	return resource.RemoteResource{}, notImplemented("create", res.Address)
}

// Update implements provider.Provider.
func (p *Provider) Update(_ context.Context, desired resource.Resource, _ resource.RemoteResource) (resource.RemoteResource, error) {
	return resource.RemoteResource{}, notImplemented("update", desired.Address)
}

// Import implements provider.Provider.
func (p *Provider) Import(_ context.Context, addr resource.Address, _ string) (resource.RemoteResource, error) {
	return resource.RemoteResource{}, notImplemented("import", addr)
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
