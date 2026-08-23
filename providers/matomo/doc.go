// Package matomo implements the Agoraform Matomo provider.
//
// This package is the provider foundation: configuration, authentication,
// a reusable HTTP client, and registration with the core provider
// contract. Resource types such as goals and Tag Manager objects are
// implemented in follow-up work and call through the client instead of
// issuing raw HTTP requests.
//
// Generic core and reconciliation packages must not import this package or
// depend on Matomo API types. The CLI composition root may import it in order
// to register the provider.
package matomo
