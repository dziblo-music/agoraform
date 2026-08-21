package cli

import "github.com/spf13/cobra"

func newPlanCommand() *cobra.Command {
	return newNotImplementedCommand(
		"plan",
		"Show the changes required to reconcile declared and live configuration",
	)
}
