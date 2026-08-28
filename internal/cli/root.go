package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dziblo-music/agoraform/internal/config"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/spf13/cobra"
)

// IOStreams holds the CLI input/output streams.
type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
}

// DefaultIOStreams returns streams bound to the process stdio.
func DefaultIOStreams() IOStreams {
	return IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}
}

type usageError struct {
	err error
}

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

// NewRootCommand builds the root agoraform command and its subcommands.
func NewRootCommand(streams IOStreams) *cobra.Command {
	return NewRootCommandWithRegistry(streams, newProviderRegistry())
}

// NewRootCommandWithRegistry builds the root command with an explicit
// provider registry. Tests use this to register the fake provider.
func NewRootCommandWithRegistry(streams IOStreams, reg *provider.Registry) *cobra.Command {
	if reg == nil {
		reg = newProviderRegistry()
	}

	cmd := &cobra.Command{
		Use:           "agoraform",
		Short:         "Marketing Infrastructure as Code",
		Long:          "Agoraform defines, plans, and applies marketing infrastructure from code.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       effectiveVersion(),
	}

	cmd.SetIn(streams.In)
	cmd.SetOut(streams.Out)
	cmd.SetErr(streams.ErrOut)
	cmd.SetVersionTemplate("{{.Version}}\n")
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageError{err: err}
	})

	cmd.AddCommand(newValidateCommand(reg))
	cmd.AddCommand(newPlanCommand(reg))
	cmd.AddCommand(newApplyCommand(reg))
	cmd.AddCommand(newDestroyCommand(reg))
	cmd.AddCommand(newImportCommand(reg))

	return cmd
}

// Execute runs the Agoraform CLI and returns a process exit code.
func Execute() int {
	return ExecuteWith(DefaultIOStreams(), os.Args[1:])
}

// ExecuteWith runs the CLI using the provided streams and args.
func ExecuteWith(streams IOStreams, args []string) int {
	if err := config.LoadLocalEnv(localEnvDirectory(args)); err != nil {
		fmt.Fprintln(streams.ErrOut, "Error:", err)
		return ExitError
	}
	return ExecuteWithRegistry(streams, args, newProviderRegistry())
}

// ExecuteWithRegistry runs the CLI with an explicit provider registry.
func ExecuteWithRegistry(streams IOStreams, args []string, reg *provider.Registry) int {
	cmd := NewRootCommandWithRegistry(streams, reg)
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		if errors.Is(err, errPlanHasChanges) {
			return ExitChanges
		}
		if errors.Is(err, errDestroyCancelled) {
			return ExitOK
		}
		fmt.Fprintln(streams.ErrOut, "Error:", err)
		if isUsageError(err) {
			return ExitUsage
		}
		return ExitError
	}

	return ExitOK
}

func isUsageError(err error) bool {
	var u usageError
	if errors.As(err, &u) {
		return true
	}

	msg := err.Error()
	return strings.Contains(msg, "unknown command") ||
		strings.Contains(msg, "unknown flag") ||
		strings.Contains(msg, "accepts no arguments")
}
