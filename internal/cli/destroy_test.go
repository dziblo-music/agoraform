package cli_test

import (
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
)

func TestDestroyHelpDocumentsBehavior(t *testing.T) {
	t.Parallel()

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"destroy", "--help"})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"destroy", "agoraform.yaml", "agoraform.state.json", "--auto-approve", "reverse"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing %q:\n%s", want, out)
		}
	}
}

func TestDestroyRequiresAutoApproveWhenNonInteractive(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustCLIAddress(t, "fake.widget.homepage")
	if err := p.Seed(resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage banner"},
		Computed:   resource.Attributes{fake.AttrSerial: 4},
	}); err != nil {
		t.Fatal(err)
	}
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", validManifest)
	st, err := state.New(state.PathForManifest(path))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(addr, resource.Identity{ID: "widget-1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"destroy", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitError, stderr.String(), stdout.String())
	}
	if !strings.Contains(stderr.String(), "--auto-approve") {
		t.Fatalf("stderr = %q, want --auto-approve guidance", stderr.String())
	}
	if p.Destroys() != 0 {
		t.Fatalf("Destroys() = %d, want 0", p.Destroys())
	}
}

func TestDestroyAutoApproveRemovesState(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustCLIAddress(t, "fake.widget.homepage")
	if err := p.Seed(resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage banner"},
		Computed:   resource.Attributes{fake.AttrSerial: 4},
	}); err != nil {
		t.Fatal(err)
	}
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", validManifest)
	st, err := state.New(state.PathForManifest(path))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(addr, resource.Identity{ID: "widget-1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"destroy", "--auto-approve", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "fake.widget.homepage") || !strings.Contains(out, "Destroy complete! 1 destroyed.") {
		t.Fatalf("stdout missing destroy progress:\n%s", out)
	}

	loaded, err := state.Load(state.PathForManifest(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := loaded.Identity(addr); err != nil || ok {
		t.Fatalf("identity still present after destroy: ok=%v err=%v", ok, err)
	}
}
