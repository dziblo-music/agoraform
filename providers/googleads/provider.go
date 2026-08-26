package googleads

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads/client"
)

const (
	// Name is the provider identifier used in resource addresses.
	Name = "googleads"
)

// Provider is the Agoraform Google Ads provider foundation.
//
// Resource types are registered by follow-up issues. This package provides
// authenticated, testable API access for those resources.
type Provider struct {
	cfg    Config
	once   sync.Once
	client *client.Client
	err    error
}

var (
	_ provider.Provider          = (*Provider)(nil)
	_ provider.ConnectionChecker = (*Provider)(nil)
	_ provider.Configurator      = (*Provider)(nil)
)

// New returns a Google Ads provider using cfg.
//
// Construction does not contact Google Ads. Missing or invalid settings are
// reported by Client or CheckConnection.
func New(cfg Config) *Provider {
	return &Provider{cfg: cfg.WithDefaults()}
}

// NewFromEnv returns a provider configured from GOOGLE_ADS_* environment
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
// The foundation does not manage resources yet. Conversion actions land in a
// follow-up issue.
func (p *Provider) ResourceTypes() []string {
	return nil
}

// Client returns the reusable Google Ads HTTP client, creating it on first use.
func (p *Provider) Client() (*client.Client, error) {
	if p == nil {
		return nil, fmt.Errorf("googleads: provider is nil")
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

// Configure implements provider.Configurator.
//
// Google Ads credentials stay in the environment. The foundation has no
// non-secret declarative fields yet; unknown keys are rejected so secrets
// cannot be smuggled into the manifest.
func (p *Provider) Configure(config resource.Attributes) error {
	if p == nil {
		return fmt.Errorf("googleads: provider is nil")
	}
	if len(config) == 0 {
		return nil
	}
	keys := make([]string, 0, len(config))
	for key := range config {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if isCredentialConfigField(key) {
			return fmt.Errorf("googleads: credentials must come from environment variables, not the manifest")
		}
		return fmt.Errorf("googleads: unknown provider configuration field %q", key)
	}
	return nil
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
	return fmt.Errorf("googleads: %s %s: resource type %q is not implemented", op, addr, addr.Type)
}

func missingConfigError(cfg Config) error {
	if strings.TrimSpace(cfg.DeveloperToken) == "" {
		return fmt.Errorf("googleads: %s is required", EnvDeveloperToken)
	}
	if strings.TrimSpace(cfg.ClientID) == "" {
		return fmt.Errorf("googleads: %s is required", EnvClientID)
	}
	if strings.TrimSpace(cfg.ClientSecret) == "" {
		return fmt.Errorf("googleads: %s is required", EnvClientSecret)
	}
	if strings.TrimSpace(cfg.RefreshToken) == "" {
		return fmt.Errorf("googleads: %s is required", EnvRefreshToken)
	}
	if strings.TrimSpace(cfg.CustomerID) == "" {
		return fmt.Errorf("googleads: %s is required", EnvCustomerID)
	}
	return nil
}

func isCredentialConfigField(key string) bool {
	switch strings.ToLower(strings.ReplaceAll(key, "_", "")) {
	case "developertoken", "clientid", "clientsecret", "refreshtoken", "accesstoken", "token":
		return true
	default:
		return false
	}
}
