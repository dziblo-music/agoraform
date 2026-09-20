package manifest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/graph"
	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestLoadDirectoryMergesFiles(t *testing.T) {
	t.Parallel()

	dir := writeConfigDir(t, map[string]string{
		"providers.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
providers:
  fake:
    enabled: true
`,
		"widgets.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: Homepage banner
`,
		"notes.agoraform.yml": `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.checkout
    attributes:
      title: Checkout prompt
      parent:
        $ref: fake.widget.homepage
`,
		"README.md":      "ignored",
		"agoraform.yaml": "apiVersion: agoraform.io/v1alpha1\nresources: []\n",
		"notes.txt":      "ignored",
	})
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nested", "hidden.agoraform.yaml"), []byte(`apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.should_not_load
    attributes:
      title: Nested
`), 0o600); err != nil {
		t.Fatal(err)
	}

	m, err := manifest.Load(dir)
	if err != nil {
		t.Fatalf("Load directory: %v", err)
	}
	if m.Origin != dir {
		t.Fatalf("Origin = %q, want %q", m.Origin, dir)
	}
	if m.BaseDir == "" {
		t.Fatal("BaseDir is empty")
	}
	if got := m.Providers["fake"]["enabled"]; got != true {
		t.Fatalf("providers.fake.enabled = %v", got)
	}
	if len(m.Resources) != 2 {
		t.Fatalf("resources = %d, want 2", len(m.Resources))
	}
	if m.Resources[0].Address.String() != "fake.widget.checkout" {
		t.Fatalf("first merged resource = %s, want address-sorted checkout", m.Resources[0].Address)
	}
	if m.Resources[1].Address.String() != "fake.widget.homepage" {
		t.Fatalf("second merged resource = %s", m.Resources[1].Address)
	}
	if len(m.Files) != 3 {
		t.Fatalf("Files = %d, want 3: %v", len(m.Files), m.Files)
	}

	g, err := graph.Build(m.Resources)
	if err != nil {
		t.Fatalf("graph.Build: %v", err)
	}
	order := addresses(g.Order())
	wantOrder := []string{"fake.widget.homepage", "fake.widget.checkout"}
	if strings.Join(order, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("execution order = %v, want %v", order, wantOrder)
	}
}

func TestLoadDirectoryResolvesCrossFileRefs(t *testing.T) {
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

	m, err := manifest.Load(dir)
	if err != nil {
		t.Fatalf("Load cross-file refs: %v", err)
	}
	g, err := graph.Build(m.Resources)
	if err != nil {
		t.Fatalf("graph.Build: %v", err)
	}
	order := addresses(g.Order())
	if len(order) != 2 || order[0] != "fake.widget.homepage" || order[1] != "fake.widget.banner" {
		t.Fatalf("order = %v, want homepage then banner", order)
	}
}

func TestLoadDirectoryDuplicateAddressNamesFiles(t *testing.T) {
	t.Parallel()

	dir := writeConfigDir(t, map[string]string{
		"a.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: One
`,
		"b.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: Two
`,
	})

	_, err := manifest.Load(dir)
	if err == nil {
		t.Fatal("Load duplicate address succeeded")
	}
	msg := err.Error()
	for _, want := range []string{"duplicate resource address", "a.agoraform.yaml", "b.agoraform.yaml", "fake.widget.homepage"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q, want substring %q", msg, want)
		}
	}
}

func TestLoadDirectoryConflictingProviders(t *testing.T) {
	t.Parallel()

	dir := writeConfigDir(t, map[string]string{
		"a.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
providers:
  fake:
    enabled: true
`,
		"b.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
providers:
  fake:
    enabled: false
`,
	})

	_, err := manifest.Load(dir)
	if err == nil {
		t.Fatal("Load conflicting providers succeeded")
	}
	msg := err.Error()
	for _, want := range []string{"providers.fake", "conflicting", "a.agoraform.yaml", "b.agoraform.yaml"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q, want substring %q", msg, want)
		}
	}
}

func TestLoadDirectoryIdenticalProvidersMerge(t *testing.T) {
	t.Parallel()

	dir := writeConfigDir(t, map[string]string{
		"a.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
providers:
  fake:
    enabled: true
resources:
  - address: fake.widget.homepage
    attributes:
      title: Homepage
`,
		"b.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
providers:
  fake:
    enabled: true
resources:
  - address: fake.widget.checkout
    attributes:
      title: Checkout
`,
	})

	m, err := manifest.Load(dir)
	if err != nil {
		t.Fatalf("identical providers: %v", err)
	}
	if got := m.Providers["fake"]["enabled"]; got != true {
		t.Fatalf("providers.fake.enabled = %v", got)
	}
	if len(m.Resources) != 2 {
		t.Fatalf("resources = %d, want 2", len(m.Resources))
	}
}

func TestLoadDirectoryIncompatibleAPIVersion(t *testing.T) {
	t.Parallel()

	dir := writeConfigDir(t, map[string]string{
		"ok.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
resources: []
`,
		"bad.agoraform.yaml": `apiVersion: agoraform.io/v0
resources: []
`,
	})

	_, err := manifest.Load(dir)
	if err == nil {
		t.Fatal("Load incompatible apiVersion succeeded")
	}
	msg := err.Error()
	if !strings.Contains(msg, "bad.agoraform.yaml") || !strings.Contains(msg, "apiVersion") {
		t.Fatalf("error %q, want filename-aware apiVersion diagnostic", msg)
	}
}

func TestLoadDirectoryMissingCrossFileRef(t *testing.T) {
	t.Parallel()

	dir := writeConfigDir(t, map[string]string{
		"child.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.banner
    attributes:
      title: Banner
      parent:
        $ref: fake.widget.homepage
`,
	})

	_, err := manifest.Load(dir)
	if err == nil {
		t.Fatal("Load missing cross-file ref succeeded")
	}
	if !strings.Contains(err.Error(), "unknown resource") {
		t.Fatalf("error %q, want unknown resource", err)
	}
}

func TestLoadDirectoryNoManifests(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := manifest.Load(dir)
	if err == nil {
		t.Fatal("Load empty directory succeeded")
	}
	if !strings.Contains(err.Error(), "*.agoraform.yaml") {
		t.Fatalf("error %q, want discovery pattern", err)
	}
}

func TestLoadDirectoryParseErrorIncludesFilename(t *testing.T) {
	t.Parallel()

	dir := writeConfigDir(t, map[string]string{
		"ok.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
resources: []
`,
		"broken.agoraform.yaml": "not: valid: yaml: [[[\n",
	})

	_, err := manifest.Load(dir)
	if err == nil {
		t.Fatal("Load malformed file succeeded")
	}
	if !strings.Contains(err.Error(), "broken.agoraform.yaml") || !strings.Contains(err.Error(), "malformed YAML") {
		t.Fatalf("error %q, want filename-aware YAML diagnostic", err)
	}
}

func TestLoadDirectoryConflictingAssetsRoot(t *testing.T) {
	t.Parallel()

	dir := writeConfigDir(t, map[string]string{
		"a.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
assets:
  root: ./media
resources: []
`,
		"b.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
assets:
  root: ./other
resources: []
`,
	})

	_, err := manifest.Load(dir)
	if err == nil {
		t.Fatal("Load conflicting assets.root succeeded")
	}
	if !strings.Contains(err.Error(), "assets.root") {
		t.Fatalf("error %q, want assets.root conflict", err)
	}
}

func TestLoadDirectoryDuplicateApplicationEvents(t *testing.T) {
	t.Parallel()

	const event = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: Homepage
applicationEvents:
  signed_up:
    matomo:
      trigger:
        $ref: matomo.trigger.signed_up
`
	dir := writeConfigDir(t, map[string]string{
		"a.agoraform.yaml": event,
		"b.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
applicationEvents:
  signed_up:
    matomo:
      trigger:
        $ref: matomo.trigger.other
`,
	})

	_, err := manifest.Load(dir)
	if err == nil {
		t.Fatal("Load duplicate applicationEvents succeeded")
	}
	msg := err.Error()
	if !strings.Contains(msg, "applicationEvents.signed_up") || !strings.Contains(msg, "a.agoraform.yaml") || !strings.Contains(msg, "b.agoraform.yaml") {
		t.Fatalf("error %q, want duplicate event filenames", msg)
	}
}

func TestLoadFileDoesNotLoadNeighbors(t *testing.T) {
	t.Parallel()

	dir := writeConfigDir(t, map[string]string{
		"main.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.banner
    attributes:
      title: Banner
      parent:
        $ref: fake.widget.homepage
`,
		"other.agoraform.yaml": `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: Homepage
`,
	})

	path := filepath.Join(dir, "main.agoraform.yaml")
	_, err := manifest.Load(path)
	if err == nil {
		t.Fatal("explicit file load succeeded with unresolved neighbor ref")
	}
	if !strings.Contains(err.Error(), "unknown resource") {
		t.Fatalf("error %q, want unknown resource from single-file load", err)
	}
}

func TestLoadMissingDirectory(t *testing.T) {
	t.Parallel()

	_, err := manifest.Load(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("Load missing directory succeeded")
	}
	if !strings.Contains(err.Error(), "read configuration") {
		t.Fatalf("error %q, want read configuration", err)
	}
}

func writeConfigDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, contents := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func addresses(addrs []resource.Address) []string {
	out := make([]string, len(addrs))
	for i, addr := range addrs {
		out[i] = addr.String()
	}
	return out
}
