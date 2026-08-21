package cli

import "github.com/spf13/cobra"

func newApplyCommand() *cobra.Command {
	return newNotImplementedCommand(
		"apply",
		"Apply reviewed changes through provider APIs",
	)
}
