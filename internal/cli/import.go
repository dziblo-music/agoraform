package cli

import (
	"fmt"
	"strings"

	"github.com/dziblo-music/agoraform/internal/importer"
	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/googleads"
	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/meta"
	"github.com/spf13/cobra"
)

func newImportCommand(reg *provider.Registry) *cobra.Command {
	var fileFlag string

	cmd := &cobra.Command{
		Use:   "import ADDRESS REMOTE-ID",
		Short: "Import existing remote resources into local management",
		Long: `Read an existing remote resource and bind it to a logical Agoraform
address without recreating it.

import resolves the provider from ADDRESS, reads the remote object identified
by REMOTE-ID, prints deterministic YAML for configurable fields, and persists
the provider-native identity in agoraform.state.json. It never creates,
updates, or deletes the remote resource. Computed and identity fields are
omitted from generated configuration.

The generated YAML is for review. Import does not rewrite an existing
manifest. Add the printed resource to your configuration, then run plan.

Use --file to locate the local state file next to a manifest. The default
manifest path is agoraform.yaml, so state is written to agoraform.state.json
in the current directory. The manifest file itself is not read.

Exit codes:
  0  import succeeded
  1  import failed
  3  invalid invocation`,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 2 {
				return usageError{err: fmt.Errorf("accepts 2 arg(s), received %d", len(args))}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			path := importManifestPath(fileFlag)

			addr, err := resource.ParseAddress(args[0])
			if err != nil {
				return fmt.Errorf("import: %w", err)
			}

			if reg == nil || reg.Len() == 0 {
				return fmt.Errorf("import requires a registered provider; none are registered")
			}

			st, err := state.Load(state.PathForManifest(path))
			if err != nil {
				return err
			}

			var catalog provider.OutputMatcher
			lookup := func(a resource.Address) (provider.Provider, error) {
				p, err := reg.LookupFor(a)
				if err != nil {
					return nil, err
				}
				attachImportIdentityCatalog(p, st)
				attachImportOutputMatcher(p, catalog)
				return p, nil
			}
			catalog = importer.NewOutputCatalog(stateBindings{st}, lookup)

			result, err := importer.Run(cmd.Context(), addr, args[1], lookup, st)
			if err != nil {
				return err
			}

			fmt.Fprint(cmd.OutOrStdout(), importer.Format(result, st.Path()))
			return nil
		},
	}

	cmd.Flags().StringVarP(&fileFlag, "file", "f", "", "Path to the Agoraform manifest used to locate local state (default agoraform.yaml)")
	return cmd
}

func attachImportIdentityCatalog(p provider.Provider, st *state.Store) {
	type matomoCatalogSetter interface {
		SetIdentityCatalog(matomo.IdentityCatalog)
	}
	type googleAdsCatalogSetter interface {
		SetIdentityCatalog(googleads.IdentityCatalog)
	}
	type metaCatalogSetter interface {
		SetIdentityCatalog(meta.IdentityCatalog)
	}
	if s, ok := p.(matomoCatalogSetter); ok {
		s.SetIdentityCatalog(st)
	}
	if s, ok := p.(googleAdsCatalogSetter); ok {
		s.SetIdentityCatalog(st)
	}
	if s, ok := p.(metaCatalogSetter); ok {
		s.SetIdentityCatalog(st)
	}
}

func attachImportOutputMatcher(p provider.Provider, matcher provider.OutputMatcher) {
	type setter interface {
		SetOutputMatcher(provider.OutputMatcher)
	}
	if s, ok := p.(setter); ok {
		s.SetOutputMatcher(matcher)
	}
}

type stateBindings struct {
	st *state.Store
}

func (b stateBindings) Bindings(providerName, resourceType string) ([]importer.RemoteBinding, error) {
	if b.st == nil {
		return nil, nil
	}
	got, err := b.st.Bindings(providerName, resourceType)
	if err != nil {
		return nil, err
	}
	out := make([]importer.RemoteBinding, len(got))
	for i, item := range got {
		out[i] = importer.RemoteBinding{Address: item.Address, RemoteID: item.RemoteID}
	}
	return out, nil
}

func importManifestPath(fileFlag string) string {
	fileFlag = strings.TrimSpace(fileFlag)
	if fileFlag != "" {
		return fileFlag
	}
	return manifest.DefaultFilename
}
