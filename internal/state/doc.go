// Package state implements Agoraform's v0.1 local identity mapping.
//
// The manifest describes desired configuration. State stores only the
// management metadata needed to bind a logical resource address to an
// opaque provider-native identity. It never contains credentials or
// provider secrets.
package state
