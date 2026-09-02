package meta

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/meta/client"
)

const Name = "meta"

// Provider is the Agoraform Meta Ads provider foundation. Resource types are
// added by the focused v0.6.0 resource issues; this foundation owns shared
// runtime configuration, connection validation, and API plumbing.
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

// New returns a Meta provider without contacting the API.
func New(cfg Config) *Provider { return &Provider{cfg: cfg.WithDefaults()} }

// NewFromEnv returns a provider configured from META_* variables.
func NewFromEnv() *Provider { return New(ConfigFromEnv()) }

// NewWithHTTPClient returns a provider using httpClient for local tests.
func NewWithHTTPClient(cfg Config, httpClient *http.Client) *Provider {
	cfg.HTTPClient = httpClient
	return New(cfg)
}

func (p *Provider) Name() string { return Name }

func (p *Provider) ResourceTypes() []string { return nil }

// Client returns the shared Meta client, constructing it on first use.
func (p *Provider) Client() (*client.Client, error) {
	if p == nil {
		return nil, fmt.Errorf("meta: provider is nil")
	}
	p.once.Do(func() { p.client, p.err = client.New(p.cfg) })
	return p.client, p.err
}

// CheckConnection validates credentials, ads_management permission, and ad
// account access without mutating Meta resources.
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

// Configure rejects manifest fields. Meta credentials and account selection
// are runtime configuration and never belong in Agoraform YAML.
func (p *Provider) Configure(config resource.Attributes) error {
	if p == nil {
		return fmt.Errorf("meta: provider is nil")
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
		if isRuntimeConfigField(key) {
			return fmt.Errorf("meta: credentials and ad account selection must come from environment variables, not the manifest")
		}
		return fmt.Errorf("meta: unknown provider configuration field %q", key)
	}
	return nil
}

func (p *Provider) Validate(_ context.Context, res resource.Resource) error {
	if res.Address.Provider != Name {
		return fmt.Errorf("resource %s: unsupported provider %q", res.Address, res.Address.Provider)
	}
	return fmt.Errorf("resource %s: unknown type %q for provider %q", res.Address, res.Address.Type, Name)
}

func (p *Provider) Read(context.Context, resource.Resource) (resource.RemoteResource, error) {
	return resource.RemoteResource{}, fmt.Errorf("meta: read is not implemented for this resource type")
}

func (p *Provider) Create(context.Context, resource.Resource) (resource.RemoteResource, error) {
	return resource.RemoteResource{}, fmt.Errorf("meta: create is not implemented for this resource type")
}

func (p *Provider) Update(context.Context, resource.Resource, resource.RemoteResource) (resource.RemoteResource, error) {
	return resource.RemoteResource{}, fmt.Errorf("meta: update is not implemented for this resource type")
}

func (p *Provider) Import(context.Context, resource.Address, string) (resource.RemoteResource, error) {
	return resource.RemoteResource{}, fmt.Errorf("meta: import is not implemented for this resource type")
}

func missingConfigError(cfg Config) error {
	if strings.TrimSpace(cfg.AccessToken) == "" {
		return fmt.Errorf("meta: %s is required", EnvAccessToken)
	}
	if strings.TrimSpace(cfg.AdAccountID) == "" {
		return fmt.Errorf("meta: %s is required", EnvAdAccountID)
	}
	return nil
}

func isRuntimeConfigField(key string) bool {
	switch strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", "")) {
	case "accesstoken", "token", "adaccountid", "accountid":
		return true
	default:
		return false
	}
}
