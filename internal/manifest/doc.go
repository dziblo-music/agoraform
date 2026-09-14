// Package manifest parses and validates Agoraform YAML manifests.
//
// The v1alpha1 schema is intentionally small: a versioned document that lists
// desired resources by logical address and, optionally, a local asset root.
// Explicit $ref values become resource.Ref values and are checked as a
// dependency graph. Local source.file values are resolved into provider-neutral
// descriptors. Provider-specific API types do not belong here.
package manifest
