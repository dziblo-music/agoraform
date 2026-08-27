// Package client is a reusable Google Ads REST client.
//
// Resource implementations should call Query, Mutate, and
// SuggestGeoTargetConstants instead of issuing raw HTTP requests. API
// version selection is centralized here so future Google Ads upgrades do
// not require resource-level rewrites.
//
// Credentials never appear in errors or diagnostic strings.
package client
