// Package integration resolves and formats the application instrumentation
// contract declared in an Agoraform manifest.
//
// It reads the applicationEvents block together with the current provider
// state to produce the non-secret provider identifiers that external
// application instrumentation needs. Agoraform does not generate application
// source files; the output is informational and read-only.
package integration
