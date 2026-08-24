// Package graph builds a deterministic directed dependency graph from
// resource references.
//
// The graph is provider-neutral: edges come from logical resource
// addresses embedded in configuration, not from provider-native IDs or
// API relationships.
package graph
