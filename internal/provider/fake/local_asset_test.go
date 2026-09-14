package fake_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/dziblo-music/agoraform/internal/asset"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestFakeCreateStreamsLocalAsset(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "hero.jpg")
	payload := []byte("stream-me")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])
	local := resource.NewLocalAsset("hero.jpg", digest, int64(len(payload)), "image/jpeg", func() (io.ReadCloser, error) {
		return os.Open(path)
	})

	p := fake.New()
	res := resource.Resource{
		Address: mustAddress(t, "fake.widget.hero"),
		Attributes: resource.Attributes{
			fake.AttrTitle: "Hero",
			asset.AttrName: map[string]any{asset.AttrFile: "hero.jpg"},
		},
		LocalAsset: &local,
	}
	created, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Identity.Fingerprint != digest {
		t.Fatalf("Fingerprint = %q, want %q", created.Identity.Fingerprint, digest)
	}
	src, _ := created.Attributes[asset.AttrName].(map[string]any)
	if src[asset.AttrFile] != "hero.jpg" {
		t.Fatalf("stored source = %#v", created.Attributes[asset.AttrName])
	}
	if _, ok := created.Attributes[asset.AttrName].(map[string]any)[asset.AttrDigest]; ok {
		t.Fatal("file digest must not be stored in ordinary attributes")
	}
	for _, v := range created.Attributes {
		if b, ok := v.([]byte); ok && bytes.Equal(b, payload) {
			t.Fatal("file bytes leaked into attributes")
		}
	}
}
