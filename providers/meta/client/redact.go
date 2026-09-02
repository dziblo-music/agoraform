package client

import (
	"regexp"
	"strings"
)

const redacted = "[redacted]"

var (
	tokenParamPattern = regexp.MustCompile(`(?i)((?:access_token|appsecret_proof)=)([^&\s"']+)`)
	authHeaderPattern = regexp.MustCompile(`(?i)(authorization:\s*(?:bearer|oauth)\s+)(\S+)`)
)

// Redact removes known secrets and common Meta credential forms from text
// that may become a user-visible diagnostic.
func Redact(s string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			s = strings.ReplaceAll(s, secret, redacted)
		}
	}
	s = tokenParamPattern.ReplaceAllString(s, "${1}"+redacted)
	s = authHeaderPattern.ReplaceAllString(s, "${1}"+redacted)
	return s
}
