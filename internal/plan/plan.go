package plan

import "github.com/dziblo-music/agoraform/internal/resource"

// Action is a planned change kind.
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
// Before and After are normalized configurable attribute maps. Computed
// fields are never included. Diffs is the attribute-level view used by
// rendering and review tools.
type Change struct {
	Address  resource.Address
	Action   Action
	Identity resource.Identity
	Before   resource.Attributes
	After    resource.Attributes
	Diffs    []AttributeDiff
}

// Plan is a deterministic, machine-usable set of resource changes.
//
// It is independent of terminal rendering. Destructive deletion is out of
// scope for v0.1; Counts always reports zero destroys.
type Plan struct {
	Changes []Change
}

// HasChanges reports whether the plan contains any create or update.
func (p Plan) HasChanges() bool {
	create, update, _ := p.Counts()
	return create+update > 0
}

// Counts returns how many resources would be created, updated, or destroyed.
// Destroy is always 0 in v0.1.
func (p Plan) Counts() (create, update, destroy int) {
	for _, c := range p.Changes {
		switch c.Action {
		case ActionCreate:
			create++
		case ActionUpdate:
			update++
		}
	}
	return create, update, 0
}
