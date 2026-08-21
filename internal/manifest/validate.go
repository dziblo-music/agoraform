package manifest

import (
	"context"
	"fmt"

	"github.com/dziblo-music/agoraform/internal/provider"
)

// CheckProviders validates desired resources against a provider registry.
//
// When reg is nil or empty, provider and type checks are skipped because the
// registry cannot determine known providers. When providers are registered,
// unknown providers, unknown resource types, and provider Validate failures
// are reported with the resource address.
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

	for i, res := range m.Resources {
		path := fmt.Sprintf("resources[%d]", i)
		p, err := reg.LookupFor(res.Address)
		if err != nil {
			return fmt.Errorf("%s: %s: %s: %w", origin, path, res.Address, err)
		}
		if err := p.Validate(ctx, res); err != nil {
			return fmt.Errorf("%s: %s: %w", origin, path, err)
		}
	}
	return nil
}
