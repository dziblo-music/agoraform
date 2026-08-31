package manifest

import (
	"context"
	"fmt"
	"sort"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

// CheckProviders validates provider configuration and desired resources against
// a provider registry.
//
// When reg is nil or empty, provider and type checks are skipped because the
// registry cannot determine known providers. When providers are registered,
// unknown providers, unsupported provider configuration, ConnectionChecker
// failures, unknown resource types, provider Validate failures, optional
// cross-resource ResourceSetValidator failures, and invalid output
// references are reported.
func CheckProviders(ctx context.Context, m *Manifest, reg *provider.Registry) error {
	if m == nil {
		return fmt.Errorf("manifest is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if reg == nil || reg.Len() == 0 {
		return nil
	}

	origin := m.Origin
	if origin == "" {
		origin = "manifest"
	}

	checked := make(map[string]struct{})
	providerNames := make([]string, 0, len(m.Providers))
	for name := range m.Providers {
		providerNames = append(providerNames, name)
	}
	sort.Strings(providerNames)

	for _, name := range providerNames {
		p, ok := reg.Lookup(name)
		if !ok {
			return fmt.Errorf("%s: providers.%s: unknown provider %q", origin, name, name)
		}
		cfg, ok := p.(provider.Configurator)
		if !ok {
			return fmt.Errorf("%s: providers.%s: provider %q does not support manifest configuration", origin, name, name)
		}
		if err := cfg.Configure(m.Providers[name].Clone()); err != nil {
			return fmt.Errorf("%s: providers.%s: %w", origin, name, err)
		}
		if c, ok := p.(provider.ConnectionChecker); ok {
			if err := c.CheckConnection(ctx); err != nil {
				return fmt.Errorf("%s: providers.%s: provider %q: %w", origin, name, p.Name(), err)
			}
		}
		checked[p.Name()] = struct{}{}
	}

	resourceSetValidators := make(map[string]provider.ResourceSetValidator)
	for i, res := range m.Resources {
		path := fmt.Sprintf("resources[%d]", i)
		p, err := reg.LookupFor(res.Address)
		if err != nil {
			return fmt.Errorf("%s: %s: %s: %w", origin, path, res.Address, err)
		}
		if _, ok := checked[p.Name()]; !ok {
			checked[p.Name()] = struct{}{}
			if c, ok := p.(provider.ConnectionChecker); ok {
				if err := c.CheckConnection(ctx); err != nil {
					return fmt.Errorf("%s: %s: provider %q: %w", origin, path, p.Name(), err)
				}
			}
		}
		if err := p.Validate(ctx, res); err != nil {
			return fmt.Errorf("%s: %s: %w", origin, path, err)
		}
		if validator, ok := p.(provider.ResourceSetValidator); ok {
			resourceSetValidators[p.Name()] = validator
		}
	}

	validatorNames := make([]string, 0, len(resourceSetValidators))
	for name := range resourceSetValidators {
		validatorNames = append(validatorNames, name)
	}
	sort.Strings(validatorNames)
	for _, name := range validatorNames {
		if err := resourceSetValidators[name].ValidateResourceSet(ctx, m.Resources); err != nil {
			return fmt.Errorf("%s: provider %q: %w", origin, name, err)
		}
	}

	if err := provider.ValidateOutputRefs(m.Resources, func(addr resource.Address) (provider.Reader, error) {
		return reg.LookupFor(addr)
	}); err != nil {
		return fmt.Errorf("%s: %w", origin, err)
	}
	return nil
}
