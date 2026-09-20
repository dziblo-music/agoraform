// Package manifest parses and validates Agoraform YAML configuration.
//
// The v1alpha1 schema is intentionally small: a versioned document that lists
// desired resources by logical address and, optionally, a local asset root.
// A configuration may be a single file or a directory of *.agoraform.yaml
// files merged into one logical document. Explicit $ref values become
// resource.Ref values and are checked as a dependency graph after all files
// are loaded. Local source.file values are resolved into provider-neutral
// descriptors. Provider-specific API types do not belong here.
package manifest
