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
	// AltName is a second in-memory provider used to test cross-provider
	// output references.
	AltName = "alt"

	// TypeNote is the resource type managed by the alt provider.
	TypeNote = "note"

	// AttrText is the required note text. It may be a literal string or an
	// output reference to another provider's resource.
	AttrText = "text"
)

// Alt is an in-memory consumer provider for cross-provider output tests.
type Alt struct {
	mu        sync.Mutex
	nextID    int
	resources map[string]resource.RemoteResource
	byID      map[string]string
	outputs   provider.OutputMatcher
	reads     int
	creates   int
	updates   int
	imports   int
}

var (
	_ provider.Provider      = (*Alt)(nil)
	_ provider.OutputCatalog = (*Alt)(nil)
)

// NewAlt returns an empty alt provider.
func NewAlt() *Alt {
	return &Alt{
		resources: make(map[string]resource.RemoteResource),
		byID:      make(map[string]string),
	}
}

// SetOutputMatcher supplies a provider-neutral catalog of declared
// non-sensitive outputs for import relationship reconstruction. Passing
// nil clears the matcher.
func (p *Alt) SetOutputMatcher(m provider.OutputMatcher) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.outputs = m
}

// Name implements provider.Provider.
func (p *Alt) Name() string { return AltName }

// ResourceTypes implements provider.Provider.
func (p *Alt) ResourceTypes() []string { return []string{TypeNote} }

// Outputs implements provider.OutputCatalog. Notes expose no selectable
// outputs; they only consume them.
func (p *Alt) Outputs(string) []provider.OutputSpec { return nil }

// Seed stores a live note without going through Create.
func (p *Alt) Seed(remote resource.RemoteResource) error {
	if err := remote.Address.Validate(); err != nil {
		return err
	}
	if remote.Address.Provider != AltName || remote.Address.Type != TypeNote {
		return fmt.Errorf("alt provider cannot seed address %s", remote.Address)
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
func (p *Alt) Calls() (reads, creates, updates, imports int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reads, p.creates, p.updates, p.imports
}

// Validate implements provider.Provider.
func (p *Alt) Validate(_ context.Context, res resource.Resource) error {
	if res.Address.Provider != AltName {
		return fmt.Errorf("resource %s: unsupported provider %q", res.Address, res.Address.Provider)
	}
	if res.Address.Type != TypeNote {
		return fmt.Errorf("resource %s: unknown type %q for provider %q", res.Address, res.Address.Type, AltName)
	}
	text, ok := res.Attributes[AttrText]
	if !ok {
		return fmt.Errorf("resource %s: missing required attribute %q", res.Address, AttrText)
	}
	if err := validateText(res.Address, text); err != nil {
		return err
	}
	for key := range res.Attributes {
		if key != AttrText {
			return fmt.Errorf("resource %s: unknown attribute %q", res.Address, key)
		}
	}
	return nil
}

// Read implements provider.Provider.
func (p *Alt) Read(_ context.Context, res resource.Resource) (resource.RemoteResource, error) {
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
func (p *Alt) Create(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
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
	remote := resource.RemoteResource{
		Address:    res.Address,
		Identity:   resource.Identity{ID: fmt.Sprintf("note-%d", p.nextID)},
		Attributes: storeNoteAttributes(res.Attributes),
		Computed:   resource.Attributes{},
	}
	p.storeLocked(remote)
	return cloneRemote(remote), nil
}

// Update implements provider.Provider.
func (p *Alt) Update(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.Validate(ctx, desired); err != nil {
		return resource.RemoteResource{}, err
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
	existing.Attributes = storeNoteAttributes(desired.Attributes)
	p.storeLocked(existing)
	live := cloneRemote(existing)
	live.Address = desired.Address
	return live, nil
}

// Import implements provider.Provider.
func (p *Alt) Import(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	if strings.TrimSpace(id) == "" {
		return resource.RemoteResource{}, fmt.Errorf("import id is empty")
	}
	if err := addr.Validate(); err != nil {
		return resource.RemoteResource{}, err
	}
	if addr.Provider != AltName || addr.Type != TypeNote {
		return resource.RemoteResource{}, fmt.Errorf("cannot import %s into alt provider", addr)
	}

	p.mu.Lock()
	p.imports++
	currentAddr, ok := p.byID[id]
	var remote resource.RemoteResource
	if ok {
		remote = cloneRemote(p.resources[currentAddr])
		remote.Address = addr
	}
	matcher := p.outputs
	p.mu.Unlock()
	if !ok {
		return resource.RemoteResource{}, provider.ErrNotFound
	}
	return reconstructNoteImport(ctx, matcher, remote)
}

func reconstructNoteImport(ctx context.Context, matcher provider.OutputMatcher, remote resource.RemoteResource) (resource.RemoteResource, error) {
	if matcher == nil {
		return remote, nil
	}
	text, ok := remote.Attributes[AttrText].(string)
	if !ok || text == "" {
		return remote, nil
	}
	ref, result, err := matcher.Match(ctx, provider.OutputMatchQuery{
		Provider:     Name,
		ResourceType: TypeWidget,
		Output:       OutputToken,
		Value:        text,
	})
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if result != provider.OutputMatchUnique {
		return remote, nil
	}
	attrs := remote.Attributes.Clone()
	attrs[AttrText] = ref
	remote.Attributes = attrs
	return remote, nil
}

func (p *Alt) storeLocked(remote resource.RemoteResource) {
	key := remote.Address.String()
	p.resources[key] = cloneRemote(remote)
	if !remote.Identity.IsZero() {
		p.byID[remote.Identity.ID] = key
	}
}

func validateText(addr resource.Address, v any) error {
	switch x := v.(type) {
	case string:
		if x == "" {
			return fmt.Errorf("resource %s: attribute %q must be a non-empty string", addr, AttrText)
		}
		return nil
	case resource.Ref:
		if x.IsZero() || !x.HasOutput() {
			return fmt.Errorf("resource %s: attribute %q must be a string or an output reference", addr, AttrText)
		}
		return nil
	default:
		return fmt.Errorf("resource %s: attribute %q must be a string or an output reference", addr, AttrText)
	}
}

func storeNoteAttributes(attrs resource.Attributes) resource.Attributes {
	out := resource.Attributes{}
	for k, v := range attrs {
		if ref, ok := resource.AsRef(v); ok {
			out[k] = ref
			continue
		}
		out[k] = resource.CloneValue(v)
	}
	return out
}
