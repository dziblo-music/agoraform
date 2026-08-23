package cli

import (
	"fmt"

	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/spf13/cobra"
)

func newValidateCommand(reg *provider.Registry) *cobra.Command {
	var fileFlag string

	cmd := &cobra.Command{
		Use:   "validate [file]",
		Short: "Validate configuration files and provider settings",
		Long: `Validate an Agoraform YAML manifest.

The command loads the manifest, checks the v0.1 schema, resource addresses,
and duplicate logical names. When providers are registered, it also checks
resource types, provider connection settings, and provider-specific required
fields.

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

			n := len(m.Resources)
			noun := "resources"
			if n == 1 {
				noun = "resource"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Validated %s (%d %s).\n", path, n, noun)
			return nil
		},
	}

	cmd.Flags().StringVarP(&fileFlag, "file", "f", "", "Path to the Agoraform manifest (default agoraform.yaml)")
	return cmd
}

func manifestPath(fileFlag string, args []string) (string, error) {
	if fileFlag != "" && len(args) > 0 {
		return "", fmt.Errorf("specify a manifest as an argument or with --file, not both")
	}
	if len(args) == 1 {
		return args[0], nil
	}
	if fileFlag != "" {
		return fileFlag, nil
	}
	return manifest.DefaultFilename, nil
}
