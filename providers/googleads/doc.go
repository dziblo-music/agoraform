// Package googleads implements the Agoraform Google Ads provider foundation.
//
// The provider registers as googleads, loads credentials from the
// environment, and talks to the Google Ads REST API through
// providers/googleads/client. Conversion actions and other resources remain
// follow-up work.
//
// Generic core and reconciliation packages must not import this package or
// depend on Google Ads API types. The CLI composition root may import it in
// order to register the provider.
package googleads
