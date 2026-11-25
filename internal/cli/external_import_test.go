package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
)

func TestImportExternalThenPlanAndDestroyDoesNotMutate(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustCLIAddress(t, "fake.widget.homepage")
	if err := p.Seed(resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage"},
		Computed:   resource.Attributes{fake.AttrSerial: 2, fake.OutputToken: "tok"},
	}); err != nil {
		t.Fatal(err)
	}
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "--external", "-f", manifestPath, "fake.widget.homepage", "widget-1"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "external reference") || !strings.Contains(out, "ownership: external") {
		t.Fatalf("stdout = %s", out)
	}
	if strings.Contains(out, "serial") {
		t.Fatalf("computed field leaked:\n%s", out)
	}
	if err := os.WriteFile(manifestPath, []byte(extractYAML(out)), 0o600); err != nil {
		t.Fatal(err)
	}

	st, err := state.Load(state.PathForManifest(manifestPath))
	if err != nil {
		t.Fatal(err)
	}
	ownership, ok, err := st.Ownership(addr)
	if err != nil || !ok || ownership != resource.OwnershipExternal {
		t.Fatalf("ownership = (%s, %v, %v)", ownership, ok, err)
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "fake.widget.homepage", "widget-1"}, reg)
	if code == cli.ExitOK || !strings.Contains(stderr.String(), "--adopt") {
		t.Fatalf("managed import of external exit = %d; stderr=%q", code, stderr.String())
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "= fake.widget.homepage") {
		t.Fatalf("plan = %s", stdout.String())
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"destroy", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("destroy exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if p.Destroys() != 0 {
		t.Fatalf("destroys = %d, want 0", p.Destroys())
	}
	if _, err := p.Read(context.Background(), resource.Resource{Address: addr, Identity: resource.Identity{ID: "widget-1"}}); err != nil {
		t.Fatalf("remote missing: %v", err)
	}
}

func TestImportReleaseAndAdoptOwnership(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustCLIAddress(t, "fake.widget.homepage")
	if err := p.Seed(resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage"},
	}); err != nil {
		t.Fatal(err)
	}
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(t.TempDir(), "agoraform.yaml")

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "fake.widget.homepage", "widget-1"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import exit = %d; stderr=%q", code, stderr.String())
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "--external", "-f", manifestPath, "fake.widget.homepage", "widget-1"}, reg)
	if code == cli.ExitOK || !strings.Contains(stderr.String(), "--release") {
		t.Fatalf("unconfirmed release exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "--external", "--release", "-f", manifestPath, "fake.widget.homepage", "widget-1"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("release exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	st, err := state.Load(state.PathForManifest(manifestPath))
	if err != nil {
		t.Fatal(err)
	}
	ownership, _, err := st.Ownership(addr)
	if err != nil || ownership != resource.OwnershipExternal {
		t.Fatalf("ownership after release = %s, err %v", ownership, err)
	}
	if p.Destroys() != 0 {
		t.Fatalf("release destroyed the resource")
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "--adopt", "-f", manifestPath, "fake.widget.homepage"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("adopt exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "Adopted fake.widget.homepage") {
		t.Fatalf("stdout = %s", stdout.String())
	}
	st, err = state.Load(state.PathForManifest(manifestPath))
	if err != nil {
		t.Fatal(err)
	}
	ownership, _, err = st.Ownership(addr)
	if err != nil || ownership != resource.OwnershipManaged {
		t.Fatalf("ownership after adopt = %s, err %v", ownership, err)
	}
	_, creates, updates, _ := p.Calls()
	if creates != 0 || updates != 0 || p.Destroys() != 0 {
		t.Fatalf("adopt mutated provider creates=%d updates=%d destroys=%d", creates, updates, p.Destroys())
	}
}

func TestImportExternalRejectsUnsupportedResourceTypeBeforeStateWrite(t *testing.T) {
	t.Parallel()

	p := &unsupportedExternalProvider{Provider: fake.New()}
	addr := mustCLIAddress(t, "fake.widget.homepage")
	if err := p.Seed(resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage"},
	}); err != nil {
		t.Fatal(err)
	}
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(t.TempDir(), "agoraform.yaml")

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "--external", "-f", manifestPath, "fake.widget.homepage", "widget-1"}, reg)
	if code == cli.ExitOK || !strings.Contains(stderr.String(), "does not support external ownership") {
		t.Fatalf("unsupported external import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}

	st, err := state.Load(state.PathForManifest(manifestPath))
	if err != nil {
		t.Fatal(err)
	}
	if _, bound, err := st.Identity(addr); err != nil || bound {
		t.Fatalf("unsupported external import wrote state: bound=%v err=%v", bound, err)
	}
}

func TestImportSeedsExternalReferencesWhenStateFileIsMissing(t *testing.T) {
	t.Parallel()

	p := fake.New()
	home := mustCLIAddress(t, "fake.widget.homepage")
	note := mustCLIAddress(t, "fake.widget.note")
	if err := p.Seed(resource.RemoteResource{
		Address:    home,
		Identity:   resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := p.Seed(resource.RemoteResource{
		Address:    note,
		Identity:   resource.Identity{ID: "note-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Note"},
	}); err != nil {
		t.Fatal(err)
	}
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(t.TempDir(), "agoraform.yaml")
	if err := os.WriteFile(manifestPath, []byte(`
apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    lifecycle:
      ownership: external
      id: widget-1
  - address: fake.widget.asset
    attributes:
      source:
        file: missing.png
`), 0o600); err != nil {
		t.Fatal(err)
	}

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "fake.widget.note", "note-1"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Bound fake.widget.homepage as an external reference") || !strings.Contains(out, "Imported fake.widget.note") {
		t.Fatalf("stdout = %s", out)
	}
	st, err := state.Load(state.PathForManifest(manifestPath))
	if err != nil {
		t.Fatal(err)
	}
	ownership, ok, err := st.Ownership(home)
	if err != nil || !ok || ownership != resource.OwnershipExternal {
		t.Fatalf("homepage ownership = (%s, %v, %v)", ownership, ok, err)
	}
	id, ok, err := st.Identity(note)
	if err != nil || !ok || id.ID != "note-1" {
		t.Fatalf("note identity = (%v, %v, %v)", id, ok, err)
	}
	noteOwnership, ok, err := st.Ownership(note)
	if err != nil || !ok || noteOwnership != resource.OwnershipManaged {
		t.Fatalf("note ownership = (%s, %v, %v)", noteOwnership, ok, err)
	}
	_, creates, updates, _ := p.Calls()
	if creates != 0 || updates != 0 || p.Destroys() != 0 {
		t.Fatalf("seed mutated provider creates=%d updates=%d destroys=%d", creates, updates, p.Destroys())
	}
}

func TestImportRollsBackStateWhenExternalSeedFails(t *testing.T) {
	t.Parallel()

	p := fake.New()
	home := mustCLIAddress(t, "fake.widget.homepage")
	if err := p.Seed(resource.RemoteResource{
		Address:    home,
		Identity:   resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage"},
	}); err != nil {
		t.Fatal(err)
	}
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(t.TempDir(), "agoraform.yaml")
	if err := os.WriteFile(manifestPath, []byte(`
apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    lifecycle:
      ownership: external
      id: widget-1
  - address: fake.widget.other
    lifecycle:
      ownership: external
      id: missing
`), 0o600); err != nil {
		t.Fatal(err)
	}

	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "fake.widget.note", "note-1"}, reg)
	if code == cli.ExitOK || !strings.Contains(stderr.String(), "fake.widget.other") {
		t.Fatalf("import exit = %d; stderr=%q", code, stderr.String())
	}
	if _, err := os.Stat(state.PathForManifest(manifestPath)); !os.IsNotExist(err) {
		t.Fatalf("partial state file remained: %v", err)
	}
}

func TestImportDoesNotSeedExternalReferencesWhenStateExists(t *testing.T) {
	t.Parallel()

	p := fake.New()
	home := mustCLIAddress(t, "fake.widget.homepage")
	if err := p.Seed(resource.RemoteResource{
		Address:    home,
		Identity:   resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage"},
	}); err != nil {
		t.Fatal(err)
	}
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(t.TempDir(), "agoraform.yaml")

	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "fake.widget.homepage", "widget-1"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("first import exit = %d; stderr=%q", code, stderr.String())
	}
	if err := os.WriteFile(manifestPath, []byte(`
apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.other
    lifecycle:
      ownership: external
      id: missing
`), 0o600); err != nil {
		t.Fatal(err)
	}

	streams, _, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "fake.widget.homepage", "widget-1"}, reg)
	if code == cli.ExitOK || !strings.Contains(stderr.String(), "already bound") {
		t.Fatalf("second import exit = %d; stderr=%q", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "bind external reference") {
		t.Fatalf("existing state re-seeded externals: %s", stderr.String())
	}
}

type unsupportedExternalProvider struct {
	*fake.Provider
}

func (*unsupportedExternalProvider) SupportsExternal(string) bool { return false }
