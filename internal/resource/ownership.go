package resource

import (
	"fmt"
	"strings"
)

// Ownership is the lifecycle contract for a logical resource.
//
// Managed resources are created, updated, and destroyed by Agoraform.
// External resources are reference-only: Agoraform may read them and expose
// their identity to $ref consumers, but it must not create, update, or
// destroy the remote object.
type Ownership string

const (
	// OwnershipManaged is Agoraform lifecycle ownership. The zero value and
	// legacy state records without an ownership marker also mean managed.
	OwnershipManaged Ownership = "managed"

	// OwnershipExternal is a reference-only binding. Destroy must not mutate
	// the remote object, even if the manifest later omits the marker, until an
	// explicit adoption changes the persisted ownership.
	OwnershipExternal Ownership = "external"
)

// Normalized maps an empty ownership to managed.
func (o Ownership) Normalized() Ownership {
	switch Ownership(strings.TrimSpace(string(o))) {
	case "", OwnershipManaged:
		return OwnershipManaged
	default:
		return Ownership(strings.TrimSpace(string(o)))
	}
}

// IsExternal reports whether ownership is the reference-only contract.
func (o Ownership) IsExternal() bool {
	return o.Normalized() == OwnershipExternal
}

// IsExternal reports whether the desired resource is reference-only.
func (r Resource) IsExternal() bool {
	return r.Ownership.IsExternal()
}

// ParseOwnership accepts the persisted and manifest ownership values.
// An empty string is managed so legacy state stays backward compatible.
func ParseOwnership(raw string) (Ownership, error) {
	switch Ownership(strings.TrimSpace(raw)) {
	case "", OwnershipManaged:
		return OwnershipManaged, nil
	case OwnershipExternal:
		return OwnershipExternal, nil
	default:
		return "", fmt.Errorf("ownership %q is invalid; want %q or %q", raw, OwnershipManaged, OwnershipExternal)
	}
}

// OwnershipConflict reports an illegal managed/external transition.
//
// A manifest edit alone must not adopt an external resource or release a
// managed one. storedBound is false when local state has no record.
func OwnershipConflict(addr Address, declared, stored Ownership, storedBound bool) error {
	if !storedBound {
		return nil
	}
	decl, err := ParseOwnership(string(declared))
	if err != nil {
		return fmt.Errorf("resource %s: %w", addr, err)
	}
	got, err := ParseOwnership(string(stored))
	if err != nil {
		return fmt.Errorf("resource %s: stored %w", addr, err)
	}
	if decl == got {
		return nil
	}
	if got == OwnershipExternal && decl == OwnershipManaged {
		return fmt.Errorf("ownership is external in local state but %s is declared as managed; a manifest edit cannot adopt this resource. Run `agoraform import --adopt %s` to take lifecycle ownership, or restore lifecycle.ownership: external", addr, addr)
	}
	if got == OwnershipManaged && decl == OwnershipExternal {
		return fmt.Errorf("ownership is managed in local state but %s is declared as external; a manifest edit cannot release this resource. Run `agoraform import --external --release %s REMOTE-ID` to stop managing it without deleting it, or remove lifecycle.ownership: external", addr, addr)
	}
	return fmt.Errorf("resource %s: cannot change ownership from %s to %s without an explicit migration", addr, got, decl)
}
