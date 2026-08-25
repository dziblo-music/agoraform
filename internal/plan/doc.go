// Package plan implements non-mutating reconciliation and execution plans.
//
// The engine reads live resources through provider.Reader, diffs configurable
// attributes, and produces a machine-usable Plan. Optional local state
// supplies opaque provider-native identities so managed resources are not
// rediscovered by mutable attributes. Resource references are validated as a
// dependency graph before any remote reads, which then follow that graph's
// prerequisite-first order. Terminal rendering is a separate
// Format step so apply and CI tooling can consume the same change model.
// Live computed outputs are retained on Change for apply-time reference
// resolution and are omitted from Before, After, Diffs, and Format.
// Plan never calls provider mutation methods. Plans use logical resource
// addresses; they do not require provider-native IDs in configuration.
package plan
