// Package manifest parses and validates Agoraform YAML manifests.
//
// The v1alpha1 schema is intentionally small: a versioned document that lists
// desired resources by logical address. Explicit $ref values become
// resource.Ref values and are checked as a dependency graph. Provider-specific
// API types do not belong here.
package manifest
