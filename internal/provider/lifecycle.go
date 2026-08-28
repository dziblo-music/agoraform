package provider

import (
	"context"

	"github.com/dziblo-music/agoraform/internal/resource"
)

// DestroyCapability describes the provider-native operation available for a
// resource through agoraform destroy.
type DestroyCapability string

const (
	// DestroyDelete means Destroy performs a provider-native deletion.
	DestroyDelete DestroyCapability = "destroy"

	// DestroyRemove means Destroy performs a provider-native remove, archive,
	// disable, or equivalent terminal operation rather than a hard delete.
	DestroyRemove DestroyCapability = "remove"

	// DestroySupported is kept as a compatibility alias for providers/tests
	// written before destroy/remove were distinguished explicitly. It means a
	// provider-native deletion and is equivalent to DestroyDelete.
	DestroySupported DestroyCapability = DestroyDelete

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
// DestroyCapability must be non-mutating and must report the provider-native
// operation that will appear in the reviewed destroy plan. Destroy translates
// that generic request into provider-native deletion, removal, or
// already-absent confirmation. Implementations must not treat authentication,
// permission, malformed-response, or wrong-target errors as already-absent.
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
