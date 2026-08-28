// Package matomo implements the Agoraform Matomo provider.
//
// The provider registers matomo.goal, matomo.container, matomo.variable,
// matomo.trigger, and matomo.tag, loads credentials from the environment,
// and talks to Matomo through providers/matomo/client. Tag Manager child
// resources either reference a managed matomo.container or use
// MATOMO_CONTAINER_ID for an externally managed container.
//
// Generic core and reconciliation packages must not import this package or
// depend on Matomo API types. The CLI composition root may import it in order
// to register the provider.
package matomo
