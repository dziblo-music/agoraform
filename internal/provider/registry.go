package provider

import (
	"fmt"
	"sync"

	"github.com/dziblo-music/agoraform/internal/resource"
)

// Registry stores named providers for lookup during validation and later
// plan/apply operations.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry returns an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

// Register adds a provider. The provider name must be unique and a valid
// address provider segment.
func (r *Registry) Register(p Provider) error {
	if r == nil {
		return fmt.Errorf("provider registry is nil")
	}
	if p == nil {
		return fmt.Errorf("provider is nil")
	}

	name := p.Name()
	if err := validateProviderName(name); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("provider %q is already registered", name)
	}
	r.providers[name] = p
	return nil
}

// Lookup returns a provider by name.
func (r *Registry) Lookup(name string) (Provider, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// LookupFor returns the provider that should manage addr.
func (r *Registry) LookupFor(addr resource.Address) (Provider, error) {
	p, ok := r.Lookup(addr.Provider)
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", addr.Provider)
	}
	if !Supports(p, addr.Type) {
		return nil, fmt.Errorf("unknown resource type %q for provider %q", addr.Type, addr.Provider)
	}
	return p, nil
}

// Len returns the number of registered providers.
func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.providers)
}

func validateProviderName(name string) error {
	addr := resource.Address{Provider: name, Type: "placeholder", Name: "placeholder"}
	if err := addr.Validate(); err != nil {
		return fmt.Errorf("provider name %q is invalid: use a lowercase letter followed by letters, digits, or underscores", name)
	}
	return nil
}
