// Package provider defines the core provider interfaces used by Agoraform.
//
// The core remains provider-independent: this package has no knowledge of
// Matomo, Google Ads, or other vendor APIs. Concrete providers belong under
// the top-level providers/ directory. A test double lives in provider/fake.
package provider
