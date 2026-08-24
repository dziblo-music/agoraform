package cli

import (
	"errors"
	"fmt"

	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/spf13/cobra"
)

// errPlanHasChanges is a successful plan that requires apply. ExecuteWith
// maps it to ExitChanges without printing an error.
var errPlanHasChanges = errors.New("plan has changes")

func newPlanCommand(reg *provider.Registry) *cobra.Command {
	var fileFlag string

	cmd := &cobra.Command{
		Use:   "plan [file]",
		Short: "Show the changes required to reconcile declared and live configuration",
		Long: `Read live resources through registered providers and show the
changes required to reach the desired manifest state.

plan only validates configuration and reads remote state. It never creates,
updates, or imports resources. Persisted identities are read from
agoraform.state.json next to the manifest.

Exit codes:
  0  plan succeeded and no changes are required
  1  planning failed
  2  plan succeeded and changes are present
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
				return fmt.Errorf("plan requires a registered provider; none are registered")
			}

			st, err := state.Load(state.PathForManifest(path))
			if err != nil {
				return err
			}

			result, err := plan.BuildWithState(cmd.Context(), m.Resources, func(addr resource.Address) (provider.Reader, error) {
				return reg.LookupFor(addr)
			}, st)
			if err != nil {
				return err
			}

			fmt.Fprint(cmd.OutOrStdout(), plan.Format(result))
			if result.HasChanges() {
				return errPlanHasChanges
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&fileFlag, "file", "f", "", "Path to the Agoraform manifest (default agoraform.yaml)")
	return cmd
}
