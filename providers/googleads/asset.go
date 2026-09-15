package googleads

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
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dziblo-music/agoraform/internal/asset"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

const (
	// TypeAsset is the Google Ads Asset type used in addresses such as
	// googleads.asset.product_image.
	TypeAsset = "asset"

	// AttrType is the Google Ads AssetType, currently IMAGE or TEXT.
	AttrType = "type"
	// AttrAsset is a $ref to a googleads.asset on a campaign-asset link.
	AttrAsset = "asset"
	// AttrFieldType is the Google Ads AssetFieldType on a campaign-asset link.
	AttrFieldType = "fieldType"

	assetTypeImage = "IMAGE"
	assetTypeText  = "TEXT"

	assetsCollection = "assets"

	maxImageBytes         = 5_120_000 // 5,120 KB Google Ads image limit
	minImageEdgePixels    = 128       // smallest documented logo edge
	maxBusinessNameRunes  = 25
	imageReplaceGuidance  = "image content is immutable after create; declare a new googleads.asset and repoint googleads.campaign_asset attachments instead of updating this resource"
	textReplaceGuidance   = "text content is immutable after create; declare a new googleads.asset and repoint googleads.campaign_asset attachments instead of updating this resource"
	automaticallyCreated  = "AUTOMATICALLY_CREATED"
	assetSourceAdvertiser = "ADVERTISER"
)

var (
	supportedAssetAttrs = map[string]struct{}{
		AttrType:       {},
		AttrName:       {},
		asset.AttrName: {},
		AttrText:       {},
	}

	computedAssetAttrs = map[string]struct{}{
		"id":              {},
		"resourceName":    {},
		"resource_name":   {},
		"source":          {},
		"policySummary":   {},
		"policy_summary":  {},
		"approvalStatus":  {},
		"approval_status": {},
		"reviewStatus":    {},
		"review_status":   {},
		"fileSize":        {},
		"file_size":       {},
		"mimeType":        {},
		"mime_type":       {},
		"url":             {},
		"width":           {},
		"height":          {},
		"widthPixels":     {},
		"heightPixels":    {},
		"imageAsset":      {},
		"image_asset":     {},
		"textAsset":       {},
		"text_asset":      {},
		"assetId":         {},
		asset.AttrDigest:  {},
		"fullSize":        {},
		"full_size":       {},
	}

	supportedAssetTypes = map[string]struct{}{
		assetTypeImage: {},
		assetTypeText:  {},
	}

	supportedImageMediaTypes = map[string]struct{}{
		"image/jpeg": {},
		"image/png":  {},
		"image/gif":  {},
	}

	assetSelect = strings.Join([]string{
		"SELECT",
		"asset.resource_name,",
		"asset.id,",
		"asset.name,",
		"asset.type,",
		"asset.source,",
		"asset.policy_summary.approval_status,",
		"asset.policy_summary.review_status,",
		"asset.image_asset.file_size,",
		"asset.image_asset.mime_type,",
		"asset.image_asset.full_size.url,",
		"asset.image_asset.full_size.height_pixels,",
		"asset.image_asset.full_size.width_pixels,",
		"asset.text_asset.text",
		"FROM asset",
	}, " ")
)

func (p *Provider) validateAsset(res resource.Resource) error {
	if err := p.requireCustomerID(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}

	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedAssetAttrs[key]; ok {
			continue
		}
		if _, computed := computedAssetAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; googleads.asset supports %s", res.Address, key, joinSorted(keys(supportedAssetAttrs)))
	}

	kind, err := requiredAssetType(res)
	if err != nil {
		return err
	}
	if _, _, err := optionalString(res, AttrName); err != nil {
		return err
	}

	switch kind {
	case assetTypeImage:
		if _, present, err := asset.SourceFile(res.Attributes); err != nil {
			return fmt.Errorf("resource %s: %w", res.Address, err)
		} else if present {
			if err := validateImageLocalAsset(res); err != nil {
				return err
			}
		} else if res.Identity.IsZero() {
			return fmt.Errorf("resource %s: IMAGE assets require source.file naming a local file", res.Address)
		}
		if _, set, err := optionalString(res, AttrText); err != nil {
			return err
		} else if set {
			return fmt.Errorf("resource %s: attribute %q is only valid for TEXT assets", res.Address, AttrText)
		}
	case assetTypeText:
		if _, present, err := asset.SourceFile(res.Attributes); err != nil {
			return fmt.Errorf("resource %s: %w", res.Address, err)
		} else if present || res.LocalAsset != nil {
			return fmt.Errorf("resource %s: TEXT assets use attribute %q, not a local %s.file", res.Address, AttrText, asset.AttrName)
		}
		if _, err := requiredBusinessNameText(res); err != nil {
			return err
		}
	}
	return p.ensureAssetIdentityMatches(res)
}

func requiredAssetType(res resource.Resource) (string, error) {
	kind, err := requiredString(res, AttrType)
	if err != nil {
		return "", err
	}
	kind = normalizeEnum(kind)
	if _, ok := supportedAssetTypes[kind]; !ok {
		return "", fmt.Errorf("resource %s: attribute %q must be one of %s", res.Address, AttrType, joinSorted(keys(supportedAssetTypes)))
	}
	return kind, nil
}

func requiredBusinessNameText(res resource.Resource) (string, error) {
	text, err := requiredString(res, AttrText)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("resource %s: attribute %q must be a non-empty string", res.Address, AttrText)
	}
	if utf8.RuneCountInString(text) > maxBusinessNameRunes {
		return "", fmt.Errorf("resource %s: attribute %q must be at most %d characters", res.Address, AttrText, maxBusinessNameRunes)
	}
	return text, nil
}

func validateImageLocalAsset(res resource.Resource) error {
	if res.LocalAsset == nil {
		return fmt.Errorf("resource %s: IMAGE assets require a readable local file via %s.%s", res.Address, asset.AttrName, asset.AttrFile)
	}
	media := strings.ToLower(strings.TrimSpace(res.LocalAsset.MediaType))
	if _, ok := supportedImageMediaTypes[media]; !ok {
		return fmt.Errorf("resource %s: local file %q has unsupported type %q; Google Ads IMAGE assets accept JPEG, PNG, or GIF", res.Address, res.LocalAsset.Path, res.LocalAsset.MediaType)
	}
	if res.LocalAsset.Size <= 0 {
		return fmt.Errorf("resource %s: local file %q is empty", res.Address, res.LocalAsset.Path)
	}
	if res.LocalAsset.Size > maxImageBytes {
		return fmt.Errorf("resource %s: local file %q exceeds the Google Ads 5,120 KB image size limit", res.Address, res.LocalAsset.Path)
	}

	rc, err := res.LocalAsset.Open()
	if err != nil {
		return fmt.Errorf("resource %s: cannot open local file %q: %w", res.Address, res.LocalAsset.Path, err)
	}
	defer rc.Close()
	cfg, format, err := image.DecodeConfig(io.LimitReader(rc, maxImageBytes+1))
	if err != nil {
		return fmt.Errorf("resource %s: local file %q is not a readable JPEG, PNG, or GIF image", res.Address, res.LocalAsset.Path)
	}
	switch strings.ToLower(format) {
	case "jpeg", "png", "gif":
	default:
		return fmt.Errorf("resource %s: local file %q has unsupported image format %q; Google Ads IMAGE assets accept JPEG, PNG, or GIF", res.Address, res.LocalAsset.Path, format)
	}
	if cfg.Width < minImageEdgePixels || cfg.Height < minImageEdgePixels {
		return fmt.Errorf("resource %s: local file %q is %dx%d; Google Ads image and logo assets require at least %dx%d pixels", res.Address, res.LocalAsset.Path, cfg.Width, cfg.Height, minImageEdgePixels, minImageEdgePixels)
	}
	return nil
}

func (p *Provider) readAsset(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateAsset(res); err != nil {
		return resource.RemoteResource{}, err
	}

	id, bound, err := boundAssetIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if bound {
		live, err := p.readAssetByID(ctx, res.Address, id, res)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return p.rememberLive(live), nil
	}

	return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, provider.ErrNotFound)
}

func (p *Provider) createAsset(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateAsset(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundAssetIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	body, fingerprint, err := p.assetMutateBody(res, "")
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}

	raw, err := c.Mutate(ctx, assetsCollection, []map[string]any{
		{"create": body},
	})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}
	id, err := parseAssetMutateID(raw, c.CustomerID())
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}

	created := res
	created.Identity = resource.Identity{ID: id, Fingerprint: fingerprint}
	live, err := p.readAssetByID(ctx, res.Address, id, created)
	if err == nil {
		return p.rememberLive(live), nil
	}
	fallback, ferr := p.remoteAssetFromDesired(res, id, c.CustomerID(), fingerprint)
	if ferr != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s succeeded but refreshing asset %q failed: %w", res.Address, id, err)
	}
	return p.rememberLive(fallback), nil
}

func (p *Provider) updateAsset(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateAsset(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: missing remote identity", desired.Address)
	}
	if id, bound, err := boundAssetIdentity(desired); err != nil {
		return resource.RemoteResource{}, err
	} else if bound && id != actual.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: persisted identity %q does not match planned remote identity %q", desired.Address, id, actual.Identity.ID)
	}

	// Google Ads assets are immutable after creation. Normalization rejects
	// content/type changes and intentionally ignores the create-time name.
	if _, _, err := p.normalizeAssetComparable(desired, &actual); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}

	live, err := p.readAssetByID(ctx, desired.Address, actual.Identity.ID, desired)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}
	return p.rememberLive(live), nil
}

func (p *Provider) importAsset(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	id, err := p.canonicalAssetImportID(addr, rawID)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireCustomerID(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	live, err := p.readAssetByID(ctx, addr, id, resource.Resource{Address: addr})
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: remote asset %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	kind, _ := coerceString(live.Attributes[AttrType])
	if _, ok := supportedAssetTypes[normalizeEnum(kind)]; !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: remote asset %q has type %s; googleads.asset currently manages IMAGE and TEXT (business name) assets", addr, id, kind)
	}
	return p.rememberLive(live), nil
}

func (p *Provider) normalizeAssetComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	if err := p.validateAsset(desired); err != nil {
		return nil, nil, err
	}
	want, err := comparableAsset(desired)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if live == nil {
		return want, nil, nil
	}

	// Asset.name is create-time metadata only. Google Ads can deduplicate
	// identical assets and retain a pre-existing name, so reconciling it after
	// creation can cause permanent drift and unsupported update attempts.
	delete(want, AttrName)

	if id, bound, err := boundAssetIdentity(desired); err != nil {
		return nil, nil, err
	} else if bound {
		if live.Identity.IsZero() || live.Identity.ID != id {
			return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, id, live.Identity.ID)
		}
	}
	got, err := comparableAssetFromLive(desired, live)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	delete(got, AttrName)
	if err := rejectImmutableAssetChanges(desired, live, want, got); err != nil {
		return nil, nil, err
	}
	return want, got, nil
}

func (p *Provider) readAssetByID(ctx context.Context, addr resource.Address, id string, desired resource.Resource) (resource.RemoteResource, error) {
	where := "asset.id = " + id
	matches, err := p.queryAssets(ctx, where)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, provider.ErrNotFound
	case 1:
		return p.remoteAsset(addr, matches[0], desired)
	default:
		return resource.RemoteResource{}, fmt.Errorf("multiple remote assets returned for id %s", id)
	}
}

func (p *Provider) queryAssets(ctx context.Context, where string) ([]assetData, error) {
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	query := assetSelect
	if strings.TrimSpace(where) != "" {
		query += " WHERE " + where
	}
	rows, err := c.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]assetData, 0, len(rows))
	for _, row := range rows {
		item, err := decodeAssetRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

type assetData struct {
	ResourceName   string
	ID             string
	Name           string
	Type           string
	Source         string
	ApprovalStatus string
	ReviewStatus   string
	FileSize       string
	MimeType       string
	URL            string
	WidthPixels    string
	HeightPixels   string
	Text           string
}

type assetJSON struct {
	ResourceName  string          `json:"resourceName"`
	ID            json.Number     `json:"id"`
	Name          string          `json:"name"`
	Type          string          `json:"type"`
	Source        string          `json:"source"`
	PolicySummary json.RawMessage `json:"policySummary"`
	ImageAsset    json.RawMessage `json:"imageAsset"`
	TextAsset     json.RawMessage `json:"textAsset"`
}

func decodeAssetRow(raw json.RawMessage) (assetData, error) {
	malformed := func(detail string) error {
		if detail == "" {
			return fmt.Errorf("malformed asset result")
		}
		return fmt.Errorf("malformed asset result: %s", detail)
	}
	var wrapper struct {
		Asset json.RawMessage `json:"asset"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil || len(wrapper.Asset) == 0 {
		return assetData{}, malformed("missing asset")
	}
	var body assetJSON
	if err := json.Unmarshal(wrapper.Asset, &body); err != nil {
		return assetData{}, malformed("")
	}
	resourceName := strings.TrimSpace(body.ResourceName)
	resourceCustomerID, resourceID, ok := splitAssetResourceName(resourceName)
	if !ok {
		return assetData{}, malformed("invalid resourceName")
	}
	id := strings.TrimSpace(body.ID.String())
	if id == "" {
		id = resourceID
	}
	if id != resourceID {
		return assetData{}, malformed("id does not match resourceName")
	}
	if _, err := NormalizeCustomerID(resourceCustomerID); err != nil {
		return assetData{}, malformed("resourceName belongs to a different customer")
	}

	item := assetData{
		ResourceName: resourceName,
		ID:           id,
		Name:         strings.TrimSpace(body.Name),
		Type:         normalizeEnum(body.Type),
		Source:       normalizeEnum(body.Source),
	}
	if item.Type == "" {
		return assetData{}, malformed("missing type")
	}

	if len(body.PolicySummary) > 0 && string(body.PolicySummary) != "null" {
		var policy struct {
			ApprovalStatus string `json:"approvalStatus"`
			ReviewStatus   string `json:"reviewStatus"`
		}
		if err := json.Unmarshal(body.PolicySummary, &policy); err != nil {
			return assetData{}, malformed("invalid policySummary")
		}
		item.ApprovalStatus = normalizeEnum(policy.ApprovalStatus)
		item.ReviewStatus = normalizeEnum(policy.ReviewStatus)
	}
	if len(body.ImageAsset) > 0 && string(body.ImageAsset) != "null" {
		var imageBody struct {
			FileSize json.Number `json:"fileSize"`
			MimeType string      `json:"mimeType"`
			FullSize struct {
				URL          string      `json:"url"`
				HeightPixels json.Number `json:"heightPixels"`
				WidthPixels  json.Number `json:"widthPixels"`
			} `json:"fullSize"`
		}
		if err := json.Unmarshal(body.ImageAsset, &imageBody); err != nil {
			return assetData{}, malformed("invalid imageAsset")
		}
		item.FileSize = strings.TrimSpace(imageBody.FileSize.String())
		item.MimeType = strings.TrimSpace(imageBody.MimeType)
		item.URL = strings.TrimSpace(imageBody.FullSize.URL)
		item.HeightPixels = strings.TrimSpace(imageBody.FullSize.HeightPixels.String())
		item.WidthPixels = strings.TrimSpace(imageBody.FullSize.WidthPixels.String())
	}
	if len(body.TextAsset) > 0 && string(body.TextAsset) != "null" {
		var textBody struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(body.TextAsset, &textBody); err != nil {
			return assetData{}, malformed("invalid textAsset")
		}
		item.Text = textBody.Text
	}
	return item, nil
}

func (p *Provider) remoteAsset(addr resource.Address, item assetData, desired resource.Resource) (resource.RemoteResource, error) {
	attrs := resource.Attributes{
		AttrType: item.Type,
	}
	if desiredHasName(desired) || desired.Attributes == nil {
		if item.Name != "" {
			attrs[AttrName] = item.Name
		}
	}
	if item.Type == assetTypeText && item.Text != "" {
		attrs[AttrText] = item.Text
	}

	computed := resource.Attributes{}
	setComputed(computed, "id", item.ID)
	setComputed(computed, "resourceName", item.ResourceName)
	setComputed(computed, "source", item.Source)
	setComputed(computed, "approvalStatus", item.ApprovalStatus)
	setComputed(computed, "reviewStatus", item.ReviewStatus)
	setComputed(computed, "fileSize", item.FileSize)
	setComputed(computed, "mimeType", item.MimeType)
	setComputed(computed, "widthPixels", item.WidthPixels)
	setComputed(computed, "heightPixels", item.HeightPixels)
	// URL is a computed hosting location, never a local path, and is omitted
	// from comparable attributes. It is still redacted from normal YAML/plan
	// by remaining computed-only.

	fingerprint := strings.TrimSpace(desired.Identity.Fingerprint)
	return resource.RemoteResource{
		Address: addr,
		Identity: resource.Identity{
			ID:          item.ID,
			Fingerprint: fingerprint,
		},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func desiredHasName(res resource.Resource) bool {
	_, set, err := optionalString(res, AttrName)
	return err == nil && set
}

func (p *Provider) remoteAssetFromDesired(res resource.Resource, id, customerID, fingerprint string) (resource.RemoteResource, error) {
	attrs, err := comparableAsset(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	kind, _ := coerceString(attrs[AttrType])
	computed := resource.Attributes{
		"id":           id,
		"resourceName": assetResourceName(customerID, id),
	}
	setComputed(computed, "source", assetSourceAdvertiser)
	return resource.RemoteResource{
		Address: res.Address,
		Identity: resource.Identity{
			ID:          id,
			Fingerprint: fingerprint,
		},
		Attributes: attrs,
		Computed:   computedWithType(computed, kind),
	}, nil
}

func computedWithType(computed resource.Attributes, kind string) resource.Attributes {
	setComputed(computed, "type", kind)
	return computed
}

func comparableAsset(res resource.Resource) (resource.Attributes, error) {
	kind, err := requiredAssetType(res)
	if err != nil {
		return nil, err
	}
	out := resource.Attributes{AttrType: kind}
	if name, set, err := optionalString(res, AttrName); err != nil {
		return nil, err
	} else if set {
		out[AttrName] = name
	}
	if kind == assetTypeText {
		text, err := requiredBusinessNameText(res)
		if err != nil {
			return nil, err
		}
		out[AttrText] = text
	}
	return out, nil
}

func comparableAssetFromLive(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, error) {
	if live == nil {
		return nil, nil
	}
	kind, err := coerceString(live.Attributes[AttrType])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrType, err)
	}
	out := resource.Attributes{AttrType: normalizeEnum(kind)}
	if desiredHasName(desired) {
		name, _ := coerceString(live.Attributes[AttrName])
		out[AttrName] = name
	}
	if normalizeEnum(kind) == assetTypeText {
		text, err := coerceString(live.Attributes[AttrText])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrText, err)
		}
		out[AttrText] = text
	}
	return out, nil
}

func rejectImmutableAssetChanges(desired resource.Resource, live *resource.RemoteResource, want, got resource.Attributes) error {
	wantType, _ := coerceString(want[AttrType])
	gotType, _ := coerceString(got[AttrType])
	if gotType != "" && normalizeEnum(wantType) != normalizeEnum(gotType) {
		return fmt.Errorf("type is immutable and cannot be changed from %s to %s; declare a new googleads.asset instead of mutating this one", gotType, wantType)
	}
	if live != nil {
		if source, _ := coerceString(live.Computed["source"]); normalizeEnum(source) == automaticallyCreated {
			return fmt.Errorf("this asset was created by Google Ads (source=%s) and cannot be updated through Agoraform", automaticallyCreated)
		}
	}
	if desired.LocalAsset != nil && live != nil && !live.Identity.IsZero() {
		wantDigest := strings.TrimSpace(desired.LocalAsset.Digest)
		gotDigest := strings.TrimSpace(live.Identity.Fingerprint)
		if gotDigest == "" {
			gotDigest = strings.TrimSpace(desired.Identity.Fingerprint)
		}
		if gotDigest == "" || (wantDigest != "" && wantDigest != gotDigest) {
			return fmt.Errorf("%s", imageReplaceGuidance)
		}
	}
	wantText, _ := coerceString(want[AttrText])
	gotText, _ := coerceString(got[AttrText])
	if gotText != "" && wantText != gotText {
		return fmt.Errorf("%s", textReplaceGuidance)
	}
	return nil
}

func (p *Provider) assetMutateBody(res resource.Resource, resourceName string) (map[string]any, string, error) {
	kind, err := requiredAssetType(res)
	if err != nil {
		return nil, "", err
	}
	body := map[string]any{"type": kind}
	if resourceName != "" {
		body["resourceName"] = resourceName
	}
	if name, set, _ := optionalString(res, AttrName); set {
		body["name"] = name
	} else {
		body["name"] = res.Address.Name
	}

	fingerprint := ""
	switch kind {
	case assetTypeImage:
		data, err := readLocalImageBytes(res)
		if err != nil {
			return nil, "", err
		}
		fingerprint = strings.TrimSpace(res.LocalAsset.Digest)
		body["imageAsset"] = map[string]any{
			"data": data,
		}
	case assetTypeText:
		text, err := requiredBusinessNameText(res)
		if err != nil {
			return nil, "", err
		}
		body["textAsset"] = map[string]any{"text": text}
	}
	return body, fingerprint, nil
}

func readLocalImageBytes(res resource.Resource) ([]byte, error) {
	if res.LocalAsset == nil {
		return nil, fmt.Errorf("IMAGE assets require a local %s.file", asset.AttrName)
	}
	rc, err := res.LocalAsset.Open()
	if err != nil {
		return nil, fmt.Errorf("cannot open local file %q: %w", res.LocalAsset.Path, err)
	}
	defer rc.Close()

	limited := io.LimitReader(rc, maxImageBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("cannot stream local file %q: %w", res.LocalAsset.Path, err)
	}
	if int64(len(data)) > maxImageBytes {
		return nil, fmt.Errorf("local file %q exceeds the Google Ads 5,120 KB image size limit", res.LocalAsset.Path)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("local file %q is empty", res.LocalAsset.Path)
	}
	return data, nil
}

func boundAssetIdentity(res resource.Resource) (string, bool, error) {
	if res.Identity.IsZero() {
		return "", false, nil
	}
	id, err := parseAssetIdentity(res.Address, res.Identity.ID)
	if err != nil {
		return "", true, err
	}
	return id, true, nil
}

func parseAssetIdentity(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("resource %s: persisted identity is empty; a Google Ads asset id is required", addr)
	}
	if err := assetIdentityIDError(addr, raw); err != nil {
		return "", err
	}
	return raw, nil
}

func (p *Provider) canonicalAssetImportID(addr resource.Address, raw string) (string, error) {
	id, err := parseImportAssetID(addr, raw)
	if err != nil {
		return "", err
	}
	if customerID, restID, ok := splitAssetResourceName(id); ok {
		configured, err := p.configuredCustomerID()
		if err != nil {
			return "", fmt.Errorf("googleads: import %s: %w", addr, err)
		}
		got, err := NormalizeCustomerID(customerID)
		if err != nil {
			return "", fmt.Errorf("googleads: import %s: %w", addr, err)
		}
		if got != configured {
			return "", fmt.Errorf("googleads: import %s: resource name customer %s does not match configured %s", addr, got, configured)
		}
		return restID, nil
	}
	return id, nil
}

func parseImportAssetID(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("googleads: import %s: remote identifier is empty; expected a numeric asset id or resource name customers/{customerId}/assets/{id}", addr)
	}
	if _, id, ok := splitAssetResourceName(raw); ok {
		if err := importAssetIDError(addr, id); err != nil {
			return "", err
		}
		return raw, nil
	}
	if err := importAssetIDError(addr, raw); err != nil {
		return "", err
	}
	return raw, nil
}

func importAssetIDError(addr resource.Address, id string) error {
	if err := positiveID(id); err != nil {
		return fmt.Errorf("googleads: import %s: %q is not a valid Google Ads asset id; expected a positive numeric id or resource name customers/{customerId}/assets/{id}", addr, id)
	}
	return nil
}

func assetIdentityIDError(addr resource.Address, id string) error {
	if err := positiveID(id); err != nil {
		return fmt.Errorf("resource %s: persisted identity %q is not a valid Google Ads asset id", addr, id)
	}
	return nil
}

func splitAssetResourceName(name string) (customerID, assetID string, ok bool) {
	name = strings.TrimSpace(name)
	parts := strings.Split(name, "/")
	if len(parts) != 4 || parts[0] != "customers" || parts[2] != assetsCollection {
		return "", "", false
	}
	if _, err := NormalizeCustomerID(parts[1]); err != nil {
		return "", "", false
	}
	if err := positiveID(parts[3]); err != nil {
		return "", "", false
	}
	return parts[1], parts[3], true
}

func assetResourceName(customerID, id string) string {
	return "customers/" + customerID + "/" + assetsCollection + "/" + id
}

func parseAssetMutateID(raw json.RawMessage, customerID string) (string, error) {
	var resp struct {
		Results []struct {
			ResourceName string `json:"resourceName"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || len(resp.Results) == 0 {
		return "", fmt.Errorf("malformed mutate response")
	}
	resourceName := strings.TrimSpace(resp.Results[0].ResourceName)
	gotCustomer, id, ok := splitAssetResourceName(resourceName)
	if !ok {
		return "", fmt.Errorf("malformed mutate response")
	}
	if customerID != "" && gotCustomer != customerID {
		return "", fmt.Errorf("mutate returned asset %s for a different customer", id)
	}
	return id, nil
}

func (p *Provider) ensureAssetIdentityMatches(res resource.Resource) error {
	id, bound, err := boundAssetIdentity(res)
	if err != nil {
		return err
	}
	if !bound {
		return nil
	}
	_ = id
	return nil
}

func positiveID(id string) error {
	n, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("invalid id")
	}
	return nil
}
