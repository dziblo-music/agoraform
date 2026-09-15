package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/url"
	"path"
	"strings"

	"github.com/dziblo-music/agoraform/internal/asset"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/meta/client"
)

const (
	// imageDestroyGuidance explains why meta.image resources are provider-owned.
	// Meta's Ad Image delete edge exists, but the same hash may be shared across
	// creatives (including ones Agoraform does not manage), so Agoraform never
	// deletes uploaded images.
	imageDestroyGuidance = "Meta image assets are provider-owned: Agoraform uploads images but does not delete them. " +
		"Uploaded images may be referenced by multiple ad creatives (including externally managed ones). " +
		"Use the Meta Business Manager to manage image lifecycle."

	// maxImageBytes is the local pre-upload size limit. Meta also enforces
	// limits server-side; this check avoids streaming obviously oversized files.
	maxImageBytes  = 30 * 1024 * 1024 // 30 MB
	imageSizeLabel = "30 MB"
)

var (
	supportedImageAttrs = map[string]struct{}{
		asset.AttrName: {},
	}
	computedImageAttrs = map[string]struct{}{
		AttrImageHash: {}, "imageHashValue": {}, "imageId": {}, "id": {},
		"url": {}, "width": {}, "height": {}, "name": {}, "status": {},
		asset.AttrDigest: {},
	}
	supportedImageMediaTypes = map[string]struct{}{
		"image/jpeg": {}, "image/png": {}, "image/gif": {},
	}
	supportedImageFormats = map[string]struct{}{
		"jpeg": {}, "png": {}, "gif": {},
	}
)

type imageUploadResponse struct {
	Images map[string]imageInfo `json:"images"`
}

type imageInfo struct {
	Hash   string `json:"hash"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type adImageList struct {
	Data []imageInfo `json:"data"`
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
	_, present, err := declaredSourceFile(res)
	if err != nil {
		return err
	}
	if present {
		if err := validateImageLocalAsset(res); err != nil {
			return err
		}
	} else if res.Identity.IsZero() {
		return fmt.Errorf("resource %s: requires %s.%s naming a local file", res.Address, asset.AttrName, asset.AttrFile)
	} else if res.LocalAsset != nil {
		return fmt.Errorf("resource %s: requires %s.%s naming a local file", res.Address, asset.AttrName, asset.AttrFile)
	}
	if _, _, _, err := boundImageIdentity(res); err != nil {
		return err
	}
	return nil
}

func validateImageLocalAsset(res resource.Resource) error {
	if err := requireLocalAsset(res); err != nil {
		return err
	}
	local := res.LocalAsset
	media := strings.ToLower(strings.TrimSpace(local.MediaType))
	if _, ok := supportedImageMediaTypes[media]; !ok {
		return fmt.Errorf("resource %s: local file %q has unsupported type %q; Meta ad images accept JPEG, PNG, or GIF", res.Address, local.Path, local.MediaType)
	}
	if local.Size <= 0 {
		return fmt.Errorf("resource %s: local file %q is empty", res.Address, local.Path)
	}
	if local.Size > maxImageBytes {
		return fmt.Errorf("resource %s: local file %q exceeds the %s size limit", res.Address, local.Path, imageSizeLabel)
	}

	rc, err := local.Open()
	if err != nil {
		return fmt.Errorf("resource %s: cannot open local file %q: %w", res.Address, local.Path, err)
	}
	defer rc.Close()
	cfg, format, err := image.DecodeConfig(io.LimitReader(rc, maxImageBytes+1))
	if err != nil {
		return fmt.Errorf("resource %s: local file %q is not a readable JPEG, PNG, or GIF image", res.Address, local.Path)
	}
	if _, ok := supportedImageFormats[strings.ToLower(format)]; !ok {
		return fmt.Errorf("resource %s: local file %q has unsupported image format %q; Meta ad images accept JPEG, PNG, or GIF", res.Address, local.Path, format)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("resource %s: local file %q is not a usable image", res.Address, local.Path)
	}
	return nil
}

func boundImageIdentity(res resource.Resource) (hash, digest string, bound bool, err error) {
	if res.Identity.IsZero() {
		return "", "", false, nil
	}
	hash = strings.TrimSpace(res.Identity.ID)
	if err := validateImageHashValue(res.Address, hash); err != nil {
		return "", "", true, err
	}
	digest = strings.TrimSpace(res.Identity.Fingerprint)
	if err := validateContentDigest(res.Address, digest); err != nil {
		return "", "", true, err
	}
	return hash, digest, true, nil
}

func (p *Provider) readImage(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateImage(res); err != nil {
		return resource.RemoteResource{}, err
	}
	hash, digest, bound, err := boundImageIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if !bound {
		return resource.RemoteResource{}, fmt.Errorf("meta: read %s: %w", res.Address, provider.ErrNotFound)
	}
	info, err := p.readAdImage(ctx, hash)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: read %s: %w", res.Address, err)
	}
	return p.rememberImageLive(remoteImage(res.Address, info, digest)), nil
}

func (p *Provider) createImage(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateImage(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, _, bound, err := boundImageIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: resource already has a persisted identity; destroy before recreating", res.Address)
	}

	rc, filename, size, err := openLocalAsset(res, maxImageBytes, imageSizeLabel)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: %w", res.Address, err)
	}
	defer rc.Close()

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	var uploadResp imageUploadResponse
	if err := c.PostMultipartStream(ctx, c.AdAccountID()+"/adimages", nil, "filename", filename, rc, size, &uploadResp); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: upload failed: %w", res.Address, err)
	}
	hash, err := extractImageHash(uploadResp, filename)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: %w", res.Address, err)
	}
	digest := localContentFingerprint(res)
	info := imageInfo{Hash: hash, Name: filename}
	if live, err := p.readAdImage(ctx, hash); err == nil {
		info = live
	}
	return p.rememberImageLive(remoteImage(res.Address, info, digest)), nil
}

func (p *Provider) updateImage(_ context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateImage(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if err := rejectChangedLocalContent(desired, &actual, imageReplaceGuidance); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: %w", desired.Address, err)
	}
	return resource.RemoteResource{}, fmt.Errorf("meta: update %s: %s", desired.Address, imageReplaceGuidance)
}

func (p *Provider) importImage(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	hash := strings.TrimSpace(rawID)
	if err := validateImageHashValue(addr, hash); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: import %s: remote identifier is not a Meta image hash", addr)
	}
	if err := p.requireConfig(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: import %s: %w", addr, err)
	}
	info, err := p.readAdImage(ctx, hash)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("meta: import %s: remote image %q was not found: %w", addr, hash, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("meta: import %s: %w", addr, err)
	}
	return p.rememberImageLive(remoteImage(addr, info, "")), nil
}

func (p *Provider) normalizeImageComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	if err := p.validateImage(desired); err != nil {
		return nil, nil, err
	}
	if err := rejectChangedLocalContent(desired, live, imageReplaceGuidance); err != nil {
		return nil, nil, err
	}
	if live == nil {
		return resource.Attributes{}, nil, nil
	}
	if hash, _, bound, err := boundImageIdentity(desired); err != nil {
		return nil, nil, err
	} else if bound && (live.Identity.IsZero() || live.Identity.ID != hash) {
		return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, hash, live.Identity.ID)
	}
	return resource.Attributes{}, resource.Attributes{}, nil
}

func (p *Provider) readAdImage(ctx context.Context, hash string) (imageInfo, error) {
	c, err := p.Client()
	if err != nil {
		return imageInfo{}, err
	}
	raw, err := json.Marshal([]string{hash})
	if err != nil {
		return imageInfo{}, err
	}
	var list adImageList
	if err := c.Get(ctx, c.AdAccountID()+"/adimages", url.Values{
		"hashes": {string(raw)},
		"fields": {"hash,url,width,height,name,status"},
	}, &list); err != nil {
		if client.IsNotFound(err) {
			return imageInfo{}, provider.ErrNotFound
		}
		return imageInfo{}, err
	}
	var match imageInfo
	for _, item := range list.Data {
		if strings.TrimSpace(item.Hash) == hash {
			match = item
			break
		}
	}
	if strings.TrimSpace(match.Hash) == "" {
		return imageInfo{}, provider.ErrNotFound
	}
	if strings.EqualFold(strings.TrimSpace(match.Status), "DELETED") {
		return imageInfo{}, provider.ErrNotFound
	}
	return match, nil
}

func remoteImage(addr resource.Address, info imageInfo, digest string) resource.RemoteResource {
	hash := strings.TrimSpace(info.Hash)
	computed := resource.Attributes{OutputImageHash: hash}
	if info.Width > 0 {
		computed["width"] = info.Width
	}
	if info.Height > 0 {
		computed["height"] = info.Height
	}
	if strings.TrimSpace(info.Name) != "" {
		computed["name"] = strings.TrimSpace(info.Name)
	}
	return resource.RemoteResource{
		Address: addr,
		Identity: resource.Identity{
			ID:          hash,
			Fingerprint: digest,
		},
		Attributes: resource.Attributes{},
		Computed:   computed,
	}
}

func extractImageHash(resp imageUploadResponse, filename string) (string, error) {
	if len(resp.Images) == 0 {
		return "", fmt.Errorf("API returned no image data in upload response")
	}
	if info, ok := resp.Images[filename]; ok && strings.TrimSpace(info.Hash) != "" {
		return strings.TrimSpace(info.Hash), nil
	}
	if base := path.Base(filename); base != filename {
		if info, ok := resp.Images[base]; ok && strings.TrimSpace(info.Hash) != "" {
			return strings.TrimSpace(info.Hash), nil
		}
	}
	for _, info := range resp.Images {
		if h := strings.TrimSpace(info.Hash); h != "" {
			return h, nil
		}
	}
	return "", fmt.Errorf("API upload response did not include an image hash")
}
