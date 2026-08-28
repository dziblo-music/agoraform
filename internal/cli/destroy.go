package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dziblo-music/agoraform/internal/destroy"
	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/spf13/cobra"
)

var errDestroyCancelled = errors.New("destroy cancelled")

func newDestroyCommand(reg *provider.Registry) *cobra.Command {
	var fileFlag string
	var autoApprove bool

	cmd := &cobra.Command{
		Use:   "destroy [file]",
		Short: "Destroy managed resources in reverse dependency order",
		Long: `Destroy managed resources using the same manifest graph and local
identity state as plan and apply.

destroy loads and validates the manifest, including the resource
dependency graph, provider lifecycle capability, and local identity
state before any mutation. Supported resources are destroyed in reverse
dependency order so dependents are removed before prerequisites.
Provider finalization actions that were visible in the destroy plan run
only after every destructive mutation succeeds.

Resources without a local state binding are reported as not managed and
are not changed remotely. Identities present in agoraform.state.json but
absent from the manifest are preserved; destroy does not prune them.
Providers may report destroy as unsupported or provider-owned: those
resources remain in state, do not block supported teardown, and cause a
non-zero exit after supported operations complete.

Interactive terminals require typing "yes" after the plan is shown.
Non-interactive sessions must pass --auto-approve. Declining confirmation
exits successfully with no mutations.

Exit codes:
  0  destroy succeeded, or confirmation was declined
  1  destroy failed, or supported teardown left unsupported resources
  3  invalid invocation

The default manifest path is agoraform.yaml.`,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usageError{err: fmt.Errorf("accepts at most 1 arg(s), received %d", len(args))}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := manifestPath(fileFlag, args)
			if err != nil {
				return usageError{err: err}
			}

			m, err := manifest.LoadFile(path)
			if err != nil {
				return err
			}
			if err := manifest.CheckProviders(cmd.Context(), m, reg); err != nil {
				return err
			}
			if len(m.Resources) > 0 && (reg == nil || reg.Len() == 0) {
				return fmt.Errorf("destroy requires a registered provider; none are registered")
			}

			st, err := state.Load(state.PathForManifest(path))
			if err != nil {
				return err
			}

			in := cmd.InOrStdin()
			approve := func(*destroy.Plan) error {
				return confirmDestroy(in, cmd.OutOrStdout(), autoApprove)
			}
			_, err = destroy.Run(cmd.Context(), m.Resources, nil, st, cmd.OutOrStdout(), approve, reg)
			return err
		},
	}

	cmd.Flags().StringVarP(&fileFlag, "file", "f", "", "Path to the Agoraform manifest (default agoraform.yaml)")
	cmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "Skip interactive confirmation")
	return cmd
}

func confirmDestroy(in io.Reader, out io.Writer, autoApprove bool) error {
	return confirmDestroyMode(in, out, autoApprove, isInteractive(in))
}

func confirmDestroyMode(in io.Reader, out io.Writer, autoApprove, interactive bool) error {
	if autoApprove {
		return nil
	}
	if !interactive {
		return fmt.Errorf("destroy requires confirmation; rerun with --auto-approve")
	}
	fmt.Fprint(out, "Do you really want to destroy these resources?\n  Only 'yes' will be accepted.\n\nEnter a value: ")
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("destroy confirmation: %w", err)
		}
		return fmt.Errorf("destroy confirmation was not provided")
	}
	if strings.TrimSpace(scanner.Text()) != "yes" {
		fmt.Fprintln(out, "Destroy cancelled.")
		return errDestroyCancelled
	}
	return nil
}

func isInteractive(in io.Reader) bool {
	f, ok := in.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
