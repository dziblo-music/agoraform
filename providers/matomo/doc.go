// Package matomo implements the Agoraform Matomo provider.
//
// The provider registers matomo.goal, matomo.variable, matomo.trigger,
// and matomo.tag, loads credentials from the environment, and talks to
// Matomo through providers/matomo/client. Explicit container publication
// is implemented by agoraform publish, not apply.
//
// Generic core and reconciliation packages must not import this package or
// depend on Matomo API types. The CLI composition root may import it in order
// to register the provider.
package matomo
