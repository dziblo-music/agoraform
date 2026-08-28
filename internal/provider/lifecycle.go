package provider

import (
	"context"

	"github.com/dziblo-music/agoraform/internal/resource"
)

// DestroyCapability describes whether a provider can destroy or otherwise
// terminate a resource through agoraform destroy.
type DestroyCapability string

const (
	// DestroySupported means Destroy performs the provider-native teardown.
	DestroySupported DestroyCapability = "supported"

	// DestroyUnsupported means Agoraform cannot destroy the resource. It
	// remains in state and is reported as a planned non-mutation.
	DestroyUnsupported DestroyCapability = "unsupported"

	// DestroyProviderOwned means the remote object is owned by the provider
	// platform and cannot be deleted by Agoraform. It remains in state and is
	// reported as a planned non-mutation.
	DestroyProviderOwned DestroyCapability = "provider-owned"
)

// DestroyStatus is the provider-native result of a destroy attempt.
type DestroyStatus string

const (
	// DestroyStatusDestroyed means the remote object was deleted.
	DestroyStatusDestroyed DestroyStatus = "destroyed"

	// DestroyStatusAlreadyAbsent means the provider confirmed the object is
	// gone. Core treats this as successful convergence.
	DestroyStatusAlreadyAbsent DestroyStatus = "already-absent"

	// DestroyStatusRemoved means the provider archived, disabled, or otherwise
	// removed the object without a hard delete.
	DestroyStatusRemoved DestroyStatus = "removed"
)

// DestroyResult is the outcome of Provider destroy/remove.
type DestroyResult struct {
	Status DestroyStatus
}

// Destroyer is an optional provider hook for agoraform destroy.
//
// DestroyCapability must be non-mutating. Destroy translates the generic
// request into provider-native deletion, removal, or already-absent
// confirmation. Implementations must not treat authentication, permission,
// malformed-response, or wrong-target errors as already-absent.
type Destroyer interface {
	DestroyCapability(res resource.Resource) (DestroyCapability, error)
	Destroy(ctx context.Context, res resource.Resource) (DestroyResult, error)
}

// ResourceDestroyCapability reports p's destroy semantics for res.
// Providers that do not implement Destroyer are unsupported.
func ResourceDestroyCapability(p Provider, res resource.Resource) (DestroyCapability, error) {
	if p == nil {
		return DestroyUnsupported, nil
	}
	d, ok := p.(Destroyer)
	if !ok {
		return DestroyUnsupported, nil
	}
	return d.DestroyCapability(res)
}
