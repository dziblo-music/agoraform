package destroy

import (
	"fmt"
	"strings"
)

// Format renders a destroy plan as deterministic, reviewable terminal text.
func Format(p *Plan) string {
	if p == nil {
		p = &Plan{}
	}

	var (
		mutations []Change
		skipped   []Change
		unmanaged []Change
	)
	for _, c := range p.Changes {
		switch c.Kind {
		case KindDestroy, KindRemove:
			mutations = append(mutations, c)
		case KindUnsupported, KindProviderOwned:
			skipped = append(skipped, c)
		case KindNotManaged:
			unmanaged = append(unmanaged, c)
		}
	}

	var b strings.Builder
	if len(mutations) == 0 && len(p.Finalizations) == 0 && len(skipped) == 0 && len(unmanaged) == 0 && len(p.Preserved) == 0 {
		b.WriteString("No managed resources to destroy.\n\n")
	} else {
		if len(mutations) > 0 {
			b.WriteString("Agoraform will destroy the following resources:\n\n")
			for _, c := range mutations {
				label := string(c.Kind)
				if label == string(KindDestroy) {
					fmt.Fprintf(&b, "- %s\n", c.Address)
				} else {
					fmt.Fprintf(&b, "- %s (%s)\n", c.Address, label)
				}
			}
			b.WriteByte('\n')
		}
		for _, f := range normalizedFinalizations(p) {
			fmt.Fprintf(&b, "> %s: %s", f.Address, f.Action)
			if f.Target != "" {
				fmt.Fprintf(&b, " -> %s", f.Target)
			}
			if f.Conditional {
				b.WriteString(" [conditional]")
			}
			b.WriteString("\n\n")
		}
		if len(skipped) > 0 {
			b.WriteString("The following resources cannot be destroyed and will remain in state:\n\n")
			for _, c := range skipped {
				fmt.Fprintf(&b, "- %s (%s)\n", c.Address, c.Kind)
			}
			b.WriteByte('\n')
		}
		if len(unmanaged) > 0 {
			b.WriteString("The following resources are not managed in local state and will not be changed:\n\n")
			for _, c := range unmanaged {
				fmt.Fprintf(&b, "- %s\n", c.Address)
			}
			b.WriteByte('\n')
		}
		if len(p.Preserved) > 0 {
			b.WriteString("The following state identities are not in the manifest and were not destroyed:\n\n")
			for _, addr := range p.Preserved {
				fmt.Fprintf(&b, "- %s\n", addr)
			}
			b.WriteByte('\n')
		}
	}

	fmt.Fprintf(&b, "Destroy: %d to destroy", len(mutations))
	if n := len(skipped); n > 0 {
		fmt.Fprintf(&b, ", %d unsupported", n)
	}
	if n := len(p.Finalizations); n > 0 {
		fmt.Fprintf(&b, ", %d provider action", n)
		if n != 1 {
			b.WriteByte('s')
		}
	}
	b.WriteString(".\n")
	return b.String()
}

type providerFinalization struct {
	Address     string
	Action      string
	Target      string
	Conditional bool
}

func normalizedFinalizations(p *Plan) []providerFinalization {
	out := make([]providerFinalization, 0, len(p.Finalizations))
	for _, f := range p.Finalizations {
		action := strings.TrimSpace(f.Action)
		if action == "" {
			action = "finalize"
		}
		out = append(out, providerFinalization{
			Address:     f.Address.String(),
			Action:      action,
			Target:      strings.TrimSpace(f.Target),
			Conditional: f.Conditional,
		})
	}
	return out
}

// FormatResult renders a successful destroy result as deterministic terminal text.
func FormatResult(r Result) string {
	parts := make([]string, 0, 4)
	parts = append(parts, fmt.Sprintf("%d destroyed", r.Destroyed))
	if r.AlreadyAbsent > 0 {
		noun := "already absent"
		parts = append(parts, fmt.Sprintf("%d %s", r.AlreadyAbsent, noun))
	}
	if r.Removed > 0 {
		parts = append(parts, fmt.Sprintf("%d removed", r.Removed))
	}
	if r.Finalized > 0 {
		actionLabel := "provider actions"
		if r.Finalized == 1 {
			actionLabel = "provider action"
		}
		parts = append(parts, fmt.Sprintf("%d %s completed", r.Finalized, actionLabel))
	}
	if r.Remaining > 0 {
		parts = append(parts, fmt.Sprintf("%d unsupported remaining", r.Remaining))
	}
	return fmt.Sprintf("Destroy complete! %s.\n", strings.Join(parts, ", "))
}
