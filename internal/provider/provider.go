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

// MissingResourceMode describes the provider-native operation represented by
// an unbound desired resource that is absent from Read.
type MissingResourceMode string

const (
	// MissingResourceCreate is the default: apply provisions a new remote
	// object through Provider.Create.
	MissingResourceCreate MissingResourceMode = "create"

	// MissingResourceAdopt means the provider creates the remote object itself
	// and apply should bind/reconcile that object rather than imply that
	// Agoraform provisions it. Core still treats this as a new local binding.
	MissingResourceAdopt MissingResourceMode = "adopt"
)

// MissingResourcePlanner is an optional read-only planning hook for resources
// whose provider-native lifecycle differs from ordinary CRUD. Plan calls it
// only after Read returns ErrNotFound for an unbound desired resource.
// Implementations must not mutate remote state.
type MissingResourcePlanner interface {
	PlanMissingResource(res resource.Resource) (MissingResourceMode, error)
}

// ResourceSetValidator is an optional provider hook for validation that needs
// to inspect relationships between multiple desired resources. It is called
// by manifest validation and planning after the core reference graph has been
// checked. Implementations must not mutate remote state.
type ResourceSetValidator interface {
	ValidateResourceSet(ctx context.Context, resources []resource.Resource) error
}

// ConnectionChecker is an optional provider hook for non-mutating
// credential and endpoint validation.
//
// When a Provider also implements ConnectionChecker, validate and plan
// call CheckConnection once per provider after provider configuration has
// been applied. Implementations must not mutate remote state or include
// credentials in returned errors.
type ConnectionChecker interface {
	CheckConnection(ctx context.Context) error
}

// Configurator is an optional provider hook for non-secret declarative
// configuration from the manifest's providers block. Credentials and other
// sensitive connection settings must remain outside the manifest.
type Configurator interface {
	Configure(config resource.Attributes) error
}

// PendingChange is a resource mutation already visible in the core plan.
// Providers may use this read-only view when deciding whether a provider-level
// finalization action will be required after those resource mutations.
type PendingChange struct {
	Address resource.Address
	Action  string
}

// FinalizationPlan is a provider-specific action considered after all resource
// creates and updates succeed. Conditional actions may be skipped when the
// provider rechecks the converged state. The CLI stays provider-neutral while
// still making the potential action visible during plan review.
type FinalizationPlan struct {
	Address     resource.Address
	Action      string
	Target      string
	Conditional bool
}

// FinalizationResult reports provider-specific progress. Details are safe,
// human-readable fragments; the CLI prefixes each with Address.
type FinalizationResult struct {
	Address resource.Address
	Details []string
	Changed bool
}

// Finalizer is an optional provider hook for declarative post-resource
// convergence such as publishing an already-applied container draft.
// PlanFinalization must be non-mutating. Finalize is called only after every
// planned resource mutation succeeds and is authoritative about whether a
// conditional finalization still needs to mutate provider state.
type Finalizer interface {
	PlanFinalization(ctx context.Context, pending []PendingChange) (*FinalizationPlan, error)
	Finalize(ctx context.Context, planned FinalizationPlan) (FinalizationResult, error)
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

	// Import reads the existing remote resource identified by id as the
	// supplied logical address without mutating remote state. The returned
	// RemoteResource must carry exactly addr and exactly the requested id as
	// its identity. If that identity does not exist, Import returns ErrNotFound.
	//
	// Callers that accept aliases should implement ImportIDNormalizer so
	// Import receives the canonical identity stored in local state.
	Import(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error)
}

// ImportIDNormalizer is an optional provider hook that rewrites a
// user-supplied import identifier into the canonical provider-native
// identity stored in local state.
//
// Agoraform calls this before Import. Implementations must not mutate
// remote state. Typical aliases are resource names or case variants of a
// stable key such as CATEGORY~ORIGIN.
type ImportIDNormalizer interface {
	NormalizeImportID(addr resource.Address, raw string) (string, error)
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
