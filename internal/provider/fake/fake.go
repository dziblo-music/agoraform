// Package fake implements an in-memory Agoraform provider for tests.
//
// It exists to prove the core provider contract without importing any
// vendor-specific (for example Matomo) types into the core.
package fake

import (
	"context"
	"fmt"
	"strings"
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

	// AttrParent is an optional reference to another fake.widget resource.
	AttrParent = "parent"

	// AttrAlso is an optional second reference, used to test branching
	// dependencies where one resource depends on two others.
	AttrAlso = "also"

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

	reads    int
	creates  int
	updates  int
	imports  int
	destroys int
}

var (
	_ provider.Provider  = (*Provider)(nil)
	_ provider.Destroyer = (*Provider)(nil)
)

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

// Remove drops a live resource by identity. Tests use this to simulate a
// remote object disappearing after import or create.
func (p *Provider) Remove(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	key, ok := p.byID[id]
	if !ok {
		return false
	}
	delete(p.resources, key)
	delete(p.byID, id)
	return true
}

// Calls returns how many times each lifecycle method has been invoked.
func (p *Provider) Calls() (reads, creates, updates, imports int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reads, p.creates, p.updates, p.imports
}

// Destroys returns how many times Destroy has been invoked.
func (p *Provider) Destroys() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.destroys
}

// DestroyCapability implements provider.Destroyer.
func (p *Provider) DestroyCapability(resource.Resource) (provider.DestroyCapability, error) {
	return provider.DestroySupported, nil
}

// Destroy implements provider.Destroyer.
func (p *Provider) Destroy(_ context.Context, res resource.Resource) (provider.DestroyResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.destroys++

	id := strings.TrimSpace(res.Identity.ID)
	if id == "" {
		return provider.DestroyResult{}, fmt.Errorf("resource %s: missing identity", res.Address)
	}
	key, ok := p.byID[id]
	if !ok {
		return provider.DestroyResult{Status: provider.DestroyStatusAlreadyAbsent}, nil
	}
	delete(p.resources, key)
	delete(p.byID, id)
	return provider.DestroyResult{Status: provider.DestroyStatusDestroyed}, nil
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
	if parent, exists := res.Attributes[AttrParent]; exists {
		if err := validateParent(res.Address, parent); err != nil {
			return err
		}
	}
	if also, exists := res.Attributes[AttrAlso]; exists {
		if err := validateReference(res.Address, AttrAlso, also); err != nil {
			return err
		}
	}
	for key := range res.Attributes {
		switch key {
		case AttrTitle, AttrColor, AttrParent, AttrAlso:
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

	if !res.Identity.IsZero() {
		key, ok := p.byID[res.Identity.ID]
		if !ok {
			return resource.RemoteResource{}, provider.ErrNotFound
		}
		remote := cloneRemote(p.resources[key])
		remote.Address = res.Address
		return remote, nil
	}

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
		Attributes: logicalAttributes(res.Attributes),
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

	key := desired.Address.String()
	if !desired.Identity.IsZero() {
		var ok bool
		key, ok = p.byID[desired.Identity.ID]
		if !ok {
			return resource.RemoteResource{}, provider.ErrNotFound
		}
	}
	existing, ok := p.resources[key]
	if !ok {
		return resource.RemoteResource{}, provider.ErrNotFound
	}

	p.nextSerial++
	existing.Attributes = logicalAttributes(desired.Attributes)
	if existing.Computed == nil {
		existing.Computed = resource.Attributes{}
	}
	existing.Computed[AttrSerial] = p.nextSerial
	p.storeLocked(existing)
	live := cloneRemote(existing)
	live.Address = desired.Address
	return live, nil
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
	remote := cloneRemote(p.resources[currentAddr])
	remote.Address = addr
	return remote, nil
}

func (p *Provider) storeLocked(remote resource.RemoteResource) {
	key := remote.Address.String()
	p.resources[key] = cloneRemote(remote)
	if !remote.Identity.IsZero() {
		p.byID[remote.Identity.ID] = key
	}
}

func validateParent(addr resource.Address, parent any) error {
	return validateReference(addr, AttrParent, parent)
}

func validateReference(addr resource.Address, attr string, v any) error {
	var target resource.Address
	if resolved, ok := resource.AsResolved(v); ok {
		target = resolved.Address
	} else {
		ref, ok := resource.AsRef(v)
		if !ok {
			return fmt.Errorf("resource %s: attribute %q must be a resource reference", addr, attr)
		}
		target = ref.Address
	}
	if target.Provider != Name || target.Type != TypeWidget {
		return fmt.Errorf("resource %s: attribute %q must reference a %s.%s resource", addr, attr, Name, TypeWidget)
	}
	return nil
}

// logicalAttributes stores comparable configuration, converting runtime
// Resolved bindings back to logical Refs so later plans do not treat
// identities as attribute drift.
func logicalAttributes(attrs resource.Attributes) resource.Attributes {
	return replaceResolvedWithRef(attrs.Clone()).(resource.Attributes)
}

func replaceResolvedWithRef(v any) any {
	if resolved, ok := resource.AsResolved(v); ok {
		return resource.Ref{Address: resolved.Address}
	}
	switch x := v.(type) {
	case resource.Attributes:
		out := make(resource.Attributes, len(x))
		for k, val := range x {
			out[k] = replaceResolvedWithRef(val)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = replaceResolvedWithRef(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = replaceResolvedWithRef(val)
		}
		return out
	default:
		return v
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
