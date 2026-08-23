package client

import (
	"regexp"
	"strings"
)

const redacted = "[redacted]"

var (
	tokenQueryRe = regexp.MustCompile(`(?i)((?:token_auth|token)=)([^&\s"']+)`)
	authHeaderRe = regexp.MustCompile(`(?i)(authorization:\s*(?:bearer|basic)\s+)(\S+)`)
)

// Redact removes secret values from diagnostic text.
//
// It replaces known secret strings and common credential-bearing query
// parameters or Authorization headers so errors and logs can be printed
// without leaking tokens.
func Redact(s string, secrets ...string) string {
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		s = strings.ReplaceAll(s, secret, redacted)
	}
	s = tokenQueryRe.ReplaceAllString(s, "${1}"+redacted)
	s = authHeaderRe.ReplaceAllString(s, "${1}"+redacted)
	return s
}
