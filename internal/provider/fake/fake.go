// Package fake implements an in-memory Agoraform provider for tests.
//
// It exists to prove the core provider contract without importing any
// vendor-specific (for example Matomo) types into the core.
package fake

import (
	"context"
	"fmt"
	"sync"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

const (
	// Name is the fake provider identifier used in resource addresses.
	Name = "fake"

	// TypeWidget is the single resource type managed by this provider.
	TypeWidget = "widget"

	// AttrTitle is the required configurable widget title.
	AttrTitle = "title"

	// AttrColor is an optional configurable widget color.
	AttrColor = "color"

	// AttrSerial is a computed (read-only) widget serial number.
	AttrSerial = "serial"
)

// Provider is an in-memory test provider.
type Provider struct {
	mu         sync.Mutex
	nextID     int
	nextSerial int
	resources  map[string]resource.RemoteResource // keyed by address.String()
	byID       map[string]string                  // identity ID -> address.String()

	reads   int
	creates int
	updates int
	imports int
}

var _ provider.Provider = (*Provider)(nil)

// New returns an empty fake provider.
func New() *Provider {
	return &Provider{
		resources: make(map[string]resource.RemoteResource),
		byID:      make(map[string]string),
	}
}

// Name implements provider.Provider.
func (p *Provider) Name() string { return Name }

// ResourceTypes implements provider.Provider.
func (p *Provider) ResourceTypes() []string { return []string{TypeWidget} }

// Seed stores a live resource without going through Create. Tests use this
// to set up remote state.
func (p *Provider) Seed(remote resource.RemoteResource) error {
	if err := remote.Address.Validate(); err != nil {
		return err
	}
	if remote.Address.Provider != Name {
		return fmt.Errorf("fake provider cannot seed address %s", remote.Address)
	}
	if remote.Address.Type != TypeWidget {
		return fmt.Errorf("fake provider cannot seed type %q", remote.Address.Type)
	}
	if remote.Attributes == nil {
		remote.Attributes = resource.Attributes{}
	}
	if remote.Computed == nil {
		remote.Computed = resource.Attributes{}
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.storeLocked(remote)
	return nil
}

// Calls returns how many times each lifecycle method has been invoked.
func (p *Provider) Calls() (reads, creates, updates, imports int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reads, p.creates, p.updates, p.imports
}

// Validate implements provider.Provider.
func (p *Provider) Validate(_ context.Context, res resource.Resource) error {
	if res.Address.Provider != Name {
		return fmt.Errorf("resource %s: unsupported provider %q", res.Address, res.Address.Provider)
	}
	if res.Address.Type != TypeWidget {
		return fmt.Errorf("resource %s: unknown type %q for provider %q", res.Address, res.Address.Type, Name)
	}
	if _, ok := res.Attributes[AttrSerial]; ok {
		return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, AttrSerial)
	}
	title, ok := res.Attributes[AttrTitle]
	if !ok {
		return fmt.Errorf("resource %s: missing required attribute %q", res.Address, AttrTitle)
	}
	s, ok := title.(string)
	if !ok || s == "" {
		return fmt.Errorf("resource %s: attribute %q must be a non-empty string", res.Address, AttrTitle)
	}
	if color, exists := res.Attributes[AttrColor]; exists {
		if _, ok := color.(string); !ok {
			return fmt.Errorf("resource %s: attribute %q must be a string", res.Address, AttrColor)
		}
	}
	for key := range res.Attributes {
		switch key {
		case AttrTitle, AttrColor:
		default:
			return fmt.Errorf("resource %s: unknown attribute %q", res.Address, key)
		}
	}
	return nil
}

// Read implements provider.Provider.
func (p *Provider) Read(_ context.Context, res resource.Resource) (resource.RemoteResource, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reads++

	remote, ok := p.resources[res.Address.String()]
	if !ok {
		return resource.RemoteResource{}, provider.ErrNotFound
	}
	return cloneRemote(remote), nil
}

// Create implements provider.Provider.
func (p *Provider) Create(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.Validate(ctx, res); err != nil {
		return resource.RemoteResource{}, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.creates++

	if _, exists := p.resources[res.Address.String()]; exists {
		return resource.RemoteResource{}, fmt.Errorf("resource %s already exists", res.Address)
	}

	p.nextID++
	p.nextSerial++
	remote := resource.RemoteResource{
		Address:    res.Address,
		Identity:   resource.Identity{ID: fmt.Sprintf("widget-%d", p.nextID)},
		Attributes: res.Attributes.Clone(),
		Computed: resource.Attributes{
			AttrSerial: p.nextSerial,
		},
	}
	p.storeLocked(remote)
	return cloneRemote(remote), nil
}

// Update implements provider.Provider.
func (p *Provider) Update(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.Validate(ctx, desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if desired.Address.String() != actual.Address.String() {
		return resource.RemoteResource{}, fmt.Errorf("update address mismatch: desired %s, actual %s", desired.Address, actual.Address)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.updates++

	existing, ok := p.resources[desired.Address.String()]
	if !ok {
		return resource.RemoteResource{}, provider.ErrNotFound
	}

	p.nextSerial++
	existing.Attributes = desired.Attributes.Clone()
	if existing.Computed == nil {
		existing.Computed = resource.Attributes{}
	}
	existing.Computed[AttrSerial] = p.nextSerial
	p.storeLocked(existing)
	return cloneRemote(existing), nil
}

// Import implements provider.Provider.
func (p *Provider) Import(_ context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	if id == "" {
		return resource.RemoteResource{}, fmt.Errorf("import id is empty")
	}
	if err := addr.Validate(); err != nil {
		return resource.RemoteResource{}, err
	}
	if addr.Provider != Name || addr.Type != TypeWidget {
		return resource.RemoteResource{}, fmt.Errorf("cannot import %s into fake provider", addr)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.imports++

	currentAddr, ok := p.byID[id]
	if !ok {
		return resource.RemoteResource{}, provider.ErrNotFound
	}
	remote := p.resources[currentAddr]
	if currentAddr != addr.String() {
		delete(p.resources, currentAddr)
		remote.Address = addr
		p.storeLocked(remote)
	}
	return cloneRemote(remote), nil
}

func (p *Provider) storeLocked(remote resource.RemoteResource) {
	key := remote.Address.String()
	p.resources[key] = cloneRemote(remote)
	if !remote.Identity.IsZero() {
		p.byID[remote.Identity.ID] = key
	}
}

func cloneRemote(remote resource.RemoteResource) resource.RemoteResource {
	return resource.RemoteResource{
		Address:    remote.Address,
		Identity:   remote.Identity,
		Attributes: remote.Attributes.Clone(),
		Computed:   remote.Computed.Clone(),
	}
}
