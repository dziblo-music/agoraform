// Package apply executes a plan through provider mutation methods.
//
// Apply never diffs desired and live state itself. It consumes the
// machine-usable Plan from package plan and dispatches only the create
// and update actions that plan produced. Local state from package state
// is the source of truth for provider-native identities after a
// successful mutation. Apply never deletes remote resources.
package apply
