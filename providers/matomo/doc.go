// Package matomo implements the Agoraform Matomo provider.
//
// This package is the provider foundation: configuration, authentication,
// a reusable HTTP client, and registration with the core provider
// contract. Resource types such as goals and Tag Manager objects are
// implemented in follow-up work and call through the client instead of
// issuing raw HTTP requests.
//
// Core packages under internal/ must not import this package or Matomo
// API types. The CLI composition root registers the provider.
package matomo
