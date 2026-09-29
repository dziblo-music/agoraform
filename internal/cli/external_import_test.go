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
