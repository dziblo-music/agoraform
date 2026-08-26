package client

import (
	"fmt"
	"strings"
	"unicode"
)

const customerIDDigits = 10

// NormalizeCustomerID returns the canonical 10-digit Google Ads customer ID.
//
// Hyphens, spaces, and an optional customers/ prefix are stripped. The
// result is always digits-only so it is safe to place in REST paths and
// the login-customer-id header.
func NormalizeCustomerID(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.TrimPrefix(s, "customers/")
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return "", fmt.Errorf("googleads: customer ID is required")
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return "", fmt.Errorf("googleads: customer ID must contain only digits")
		}
	}
	if len(s) != customerIDDigits {
		return "", fmt.Errorf("googleads: customer ID must be %d digits", customerIDDigits)
	}
	return s, nil
}

// CustomerResourceName returns the Google Ads resource name customers/{id}.
func CustomerResourceName(customerID string) (string, error) {
	id, err := NormalizeCustomerID(customerID)
	if err != nil {
		return "", err
	}
	return "customers/" + id, nil
}
