package importer

import (
	"fmt"
	"strings"
)

// Format renders a successful import as deterministic terminal text.
func Format(r Result, statePath string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Imported %s (remote identity %s).\n", r.Address, r.Identity.ID)
	if strings.TrimSpace(statePath) != "" {
		fmt.Fprintf(&b, "Identity persisted to %s.\n", statePath)
	}
	b.WriteString("\n")
	b.WriteString("Review this configuration and add it to your Agoraform manifest.\n")
	b.WriteString("Provider-native identity is stored in local state, not in configuration.\n\n")
	b.WriteString(r.YAML)
	if !strings.HasSuffix(r.YAML, "\n") {
		b.WriteByte('\n')
	}
	return b.String()
}
