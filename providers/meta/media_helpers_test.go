package meta_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/jpeg"
	"io"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/asset"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func localJPEG(t *testing.T, path string, width, height int) resource.LocalAsset {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return localAsset(t, path, "image/jpeg", buf.Bytes())
}

func localPNGAsset(t *testing.T, path string, payload []byte) resource.LocalAsset {
	t.Helper()
	return localAsset(t, path, "image/png", payload)
}

func localMP4(t *testing.T, path string) resource.LocalAsset {
	t.Helper()
	payload := []byte{
		0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p',
		'i', 's', 'o', 'm', 0x00, 0x00, 0x02, 0x00,
		'i', 's', 'o', 'm', 'm', 'p', '4', '2',
	}
	payload = append(payload, bytes.Repeat([]byte{0x00}, 64)...)
	return localAsset(t, path, "video/mp4", payload)
}

func localAsset(t *testing.T, path, mediaType string, payload []byte) resource.LocalAsset {
	t.Helper()
	sum := sha256.Sum256(payload)
	data := payload
	return resource.NewLocalAsset(path, hex.EncodeToString(sum[:]), int64(len(data)), mediaType, func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	})
}

func imageResourceFrom(t *testing.T, name string, local resource.LocalAsset) resource.Resource {
	t.Helper()
	res := resource.Resource{
		Address:    imageAddress(t, name),
		Attributes: sourceFileAttrs(local.Path),
	}
	res.LocalAsset = &local
	return res
}

func videoResourceFrom(t *testing.T, name string, local resource.LocalAsset) resource.Resource {
	t.Helper()
	res := resource.Resource{
		Address:    videoAddress(t, name),
		Attributes: sourceFileAttrs(local.Path),
	}
	res.LocalAsset = &local
	return res
}

func sourceFileAttrs(path string) resource.Attributes {
	return resource.Attributes{
		asset.AttrName: map[string]any{asset.AttrFile: path},
	}
}

func sha256HexOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func assertNoBinary(t *testing.T, label, value string, payload []byte) {
	t.Helper()
	if len(payload) == 0 {
		return
	}
	if strings.Contains(value, string(payload)) {
		t.Fatalf("%s leaked file bytes", label)
	}
}
