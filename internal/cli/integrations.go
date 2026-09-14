package cli

import (
	"fmt"

	"github.com/dziblo-music/agoraform/internal/integration"
	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/spf13/cobra"
)

func newIntegrationsCommand(reg *provider.Registry) *cobra.Command {
	var fileFlag string

	cmd := &cobra.Command{
		Use:   "integrations [file]",
		Short: "Show the application integration contract for declared events",
		Long: `Read the applicationEvents block and display the non-secret provider
identifiers that external application instrumentation must use to connect
its event emission to the managed marketing infrastructure.

For resources that have already been applied, provider identifiers such as
Google Ads conversion IDs and Meta Pixel IDs are resolved from live provider
state. Resources that have not yet been applied show "(not yet applied)".

Agoraform never generates, modifies, deploys, or executes application code.
This command is read-only and informational.

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

			if len(m.ApplicationEvents) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No application events declared.")
				return nil
			}

			if err := manifest.CheckProviders(cmd.Context(), m, reg); err != nil {
				return err
			}

			st, err := state.Load(state.PathForManifest(path))
			if err != nil {
				return err
			}

			events, err := integration.Resolve(
				cmd.Context(),
				m.ApplicationEvents,
				m.Resources,
				st,
				func(addr resource.Address) (provider.Reader, error) {
					return reg.LookupFor(addr)
				},
			)
			if err != nil {
				return err
			}

			fmt.Fprint(cmd.OutOrStdout(), integration.Format(events))
			return nil
		},
	}

	cmd.Flags().StringVarP(&fileFlag, "file", "f", "", "Path to the Agoraform manifest (default agoraform.yaml)")
	return cmd
}
