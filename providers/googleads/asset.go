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
	"reflect"
	"sort"
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

	// AttrType is the Google Ads AssetType: IMAGE, TEXT, SITELINK, or CALLOUT.
	AttrType = "type"
	// AttrAsset is a $ref to a googleads.asset on a campaign-asset link.
	AttrAsset = "asset"
	// AttrFieldType is the Google Ads AssetFieldType on a campaign-asset link.
	AttrFieldType = "fieldType"
	// AttrLinkText is the sitelink display text (SitelinkAsset.link_text).
	AttrLinkText = "linkText"
	// AttrCalloutText is the callout display text (CalloutAsset.callout_text).
	AttrCalloutText = "calloutText"
	// AttrDescription1 is the optional first sitelink description line.
	AttrDescription1 = "description1"
	// AttrDescription2 is the optional second sitelink description line.
	AttrDescription2 = "description2"

	assetTypeImage    = "IMAGE"
	assetTypeText     = "TEXT"
	assetTypeSitelink = "SITELINK"
	assetTypeCallout  = "CALLOUT"

	assetsCollection = "assets"

	maxImageBytes               = 5_120_000 // 5,120 KB Google Ads image limit
	minImageEdgePixels          = 128       // smallest documented logo edge
	maxBusinessNameRunes        = 25
	maxSitelinkLinkTextRunes    = 25
	maxSitelinkDescriptionRunes = 35
	maxCalloutTextRunes         = 25
	imageReplaceGuidance        = "image content is immutable after create; declare a new googleads.asset and repoint googleads.campaign_asset attachments instead of updating this resource"
	textReplaceGuidance         = "text content is immutable after create; declare a new googleads.asset and repoint googleads.campaign_asset attachments instead of updating this resource"
	automaticallyCreated        = "AUTOMATICALLY_CREATED"
	assetSourceAdvertiser       = "ADVERTISER"
)

var (
	supportedAssetAttrs = map[string]struct{}{
		AttrType:         {},
		AttrName:         {},
		asset.AttrName:   {},
		AttrText:         {},
		AttrLinkText:     {},
		AttrCalloutText:  {},
		AttrDescription1: {},
		AttrDescription2: {},
		AttrFinalUrls:    {},
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
		"sitelinkAsset":   {},
		"sitelink_asset":  {},
		"calloutAsset":    {},
		"callout_asset":   {},
		"assetId":         {},
		asset.AttrDigest:  {},
		"fullSize":        {},
		"full_size":       {},
	}

	supportedAssetTypes = map[string]struct{}{
		assetTypeImage:    {},
		assetTypeText:     {},
		assetTypeSitelink: {},
		assetTypeCallout:  {},
	}

	assetTypeSpecificAttrs = map[string]map[string]struct{}{
		assetTypeImage: {
			asset.AttrName: {},
		},
		assetTypeText: {
			AttrText: {},
		},
		assetTypeSitelink: {
			AttrLinkText:     {},
			AttrFinalUrls:    {},
			AttrDescription1: {},
			AttrDescription2: {},
		},
		assetTypeCallout: {
			AttrCalloutText: {},
		},
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
		"asset.text_asset.text,",
		"asset.final_urls,",
		"asset.sitelink_asset.link_text,",
		"asset.sitelink_asset.description1,",
		"asset.sitelink_asset.description2,",
		"asset.callout_asset.callout_text",
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
	if err := validateAssetAttrsForType(res, kind); err != nil {
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
	case assetTypeText:
		if _, present, err := asset.SourceFile(res.Attributes); err != nil {
			return fmt.Errorf("resource %s: %w", res.Address, err)
		} else if present || res.LocalAsset != nil {
			return fmt.Errorf("resource %s: TEXT assets use attribute %q, not a local %s.file", res.Address, AttrText, asset.AttrName)
		}
		if _, err := requiredBusinessNameText(res); err != nil {
			return err
		}
	case assetTypeSitelink:
		if err := rejectLocalAssetSource(res, assetTypeSitelink); err != nil {
			return err
		}
		if _, err := requiredLimitedText(res, AttrLinkText, maxSitelinkLinkTextRunes); err != nil {
			return err
		}
		if _, err := requiredRSAFinalURLs(res); err != nil {
			return err
		}
		if _, _, err := requiredSitelinkDescriptions(res); err != nil {
			return err
		}
	case assetTypeCallout:
		if err := rejectLocalAssetSource(res, assetTypeCallout); err != nil {
			return err
		}
		if _, err := requiredLimitedText(res, AttrCalloutText, maxCalloutTextRunes); err != nil {
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
	return requiredLimitedText(res, AttrText, maxBusinessNameRunes)
}

func requiredLimitedText(res resource.Resource, key string, maxRunes int) (string, error) {
	text, err := requiredString(res, key)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("resource %s: attribute %q must be a non-empty string", res.Address, key)
	}
	if utf8.RuneCountInString(text) > maxRunes {
		return "", fmt.Errorf("resource %s: attribute %q must be at most %d characters", res.Address, key, maxRunes)
	}
	return text, nil
}

func optionalLimitedText(res resource.Resource, key string, maxRunes int) (string, bool, error) {
	text, set, err := optionalString(res, key)
	if err != nil || !set {
		return text, set, err
	}
	if strings.TrimSpace(text) == "" {
		return "", true, fmt.Errorf("resource %s: attribute %q must be a non-empty string", res.Address, key)
	}
	if utf8.RuneCountInString(text) > maxRunes {
		return "", true, fmt.Errorf("resource %s: attribute %q must be at most %d characters", res.Address, key, maxRunes)
	}
	return text, true, nil
}

func requiredSitelinkDescriptions(res resource.Resource) (string, string, error) {
	d1, set1, err := optionalLimitedText(res, AttrDescription1, maxSitelinkDescriptionRunes)
	if err != nil {
		return "", "", err
	}
	d2, set2, err := optionalLimitedText(res, AttrDescription2, maxSitelinkDescriptionRunes)
	if err != nil {
		return "", "", err
	}
	if set1 != set2 {
		return "", "", fmt.Errorf("resource %s: attributes %q and %q must both be set or both omitted", res.Address, AttrDescription1, AttrDescription2)
	}
	return d1, d2, nil
}

func validateAssetAttrsForType(res resource.Resource, kind string) error {
	allowed := map[string]struct{}{
		AttrType: {},
		AttrName: {},
	}
	for key := range assetTypeSpecificAttrs[kind] {
		allowed[key] = struct{}{}
	}
	for key := range res.Attributes {
		if _, ok := allowed[key]; ok {
			continue
		}
		if _, supported := supportedAssetAttrs[key]; !supported {
			continue
		}
		for typ, attrs := range assetTypeSpecificAttrs {
			if _, ok := attrs[key]; ok {
				return fmt.Errorf("resource %s: attribute %q is only valid for %s assets", res.Address, key, typ)
			}
		}
		return fmt.Errorf("resource %s: attribute %q is not valid for %s assets", res.Address, key, kind)
	}
	return nil
}

func rejectLocalAssetSource(res resource.Resource, kind string) error {
	if _, present, err := asset.SourceFile(res.Attributes); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	} else if present || res.LocalAsset != nil {
		return fmt.Errorf("resource %s: %s assets do not use a local %s.file", res.Address, kind, asset.AttrName)
	}
	return nil
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

	// Google Ads IMAGE and TEXT assets are immutable after creation.
	// Sitelink and callout copy can be updated in place. Normalization
	// rejects type/content replacement that would hide a new identity, and
	// ignores the create-time name.
	want, got, err := p.normalizeAssetComparable(desired, &actual)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}
	if reflect.DeepEqual(want, got) {
		live, err := p.readAssetByID(ctx, desired.Address, actual.Identity.ID, desired)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
		}
		return p.rememberLive(live), nil
	}

	kind, err := requiredAssetType(desired)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if kind == assetTypeImage || kind == assetTypeText {
		live, err := p.readAssetByID(ctx, desired.Address, actual.Identity.ID, desired)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
		}
		return p.rememberLive(live), nil
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	body, mask, err := assetUpdateBody(desired, assetResourceName(c.CustomerID(), actual.Identity.ID), want, got)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}
	if len(mask) == 0 {
		live, err := p.readAssetByID(ctx, desired.Address, actual.Identity.ID, desired)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
		}
		return p.rememberLive(live), nil
	}
	_, err = c.Mutate(ctx, assetsCollection, []map[string]any{
		{
			"update":     body,
			"updateMask": strings.Join(mask, ","),
		},
	})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}
	live, err := p.readAssetByID(ctx, desired.Address, actual.Identity.ID, desired)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: refreshing current asset: %w", desired.Address, err)
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
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: remote asset %q has type %s; googleads.asset currently manages %s assets", addr, id, kind, joinSorted(keys(supportedAssetTypes)))
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
	LinkText       string
	Description1   string
	Description2   string
	CalloutText    string
	FinalURLs      []string
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
	SitelinkAsset json.RawMessage `json:"sitelinkAsset"`
	CalloutAsset  json.RawMessage `json:"calloutAsset"`
	FinalURLs     json.RawMessage `json:"finalUrls"`
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
	if len(body.SitelinkAsset) > 0 && string(body.SitelinkAsset) != "null" {
		var sitelinkBody struct {
			LinkText     string `json:"linkText"`
			Description1 string `json:"description1"`
			Description2 string `json:"description2"`
		}
		if err := json.Unmarshal(body.SitelinkAsset, &sitelinkBody); err != nil {
			return assetData{}, malformed("invalid sitelinkAsset")
		}
		item.LinkText = sitelinkBody.LinkText
		item.Description1 = sitelinkBody.Description1
		item.Description2 = sitelinkBody.Description2
	}
	if len(body.CalloutAsset) > 0 && string(body.CalloutAsset) != "null" {
		var calloutBody struct {
			CalloutText string `json:"calloutText"`
		}
		if err := json.Unmarshal(body.CalloutAsset, &calloutBody); err != nil {
			return assetData{}, malformed("invalid calloutAsset")
		}
		item.CalloutText = calloutBody.CalloutText
	}
	if urls, err := decodeAssetFinalURLs(body.FinalURLs); err != nil {
		return assetData{}, malformed("invalid finalUrls")
	} else {
		item.FinalURLs = urls
	}
	return item, nil
}

func decodeAssetFinalURLs(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var strs []string
	if err := json.Unmarshal(raw, &strs); err == nil {
		return strs, nil
	}
	var anyList []any
	if err := json.Unmarshal(raw, &anyList); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(anyList))
	for _, item := range anyList {
		s, err := coerceString(item)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
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
	if item.Type == assetTypeSitelink {
		if item.LinkText != "" {
			attrs[AttrLinkText] = item.LinkText
		}
		if len(item.FinalURLs) > 0 {
			attrs[AttrFinalUrls] = comparableAnyList(item.FinalURLs)
		}
		if importOrManages(desired, AttrDescription1) || importOrManages(desired, AttrDescription2) || desired.Attributes == nil {
			if item.Description1 != "" {
				attrs[AttrDescription1] = item.Description1
			}
			if item.Description2 != "" {
				attrs[AttrDescription2] = item.Description2
			}
		}
	}
	if item.Type == assetTypeCallout && item.CalloutText != "" {
		attrs[AttrCalloutText] = item.CalloutText
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
	switch kind {
	case assetTypeText:
		text, err := requiredBusinessNameText(res)
		if err != nil {
			return nil, err
		}
		out[AttrText] = text
	case assetTypeSitelink:
		linkText, err := requiredLimitedText(res, AttrLinkText, maxSitelinkLinkTextRunes)
		if err != nil {
			return nil, err
		}
		urls, err := requiredRSAFinalURLs(res)
		if err != nil {
			return nil, err
		}
		out[AttrLinkText] = linkText
		out[AttrFinalUrls] = comparableAnyList(urls)
		d1, d2, err := requiredSitelinkDescriptions(res)
		if err != nil {
			return nil, err
		}
		if d1 != "" || d2 != "" {
			out[AttrDescription1] = d1
			out[AttrDescription2] = d2
		}
	case assetTypeCallout:
		text, err := requiredLimitedText(res, AttrCalloutText, maxCalloutTextRunes)
		if err != nil {
			return nil, err
		}
		out[AttrCalloutText] = text
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
	kind = normalizeEnum(kind)
	out := resource.Attributes{AttrType: kind}
	if desiredHasName(desired) {
		name, _ := coerceString(live.Attributes[AttrName])
		out[AttrName] = name
	}
	switch kind {
	case assetTypeText:
		text, err := coerceString(live.Attributes[AttrText])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrText, err)
		}
		out[AttrText] = text
	case assetTypeSitelink:
		linkText, err := coerceString(live.Attributes[AttrLinkText])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrLinkText, err)
		}
		out[AttrLinkText] = linkText
		urls, err := liveAssetFinalURLs(live.Attributes[AttrFinalUrls])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrFinalUrls, err)
		}
		out[AttrFinalUrls] = comparableAnyList(urls)
		if importOrManages(desired, AttrDescription1) || importOrManages(desired, AttrDescription2) {
			d1, _ := coerceString(live.Attributes[AttrDescription1])
			d2, _ := coerceString(live.Attributes[AttrDescription2])
			out[AttrDescription1] = d1
			out[AttrDescription2] = d2
		}
	case assetTypeCallout:
		text, err := coerceString(live.Attributes[AttrCalloutText])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrCalloutText, err)
		}
		out[AttrCalloutText] = text
	}
	return out, nil
}

func liveAssetFinalURLs(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	list, err := asAnyList(v)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		s, err := coerceString(item)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func importOrManages(res resource.Resource, key string) bool {
	if res.Attributes == nil {
		return true
	}
	_, ok := res.Attributes[key]
	return ok
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
	case assetTypeSitelink:
		linkText, err := requiredLimitedText(res, AttrLinkText, maxSitelinkLinkTextRunes)
		if err != nil {
			return nil, "", err
		}
		urls, err := requiredRSAFinalURLs(res)
		if err != nil {
			return nil, "", err
		}
		sitelink := map[string]any{"linkText": linkText}
		d1, d2, err := requiredSitelinkDescriptions(res)
		if err != nil {
			return nil, "", err
		}
		if d1 != "" || d2 != "" {
			sitelink["description1"] = d1
			sitelink["description2"] = d2
		}
		body["sitelinkAsset"] = sitelink
		body["finalUrls"] = comparableAnyList(urls)
	case assetTypeCallout:
		text, err := requiredLimitedText(res, AttrCalloutText, maxCalloutTextRunes)
		if err != nil {
			return nil, "", err
		}
		body["calloutAsset"] = map[string]any{"calloutText": text}
	}
	return body, fingerprint, nil
}

func assetUpdateBody(desired resource.Resource, resourceName string, want, got resource.Attributes) (map[string]any, []string, error) {
	kind, err := requiredAssetType(desired)
	if err != nil {
		return nil, nil, err
	}
	body := map[string]any{"resourceName": resourceName}
	mask := make([]string, 0, 4)
	switch kind {
	case assetTypeSitelink:
		if !reflect.DeepEqual(want[AttrLinkText], got[AttrLinkText]) {
			setNestedMutateValue(body, "sitelinkAsset.linkText", want[AttrLinkText])
			mask = append(mask, "sitelinkAsset.linkText")
		}
		if importOrManages(desired, AttrDescription1) || importOrManages(desired, AttrDescription2) {
			if !reflect.DeepEqual(want[AttrDescription1], got[AttrDescription1]) || !reflect.DeepEqual(want[AttrDescription2], got[AttrDescription2]) {
				setNestedMutateValue(body, "sitelinkAsset.description1", want[AttrDescription1])
				setNestedMutateValue(body, "sitelinkAsset.description2", want[AttrDescription2])
				mask = append(mask, "sitelinkAsset.description1", "sitelinkAsset.description2")
			}
		}
		if !reflect.DeepEqual(want[AttrFinalUrls], got[AttrFinalUrls]) {
			body["finalUrls"] = want[AttrFinalUrls]
			mask = append(mask, "finalUrls")
		}
	case assetTypeCallout:
		if !reflect.DeepEqual(want[AttrCalloutText], got[AttrCalloutText]) {
			setNestedMutateValue(body, "calloutAsset.calloutText", want[AttrCalloutText])
			mask = append(mask, "calloutAsset.calloutText")
		}
	}
	sort.Strings(mask)
	return body, mask, nil
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
