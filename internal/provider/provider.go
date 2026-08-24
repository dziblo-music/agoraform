package provider

import (
	"context"
	"errors"

	"github.com/dziblo-music/agoraform/internal/resource"
)

// ErrNotFound reports that a resource does not exist remotely.
//
// Read and Import should return this error when the target is absent. Callers
// such as the plan engine may treat absence as a normal create candidate
// rather than a fatal failure.
var ErrNotFound = errors.New("resource not found")

// Reader is the read-only subset of Provider used by the plan engine.
//
// Plan accepts Reader rather than Provider so it is structurally unable to
// call Create, Update, or Import.
type Reader interface {
	Name() string
	ResourceTypes() []string
	Validate(ctx context.Context, res resource.Resource) error

	// Read resolves the live resource represented by res. When res.Identity is
	// non-zero, implementations must resolve that exact provider-native
	// identity and return the same identity on RemoteResource. They must return
	// ErrNotFound when that identity is absent and must not fall back to mutable
	// discovery fields such as name. Core reconciliation independently verifies
	// this invariant before accepting a state-bound read.
	Read(ctx context.Context, res resource.Resource) (resource.RemoteResource, error)
}

// Normalizer is an optional provider hook for comparable attribute views.
//
// When a Reader also implements Normalizer, the plan engine uses it before
// diffing so provider-specific defaults and omitted values do not produce
// false changes. Implementations must omit computed/read-only fields.
type Normalizer interface {
	NormalizeComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error)
}

// ConnectionChecker is an optional provider hook for non-mutating
// credential and endpoint validation.
//
// When a Provider also implements ConnectionChecker, validate and plan
// call CheckConnection once per provider after the provider and a
// supported resource type are resolved. Implementations must not mutate
// remote state or include credentials in returned errors.
type ConnectionChecker interface {
	CheckConnection(ctx context.Context) error
}

// Provider is the v0.1 contract between Agoraform's core and a provider.
//
// Implementations must not log credentials. Mutation methods (Create, Update)
// are used by apply. Import reads an existing remote identity and must not
// create, update, or delete remote resources. Plan must only call Name,
// ResourceTypes, Validate, and Read.
type Provider interface {
	Reader

	// Create provisions a desired resource and returns the live result,
	// including any provider-native identity and computed attributes.
	Create(ctx context.Context, res resource.Resource) (resource.RemoteResource, error)

	// Update reconciles a desired resource with a previously read live
	// resource and returns the updated live result.
	Update(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error)

	// Import binds an existing remote identity to a logical address and
	// returns the live resource. If the identity does not exist, Import
	// returns ErrNotFound.
	Import(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error)
}

// Supports reports whether p manages the given resource type.
func Supports(p Provider, resourceType string) bool {
	if p == nil {
		return false
	}
	for _, t := range p.ResourceTypes() {
		if t == resourceType {
			return true
		}
	}
	return false
}
