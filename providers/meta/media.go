package meta

import (
	"fmt"
	"io"
	"path"
	"strings"
	"unicode"

	"github.com/dziblo-music/agoraform/internal/asset"
	"github.com/dziblo-music/agoraform/internal/resource"
)

const (
	imageReplaceGuidance = "image content is immutable after create; declare a new logical meta.image resource and repoint the creative instead of replacing it implicitly"
	videoReplaceGuidance = "video content is immutable after create; declare a new logical meta.video resource and repoint the creative instead of replacing it implicitly"
)

func declaredSourceFile(res resource.Resource) (string, bool, error) {
	file, present, err := asset.SourceFile(res.Attributes)
	if err != nil {
		return "", present, fmt.Errorf("resource %s: %w", res.Address, err)
	}
	return file, present, nil
}

func requireLocalAsset(res resource.Resource) error {
	if res.LocalAsset == nil {
		return fmt.Errorf("resource %s: requires a readable local file via %s.%s", res.Address, asset.AttrName, asset.AttrFile)
	}
	return nil
}

func rejectChangedLocalContent(res resource.Resource, live *resource.RemoteResource, guidance string) error {
	if res.LocalAsset == nil || live == nil || live.Identity.IsZero() {
		return nil
	}
	want := strings.TrimSpace(res.LocalAsset.Digest)
	got := strings.TrimSpace(live.Identity.Fingerprint)
	if got == "" {
		got = strings.TrimSpace(res.Identity.Fingerprint)
	}
	if got == "" || (want != "" && want != got) {
		return fmt.Errorf("resource %s: %s", res.Address, guidance)
	}
	return nil
}

func localContentFingerprint(res resource.Resource) string {
	if res.LocalAsset != nil {
		return strings.TrimSpace(res.LocalAsset.Digest)
	}
	return strings.TrimSpace(res.Identity.Fingerprint)
}

func openLocalAsset(res resource.Resource, maxBytes int64, limitLabel string) (io.ReadCloser, string, int64, error) {
	if err := requireLocalAsset(res); err != nil {
		return nil, "", 0, err
	}
	local := res.LocalAsset
	if local.Size <= 0 {
		return nil, "", 0, fmt.Errorf("resource %s: local file %q is empty", res.Address, local.Path)
	}
	if maxBytes > 0 && local.Size > maxBytes {
		return nil, "", 0, fmt.Errorf("resource %s: local file %q exceeds the %s size limit", res.Address, local.Path, limitLabel)
	}
	rc, err := local.Open()
	if err != nil {
		return nil, "", 0, fmt.Errorf("resource %s: cannot open local file %q: %w", res.Address, local.Path, err)
	}
	filename := path.Base(local.Path)
	if filename == "" || filename == "." || filename == "/" {
		filename = "upload"
	}
	return rc, filename, local.Size, nil
}

func validateContentDigest(addr resource.Address, digest string) error {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return nil
	}
	if len(digest) != 64 || !isHexString(digest) {
		return fmt.Errorf("resource %s: persisted content fingerprint is invalid: expected 64-character SHA-256 hex", addr)
	}
	return nil
}

func validateImageHashValue(addr resource.Address, hash string) error {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return fmt.Errorf("resource %s: persisted image identity is empty; a Meta image hash is required", addr)
	}
	if strings.IndexFunc(hash, unicode.IsSpace) >= 0 {
		return fmt.Errorf("resource %s: persisted image identity %q is not a Meta image hash", addr, hash)
	}
	return nil
}

func isHexString(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return s != ""
}
