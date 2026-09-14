package plan

import (
	"strings"

	"github.com/dziblo-music/agoraform/internal/asset"
	"github.com/dziblo-music/agoraform/internal/resource"
)

// overlayLocalAsset injects a reviewable relative path and content digest
// into comparable attributes. File bytes and absolute host paths are never
// copied into the plan.
func overlayLocalAsset(desired resource.Resource, want, got resource.Attributes, live *resource.RemoteResource) (resource.Attributes, resource.Attributes) {
	if desired.LocalAsset == nil {
		return want, got
	}
	want = overlaySource(want, desired.LocalAsset.Path, desired.LocalAsset.DigestLabel())
	if live == nil {
		return want, got
	}
	livePath := nestedString(got, asset.AttrName, asset.AttrFile)
	if livePath == "" {
		livePath = desired.LocalAsset.Path
	}
	liveDigest := strings.TrimSpace(live.Identity.Fingerprint)
	if liveDigest == "" {
		liveDigest = nestedString(got, asset.AttrName, asset.AttrDigest)
	}
	got = overlaySource(got, livePath, digestLabel(liveDigest))
	return want, got
}

func overlaySource(attrs resource.Attributes, path, digest string) resource.Attributes {
	out := attrs.Clone()
	src := map[string]any{}
	if existing, ok := out[asset.AttrName].(map[string]any); ok {
		for k, v := range existing {
			src[k] = v
		}
	}
	src[asset.AttrFile] = path
	if digest != "" {
		src[asset.AttrDigest] = digest
	} else {
		delete(src, asset.AttrDigest)
	}
	out[asset.AttrName] = src
	return out
}

func nestedString(attrs resource.Attributes, keys ...string) string {
	var cur any = attrs
	for _, key := range keys {
		m, ok := asStringMap(cur)
		if !ok {
			return ""
		}
		cur, ok = m[key]
		if !ok {
			return ""
		}
	}
	s, _ := cur.(string)
	return s
}

func asStringMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case resource.Attributes:
		return map[string]any(m), true
	default:
		return nil, false
	}
}

func digestLabel(digest string) string {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return ""
	}
	if strings.Contains(digest, ":") {
		return digest
	}
	return resource.DigestAlgorithm + ":" + digest
}
