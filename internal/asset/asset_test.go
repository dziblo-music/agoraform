package asset_test

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/asset"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestSourceFileAbsent(t *testing.T) {
	t.Parallel()

	path, present, err := asset.SourceFile(resource.Attributes{"title": "Homepage"})
	if err != nil || present || path != "" {
		t.Fatalf("SourceFile = %q present=%v err=%v", path, present, err)
	}
}

func TestSourceFileRejectsUnknownKeys(t *testing.T) {
	t.Parallel()

	_, _, err := asset.SourceFile(resource.Attributes{
		asset.AttrName: map[string]any{asset.AttrFile: "hero.jpg", "note": "nope"},
	})
	if err == nil || !strings.Contains(err.Error(), "may only contain file") {
		t.Fatalf("error = %v, want unknown key diagnostic", err)
	}
}

func TestResolveDeterministicPathAndDigest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "hero.jpg"), []byte("hero-bytes"))
	root, err := asset.NewRoot(dir, "")
	if err != nil {
		t.Fatal(err)
	}

	first, err := root.Resolve("./foo/../hero.jpg")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	second, err := root.Resolve("hero.jpg")
	if err != nil {
		t.Fatalf("Resolve again: %v", err)
	}
	if first.Path != "hero.jpg" {
		t.Fatalf("Path = %q, want hero.jpg", first.Path)
	}
	if first.Digest != second.Digest || first.Digest != sha256Hex("hero-bytes") {
		t.Fatalf("digest = %q %q, want %s", first.Digest, second.Digest, sha256Hex("hero-bytes"))
	}
	if first.Size != int64(len("hero-bytes")) {
		t.Fatalf("Size = %d", first.Size)
	}
	if first.String() != "hero.jpg sha256:"+first.Digest {
		t.Fatalf("String = %q", first.String())
	}
	if strings.Contains(first.String(), dir) || strings.Contains(first.GoString(), dir) {
		t.Fatalf("descriptor leaked absolute path %q: %s / %#v", dir, first.String(), first)
	}
}

func TestResolveDetectsContentChange(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "hero.jpg")
	writeFile(t, path, []byte("before"))
	root, err := asset.NewRoot(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	before, err := root.Resolve("hero.jpg")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, []byte("after-bytes"))
	after, err := root.Resolve("hero.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if before.Path != after.Path {
		t.Fatalf("path changed: %q -> %q", before.Path, after.Path)
	}
	if before.Digest == after.Digest {
		t.Fatal("digest did not change when bytes changed")
	}
}

func TestResolveMissingAndDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := asset.NewRoot(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = root.Resolve("missing.jpg")
	if err == nil || !strings.Contains(err.Error(), "file not found") || !strings.Contains(err.Error(), "missing.jpg") {
		t.Fatalf("missing error = %v", err)
	}
	if strings.Contains(err.Error(), dir) {
		t.Fatalf("missing error leaked absolute path: %v", err)
	}
	_, err = root.Resolve("nested")
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("directory error = %v", err)
	}
}

func TestResolveRejectsAbsoluteAndTraversal(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	dir := filepath.Join(parent, "project")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(parent, "secret.txt"), []byte("secret"))
	writeFile(t, filepath.Join(dir, "hero.jpg"), []byte("hero"))
	root, err := asset.NewRoot(dir, "")
	if err != nil {
		t.Fatal(err)
	}

	_, err = root.Resolve(filepath.Join(parent, "secret.txt"))
	if err == nil || !strings.Contains(err.Error(), "relative") {
		t.Fatalf("absolute error = %v", err)
	}
	if strings.Contains(err.Error(), parent) {
		t.Fatalf("absolute error leaked host path: %v", err)
	}

	_, err = root.Resolve("../secret.txt")
	if err == nil || !strings.Contains(err.Error(), "escapes the asset root") {
		t.Fatalf("traversal error = %v", err)
	}
}

func TestResolveRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	writeFile(t, outside, []byte("secret"))
	link := filepath.Join(dir, "link.jpg")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks not available: %v", err)
	}
	root, err := asset.NewRoot(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = root.Resolve("link.jpg")
	if err == nil || !strings.Contains(err.Error(), "escapes the asset root") {
		t.Fatalf("symlink error = %v", err)
	}
	if strings.Contains(err.Error(), outside) {
		t.Fatalf("symlink error leaked target path: %v", err)
	}
}

func TestResolveConfiguredRoot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "media", "hero.jpg"), []byte("\xff\xd8\xff"))
	root, err := asset.NewRoot(dir, "./media")
	if err != nil {
		t.Fatal(err)
	}
	if root.Display != "media" {
		t.Fatalf("Display = %q, want media", root.Display)
	}
	got, err := root.Resolve("hero.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "hero.jpg" {
		t.Fatalf("Path = %q", got.Path)
	}
	if !strings.HasPrefix(got.MediaType, "image/jpeg") {
		t.Fatalf("MediaType = %q, want jpeg", got.MediaType)
	}
}

func TestResolveLargeFileStreaming(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	const size = 2 * 1024 * 1024
	data := bytesOf(size, 'A')
	writeFile(t, filepath.Join(dir, "large.bin"), data)
	root, err := asset.NewRoot(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := root.Resolve("large.bin")
	if err != nil {
		t.Fatal(err)
	}
	if got.Size != size {
		t.Fatalf("Size = %d, want %d", got.Size, size)
	}
	if got.Digest != sha256Hex(string(data)) {
		t.Fatalf("digest mismatch for streamed file")
	}
	rc, err := got.Open()
	if err != nil {
		t.Fatal(err)
	}
	n, err := io.Copy(io.Discard, rc)
	rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if n != size {
		t.Fatalf("streamed %d bytes, want %d", n, size)
	}
}

func TestBindAttachesDescriptor(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "assets", "pic.jpg"), []byte("pic"))
	resources := []resource.Resource{{
		Address: resource.Address{Provider: "fake", Type: "widget", Name: "hero"},
		Attributes: resource.Attributes{
			"title": "Hero",
			asset.AttrName: map[string]any{
				asset.AttrFile: "pic.jpg",
			},
		},
	}}
	if err := asset.Bind("site/agoraform.yaml", dir, "./assets", resources); err != nil {
		t.Fatal(err)
	}
	if resources[0].LocalAsset == nil {
		t.Fatal("LocalAsset was not attached")
	}
	if resources[0].LocalAsset.Path != "pic.jpg" {
		t.Fatalf("Path = %q", resources[0].LocalAsset.Path)
	}
	if _, ok := resources[0].Attributes[asset.AttrName].(map[string]any)["digest"]; ok {
		t.Fatal("digest must not be copied into resource attributes")
	}
}

func TestBindMissingFileUsesRelativeDiagnostic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	resources := []resource.Resource{{
		Address: resource.Address{Provider: "fake", Type: "widget", Name: "hero"},
		Attributes: resource.Attributes{
			asset.AttrName: map[string]any{asset.AttrFile: "missing.jpg"},
		},
	}}
	err := asset.Bind("agoraform.yaml", dir, "", resources)
	if err == nil {
		t.Fatal("Bind succeeded, want missing file")
	}
	msg := err.Error()
	if !strings.Contains(msg, "missing.jpg") || !strings.Contains(msg, "agoraform.yaml") {
		t.Fatalf("error = %q", msg)
	}
	if strings.Contains(msg, dir) {
		t.Fatalf("error leaked absolute path: %q", msg)
	}
}

func TestNewRootMissingConfiguredDirectory(t *testing.T) {
	t.Parallel()

	_, err := asset.NewRoot(t.TempDir(), "./assets")
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("error = %v", err)
	}
}

func TestBindSkipsManifestsWithoutSources(t *testing.T) {
	t.Parallel()

	resources := []resource.Resource{{
		Address:    resource.Address{Provider: "fake", Type: "widget", Name: "homepage"},
		Attributes: resource.Attributes{"title": "Homepage"},
	}}
	if err := asset.Bind("agoraform.yaml", "", "", resources); err != nil {
		t.Fatalf("Bind without sources: %v", err)
	}
	if resources[0].LocalAsset != nil {
		t.Fatal("unexpected LocalAsset")
	}
}

func TestUnreadableFile(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based unreadability is not reliable on Windows")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "secret.jpg")
	writeFile(t, path, []byte("secret"))
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	root, err := asset.NewRoot(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = root.Resolve("secret.jpg")
	if err == nil || !strings.Contains(err.Error(), "unreadable") {
		t.Fatalf("error = %v, want unreadable", err)
	}
	if strings.Contains(err.Error(), dir) {
		t.Fatalf("error leaked absolute path: %v", err)
	}
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func bytesOf(n int, b byte) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}
