package cli

import (
	"fmt"

	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/spf13/cobra"
)

func newPublishCommand(reg *provider.Registry) *cobra.Command {
	var fileFlag string

	cmd := &cobra.Command{
		Use:   "publish [file]",
		Short: "Publish an already-applied Tag Manager container draft",
		Long: `Create a Tag Manager container version from the current draft and
publish it to the configured environment.

publish loads and validates the manifest using the same file selection
conventions as validate, plan, and apply. It never creates or updates
individual resources. apply writes Tag Manager changes to the container
draft only and never publishes as a side effect.

Re-running publish when the draft is already represented by the currently
published version does not create or publish a duplicate version.

Exit codes:
  0  publish succeeded (including a no-op)
  1  publish failed
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
				return fmt.Errorf("publish requires a registered provider; none are registered")
			}

			return runPublish(cmd, reg)
		},
	}

	cmd.Flags().StringVarP(&fileFlag, "file", "f", "", "Path to the Agoraform manifest (default agoraform.yaml)")
	return cmd
}

func runPublish(cmd *cobra.Command, reg *provider.Registry) error {
	out := cmd.OutOrStdout()
	if reg == nil || reg.Len() == 0 {
		fmt.Fprint(out, "No publication required.\n")
		return nil
	}

	found := false
	for _, p := range reg.List() {
		pub, ok := p.(provider.ContainerPublisher)
		if !ok {
			continue
		}
		found = true
		result, err := pub.PublishContainer(cmd.Context())
		if err != nil {
			return err
		}
		fmt.Fprint(out, formatPublish(result))
	}
	if !found {
		fmt.Fprint(out, "No publication required.\n")
	}
	return nil
}

func formatPublish(result provider.PublishResult) string {
	addr := result.Address
	if addr == "" {
		addr = "container"
	}
	if !result.Created {
		return fmt.Sprintf("%s: no publication required\n", addr)
	}
	return fmt.Sprintf("%s: creating version...\n%s: published\n", addr, addr)
}
