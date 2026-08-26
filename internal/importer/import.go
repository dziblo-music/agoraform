package importer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

// Lookup resolves the mutating provider for a resource address.
type Lookup func(addr resource.Address) (provider.Provider, error)

// Store persists a logical-address to provider-native identity mapping.
//
// *state.Store implements this interface.
type Store interface {
	Identity(addr resource.Address) (resource.Identity, bool, error)
	RecordImport(addr resource.Address, remoteID string) error
}

// Result is a successful import. YAML is a complete v0.1 manifest the user
// can review and add to configuration. Identity is not included in YAML.
type Result struct {
	Address  resource.Address
	Identity resource.Identity
	YAML     string
}

// Run reads an existing remote resource, emits configurable YAML, and
// persists the provider-native identity. It never calls Create or Update.
func Run(ctx context.Context, addr resource.Address, remoteID string, lookup Lookup, st Store) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := addr.Validate(); err != nil {
		return Result{}, fmt.Errorf("import: %w", err)
	}
	remoteID = strings.TrimSpace(remoteID)
	if remoteID == "" {
		return Result{}, fmt.Errorf("import %s: remote identifier is empty", addr)
	}
	if lookup == nil {
		return Result{}, fmt.Errorf("import %s: provider lookup is required", addr)
	}
	if st == nil {
		return Result{}, fmt.Errorf("import %s: state store is required", addr)
	}

	existing, ok, err := st.Identity(addr)
	if err != nil {
		return Result{}, fmt.Errorf("import %s: %w", addr, err)
	}
	if ok {
		return Result{}, fmt.Errorf("import %s: resource is already bound to remote identity %q", addr, existing.ID)
	}

	p, err := lookup(addr)
	if err != nil {
		return Result{}, fmt.Errorf("import %s: %w", addr, err)
	}
	if p == nil {
		return Result{}, fmt.Errorf("import %s: provider is nil", addr)
	}

	if c, ok := p.(provider.ConnectionChecker); ok {
		if err := c.CheckConnection(ctx); err != nil {
			return Result{}, fmt.Errorf("import %s: provider %q: %w", addr, p.Name(), err)
		}
	}

	canonicalID := remoteID
	if n, ok := p.(provider.ImportIDNormalizer); ok {
		canonicalID, err = n.NormalizeImportID(addr, remoteID)
		if err != nil {
			return Result{}, fmt.Errorf("import %s: %w", addr, err)
		}
		canonicalID = strings.TrimSpace(canonicalID)
		if canonicalID == "" {
			return Result{}, fmt.Errorf("import %s: remote identifier is empty", addr)
		}
	}

	live, err := p.Import(ctx, addr, canonicalID)
	if errors.Is(err, provider.ErrNotFound) {
		return Result{}, fmt.Errorf("import %s: remote resource %q was not found: %w", addr, canonicalID, err)
	}
	if err != nil {
		return Result{}, fmt.Errorf("import %s: %w", addr, err)
	}
	if live.Identity.IsZero() {
		return Result{}, fmt.Errorf("import %s: provider returned no identity", addr)
	}
	if live.Identity.ID != canonicalID {
		return Result{}, fmt.Errorf("import %s: provider returned identity %q for requested remote identity %q; refusing to bind a different remote resource", addr, live.Identity.ID, canonicalID)
	}
	if live.Address != addr {
		return Result{}, fmt.Errorf("import %s: provider returned logical address %s for requested address %s; refusing to bind a different resource", addr, live.Address, addr)
	}

	desired := resource.Resource{
		Address:    addr,
		Attributes: configurableAttributes(live),
	}
	if err := p.Validate(ctx, desired); err != nil {
		return Result{}, fmt.Errorf("import %s: remote configuration cannot be represented by the supported schema: %w", addr, err)
	}

	yamlText, err := manifestYAML(addr, desired.Attributes)
	if err != nil {
		return Result{}, fmt.Errorf("import %s: cannot encode configuration: %w", addr, err)
	}

	if err := st.RecordImport(addr, canonicalID); err != nil {
		return Result{}, fmt.Errorf("import %s: could not persist identity: %w", addr, err)
	}

	return Result{
		Address:  addr,
		Identity: resource.Identity{ID: canonicalID},
		YAML:     yamlText,
	}, nil
}

func configurableAttributes(live resource.RemoteResource) resource.Attributes {
	out := live.Attributes.Clone()
	for k := range live.Computed {
		delete(out, k)
	}
	for k, v := range out {
		if v == nil {
			delete(out, k)
		}
	}
	return out
}
