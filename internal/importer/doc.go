// Package importer binds an existing remote resource to a logical address.
//
// Import optionally canonicalizes a user-supplied identifier, reads through
// provider.Import, emits deterministic YAML for configurable fields, and
// persists the provider-native identity in local state. It never creates,
// updates, or deletes remote resources. Generated configuration is desired
// state only; identity belongs in state, not in the manifest.
//
// During import, core builds an ephemeral read-only catalog of declared
// non-sensitive outputs from already-bound state resources. Providers may
// request a unique match and reconstruct a logical { $ref, output } without
// importing another provider package. Zero and multiple matches are distinct
// results; providers must not guess.
package importer
