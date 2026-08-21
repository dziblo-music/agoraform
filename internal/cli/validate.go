package cli

import "github.com/spf13/cobra"

func newValidateCommand() *cobra.Command {
	return newNotImplementedCommand(
		"validate",
		"Validate configuration files and provider settings",
	)
}
