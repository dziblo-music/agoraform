package cli_test

import (
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

func TestValidateDirectoryMergesConfiguration(t *testing.T) {
	t.Parallel()

	dir := writeConfigDir(t, map[string]string{
		"z-parent.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: Homepage banner
`,
		"a-child.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.banner
    attributes:
      title: Banner
      parent:
        $ref: fake.widget.homepage
`,
	})

	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", dir}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, dir) || !strings.Contains(out, "2 resources") {
		t.Fatalf("stdout = %q, want directory path and 2 resources", out)
	}
}

func TestValidateDirectoryDuplicateReportsFiles(t *testing.T) {
	t.Parallel()

	dir := writeConfigDir(t, map[string]string{
		"one.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: One
`,
		"two.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: Two
`,
	})

	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", dir})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	msg := stderr.String()
	for _, want := range []string{"duplicate resource address", "one.agoraform.yaml", "two.agoraform.yaml"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("stderr %q, want substring %q", msg, want)
		}
	}
}

func TestPlanApplyDirectoryUsesSingleState(t *testing.T) {
	t.Parallel()

	p := fake.New()
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := writeConfigDir(t, map[string]string{
		"homepage.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: Homepage banner
`,
		"checkout.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.checkout
    attributes:
      title: Checkout prompt
      parent:
        $ref: fake.widget.homepage
`,
	})

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"apply", "-f", dir}, reg)
	if code != cli.ExitOK {
		t.Fatalf("apply exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "fake.widget.homepage: created") || !strings.Contains(out, "fake.widget.checkout: created") {
		t.Fatalf("apply stdout missing creates:\n%s", out)
	}
	if strings.Index(out, "fake.widget.homepage: created") > strings.Index(out, "fake.widget.checkout: created") {
		t.Fatalf("apply created dependent before prerequisite:\n%s", out)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var stateFiles []string
	for _, entry := range entries {
		if entry.Name() == "agoraform.state.json" {
			stateFiles = append(stateFiles, entry.Name())
		}
	}
	if len(stateFiles) != 1 {
		t.Fatalf("state files = %v, want one agoraform.state.json", stateFiles)
	}

	st, err := state.Load(state.PathForConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range []string{"fake.widget.homepage", "fake.widget.checkout"} {
		id, ok, err := st.Identity(mustCLIAddress(t, addr))
		if err != nil || !ok || id.ID == "" {
			t.Fatalf("persisted identity %s = (%v,%v,%v)", addr, id, ok, err)
		}
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", dir}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after apply exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after apply = %q, want no changes", stdout.String())
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"destroy", "--auto-approve", "-f", dir}, reg)
	if code != cli.ExitOK {
		t.Fatalf("destroy exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	destroyed := stdout.String()
	checkoutIdx := strings.Index(destroyed, "fake.widget.checkout")
	homepageIdx := strings.Index(destroyed, "fake.widget.homepage")
	if checkoutIdx < 0 || homepageIdx < 0 || checkoutIdx > homepageIdx {
		t.Fatalf("destroy order = checkout@%d homepage@%d, want dependent first:\n%s", checkoutIdx, homepageIdx, destroyed)
	}
}

func TestValidateExplicitFileIgnoresNeighbors(t *testing.T) {
	t.Parallel()

	dir := writeConfigDir(t, map[string]string{
		"main.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: Homepage banner
`,
		"extra.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.checkout
    attributes:
      title: Checkout prompt
`,
	})

	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "main.agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 resource") {
		t.Fatalf("stdout = %q, want single-file 1 resource", stdout.String())
	}
}

func TestImportDirectoryLocatesState(t *testing.T) {
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

	dir := t.TempDir()
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", dir, "fake.widget.homepage", "widget-1"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import exit = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	st, err := state.Load(state.PathForConfig(dir))
	if err != nil {
		t.Fatal(err)
	}
	id, ok, err := st.Identity(addr)
	if err != nil || !ok || id.ID != "widget-1" {
		t.Fatalf("imported identity = (%v,%v,%v)", id, ok, err)
	}
}

func writeConfigDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestValidateDirectoryProviderErrorNamesSource(t *testing.T) {
	t.Parallel()
	dir := writeConfigDir(t, map[string]string{
		"invalid.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.banner
    attributes:
      color: red
`,
	})
	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}
	streams, _, stderr := testStreams()
	if code := cli.ExecuteWithRegistry(streams, []string{"validate", dir}, reg); code != cli.ExitError {
		t.Fatalf("validate exit = %d; stderr=%q", code, stderr.String())
	}
	for _, want := range []string{"invalid.agoraform.yaml", "missing required attribute"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr %q missing %q", stderr.String(), want)
		}
	}
}

func TestSplitSingleFileKeepsStateAndProducesNoChanges(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	original := filepath.Join(dir, "agoraform.yaml")
	contents := `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: Homepage banner
`
	if err := os.WriteFile(original, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	p := fake.New()
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	streams, _, stderr := testStreams()
	if code := cli.ExecuteWithRegistry(streams, []string{"apply", "-f", original}, reg); code != cli.ExitOK {
		t.Fatalf("single-file apply exit = %d; stderr=%q", code, stderr.String())
	}
	if err := os.WriteFile(filepath.Join(dir, "resources.agoraform.yaml"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(original); err != nil {
		t.Fatal(err)
	}
	streams, stdout, stderr := testStreams()
	if code := cli.ExecuteWithRegistry(streams, []string{"plan", dir}, reg); code != cli.ExitOK {
		t.Fatalf("directory plan exit = %d; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("directory plan = %q, want no changes", stdout.String())
	}
	_, creates, updates, _ := p.Calls()
	if creates != 1 || updates != 0 {
		t.Fatalf("provider creates=%d updates=%d; want 1 and 0", creates, updates)
	}
}
