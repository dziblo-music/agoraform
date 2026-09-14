package resource_test

import (
	"io"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestLocalAssetStringOmitsAbsolutePath(t *testing.T) {
	t.Parallel()

	asset := resource.NewLocalAsset("assets/hero.jpg", "abc123", 12, "image/jpeg", nil)
	got := asset.String()
	if got != "assets/hero.jpg sha256:abc123" {
		t.Fatalf("String = %q", got)
	}
	if strings.Contains(got, `\`) || strings.Contains(strings.ToLower(got), "c:") {
		t.Fatalf("String leaked a host path: %q", got)
	}
	if strings.Contains(asset.GoString(), "open") {
		t.Fatalf("GoString leaked opener: %s", asset.GoString())
	}
}

func TestLocalAssetOpenStreamsBytes(t *testing.T) {
	t.Parallel()

	asset := resource.NewLocalAsset("hero.jpg", "d", 4, "text/plain", func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("data")), nil
	})
	rc, err := asset.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "data" {
		t.Fatalf("bytes = %q, want data", got)
	}
}

func TestLocalAssetOpenWithoutOpener(t *testing.T) {
	t.Parallel()

	asset := resource.NewLocalAsset("hero.jpg", "d", 4, "text/plain", nil)
	if _, err := asset.Open(); err == nil {
		t.Fatal("Open succeeded, want error")
	}
}
