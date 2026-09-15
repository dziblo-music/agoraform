package googleads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

const (
	// TypeCampaignAsset is the Google Ads CampaignAsset type used in
	// addresses such as googleads.campaign_asset.product_image.
	TypeCampaignAsset = "campaign_asset"

	campaignAssetsCollection = "campaignAssets"

	fieldTypeAdImage      = "AD_IMAGE"
	fieldTypeBusinessLogo = "BUSINESS_LOGO"
	fieldTypeBusinessName = "BUSINESS_NAME"
	campaignAssetEnabled  = "ENABLED"
	campaignAssetPaused   = "PAUSED"
	campaignAssetRemoved  = "REMOVED"
)

var (
	supportedCampaignAssetAttrs = map[string]struct{}{
		AttrCampaign:  {},
		AttrAsset:     {},
		AttrFieldType: {},
		AttrStatus:    {},
	}

	computedCampaignAssetAttrs = map[string]struct{}{
		"id":                     {},
		"resourceName":           {},
		"resource_name":          {},
		"source":                 {},
		"primaryStatus":          {},
		"primary_status":         {},
		"primaryStatusReasons":   {},
		"primary_status_reasons": {},
		"policySummary":          {},
		"policy_summary":         {},
		"approvalStatus":         {},
		"reviewStatus":           {},
	}

	supportedCampaignAssetFieldTypes = map[string]struct{}{
		fieldTypeAdImage:      {},
		fieldTypeBusinessLogo: {},
		fieldTypeBusinessName: {},
	}

	campaignAssetFieldTypeAssetTypes = map[string]string{
		fieldTypeAdImage:      assetTypeImage,
		fieldTypeBusinessLogo: assetTypeImage,
		fieldTypeBusinessName: assetTypeText,
	}

	campaignAssetStatuses = map[string]struct{}{
		campaignAssetEnabled: {},
		campaignAssetPaused:  {},
	}

	campaignAssetSelect = strings.Join([]string{
		"SELECT",
		"campaign_asset.resource_name,",
		"campaign_asset.campaign,",
		"campaign_asset.asset,",
		"campaign_asset.field_type,",
		"campaign_asset.status,",
		"campaign_asset.source,",
		"campaign_asset.primary_status",
		"FROM campaign_asset",
	}, " ")
)

func (p *Provider) validateCampaignAsset(res resource.Resource) error {
	if err := p.requireCustomerID(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}

	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedCampaignAssetAttrs[key]; ok {
			continue
		}
		if _, computed := computedCampaignAssetAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; googleads.campaign_asset supports %s", res.Address, key, joinSorted(keys(supportedCampaignAssetAttrs)))
	}

	if _, err := requiredCampaignRef(res); err != nil {
		return err
	}
	if _, err := requiredAssetRef(res); err != nil {
		return err
	}
	if _, err := requiredCampaignAssetFieldType(res); err != nil {
		return err
	}
	if status, set, err := optionalEnum(res, AttrStatus, campaignAssetStatuses); err != nil {
		return err
	} else if set && status == campaignAssetRemoved {
		return fmt.Errorf("resource %s: attribute %q cannot be %s; use agoraform destroy to detach the campaign asset", res.Address, AttrStatus, campaignAssetRemoved)
	}
	return p.ensureCampaignAssetIdentityMatches(res)
}

func requiredAssetRef(res resource.Resource) (resource.Ref, error) {
	v, ok := res.Attributes[AttrAsset]
	if !ok {
		return resource.Ref{}, fmt.Errorf("resource %s: missing required attribute %q", res.Address, AttrAsset)
	}
	ref, err := assetRefValue(v)
	if err != nil {
		return resource.Ref{}, fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrAsset, err)
	}
	if ref.Address.Provider != Name || ref.Address.Type != TypeAsset {
		return resource.Ref{}, fmt.Errorf("resource %s: attribute %q must reference a %s.%s resource", res.Address, AttrAsset, Name, TypeAsset)
	}
	return ref, nil
}

func assetRefValue(v any) (resource.Ref, error) {
	if resolved, ok := resource.AsResolved(v); ok {
		return resource.Ref{Address: resolved.Address}, nil
	}
	ref, ok := resource.AsRef(v)
	if !ok {
		return resource.Ref{}, fmt.Errorf("must be a resource reference ($ref) to a %s.%s resource", Name, TypeAsset)
	}
	return ref, nil
}

func requiredCampaignAssetFieldType(res resource.Resource) (string, error) {
	fieldType, err := requiredString(res, AttrFieldType)
	if err != nil {
		return "", err
	}
	fieldType = normalizeEnum(fieldType)
	if fieldType == assetTypeImage {
		return "", fmt.Errorf("resource %s: attribute %q IMAGE is not a Google Ads campaign-asset field type; use %s for Search image assets", res.Address, AttrFieldType, fieldTypeAdImage)
	}
	if _, ok := supportedCampaignAssetFieldTypes[fieldType]; !ok {
		return "", fmt.Errorf("resource %s: attribute %q must be one of %s", res.Address, AttrFieldType, joinSorted(keys(supportedCampaignAssetFieldTypes)))
	}
	return fieldType, nil
}

func (p *Provider) readCampaignAsset(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateCampaignAsset(res); err != nil {
		return resource.RemoteResource{}, err
	}

	id, bound, err := boundCampaignAssetIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if bound {
		live, err := p.readCampaignAssetByID(ctx, res.Address, id, res.Attributes)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return p.rememberLive(live), nil
	}

	campaignID, ok := p.campaignIDFromRef(res.Attributes[AttrCampaign])
	if !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, provider.ErrNotFound)
	}
	assetID, ok := p.assetIDFromRef(res.Attributes[AttrAsset])
	if !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, provider.ErrNotFound)
	}
	fieldType, _ := requiredCampaignAssetFieldType(res)
	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	where := strings.Join([]string{
		"campaign.id = " + campaignID,
		"campaign_asset.asset = " + gaqlString(assetResourceName(c.CustomerID(), assetID)),
		"campaign_asset.field_type = " + gaqlString(fieldType),
		"campaign_asset.status != " + gaqlString(campaignAssetRemoved),
	}, " AND ")
	matches, err := p.queryCampaignAssets(ctx, where)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, provider.ErrNotFound)
	case 1:
		live, err := p.remoteCampaignAsset(res.Address, matches[0], res.Attributes)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return p.rememberLive(live), nil
	default:
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: multiple remote campaign assets for asset %s with field type %s on campaign %s", res.Address, assetID, fieldType, campaignID)
	}
}

func (p *Provider) createCampaignAsset(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateCampaignAsset(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundCampaignAssetIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	body, err := p.campaignAssetMutateBody(res, "")
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}

	raw, err := c.Mutate(ctx, campaignAssetsCollection, []map[string]any{
		{"create": body},
	})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}
	id, err := parseCampaignAssetMutateID(raw, c.CustomerID())
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}

	live, err := p.readCampaignAssetByID(ctx, res.Address, id, res.Attributes)
	if err == nil {
		return p.rememberLive(live), nil
	}
	fallback, ferr := p.remoteCampaignAssetFromDesired(res, id, c.CustomerID())
	if ferr != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s succeeded but refreshing campaign asset %q failed: %w", res.Address, id, err)
	}
	return p.rememberLive(fallback), nil
}

func (p *Provider) updateCampaignAsset(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateCampaignAsset(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: missing remote identity", desired.Address)
	}
	if id, bound, err := boundCampaignAssetIdentity(desired); err != nil {
		return resource.RemoteResource{}, err
	} else if bound && id != actual.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: persisted identity %q does not match planned remote identity %q", desired.Address, id, actual.Identity.ID)
	}

	want, got, err := p.normalizeCampaignAssetComparable(desired, &actual)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}
	if err := rejectImmutableCampaignAssetChanges(want, got); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	resourceName := campaignAssetResourceName(c.CustomerID(), actual.Identity.ID)
	status, set, _ := optionalEnum(desired, AttrStatus, campaignAssetStatuses)
	if !set {
		live, err := p.readCampaignAssetByID(ctx, desired.Address, actual.Identity.ID, desired.Attributes)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
		}
		return p.rememberLive(live), nil
	}
	_, err = c.Mutate(ctx, campaignAssetsCollection, []map[string]any{
		{
			"update": map[string]any{
				"resourceName": resourceName,
				"status":       status,
			},
			"updateMask": "status",
		},
	})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}
	live, err := p.readCampaignAssetByID(ctx, desired.Address, actual.Identity.ID, desired.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: refreshing current campaign asset: %w", desired.Address, err)
	}
	return p.rememberLive(live), nil
}

func (p *Provider) importCampaignAsset(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	id, err := p.canonicalCampaignAssetImportID(addr, rawID)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireCustomerID(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	live, err := p.readCampaignAssetByID(ctx, addr, id, nil)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: remote campaign asset %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	if _, ok := live.Attributes[AttrCampaign]; !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: campaign is not bound in local state; import the googleads.campaign resource first (or apply it), then re-import this campaign asset", addr)
	}
	if _, ok := live.Attributes[AttrAsset]; !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: asset is not bound in local state; import the googleads.asset resource first (or apply it), then re-import this campaign asset", addr)
	}
	fieldType, _ := coerceString(live.Attributes[AttrFieldType])
	if _, ok := supportedCampaignAssetFieldTypes[normalizeEnum(fieldType)]; !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: remote campaign asset %q has field type %s; googleads.campaign_asset currently manages %s", addr, id, fieldType, joinSorted(keys(supportedCampaignAssetFieldTypes)))
	}
	return p.rememberLive(live), nil
}

func (p *Provider) normalizeCampaignAssetComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	if err := p.validateCampaignAsset(desired); err != nil {
		return nil, nil, err
	}
	want, err := comparableCampaignAsset(desired.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if live == nil {
		return want, nil, nil
	}
	if id, bound, err := boundCampaignAssetIdentity(desired); err != nil {
		return nil, nil, err
	} else if bound {
		if live.Identity.IsZero() || live.Identity.ID != id {
			return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, id, live.Identity.ID)
		}
	}
	got, err := comparableCampaignAsset(live.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if err := rejectImmutableCampaignAssetChanges(want, got); err != nil {
		return nil, nil, err
	}
	return want, got, nil
}

func (p *Provider) readCampaignAssetByID(ctx context.Context, addr resource.Address, id string, desired resource.Attributes) (resource.RemoteResource, error) {
	campaignID, assetID, fieldType, err := parseCampaignAssetID(addr, id)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	where := strings.Join([]string{
		"campaign.id = " + campaignID,
		"campaign_asset.asset = " + gaqlString(assetResourceName(c.CustomerID(), assetID)),
		"campaign_asset.field_type = " + gaqlString(fieldType),
	}, " AND ")
	matches, err := p.queryCampaignAssets(ctx, where)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, provider.ErrNotFound
	case 1:
		return p.remoteCampaignAsset(addr, matches[0], desired)
	default:
		return resource.RemoteResource{}, fmt.Errorf("multiple remote campaign assets returned for id %s", id)
	}
}

func (p *Provider) queryCampaignAssets(ctx context.Context, where string) ([]campaignAssetData, error) {
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	query := campaignAssetSelect
	if strings.TrimSpace(where) != "" {
		query += " WHERE " + where
	}
	rows, err := c.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]campaignAssetData, 0, len(rows))
	for _, row := range rows {
		item, err := decodeCampaignAssetRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

type campaignAssetData struct {
	ResourceName  string
	Campaign      string
	Asset         string
	FieldType     string
	Status        string
	Source        string
	PrimaryStatus string
	CampaignID    string
	AssetID       string
}

type campaignAssetJSON struct {
	ResourceName  string `json:"resourceName"`
	Campaign      string `json:"campaign"`
	Asset         string `json:"asset"`
	FieldType     string `json:"fieldType"`
	Status        string `json:"status"`
	Source        string `json:"source"`
	PrimaryStatus string `json:"primaryStatus"`
}

func decodeCampaignAssetRow(raw json.RawMessage) (campaignAssetData, error) {
	malformed := func(detail string) error {
		if detail == "" {
			return fmt.Errorf("malformed campaign asset result")
		}
		return fmt.Errorf("malformed campaign asset result: %s", detail)
	}
	var wrapper struct {
		CampaignAsset json.RawMessage `json:"campaignAsset"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil || len(wrapper.CampaignAsset) == 0 {
		return campaignAssetData{}, malformed("missing campaignAsset")
	}
	var body campaignAssetJSON
	if err := json.Unmarshal(wrapper.CampaignAsset, &body); err != nil {
		return campaignAssetData{}, malformed("")
	}
	resourceName := strings.TrimSpace(body.ResourceName)
	resourceCustomerID, campaignID, assetID, fieldType, ok := splitCampaignAssetResourceName(resourceName)
	if !ok {
		return campaignAssetData{}, malformed("invalid resourceName")
	}
	if _, err := NormalizeCustomerID(resourceCustomerID); err != nil {
		return campaignAssetData{}, malformed("resourceName belongs to a different customer")
	}
	if body.Campaign != "" {
		_, campaignFromName, campOK := splitCampaignResourceName(body.Campaign)
		if !campOK || campaignFromName != campaignID {
			return campaignAssetData{}, malformed("campaign does not match resourceName")
		}
	}
	if body.Asset != "" {
		_, assetFromName, assetOK := splitAssetResourceName(body.Asset)
		if !assetOK || assetFromName != assetID {
			return campaignAssetData{}, malformed("asset does not match resourceName")
		}
	}
	gotField := normalizeEnum(body.FieldType)
	if gotField == "" {
		gotField = fieldType
	}
	if gotField != fieldType {
		return campaignAssetData{}, malformed("fieldType does not match resourceName")
	}

	status := normalizeEnum(body.Status)
	if status == "" {
		status = campaignAssetEnabled
	}
	return campaignAssetData{
		ResourceName:  resourceName,
		Campaign:      strings.TrimSpace(body.Campaign),
		Asset:         strings.TrimSpace(body.Asset),
		FieldType:     fieldType,
		Status:        status,
		Source:        normalizeEnum(body.Source),
		PrimaryStatus: normalizeEnum(body.PrimaryStatus),
		CampaignID:    campaignID,
		AssetID:       assetID,
	}, nil
}

func (p *Provider) remoteCampaignAsset(addr resource.Address, item campaignAssetData, desired resource.Attributes) (resource.RemoteResource, error) {
	if item.Status == campaignAssetRemoved {
		return resource.RemoteResource{}, provider.ErrNotFound
	}
	attrs := resource.Attributes{
		AttrFieldType: item.FieldType,
		AttrStatus:    item.Status,
	}
	campaign, err := p.liveCampaignGoalAttr(addr, item.Campaign, desired[AttrCampaign])
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if campaign != nil {
		attrs[AttrCampaign] = campaign
	}
	assetRef, err := p.liveAssetAttr(addr, item.Asset, desired[AttrAsset])
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if assetRef != nil {
		attrs[AttrAsset] = assetRef
	}

	id := campaignAssetID(item.CampaignID, item.AssetID, item.FieldType)
	computed := resource.Attributes{}
	setComputed(computed, "id", id)
	setComputed(computed, "resourceName", item.ResourceName)
	setComputed(computed, "source", item.Source)
	setComputed(computed, "primaryStatus", item.PrimaryStatus)

	return resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: id},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func (p *Provider) liveAssetAttr(addr resource.Address, assetResourceName string, desired any) (any, error) {
	want := logicalRef(desired)
	_, assetID, ok := splitAssetResourceName(assetResourceName)
	if !ok {
		if assetResourceName == "" {
			return nil, nil
		}
		return nil, fmt.Errorf("resource %s: remote asset resource name is invalid", addr)
	}
	if !want.IsZero() {
		wantID := ""
		if resolved, ok := resource.AsResolved(desired); ok {
			wantID = resolved.Identity.ID
		}
		if wantID == "" {
			wantID = p.lookupID(want.Address)
		}
		if wantID != "" && wantID == assetID {
			return want, nil
		}
	}
	managed, found, err := p.lookupManagedAddress(TypeAsset, assetID)
	if err != nil {
		return nil, err
	}
	if found {
		return resource.Ref{Address: managed}, nil
	}
	if !want.IsZero() {
		return assetID, nil
	}
	return nil, nil
}

func (p *Provider) remoteCampaignAssetFromDesired(res resource.Resource, id, customerID string) (resource.RemoteResource, error) {
	attrs, err := comparableCampaignAsset(res.Attributes)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	computed := resource.Attributes{
		"id":           id,
		"resourceName": campaignAssetResourceName(customerID, id),
	}
	return resource.RemoteResource{
		Address:    res.Address,
		Identity:   resource.Identity{ID: id},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func comparableCampaignAsset(attrs resource.Attributes) (resource.Attributes, error) {
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	campaign, err := comparableCampaignAttr(attrs[AttrCampaign])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrCampaign, err)
	}
	assetRef, err := comparableAssetAttr(attrs[AttrAsset])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrAsset, err)
	}
	fieldType, err := coerceString(attrs[AttrFieldType])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrFieldType, err)
	}
	fieldType = normalizeEnum(fieldType)
	out := resource.Attributes{
		AttrCampaign:  campaign,
		AttrAsset:     assetRef,
		AttrFieldType: fieldType,
	}
	if _, ok := attrs[AttrStatus]; ok {
		status, err := coerceString(attrs[AttrStatus])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrStatus, err)
		}
		out[AttrStatus] = normalizeEnum(status)
	} else {
		out[AttrStatus] = campaignAssetEnabled
	}
	return out, nil
}

func comparableAssetAttr(v any) (resource.Ref, error) {
	return assetRefValue(v)
}

func rejectImmutableCampaignAssetChanges(want, got resource.Attributes) error {
	if !sameRef(want[AttrCampaign], got[AttrCampaign]) {
		return fmt.Errorf("campaign is immutable and cannot be changed from %s to %s; create a new googleads.campaign_asset instead of mutating this attachment", logicalRef(got[AttrCampaign]).Address, logicalRef(want[AttrCampaign]).Address)
	}
	if !sameRef(want[AttrAsset], got[AttrAsset]) {
		return fmt.Errorf("asset is immutable and cannot be changed from %s to %s; create a new googleads.campaign_asset instead of mutating this attachment", logicalRef(got[AttrAsset]).Address, logicalRef(want[AttrAsset]).Address)
	}
	wantField, _ := coerceString(want[AttrFieldType])
	gotField, _ := coerceString(got[AttrFieldType])
	if gotField != "" && wantField != gotField {
		return fmt.Errorf("fieldType is immutable and cannot be changed from %s to %s; create a new googleads.campaign_asset instead of mutating this attachment", gotField, wantField)
	}
	return nil
}

func (p *Provider) campaignAssetMutateBody(res resource.Resource, resourceName string) (map[string]any, error) {
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	campaignName, err := p.campaignResourceNameFromRef(res.Attributes[AttrCampaign], c.CustomerID())
	if err != nil {
		return nil, err
	}
	assetName, err := p.assetResourceNameFromRef(res.Attributes[AttrAsset], c.CustomerID())
	if err != nil {
		return nil, err
	}
	fieldType, err := requiredCampaignAssetFieldType(res)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"campaign":  campaignName,
		"asset":     assetName,
		"fieldType": fieldType,
	}
	if resourceName != "" {
		body["resourceName"] = resourceName
	}
	if status, set, err := optionalEnum(res, AttrStatus, campaignAssetStatuses); err != nil {
		return nil, err
	} else if set {
		body["status"] = status
	} else {
		body["status"] = campaignAssetEnabled
	}
	return body, nil
}

func (p *Provider) assetResourceNameFromRef(v any, customerID string) (string, error) {
	if resolved, ok := resource.AsResolved(v); ok && resolved.Identity.ID != "" {
		return assetResourceName(customerID, resolved.Identity.ID), nil
	}
	id, ok := p.assetIDFromRef(v)
	if !ok {
		return "", fmt.Errorf("asset reference has no provider-native identity")
	}
	return assetResourceName(customerID, id), nil
}

func (p *Provider) assetIDFromRef(v any) (string, bool) {
	if resolved, ok := resource.AsResolved(v); ok {
		if resolved.Identity.ID != "" {
			return resolved.Identity.ID, true
		}
	}
	ref := logicalRef(v)
	if ref.IsZero() {
		return "", false
	}
	if id := p.lookupID(ref.Address); id != "" {
		return id, true
	}
	return "", false
}

func campaignAssetID(campaignID, assetID, fieldType string) string {
	return campaignID + "~" + assetID + "~" + fieldType
}

func campaignAssetResourceName(customerID, id string) string {
	return "customers/" + customerID + "/" + campaignAssetsCollection + "/" + id
}

func boundCampaignAssetIdentity(res resource.Resource) (string, bool, error) {
	if res.Identity.IsZero() {
		return "", false, nil
	}
	id, err := parseCampaignAssetIdentity(res.Address, res.Identity.ID)
	if err != nil {
		return "", true, err
	}
	return id, true, nil
}

func parseCampaignAssetIdentity(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("resource %s: persisted identity is empty; a Google Ads campaign asset id of the form campaignId~assetId~FIELD_TYPE is required", addr)
	}
	campaignID, assetID, fieldType, err := parseCampaignAssetID(addr, raw)
	if err != nil {
		return "", err
	}
	return campaignAssetID(campaignID, assetID, fieldType), nil
}

func parseCampaignAssetID(addr resource.Address, raw string) (campaignID, assetID, fieldType string, err error) {
	raw = strings.TrimSpace(raw)
	if _, campID, aID, ft, ok := splitCampaignAssetResourceName(raw); ok {
		return campID, aID, ft, nil
	}
	parts := strings.Split(raw, "~")
	if len(parts) != 3 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" || strings.TrimSpace(parts[2]) == "" {
		return "", "", "", fmt.Errorf("resource %s: %q is not a valid Google Ads campaign asset id; expected campaignId~assetId~FIELD_TYPE", addr, raw)
	}
	campaignID = strings.TrimSpace(parts[0])
	assetID = strings.TrimSpace(parts[1])
	fieldType = normalizeEnum(parts[2])
	if err := positiveID(campaignID); err != nil {
		return "", "", "", fmt.Errorf("resource %s: %q is not a valid Google Ads campaign asset id; expected campaignId~assetId~FIELD_TYPE", addr, raw)
	}
	if err := positiveID(assetID); err != nil {
		return "", "", "", fmt.Errorf("resource %s: %q is not a valid Google Ads campaign asset id; expected campaignId~assetId~FIELD_TYPE", addr, raw)
	}
	if _, ok := supportedCampaignAssetFieldTypes[fieldType]; !ok {
		return "", "", "", fmt.Errorf("resource %s: %q is not a valid Google Ads campaign asset id; expected campaignId~assetId~FIELD_TYPE", addr, raw)
	}
	return campaignID, assetID, fieldType, nil
}

func (p *Provider) canonicalCampaignAssetImportID(addr resource.Address, raw string) (string, error) {
	id, err := parseImportCampaignAssetID(addr, raw)
	if err != nil {
		return "", err
	}
	if customerID, campaignID, assetID, fieldType, ok := splitCampaignAssetResourceName(id); ok {
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
		return campaignAssetID(campaignID, assetID, fieldType), nil
	}
	return id, nil
}

func parseImportCampaignAssetID(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("googleads: import %s: remote identifier is empty; expected campaignId~assetId~FIELD_TYPE or resource name customers/{customerId}/campaignAssets/{campaignId}~{assetId}~{fieldType}", addr)
	}
	if _, campaignID, assetID, fieldType, ok := splitCampaignAssetResourceName(raw); ok {
		if err := positiveID(campaignID); err != nil || positiveID(assetID) != nil {
			return "", fmt.Errorf("googleads: import %s: %q is not a valid Google Ads campaign asset id; expected campaignId~assetId~FIELD_TYPE or resource name customers/{customerId}/campaignAssets/{campaignId}~{assetId}~{fieldType}", addr, raw)
		}
		if _, ok := supportedCampaignAssetFieldTypes[fieldType]; !ok {
			return "", fmt.Errorf("googleads: import %s: field type %s is not supported; googleads.campaign_asset currently manages %s", addr, fieldType, joinSorted(keys(supportedCampaignAssetFieldTypes)))
		}
		return raw, nil
	}
	campaignID, assetID, fieldType, err := parseCampaignAssetID(addr, raw)
	if err != nil {
		return "", fmt.Errorf("googleads: import %s: %q is not a valid Google Ads campaign asset id; expected campaignId~assetId~FIELD_TYPE or resource name customers/{customerId}/campaignAssets/{campaignId}~{assetId}~{fieldType}", addr, raw)
	}
	return campaignAssetID(campaignID, assetID, fieldType), nil
}

func splitCampaignAssetResourceName(name string) (customerID, campaignID, assetID, fieldType string, ok bool) {
	name = strings.TrimSpace(name)
	parts := strings.Split(name, "/")
	if len(parts) != 4 || parts[0] != "customers" || parts[2] != campaignAssetsCollection {
		return "", "", "", "", false
	}
	if _, err := NormalizeCustomerID(parts[1]); err != nil {
		return "", "", "", "", false
	}
	ids := strings.Split(parts[3], "~")
	if len(ids) != 3 {
		return "", "", "", "", false
	}
	if err := positiveID(ids[0]); err != nil {
		return "", "", "", "", false
	}
	if err := positiveID(ids[1]); err != nil {
		return "", "", "", "", false
	}
	fieldType = normalizeEnum(ids[2])
	if fieldType == "" {
		return "", "", "", "", false
	}
	return parts[1], ids[0], ids[1], fieldType, true
}

func parseCampaignAssetMutateID(raw json.RawMessage, customerID string) (string, error) {
	var resp struct {
		Results []struct {
			ResourceName string `json:"resourceName"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || len(resp.Results) == 0 {
		return "", fmt.Errorf("malformed mutate response")
	}
	resourceName := strings.TrimSpace(resp.Results[0].ResourceName)
	gotCustomer, campaignID, assetID, fieldType, ok := splitCampaignAssetResourceName(resourceName)
	if !ok {
		return "", fmt.Errorf("malformed mutate response")
	}
	if customerID != "" && gotCustomer != customerID {
		return "", fmt.Errorf("mutate returned campaign asset for a different customer")
	}
	return campaignAssetID(campaignID, assetID, fieldType), nil
}

func (p *Provider) ensureCampaignAssetIdentityMatches(res resource.Resource) error {
	id, bound, err := boundCampaignAssetIdentity(res)
	if err != nil {
		return err
	}
	if !bound {
		return nil
	}
	campaignID, assetID, fieldType, err := parseCampaignAssetID(res.Address, id)
	if err != nil {
		return err
	}
	wantField, err := requiredCampaignAssetFieldType(res)
	if err != nil {
		return err
	}
	if fieldType != wantField {
		return fmt.Errorf("resource %s: persisted identity %q does not match configured field type %s", res.Address, id, wantField)
	}
	if gotCampaignID, ok := p.campaignIDFromRef(res.Attributes[AttrCampaign]); ok && gotCampaignID != campaignID {
		return fmt.Errorf("resource %s: persisted identity %q does not match referenced campaign %s", res.Address, id, gotCampaignID)
	}
	if gotAssetID, ok := p.assetIDFromRef(res.Attributes[AttrAsset]); ok && gotAssetID != assetID {
		return fmt.Errorf("resource %s: persisted identity %q does not match referenced asset %s", res.Address, id, gotAssetID)
	}
	return nil
}
