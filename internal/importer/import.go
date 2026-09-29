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

// ExternalStore persists reference-only ownership separately from managed import.
//
// *state.Store implements this interface.
type ExternalStore interface {
	Store
	Ownership(addr resource.Address) (resource.Ownership, bool, error)
	SetOwnership(addr resource.Address, id resource.Identity, ownership resource.Ownership) error
}

// Binding is how an import result should be described to the user.
type Binding string

const (
	// BindingManaged is a normal import that takes lifecycle ownership.
	BindingManaged Binding = "managed"

	// BindingExternal is a new reference-only binding.
	BindingExternal Binding = "external"

	// BindingRelease changes a managed binding to external without deleting it.
	BindingRelease Binding = "release"

	// BindingAdopt changes an external binding to managed without mutating it.
	BindingAdopt Binding = "adopt"
)

// Result is a successful import. YAML is a complete v0.1 manifest the user
// can review and add to configuration. Identity is not included in managed
// YAML. External YAML carries lifecycle.id because that identity is the
// lookup key rather than Agoraform-owned state alone.
type Result struct {
	Address  resource.Address
	Identity resource.Identity
	YAML     string
	Binding  Binding
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
		if ext, isExt := st.(ExternalStore); isExt {
			ownership, _, ownErr := ext.Ownership(addr)
			if ownErr != nil {
				return Result{}, fmt.Errorf("import %s: %w", addr, ownErr)
			}
			if ownership == resource.OwnershipExternal {
				return Result{}, fmt.Errorf("import %s: resource is an external reference to remote identity %q; a normal import cannot take ownership. Run `agoraform import --adopt %s`", addr, existing.ID, addr)
			}
		}
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
		Binding:  BindingManaged,
	}, nil
}

// RunExternal reads an existing remote resource and persists it as a
// reference-only binding. release must be set to convert an existing managed
// binding; the remote object is never mutated.
func RunExternal(ctx context.Context, addr resource.Address, remoteID string, lookup Lookup, st ExternalStore, release bool) (Result, error) {
	live, canonicalID, p, err := readImport(ctx, addr, remoteID, lookup, st)
	if err != nil {
		return Result{}, err
	}
	external := resource.Resource{
		Address:    addr,
		Ownership:  resource.OwnershipExternal,
		ExternalID: canonicalID,
	}
	if err := provider.ValidateExternal(p, external); err != nil {
		return Result{}, fmt.Errorf("import %s: %w", addr, err)
	}
	existing, bound, ownership, err := currentOwnership(st, addr)
	if err != nil {
		return Result{}, err
	}

	binding := BindingExternal
	switch {
	case bound && ownership == resource.OwnershipExternal:
		if existing.ID != canonicalID {
			return Result{}, fmt.Errorf("import %s: resource is already an external reference to remote identity %q", addr, existing.ID)
		}
	case bound && ownership == resource.OwnershipManaged:
		if !release {
			return Result{}, fmt.Errorf("import %s: resource is already managed as remote identity %q; a normal binding cannot become external without confirmation. Run `agoraform import --external --release %s %s` to stop managing it without deleting it", addr, existing.ID, addr, existing.ID)
		}
		if existing.ID != canonicalID {
			return Result{}, fmt.Errorf("import %s: refusing to release managed identity %q onto a different remote identity %q", addr, existing.ID, canonicalID)
		}
		binding = BindingRelease
	case bound:
		return Result{}, fmt.Errorf("import %s: stored ownership %q cannot be changed to external", addr, ownership)
	}

	yamlText, err := externalManifestYAML(addr, canonicalID)
	if err != nil {
		return Result{}, fmt.Errorf("import %s: cannot encode configuration: %w", addr, err)
	}
	if err := st.SetOwnership(addr, resource.Identity{ID: canonicalID, Fingerprint: live.Identity.Fingerprint}, resource.OwnershipExternal); err != nil {
		return Result{}, fmt.Errorf("import %s: could not persist external ownership: %w", addr, err)
	}
	return Result{
		Address:  addr,
		Identity: resource.Identity{ID: canonicalID},
		YAML:     yamlText,
		Binding:  binding,
	}, nil
}

// RunAdopt converts an external binding into managed ownership after verifying
// the remote object still exists. It does not mutate the remote object.
func RunAdopt(ctx context.Context, addr resource.Address, lookup Lookup, st ExternalStore) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := addr.Validate(); err != nil {
		return Result{}, fmt.Errorf("import: %w", err)
	}
	if lookup == nil {
		return Result{}, fmt.Errorf("import %s: provider lookup is required", addr)
	}
	if st == nil {
		return Result{}, fmt.Errorf("import %s: state store is required", addr)
	}
	existing, bound, ownership, err := currentOwnership(st, addr)
	if err != nil {
		return Result{}, err
	}
	if !bound {
		return Result{}, fmt.Errorf("import %s: resource is not bound; use `agoraform import %s REMOTE-ID` to take managed ownership of an existing object", addr, addr)
	}
	if ownership != resource.OwnershipExternal {
		return Result{}, fmt.Errorf("import %s: resource is already %s; --adopt only converts an external reference into managed ownership", addr, ownership)
	}

	live, canonicalID, p, err := readImport(ctx, addr, existing.ID, lookup, st)
	if err != nil {
		return Result{}, err
	}
	if canonicalID != existing.ID {
		return Result{}, fmt.Errorf("import %s: provider returned identity %q for external identity %q; refusing to adopt a different remote resource", addr, canonicalID, existing.ID)
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
	if err := st.SetOwnership(addr, resource.Identity{ID: canonicalID, Fingerprint: live.Identity.Fingerprint}, resource.OwnershipManaged); err != nil {
		return Result{}, fmt.Errorf("import %s: could not persist managed ownership: %w", addr, err)
	}
	return Result{
		Address:  addr,
		Identity: resource.Identity{ID: canonicalID},
		YAML:     yamlText,
		Binding:  BindingAdopt,
	}, nil
}

func currentOwnership(st ExternalStore, addr resource.Address) (resource.Identity, bool, resource.Ownership, error) {
	existing, bound, err := st.Identity(addr)
	if err != nil {
		return resource.Identity{}, false, "", fmt.Errorf("import %s: %w", addr, err)
	}
	if !bound {
		return resource.Identity{}, false, "", nil
	}
	ownership, ok, err := st.Ownership(addr)
	if err != nil {
		return resource.Identity{}, true, "", fmt.Errorf("import %s: %w", addr, err)
	}
	if !ok {
		ownership = resource.OwnershipManaged
	}
	return existing, true, ownership, nil
}

func readImport(ctx context.Context, addr resource.Address, remoteID string, lookup Lookup, st Store) (resource.RemoteResource, string, provider.Provider, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := addr.Validate(); err != nil {
		return resource.RemoteResource{}, "", nil, fmt.Errorf("import: %w", err)
	}
	remoteID = strings.TrimSpace(remoteID)
	if remoteID == "" {
		return resource.RemoteResource{}, "", nil, fmt.Errorf("import %s: remote identifier is empty", addr)
	}
	if lookup == nil {
		return resource.RemoteResource{}, "", nil, fmt.Errorf("import %s: provider lookup is required", addr)
	}
	if st == nil {
		return resource.RemoteResource{}, "", nil, fmt.Errorf("import %s: state store is required", addr)
	}
	p, err := lookup(addr)
	if err != nil {
		return resource.RemoteResource{}, "", nil, fmt.Errorf("import %s: %w", addr, err)
	}
	if p == nil {
		return resource.RemoteResource{}, "", nil, fmt.Errorf("import %s: provider is nil", addr)
	}
	if c, ok := p.(provider.ConnectionChecker); ok {
		if err := c.CheckConnection(ctx); err != nil {
			return resource.RemoteResource{}, "", nil, fmt.Errorf("import %s: provider %q: %w", addr, p.Name(), err)
		}
	}
	canonicalID := remoteID
	if n, ok := p.(provider.ImportIDNormalizer); ok {
		canonicalID, err = n.NormalizeImportID(addr, remoteID)
		if err != nil {
			return resource.RemoteResource{}, "", nil, fmt.Errorf("import %s: %w", addr, err)
		}
		canonicalID = strings.TrimSpace(canonicalID)
		if canonicalID == "" {
			return resource.RemoteResource{}, "", nil, fmt.Errorf("import %s: remote identifier is empty", addr)
		}
	}
	live, err := p.Import(ctx, addr, canonicalID)
	if errors.Is(err, provider.ErrNotFound) {
		return resource.RemoteResource{}, "", nil, fmt.Errorf("import %s: remote resource %q was not found: %w", addr, canonicalID, err)
	}
	if err != nil {
		return resource.RemoteResource{}, "", nil, fmt.Errorf("import %s: %w", addr, err)
	}
	if live.Identity.IsZero() {
		return resource.RemoteResource{}, "", nil, fmt.Errorf("import %s: provider returned no identity", addr)
	}
	if live.Identity.ID != canonicalID {
		return resource.RemoteResource{}, "", nil, fmt.Errorf("import %s: provider returned identity %q for requested remote identity %q; refusing to bind a different remote resource", addr, live.Identity.ID, canonicalID)
	}
	if live.Address != addr {
		return resource.RemoteResource{}, "", nil, fmt.Errorf("import %s: provider returned logical address %s for requested address %s; refusing to bind a different resource", addr, live.Address, addr)
	}
	return live, canonicalID, p, nil
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
