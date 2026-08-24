package importer_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/importer"
	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
)

func TestRunSuccessPersistsIdentityAndOmitsComputed(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustAddress(t, "fake.widget.homepage")
	seedImported(t, p, addr, resource.Attributes{
		fake.AttrTitle: "Imported",
		fake.AttrColor: "blue",
	}, resource.Attributes{fake.AttrSerial: 9})

	st := mustStore(t)
	got, err := importer.Run(context.Background(), addr, "widget-imported", lookupProvider(p), st)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Identity.ID != "widget-imported" {
		t.Fatalf("identity = %q, want widget-imported", got.Identity.ID)
	}

	id, ok, err := st.Identity(addr)
	if err != nil || !ok || id.ID != "widget-imported" {
		t.Fatalf("persisted identity = (%v,%v,%v)", id, ok, err)
	}

	parsed, err := manifest.Parse([]byte(got.YAML), "generated")
	if err != nil {
		t.Fatalf("generated YAML is not a valid manifest: %v\n%s", err, got.YAML)
	}
	if len(parsed.Resources) != 1 {
		t.Fatalf("generated resources = %d, want 1", len(parsed.Resources))
	}
	res := parsed.Resources[0]
	if res.Address != addr {
		t.Fatalf("generated address = %s, want %s", res.Address, addr)
	}
	if _, ok := res.Attributes[fake.AttrSerial]; ok {
		t.Fatalf("computed serial leaked into YAML:\n%s", got.YAML)
	}
	if strings.Contains(got.YAML, "widget-imported") {
		t.Fatalf("provider identity leaked into YAML:\n%s", got.YAML)
	}
	if res.Attributes[fake.AttrTitle] != "Imported" {
		t.Fatalf("title = %v, want Imported", res.Attributes[fake.AttrTitle])
	}
	if res.Attributes[fake.AttrColor] != "blue" {
		t.Fatalf("color = %v, want blue", res.Attributes[fake.AttrColor])
	}

	wantYAML := `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      color: blue
      title: Imported
`
	if got.YAML != wantYAML {
		t.Fatalf("YAML = %q, want %q", got.YAML, wantYAML)
	}

	_, creates, updates, imports := p.Calls()
	if creates != 0 || updates != 0 {
		t.Fatalf("import mutated remote state: creates=%d updates=%d", creates, updates)
	}
	if imports != 1 {
		t.Fatalf("imports = %d, want 1", imports)
	}
}

func TestRunYAMLEmitsLogicalReferenceNotIdentity(t *testing.T) {
	t.Parallel()

	p := fake.New()
	parent := mustAddress(t, "fake.widget.homepage")
	addr := mustAddress(t, "fake.widget.banner")
	seedImported(t, p, addr, resource.Attributes{
		fake.AttrTitle:  "Banner",
		fake.AttrParent: resource.Ref{Address: parent},
	}, nil)

	st := mustStore(t)
	got, err := importer.Run(context.Background(), addr, "widget-imported", lookupProvider(p), st)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(got.YAML, "parent: fake.widget.homepage") {
		t.Fatalf("YAML missing logical reference:\n%s", got.YAML)
	}
	if strings.Contains(got.YAML, "widget-imported") {
		t.Fatalf("provider identity leaked into YAML:\n%s", got.YAML)
	}
}

func TestRunYAMLIsDeterministic(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustAddress(t, "fake.widget.homepage")
	seedImported(t, p, addr, resource.Attributes{
		fake.AttrColor: "blue",
		fake.AttrTitle: "Imported",
	}, resource.Attributes{fake.AttrSerial: 3})

	first, err := importer.Run(context.Background(), addr, "widget-imported", lookupProvider(p), mustStore(t))
	if err != nil {
		t.Fatal(err)
	}
	second, err := importer.Run(context.Background(), addr, "widget-imported", lookupProvider(p), mustStore(t))
	if err != nil {
		t.Fatal(err)
	}
	if first.YAML != second.YAML {
		t.Fatalf("YAML differed:\n%s\n---\n%s", first.YAML, second.YAML)
	}
}

func TestRunThenPlanIsUnchanged(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustAddress(t, "fake.widget.homepage")
	seedImported(t, p, addr, resource.Attributes{fake.AttrTitle: "Imported"}, resource.Attributes{fake.AttrSerial: 4})

	st := mustStore(t)
	got, err := importer.Run(context.Background(), addr, "widget-imported", lookupProvider(p), st)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := manifest.Parse([]byte(got.YAML), "generated")
	if err != nil {
		t.Fatal(err)
	}

	planned, err := plan.BuildWithState(context.Background(), parsed.Resources, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("plan after import: %v", err)
	}
	if planned.HasChanges() {
		t.Fatalf("plan after import has changes: %+v", planned.Changes)
	}
	if planned.Changes[0].Identity.ID != "widget-imported" {
		t.Fatalf("plan identity = %q, want widget-imported", planned.Changes[0].Identity.ID)
	}

	_, creates, updates, _ := p.Calls()
	if creates != 0 || updates != 0 {
		t.Fatalf("plan after import mutated provider: creates=%d updates=%d", creates, updates)
	}
}

func TestRunThenStaleIdentityIsNotCreate(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustAddress(t, "fake.widget.homepage")
	seedImported(t, p, addr, resource.Attributes{fake.AttrTitle: "Imported"}, nil)

	st := mustStore(t)
	got, err := importer.Run(context.Background(), addr, "widget-imported", lookupProvider(p), st)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := manifest.Parse([]byte(got.YAML), "generated")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Remove("widget-imported") {
		t.Fatal("failed to remove imported identity")
	}

	_, err = plan.BuildWithState(context.Background(), parsed.Resources, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err == nil {
		t.Fatal("plan succeeded, want stale identity error")
	}
	if !errors.Is(err, state.ErrStaleIdentity) {
		t.Fatalf("error = %v, want ErrStaleIdentity", err)
	}
}

func TestRunRejectsConflictingState(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustAddress(t, "fake.widget.homepage")
	seedImported(t, p, addr, resource.Attributes{fake.AttrTitle: "Imported"}, nil)
	st := mustStore(t)
	if err := st.Bind(addr, resource.Identity{ID: "already-bound"}); err != nil {
		t.Fatal(err)
	}

	_, err := importer.Run(context.Background(), addr, "widget-imported", lookupProvider(p), st)
	if err == nil {
		t.Fatal("Run succeeded, want conflicting state error")
	}
	if !strings.Contains(err.Error(), "already bound") {
		t.Fatalf("error = %q, want already bound", err)
	}
	_, creates, updates, imports := p.Calls()
	if creates != 0 || updates != 0 || imports != 0 {
		t.Fatalf("conflicting import called provider: creates=%d updates=%d imports=%d", creates, updates, imports)
	}
}

func TestRunRejectsDuplicateIdentity(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustAddress(t, "fake.widget.homepage")
	other := mustAddress(t, "fake.widget.other")
	seedImported(t, p, addr, resource.Attributes{fake.AttrTitle: "Imported"}, nil)
	st := mustStore(t)
	if err := st.Bind(other, resource.Identity{ID: "widget-imported"}); err != nil {
		t.Fatal(err)
	}

	_, err := importer.Run(context.Background(), addr, "widget-imported", lookupProvider(p), st)
	if err == nil {
		t.Fatal("Run succeeded, want duplicate identity error")
	}
	if !strings.Contains(err.Error(), "could not persist identity") && !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error = %q, want duplicate identity", err)
	}
}

func TestRunEmptyRemoteID(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustAddress(t, "fake.widget.homepage")
	_, err := importer.Run(context.Background(), addr, "  ", lookupProvider(p), mustStore(t))
	if err == nil || !strings.Contains(err.Error(), "remote identifier is empty") {
		t.Fatalf("error = %v, want empty remote identifier", err)
	}
	_, creates, updates, imports := p.Calls()
	if creates != 0 || updates != 0 || imports != 0 {
		t.Fatalf("empty id called provider: creates=%d updates=%d imports=%d", creates, updates, imports)
	}
}

func TestRunResourceNotFound(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustAddress(t, "fake.widget.homepage")
	_, err := importer.Run(context.Background(), addr, "missing", lookupProvider(p), mustStore(t))
	if err == nil {
		t.Fatal("Run succeeded, want not found")
	}
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "was not found") {
		t.Fatalf("error = %q, want not found diagnostic", err)
	}
}

func TestRunProviderError(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	addr := mustAddress(t, "fake.widget.homepage")
	seedImported(t, inner, addr, resource.Attributes{fake.AttrTitle: "Imported"}, nil)
	p := &scriptedProvider{Provider: inner, importErr: errors.New("provider exploded")}

	_, err := importer.Run(context.Background(), addr, "widget-imported", lookupProvider(p), mustStore(t))
	if err == nil || !strings.Contains(err.Error(), "provider exploded") {
		t.Fatalf("error = %v, want provider exploded", err)
	}
}

func TestRunUnrepresentableRemoteConfig(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	addr := mustAddress(t, "fake.widget.homepage")
	seedImported(t, inner, addr, resource.Attributes{fake.AttrTitle: "Imported"}, nil)
	p := &scriptedProvider{Provider: inner, attrs: resource.Attributes{
		fake.AttrTitle:  "Imported",
		fake.AttrSerial: 1,
	}}

	st := mustStore(t)
	_, err := importer.Run(context.Background(), addr, "widget-imported", lookupProvider(p), st)
	if err == nil || !strings.Contains(err.Error(), "cannot be represented") {
		t.Fatalf("error = %v, want unrepresentable schema", err)
	}
	if _, ok, _ := st.Identity(addr); ok {
		t.Fatal("unrepresentable import persisted identity")
	}
}

func TestRunEmptyProviderIdentity(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	addr := mustAddress(t, "fake.widget.homepage")
	seedImported(t, inner, addr, resource.Attributes{fake.AttrTitle: "Imported"}, nil)
	p := &scriptedProvider{Provider: inner, stripIdentity: true}

	st := mustStore(t)
	_, err := importer.Run(context.Background(), addr, "widget-imported", lookupProvider(p), st)
	if err == nil || !strings.Contains(err.Error(), "no identity") {
		t.Fatalf("error = %v, want missing identity", err)
	}
	if _, ok, _ := st.Identity(addr); ok {
		t.Fatal("empty identity was persisted")
	}
}

func TestRunStateWriteFailure(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustAddress(t, "fake.widget.homepage")
	seedImported(t, p, addr, resource.Attributes{fake.AttrTitle: "Imported"}, nil)
	st := &failingStore{err: errors.New("disk full")}

	_, err := importer.Run(context.Background(), addr, "widget-imported", lookupProvider(p), st)
	if err == nil {
		t.Fatal("Run succeeded, want state write failure")
	}
	if !strings.Contains(err.Error(), "could not persist identity") {
		t.Fatalf("error = %q, want persist failure", err)
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("error = %q, want underlying write error", err)
	}
}

func TestRunUnknownResourceType(t *testing.T) {
	t.Parallel()

	addr := mustAddress(t, "fake.gadget.homepage")
	_, err := importer.Run(context.Background(), addr, "1", func(a resource.Address) (provider.Provider, error) {
		return nil, errors.New("unknown resource type \"gadget\" for provider \"fake\"")
	}, mustStore(t))
	if err == nil || !strings.Contains(err.Error(), "unknown resource type") {
		t.Fatalf("error = %v, want unknown resource type", err)
	}
}

func TestRunConnectionFailure(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	addr := mustAddress(t, "fake.widget.homepage")
	seedImported(t, inner, addr, resource.Attributes{fake.AttrTitle: "Imported"}, nil)
	p := &connFailProvider{Provider: inner, err: errors.New("unauthorized")}

	_, err := importer.Run(context.Background(), addr, "widget-imported", lookupProvider(p), mustStore(t))
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("error = %v, want connection failure", err)
	}
	_, _, _, imports := inner.Calls()
	if imports != 0 {
		t.Fatalf("imports = %d, want 0 after connection failure", imports)
	}
}

func TestRunRequiresStoreAndLookup(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustAddress(t, "fake.widget.homepage")
	_, err := importer.Run(context.Background(), addr, "1", lookupProvider(p), nil)
	if err == nil || !strings.Contains(err.Error(), "state store is required") {
		t.Fatalf("error = %v, want state store required", err)
	}
	_, err = importer.Run(context.Background(), addr, "1", nil, mustStore(t))
	if err == nil || !strings.Contains(err.Error(), "provider lookup is required") {
		t.Fatalf("error = %v, want lookup required", err)
	}
}

func TestFormat(t *testing.T) {
	t.Parallel()

	got := importer.Format(importer.Result{
		Address:  mustAddress(t, "fake.widget.homepage"),
		Identity: resource.Identity{ID: "widget-1"},
		YAML:     "apiVersion: agoraform.io/v1alpha1\n",
	}, "agoraform.state.json")
	for _, want := range []string{
		"Imported fake.widget.homepage (remote identity widget-1).",
		"Identity persisted to agoraform.state.json.",
		"Review this configuration",
		"apiVersion: agoraform.io/v1alpha1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Format missing %q:\n%s", want, got)
		}
	}
}

func seedImported(t *testing.T, p *fake.Provider, addr resource.Address, attrs, computed resource.Attributes) {
	t.Helper()
	if err := p.Seed(resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: "widget-imported"},
		Attributes: attrs.Clone(),
		Computed:   computed.Clone(),
	}); err != nil {
		t.Fatal(err)
	}
}

func lookupProvider(p provider.Provider) importer.Lookup {
	return func(resource.Address) (provider.Provider, error) {
		return p, nil
	}
}

func mustStore(t *testing.T) *state.Store {
	t.Helper()
	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func mustAddress(t *testing.T, s string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress(s)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

type scriptedProvider struct {
	provider.Provider
	importErr     error
	stripIdentity bool
	attrs         resource.Attributes
}

func (p *scriptedProvider) Import(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	if p.importErr != nil {
		return resource.RemoteResource{}, p.importErr
	}
	live, err := p.Provider.Import(ctx, addr, id)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if p.stripIdentity {
		live.Identity = resource.Identity{}
	}
	if p.attrs != nil {
		live.Attributes = p.attrs.Clone()
	}
	return live, nil
}

type connFailProvider struct {
	provider.Provider
	err error
}

func (p *connFailProvider) CheckConnection(context.Context) error {
	return p.err
}

type failingStore struct {
	err error
}

func (s *failingStore) Identity(resource.Address) (resource.Identity, bool, error) {
	return resource.Identity{}, false, nil
}

func (s *failingStore) RecordImport(resource.Address, string) error {
	return s.err
}

var _ importer.Store = (*failingStore)(nil)
var _ importer.Store = (*state.Store)(nil)
