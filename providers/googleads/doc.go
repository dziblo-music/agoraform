// Package googleads implements the Agoraform Google Ads provider.
//
// The provider registers as googleads, loads credentials from the
// environment, and talks to the Google Ads REST API through
// providers/googleads/client. It manages website conversion actions
// (googleads.conversion_action), customer conversion-goal biddability
// (googleads.customer_conversion_goal), daily Search campaign budgets
// (googleads.campaign_budget), Search campaigns (googleads.campaign), and
// campaign conversion-goal biddability (googleads.campaign_conversion_goal).
// Other Google Ads resources remain follow-up work.
//
// Generic core and reconciliation packages must not import this package or
// depend on Google Ads API types. The CLI composition root may import it in
// order to register the provider.
package googleads
