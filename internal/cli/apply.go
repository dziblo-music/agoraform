package cli

import (
	"fmt"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/spf13/cobra"
)

func newApplyCommand(reg *provider.Registry) *cobra.Command {
	var fileFlag string

	cmd := &cobra.Command{
		Use:   "apply [file]",
		Short: "Apply reviewed changes through provider APIs",
		Long: `Read live resources, plan the required create and update actions, and
apply them through registered providers.

apply loads and validates the manifest and local identity state before any
mutation. It reuses the same plan engine as the plan command and never
deletes remote resources. Successful creates persist the provider-native
identity returned by the provider in agoraform.state.json next to the
manifest.

Exit codes:
  0  apply succeeded
  1  apply failed
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
				return fmt.Errorf("apply requires a registered provider; none are registered")
			}

			st, err := state.Load(state.PathForManifest(path))
			if err != nil {
				return err
			}

			_, err = apply.Run(cmd.Context(), m.Resources, func(addr resource.Address) (provider.Provider, error) {
				return reg.LookupFor(addr)
			}, st, cmd.OutOrStdout())
			return err
		},
	}

	cmd.Flags().StringVarP(&fileFlag, "file", "f", "", "Path to the Agoraform manifest (default agoraform.yaml)")
	return cmd
}
