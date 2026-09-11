package meta

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

const (
	// imageDestroyGuidance explains why meta.image resources are provider-owned.
	// Meta does not provide a safe delete API for images referenced by creatives,
	// and the same image hash may be shared across multiple creatives.
	imageDestroyGuidance = "Meta image assets are provider-owned: Agoraform uploads images but does not delete them. " +
		"Uploaded images may be referenced by multiple ad creatives (including externally managed ones). " +
		"Use the Meta Business Manager to manage image lifecycle."

	// imageReplaceGuidance is shown when the user tries to apply a content change.
	imageReplaceGuidance = "image content has changed since the last upload; " +
		"to replace: remove this resource's state binding with 'agoraform destroy' and run apply again to upload the new file"

	// maxImageBytes is the maximum file size enforced locally before upload.
	// Meta enforces its own limits server-side; this local check avoids wasting
	// bandwidth on obviously oversized files.
	maxImageBytes = 30 * 1024 * 1024 // 30 MB
)

var (
	supportedImageAttrs = map[string]struct{}{
		AttrFile: {},
	}
	computedImageAttrs = map[string]struct{}{
		AttrImageHash: {}, "imageHashValue": {}, "imageId": {}, "url": {}, "width": {}, "height": {},
	}
	supportedImageExts = map[string]struct{}{
		".jpg": {}, ".jpeg": {}, ".png": {}, ".gif": {},
	}
)

// imageUploadResponse is the JSON shape returned by the Meta adimages upload API.
type imageUploadResponse struct {
	Images map[string]imageInfo `json:"images"`
}

type imageInfo struct {
	Hash   string `json:"hash"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func (p *Provider) validateImage(res resource.Resource) error {
	if err := p.requireAdAccount(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}
	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedImageAttrs[key]; ok {
			continue
		}
		if _, computed := computedImageAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; meta.image supports %s", res.Address, key, joinSorted(keys(supportedImageAttrs)))
	}
	filePath, err := requiredString(res, AttrFile)
	if err != nil {
		return err
	}
	if err := validateImageFilePath(res.Address, filePath); err != nil {
		return err
	}
	return nil
}

func validateImageFilePath(addr resource.Address, path string) error {
	ext := strings.ToLower(filepath.Ext(path))
	if _, ok := supportedImageExts[ext]; !ok {
		return fmt.Errorf("resource %s: attribute %q has unsupported file extension %q; supported: .jpg, .jpeg, .png, .gif", addr, AttrFile, ext)
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("resource %s: attribute %q: file not found: %s", addr, AttrFile, path)
		}
		return fmt.Errorf("resource %s: attribute %q: cannot access file %q: %w", addr, AttrFile, path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("resource %s: attribute %q: %q is a directory, not a file", addr, AttrFile, path)
	}
	if info.Size() > maxImageBytes {
		return fmt.Errorf("resource %s: attribute %q: file %q exceeds the %d MB size limit", addr, AttrFile, path, maxImageBytes/(1024*1024))
	}
	return nil
}

// fileSHA256 computes the SHA-256 hex digest of a file's contents.
func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// readFileContent reads a file and returns its bytes and SHA-256 fingerprint.
func readFileContent(path string) ([]byte, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxImageBytes {
		return nil, "", fmt.Errorf("file %q exceeds the %d MB size limit", path, maxImageBytes/(1024*1024))
	}
	sum := sha256.Sum256(data)
	fingerprint := hex.EncodeToString(sum[:])
	return data, fingerprint, nil
}

// boundImageIdentity returns the sha256 fingerprint and meta image hash stored
// in state for a meta.image resource. It does not call normalizeObjectID
// because the remote ID for images is a sha256 hex string, not a numeric Meta
// object ID.
func boundImageIdentity(res resource.Resource) (sha256Hex, metaHash string, bound bool, err error) {
	if res.Identity.IsZero() {
		return "", "", false, nil
	}
	sha256Hex = strings.TrimSpace(res.Identity.ID)
	metaHash = strings.TrimSpace(res.Identity.Fingerprint)
	if len(sha256Hex) != 64 || !isHexString(sha256Hex) {
		return "", "", true, fmt.Errorf("resource %s: persisted image identity is invalid: expected 64-character SHA-256 hex", res.Address)
	}
	return sha256Hex, metaHash, true, nil
}

func isHexString(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

// readImage returns the live state of a meta.image resource.
//
// readImage is non-mutating and does not contact the Meta API. It validates
// the local file path, confirms the file is accessible, and returns the
// stored SHA-256 as the comparable live attribute so the plan engine can
// detect content changes. The Meta image hash is surfaced as a computed output
// for dependent meta.ad_creative resources to consume.
func (p *Provider) readImage(_ context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateImage(res); err != nil {
		return resource.RemoteResource{}, err
	}
	storedSHA256, metaHash, bound, err := boundImageIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if !bound {
		return resource.RemoteResource{}, fmt.Errorf("meta: read %s: %w", res.Address, provider.ErrNotFound)
	}
	live := resource.RemoteResource{
		Address: res.Address,
		Identity: resource.Identity{
			ID:          storedSHA256,
			Fingerprint: metaHash,
		},
		// Attributes["file"] holds the stored sha256 so NormalizeComparable
		// can compare it against the current local file's sha256.
		Attributes: resource.Attributes{
			AttrFile: storedSHA256,
		},
		// Computed["imageHash"] is the Meta image hash, available to dependent
		// meta.ad_creative resources via $ref output resolution.
		Computed: resource.Attributes{
			OutputImageHash: metaHash,
		},
	}
	return p.rememberImageLive(live), nil
}

// createImage uploads the local file to Meta and persists the resulting image
// hash. The SHA-256 fingerprint of the uploaded content is stored as the
// state remote ID so subsequent plans can detect file content changes.
//
// createImage contacts the Meta API and is called only during apply, never
// during plan.
func (p *Provider) createImage(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateImage(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, _, bound, err := boundImageIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: resource already has a persisted identity; destroy before recreating", res.Address)
	}
	filePath, _ := requiredString(res, AttrFile)

	data, sha256Hex, err := readFileContent(filePath)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: cannot read file %q: %w", res.Address, filePath, err)
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}

	filename := filepath.Base(filePath)
	var uploadResp imageUploadResponse
	if err := c.PostMultipart(ctx, c.AdAccountID()+"/adimages", nil, "filename", filename, data, &uploadResp); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: upload failed: %w", res.Address, err)
	}

	metaHash, err := extractImageHash(uploadResp, filename)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: %w", res.Address, err)
	}

	live := resource.RemoteResource{
		Address: res.Address,
		Identity: resource.Identity{
			// ID is the sha256 fingerprint: stable local identity used for
			// content-change detection in subsequent plans.
			ID: sha256Hex,
			// Fingerprint is the Meta image hash: the provider-native
			// identifier required by ad creatives.
			Fingerprint: metaHash,
		},
		Attributes: resource.Attributes{
			AttrFile: sha256Hex,
		},
		Computed: resource.Attributes{
			OutputImageHash: metaHash,
		},
	}
	return p.rememberImageLive(live), nil
}

// updateImage is called when the plan detects a content change (SHA-256 differs
// from what was uploaded). Agoraform does not silently re-upload images because
// doing so would invalidate existing ad creatives that reference the previous
// image hash without surfacing the change to dependent resources.
//
// To replace a managed image: destroy the meta.image resource (removing its
// state binding) and run apply again to upload the new file.
func (p *Provider) updateImage(_ context.Context, desired resource.Resource, _ resource.RemoteResource) (resource.RemoteResource, error) {
	return resource.RemoteResource{}, fmt.Errorf("meta: update %s: %s", desired.Address, imageReplaceGuidance)
}

// importImage is not supported. meta.image resources are create-managed: the
// local file is the source of truth. Use imageHash on meta.ad_creative for
// images managed outside Agoraform.
func (p *Provider) importImage(_ context.Context, addr resource.Address, _ string) (resource.RemoteResource, error) {
	return resource.RemoteResource{}, fmt.Errorf("meta: import %s: meta.image resources are create-managed; "+
		"declare the image with file: and run apply to upload it, or use imageHash on meta.ad_creative for externally managed images", addr)
}

func (p *Provider) normalizeImageComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	filePath, err := requiredString(desired, AttrFile)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	// Compute the sha256 of the current local file as the desired comparable.
	currentSHA256, err := fileSHA256(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: cannot fingerprint file %q: %w", desired.Address, filePath, err)
	}
	wantAttrs := resource.Attributes{AttrFile: currentSHA256}
	if live == nil {
		return wantAttrs, nil, nil
	}
	// live.Attributes["file"] is the sha256 that was stored at upload time.
	gotAttrs := resource.Attributes{AttrFile: live.Attributes[AttrFile]}
	return wantAttrs, gotAttrs, nil
}

// extractImageHash parses the Meta adimages upload response and returns the
// image hash for the uploaded file. The response maps filenames to image info.
func extractImageHash(resp imageUploadResponse, filename string) (string, error) {
	if len(resp.Images) == 0 {
		return "", fmt.Errorf("API returned no image data in upload response")
	}
	// Try exact filename match first.
	if info, ok := resp.Images[filename]; ok && strings.TrimSpace(info.Hash) != "" {
		return strings.TrimSpace(info.Hash), nil
	}
	// Fall back to first entry if filename key differs (some API versions use
	// the original name, others use a normalized key).
	for _, info := range resp.Images {
		if h := strings.TrimSpace(info.Hash); h != "" {
			return h, nil
		}
	}
	return "", fmt.Errorf("API upload response did not include an image hash")
}
