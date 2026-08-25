// Package apply owns Agoraform's provider-neutral apply lifecycle.
//
// Run is the canonical high-level entry point: it builds the reconciliation
// plan, attaches provider finalization actions, executes resource creates and
// updates in deterministic dependency order, persists provider-native
// identities, and finally executes planned provider finalizations. A resource
// or required state-write failure prevents finalization.
//
// Execute is the lower-level resource-CRUD primitive. It consumes an existing
// machine-usable plan and dispatches create/update actions only; callers that
// need complete apply semantics should use Run.
//
// Local state from package state remains the source of truth for
// provider-native identities after successful mutations. Apply never deletes
// remote resources.
package apply
