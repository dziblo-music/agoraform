package importer

import (
	"fmt"
	"strings"
)

// Format renders a successful import as deterministic terminal text.
func Format(r Result, statePath string) string {
	var b strings.Builder
	switch r.Binding {
	case BindingExternal:
		fmt.Fprintf(&b, "Bound %s as an external reference (remote identity %s).\n", r.Address, r.Identity.ID)
		b.WriteString("Agoraform will not create, update, or destroy this remote resource.\n")
	case BindingRelease:
		fmt.Fprintf(&b, "Released %s from managed ownership (remote identity %s).\n", r.Address, r.Identity.ID)
		b.WriteString("The remote resource was not modified. Destroy will remove only the local binding.\n")
	case BindingAdopt:
		fmt.Fprintf(&b, "Adopted %s into managed ownership (remote identity %s).\n", r.Address, r.Identity.ID)
		b.WriteString("The remote resource was not modified. Destroy can now delete it.\n")
		b.WriteString("Remove lifecycle.ownership from the manifest resource before the next plan.\n")
	default:
		fmt.Fprintf(&b, "Imported %s (remote identity %s).\n", r.Address, r.Identity.ID)
	}
	if strings.TrimSpace(statePath) != "" {
		fmt.Fprintf(&b, "Identity persisted to %s.\n", statePath)
	}
	b.WriteString("\n")
	switch r.Binding {
	case BindingExternal, BindingRelease:
		b.WriteString("Review this configuration and add it to your Agoraform manifest.\n")
		b.WriteString("lifecycle.id is the lookup key. Agoraform does not reconcile other attributes.\n\n")
	case BindingAdopt:
		b.WriteString("Review this managed configuration. Replace the external resource in your manifest with it.\n")
		b.WriteString("Provider-native identity stays in local state, not in configuration.\n\n")
	default:
		b.WriteString("Review this configuration and add it to your Agoraform manifest.\n")
		b.WriteString("Provider-native identity is stored in local state, not in configuration.\n\n")
	}
	b.WriteString(r.YAML)
	if !strings.HasSuffix(r.YAML, "\n") {
		b.WriteByte('\n')
	}
	return b.String()
}
