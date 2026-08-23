package plan

import (
	"fmt"
	"sort"
	"strings"
)

// Format renders a plan as deterministic, reviewable terminal text.
//
// The output is derived from Plan; it does not query providers. Unchanged
// resources are omitted from the action list but counted in the summary.
func Format(p *Plan) string {
	if p == nil {
		p = &Plan{}
	}

	changes := append([]Change(nil), p.Changes...)
	sort.SliceStable(changes, func(i, j int) bool {
		return changes[i].Address.String() < changes[j].Address.String()
	})

	create, update, destroy := Plan{Changes: changes}.Counts()

	var b strings.Builder
	if create+update == 0 {
		b.WriteString("No changes. Desired configuration matches live resources.\n\n")
	} else {
		b.WriteString("Agoraform will perform the following actions:\n\n")
		for _, c := range changes {
			if c.Action == ActionUnchanged {
				continue
			}
			writeChange(&b, c)
			b.WriteByte('\n')
		}
	}

	fmt.Fprintf(&b, "Plan: %d to create, %d to update, %d to destroy.\n", create, update, destroy)
	return b.String()
}

func writeChange(b *strings.Builder, c Change) {
	switch c.Action {
	case ActionCreate:
		fmt.Fprintf(b, "+ %s\n", c.Address)
	case ActionUpdate:
		fmt.Fprintf(b, "~ %s\n", c.Address)
	default:
		fmt.Fprintf(b, "  %s\n", c.Address)
	}

	diffs := append([]AttributeDiff(nil), c.Diffs...)
	sort.SliceStable(diffs, func(i, j int) bool {
		return diffs[i].Path < diffs[j].Path
	})

	if len(diffs) == 0 {
		return
	}
	b.WriteByte('\n')
	for _, d := range diffs {
		path := d.Path
		if path == "" {
			path = "(attributes)"
		}
		if c.Action == ActionCreate && d.Before == nil {
			fmt.Fprintf(b, "    %s: %s\n", path, formatValue(d.After))
			continue
		}
		fmt.Fprintf(b, "    %s:\n", path)
		fmt.Fprintf(b, "      %s -> %s\n", formatValue(d.Before), formatValue(d.After))
	}
}
