// Package resource holds provider-independent resource identity and modeling
// types used by the Agoraform core.
//
// Desired resources come from configuration. Remote resources are reported by
// providers and may include a provider-native identity plus computed
// (read-only) attributes that are not set in configuration.
//
// Ref is a first-class reference to another logical resource address. It is
// configuration, not a provider-native identity. An optional Output selector
// names one declared non-sensitive runtime value. Apply replaces an
// address-only Ref with a Resolved binding immediately before Create/Update,
// and replaces an output Ref with a clone of that named value.
package resource
