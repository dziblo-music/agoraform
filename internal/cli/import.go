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
	var external bool
	var release bool
	var adopt bool

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

A normal import takes managed ownership. Destroy can later delete that object.

--external binds the same object as a reference-only resource. Plan and apply
refresh its identity for $ref consumers and never create or update it. Destroy
removes only the local binding. Add the printed lifecycle block to the manifest.

--external --release converts an existing managed binding into an external
reference without deleting the remote object. The REMOTE-ID must be the
identity already stored for ADDRESS.

--adopt ADDRESS converts an external reference into managed ownership after
reading the remote object again. It does not accept a different REMOTE-ID and
does not modify the remote object. Remove lifecycle.ownership from the manifest
afterward.

The generated YAML is for review. Import does not rewrite an existing
manifest. Add the printed resource to your configuration, then run plan.

Use --file to locate the local state file next to a manifest or
configuration directory. The default path is agoraform.yaml, so state is
written to agoraform.state.json in the current directory. The manifest
itself is not read. A directory path uses one state file in that directory.

Exit codes:
  0  import succeeded
  1  import failed
  3  invalid invocation`,
		Args: func(cmd *cobra.Command, args []string) error {
			external, _ = cmd.Flags().GetBool("external")
			release, _ = cmd.Flags().GetBool("release")
			adopt, _ = cmd.Flags().GetBool("adopt")
			if adopt && (external || release) {
				return usageError{err: fmt.Errorf("--adopt cannot be combined with --external or --release")}
			}
			if release && !external {
				return usageError{err: fmt.Errorf("--release requires --external")}
			}
			if adopt {
				if len(args) != 1 {
					return usageError{err: fmt.Errorf("--adopt accepts 1 arg(s), received %d", len(args))}
				}
				return nil
			}
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

			st, err := state.Load(state.PathForConfig(path))
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

			var result importer.Result
			switch {
			case adopt:
				result, err = importer.RunAdopt(cmd.Context(), addr, lookup, st)
			case external:
				result, err = importer.RunExternal(cmd.Context(), addr, args[1], lookup, st, release)
			default:
				result, err = importer.Run(cmd.Context(), addr, args[1], lookup, st)
			}
			if err != nil {
				return err
			}

			fmt.Fprint(cmd.OutOrStdout(), importer.Format(result, st.Path()))
			return nil
		},
	}

	cmd.Flags().StringVarP(&fileFlag, "file", "f", "", "Path to the Agoraform manifest or configuration directory used to locate local state (default agoraform.yaml)")
	cmd.Flags().BoolVar(&external, "external", false, "Bind ADDRESS as a reference-only external resource instead of taking managed ownership")
	cmd.Flags().BoolVar(&release, "release", false, "With --external, convert an existing managed binding to external without deleting the remote object")
	cmd.Flags().BoolVar(&adopt, "adopt", false, "Convert an external reference to managed ownership without modifying the remote object")
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
