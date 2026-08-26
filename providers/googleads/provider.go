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

// Provider is the Agoraform Google Ads provider.
//
// It registers googleads.conversion_action and shares a reusable REST client
// for authenticated query and mutate operations.
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
	_ provider.Normalizer        = (*Provider)(nil)
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
func (p *Provider) ResourceTypes() []string {
	return []string{TypeConversionAction}
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
	switch res.Address.Type {
	case TypeConversionAction:
		return p.validateConversionAction(res)
	default:
		return nil
	}
}

// Read implements provider.Provider.
func (p *Provider) Read(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	switch res.Address.Type {
	case TypeConversionAction:
		return p.readConversionAction(ctx, res)
	default:
		return resource.RemoteResource{}, notImplemented("read", res.Address)
	}
}

// Create implements provider.Provider.
func (p *Provider) Create(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	switch res.Address.Type {
	case TypeConversionAction:
		return p.createConversionAction(ctx, res)
	default:
		return resource.RemoteResource{}, notImplemented("create", res.Address)
	}
}

// Update implements provider.Provider.
func (p *Provider) Update(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	switch desired.Address.Type {
	case TypeConversionAction:
		return p.updateConversionAction(ctx, desired, actual)
	default:
		return resource.RemoteResource{}, notImplemented("update", desired.Address)
	}
}

// Import implements provider.Provider.
func (p *Provider) Import(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	switch addr.Type {
	case TypeConversionAction:
		return p.importConversionAction(ctx, addr, id)
	default:
		return resource.RemoteResource{}, notImplemented("import", addr)
	}
}

// NormalizeComparable implements provider.Normalizer.
func (p *Provider) NormalizeComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	switch desired.Address.Type {
	case TypeConversionAction:
		return p.normalizeConversionActionComparable(desired, live)
	default:
		want := desired.Attributes.Clone()
		if live == nil {
			return want, nil, nil
		}
		return want, live.Attributes.Clone(), nil
	}
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
