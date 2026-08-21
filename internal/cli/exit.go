package cli

// Exit codes used by the Agoraform CLI.
const (
	// ExitOK indicates successful completion.
	ExitOK = 0

	// ExitError indicates a general runtime or command failure,
	// including intentionally unimplemented commands.
	ExitError = 1

	// ExitUsage indicates invalid invocation or flag usage.
	ExitUsage = 2
)
