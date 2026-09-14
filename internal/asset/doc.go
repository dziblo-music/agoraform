// Package asset resolves provider-neutral local file sources.
//
// Agoraform does not generate, edit, or transcode creative assets. This
// package starts at the boundary where a finished file already exists on
// disk: it validates path safety, fingerprints content, and exposes a
// descriptor providers can stream during apply. Binary data never enters
// ordinary resource attributes, plans, logs, YAML, or state.
package asset
