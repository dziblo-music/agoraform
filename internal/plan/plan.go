package plan

import (
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

// Action is a planned core state-transition kind.
type Action string

const (
	// ActionCreate means the desired resource does not exist remotely.
	ActionCreate Action = "create"

	// ActionUpdate means configurable attributes differ from live state.
	ActionUpdate Action = "update"

	// ActionUnchanged means desired and live configurable state match.
	ActionUnchanged Action = "unchanged"
)

// AttributeDiff is a single comparable attribute path that would change.
type AttributeDiff struct {
	// Path is a deterministic dotted/indexed attribute path, for example
	// "name" or "settings.theme" or "tags[0]".
	Path string

	// Before is the live value. A nil interface means the path is absent.
	Before any

	// After is the desired value. A nil interface means the path is absent.
	After any
}

// Change is one planned resource outcome.
//
// Before and After are normalized configurable attribute maps. Diffs is
// the attribute-level view used by rendering and review tools.
type Change struct {
	Address  resource.Address
	Action   Action
	Identity resource.Identity
	Before   resource.Attributes
	After    resource.Attributes
	Diffs    []AttributeDiff

	// Operation optionally describes provider-native semantics when they differ
	// from the core state transition. For example, a provider-created object can
	// be an ActionCreate from local-state perspective while Operation is
	// "adopt", making plan review clear that Agoraform does not create the
	// remote object.
	Operation string

	// Computed is the live provider-reported read-only view observed
	// while planning. Apply uses it to seed runtime Resolved bindings
	// for unchanged prerequisites. It is not configuration, is never
	// copied into Before or After, and Format ignores it.
	Computed resource.Attributes
}

// Plan is a deterministic, machine-usable set of resource changes plus any
// provider-level finalization actions that must happen after resource
// mutations succeed.
type Plan struct {
	Changes       []Change
	Finalizations []provider.FinalizationPlan
}

// HasChanges reports whether the plan contains any create, adopt, update, or
// provider finalization action.
func (p Plan) HasChanges() bool {
	create, update, _ := p.Counts()
	return create+p.AdoptionCount()+update > 0 || len(p.Finalizations) > 0
}

// Counts returns how many resources would be created, updated, or destroyed.
// Provider-created resources whose Operation is "adopt" are excluded from the
// create count and reported separately by AdoptionCount. Destroy is always 0
// in v0.1.
func (p Plan) Counts() (create, update, destroy int) {
	for _, c := range p.Changes {
		switch c.Action {
		case ActionCreate:
			if c.Operation == string(provider.MissingResourceAdopt) {
				continue
			}
			create++
		case ActionUpdate:
			update++
		}
	}
	return create, update, 0
}

// AdoptionCount returns how many missing desired resources are provider-created
// objects that Agoraform will adopt/reconcile rather than provision remotely.
func (p Plan) AdoptionCount() int {
	adopt := 0
	for _, c := range p.Changes {
		if c.Action == ActionCreate && c.Operation == string(provider.MissingResourceAdopt) {
			adopt++
		}
	}
	return adopt
}
