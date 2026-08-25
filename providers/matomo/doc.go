// Package matomo implements the Agoraform Matomo provider.
//
// The provider registers matomo.goal and matomo.variable, loads credentials
// from the environment, and talks to Matomo through providers/matomo/client.
// Tag Manager triggers, tags, and versions are implemented in follow-up work.
//
// Generic core and reconciliation packages must not import this package or
// depend on Matomo API types. The CLI composition root may import it in order
// to register the provider.
package matomo
