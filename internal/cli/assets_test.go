package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/state"
)

func TestValidateLocalAssetRootRelativeToManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	site := filepath.Join(dir, "site")
	writeAssetProject(t, site, `apiVersion: agoraform.io/v1alpha1
assets:
  root: ./assets
resources:
  - address: fake.widget.hero
    attributes:
      title: Hero
      source:
        file: pic.jpg
`, "assets/pic.jpg", []byte("pic-bytes"))

	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(site, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit = %d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Validated") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestValidateLocalAssetMissingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeAssetProject(t, dir, `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.hero
    attributes:
      title: Hero
      source:
        file: missing.jpg
`, "", nil)

	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "agoraform.yaml")
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want error; stderr=%q", code, stderr.String())
	}
	msg := stderr.String()
	if !strings.Contains(msg, "missing.jpg") {
		t.Fatalf("stderr = %q, want missing.jpg", msg)
	}
	if strings.Contains(msg, filepath.Join(dir, "missing.jpg")) {
		t.Fatalf("stderr leaked absolute asset path: %q", msg)
	}
}

func TestValidateLocalAssetTraversal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeAssetProject(t, dir, `apiVersion: agoraform.io/v1alpha1
assets:
  root: ./assets
resources:
  - address: fake.widget.hero
    attributes:
      title: Hero
      source:
        file: ../secret.txt
`, "assets/.keep", []byte("x"))
	if err := os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "agoraform.yaml")
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want error; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "escapes the asset root") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestPlanApplyLocalAssetFingerprint(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "hero.jpg")
	writeAssetProject(t, dir, `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.hero
    attributes:
      title: Hero
      source:
        file: hero.jpg
`, "hero.jpg", []byte("first"))

	p := fake.New()
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "agoraform.yaml")

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, reg)
	if code != cli.ExitChanges {
		t.Fatalf("plan exit = %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, `source.file: "hero.jpg"`) || !strings.Contains(out, "source.digest:") {
		t.Fatalf("plan missing reviewable source:\n%s", out)
	}
	if strings.Contains(out, filepath.ToSlash(filePath)) || strings.Contains(out, filePath) {
		t.Fatalf("plan leaked absolute path:\n%s", out)
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"apply", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("apply exit = %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}

	st, err := state.Load(state.PathForManifest(path))
	if err != nil {
		t.Fatal(err)
	}
	id, ok, err := st.Identity(mustCLIAddress(t, "fake.widget.hero"))
	if err != nil || !ok || id.Fingerprint == "" {
		t.Fatalf("state identity = %+v ok=%v err=%v, want fingerprint", id, ok, err)
	}
	if id.Fingerprint == filepath.ToSlash(filePath) || strings.Contains(id.Fingerprint, dir) {
		t.Fatalf("state persisted a path: %+v", id)
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("unchanged plan exit = %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("expected stable plan, got:\n%s", stdout.String())
	}

	if err := os.WriteFile(filePath, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, reg)
	if code != cli.ExitChanges {
		t.Fatalf("changed plan exit = %d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "source.digest:") {
		t.Fatalf("changed plan missing digest:\n%s", stdout.String())
	}
}

func TestValidateManifestWithoutAssetsRemainsValid(t *testing.T) {
	t.Parallel()

	p := fake.New()
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", validManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit = %d stderr=%q", code, stderr.String())
	}
}

func writeAssetProject(t *testing.T, dir, manifest, relative string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agoraform.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if relative == "" {
		return
	}
	path := filepath.Join(dir, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
