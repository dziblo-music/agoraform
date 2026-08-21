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

// Provider is the v0.1 contract between Agoraform's core and a provider.
//
// Implementations must not log credentials. Mutation methods (Create, Update)
// are used by apply/import workflows; plan must only call Name,
// ResourceTypes, Validate, and Read.
type Provider interface {
	// Name returns the provider identifier used in resource addresses.
	Name() string

	// ResourceTypes returns the resource types this provider manages,
	// for example "widget" or "goal".
	ResourceTypes() []string

	// Validate checks a desired resource against provider rules.
	Validate(ctx context.Context, res resource.Resource) error

	// Read returns the current live representation of a resource.
	// If the resource does not exist remotely, Read returns ErrNotFound.
	Read(ctx context.Context, res resource.Resource) (resource.RemoteResource, error)

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
