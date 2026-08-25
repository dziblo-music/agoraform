package cli

import (
	"fmt"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/plan"
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
		Long: `Read live resources, plan the required actions, and apply them
through registered providers.

apply loads and validates the manifest, including provider-specific
non-secret desired state, the resource dependency graph, and local identity
state before any mutation. Creates and updates run sequentially in
prerequisite-first order. Provider finalization actions that were visible in
the plan run only after every resource mutation succeeds. Referenced
resources receive provider-native identities at apply time; those identities
are not written into the manifest. apply never deletes remote resources.
Successful creates persist the provider-native identity returned by the
provider in agoraform.state.json next to the manifest.

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

			planned, err := plan.BuildWithState(cmd.Context(), m.Resources, func(addr resource.Address) (provider.Reader, error) {
				return reg.LookupFor(addr)
			}, st)
			if err != nil {
				return err
			}
			if err := attachFinalizations(cmd.Context(), reg, planned); err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			result, err := apply.Execute(cmd.Context(), planned, m.Resources, func(addr resource.Address) (provider.Provider, error) {
				return reg.LookupFor(addr)
			}, st, out)
			if err != nil {
				return err
			}
			if err := executeFinalizations(cmd.Context(), reg, planned.Finalizations, out); err != nil {
				return err
			}
			if result.Created+result.Updated > 0 || len(planned.Finalizations) > 0 {
				fmt.Fprintln(out)
			}
			fmt.Fprint(out, apply.Format(result))
			return nil
		},
	}

	cmd.Flags().StringVarP(&fileFlag, "file", "f", "", "Path to the Agoraform manifest (default agoraform.yaml)")
	return cmd
}
