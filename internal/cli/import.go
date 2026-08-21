package cli

import "github.com/spf13/cobra"

func newImportCommand() *cobra.Command {
	return newNotImplementedCommand(
		"import",
		"Import existing remote resources into local management",
	)
}
