// Package importer binds an existing remote resource to a logical address.
//
// Import optionally canonicalizes a user-supplied identifier, reads through
// provider.Import, emits deterministic YAML for configurable fields, and
// persists the provider-native identity in local state. It never creates,
// updates, or deletes remote resources. Generated configuration is desired
// state only; identity belongs in state, not in the manifest.
package importer
