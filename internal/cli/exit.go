package cli

// Exit codes used by the Agoraform CLI.
const (
	// ExitOK indicates successful completion. For plan, this means the
	// plan succeeded and no changes are required. For apply, this means
	// planned mutations completed (including a zero-change apply).
	ExitOK = 0

	// ExitError indicates a general runtime or command failure,
	// including failed planning, apply, or import.
	ExitError = 1

	// ExitChanges indicates plan succeeded and changes are present.
	// This is intended for CI/GitOps workflows that treat a non-empty
	// plan as an actionable result rather than a failure.
	ExitChanges = 2

	// ExitUsage indicates invalid invocation or flag usage.
	ExitUsage = 3
)
