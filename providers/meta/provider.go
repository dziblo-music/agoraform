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

// Provider is the Agoraform Meta Ads provider.
type Provider struct {
	cfg    Config
	once   sync.Once
	client *client.Client
	err    error

	mu         sync.Mutex
	known      map[string]remoteBinding
	identities IdentityCatalog
}

var (
	_ provider.Provider               = (*Provider)(nil)
	_ provider.ConnectionChecker      = (*Provider)(nil)
	_ provider.Configurator           = (*Provider)(nil)
	_ provider.Normalizer             = (*Provider)(nil)
	_ provider.ImportIDNormalizer     = (*Provider)(nil)
	_ provider.Destroyer              = (*Provider)(nil)
	_ provider.OutputCatalog          = (*Provider)(nil)
	_ provider.MissingResourcePlanner = (*Provider)(nil)
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

func (p *Provider) ResourceTypes() []string {
	return []string{TypePixel, TypeCustomConversion, TypeCampaign}
}

// Outputs implements provider.OutputCatalog.
func (p *Provider) Outputs(resourceType string) []provider.OutputSpec {
	switch resourceType {
	case TypePixel:
		return []provider.OutputSpec{{Name: OutputPixelID, Kind: provider.OutputKindString}}
	case TypeCustomConversion:
		return []provider.OutputSpec{{Name: OutputCustomConversionID, Kind: provider.OutputKindString}}
	case TypeCampaign:
		return []provider.OutputSpec{{Name: OutputCampaignID, Kind: provider.OutputKindString}}
	default:
		return nil
	}
}

// PlanMissingResource implements provider.MissingResourcePlanner.
func (p *Provider) PlanMissingResource(res resource.Resource) (provider.MissingResourceMode, error) {
	if res.Address.Provider == Name && res.Address.Type == TypePixel {
		return provider.MissingResourceAdopt, nil
	}
	return provider.MissingResourceCreate, nil
}

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
	if !provider.Supports(p, res.Address.Type) {
		return fmt.Errorf("resource %s: unknown type %q for provider %q", res.Address, res.Address.Type, Name)
	}
	switch res.Address.Type {
	case TypePixel:
		return p.validatePixel(res)
	case TypeCustomConversion:
		return p.validateCustomConversion(res)
	case TypeCampaign:
		return p.validateCampaign(res)
	default:
		return fmt.Errorf("resource %s: unknown type %q for provider %q", res.Address, res.Address.Type, Name)
	}
}

func (p *Provider) Read(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	switch res.Address.Type {
	case TypePixel:
		return p.readPixel(ctx, res)
	case TypeCustomConversion:
		return p.readCustomConversion(ctx, res)
	case TypeCampaign:
		return p.readCampaign(ctx, res)
	default:
		return resource.RemoteResource{}, fmt.Errorf("meta: read is not implemented for this resource type")
	}
}

func (p *Provider) Create(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	switch res.Address.Type {
	case TypePixel:
		return p.createPixel(ctx, res)
	case TypeCustomConversion:
		return p.createCustomConversion(ctx, res)
	case TypeCampaign:
		return p.createCampaign(ctx, res)
	default:
		return resource.RemoteResource{}, fmt.Errorf("meta: create is not implemented for this resource type")
	}
}

func (p *Provider) Update(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	switch desired.Address.Type {
	case TypePixel:
		return p.updatePixel(ctx, desired, actual)
	case TypeCustomConversion:
		return p.updateCustomConversion(ctx, desired, actual)
	case TypeCampaign:
		return p.updateCampaign(ctx, desired, actual)
	default:
		return resource.RemoteResource{}, fmt.Errorf("meta: update is not implemented for this resource type")
	}
}

func (p *Provider) Import(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	switch addr.Type {
	case TypePixel:
		return p.importPixel(ctx, addr, id)
	case TypeCustomConversion:
		return p.importCustomConversion(ctx, addr, id)
	case TypeCampaign:
		return p.importCampaign(ctx, addr, id)
	default:
		return resource.RemoteResource{}, fmt.Errorf("meta: import is not implemented for this resource type")
	}
}

// NormalizeImportID implements provider.ImportIDNormalizer.
func (p *Provider) NormalizeImportID(addr resource.Address, raw string) (string, error) {
	switch addr.Type {
	case TypePixel:
		return p.canonicalPixelImportID(addr, raw)
	case TypeCustomConversion, TypeCampaign:
		return p.canonicalCustomConversionImportID(addr, raw)
	default:
		return strings.TrimSpace(raw), nil
	}
}

// NormalizeComparable implements provider.Normalizer.
func (p *Provider) NormalizeComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	switch desired.Address.Type {
	case TypePixel:
		return p.normalizePixelComparable(desired, live)
	case TypeCustomConversion:
		return p.normalizeCustomConversionComparable(desired, live)
	case TypeCampaign:
		return p.normalizeCampaignComparable(desired, live)
	default:
		return desired.Attributes.Clone(), nil, nil
	}
}

func (p *Provider) requireConfig() error {
	return missingConfigError(p.cfg)
}

func (p *Provider) requireAdAccount() error {
	if strings.TrimSpace(p.cfg.AdAccountID) == "" {
		return fmt.Errorf("meta: %s is required", EnvAdAccountID)
	}
	return nil
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
