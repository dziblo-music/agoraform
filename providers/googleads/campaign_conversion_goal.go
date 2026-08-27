package googleads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

const (
	// TypeCampaignConversionGoal is the Google Ads campaign conversion-goal
	// type used in addresses such as
	// googleads.campaign_conversion_goal.trial_signup.
	TypeCampaignConversionGoal = "campaign_conversion_goal"

	// AttrCampaign is a $ref to a googleads.campaign.
	AttrCampaign = "campaign"

	campaignConversionGoalOriginWebsite = "WEBSITE"
	campaignConversionGoalsCollection   = "campaignConversionGoals"
)

var (
	supportedCampaignConversionGoalAttrs = map[string]struct{}{
		AttrCampaign:         {},
		AttrCategory:         {},
		AttrOrigin:           {},
		AttrBiddable:         {},
		AttrConversionAction: {},
	}

	computedCampaignConversionGoalAttrs = map[string]struct{}{
		"id":            {},
		"resourceName":  {},
		"resource_name": {},
	}

	campaignConversionGoalSelect = strings.Join([]string{
		"SELECT",
		"campaign_conversion_goal.resource_name,",
		"campaign_conversion_goal.campaign,",
		"campaign_conversion_goal.category,",
		"campaign_conversion_goal.origin,",
		"campaign_conversion_goal.biddable",
		"FROM campaign_conversion_goal",
	}, " ")
)

func (p *Provider) validateCampaignConversionGoal(res resource.Resource) error {
	if err := p.requireCustomerID(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}

	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedCampaignConversionGoalAttrs[key]; ok {
			continue
		}
		if _, computed := computedCampaignConversionGoalAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; googleads.campaign_conversion_goal supports %s", res.Address, key, joinSorted(keys(supportedCampaignConversionGoalAttrs)))
	}

	if _, err := requiredCampaignRef(res); err != nil {
		return err
	}
	if _, err := requiredCampaignConversionGoalCategory(res); err != nil {
		return err
	}
	if _, err := requiredCampaignConversionGoalOrigin(res); err != nil {
		return err
	}
	if _, err := requiredBool(res, AttrBiddable); err != nil {
		return err
	}
	if _, _, err := optionalConversionActionRef(res); err != nil {
		return err
	}
	if _, _, err := boundCampaignConversionGoalIdentity(res); err != nil {
		return err
	}
	return nil
}

func (p *Provider) readCampaignConversionGoal(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateCampaignConversionGoal(res); err != nil {
		return resource.RemoteResource{}, err
	}

	id, bound, err := boundCampaignConversionGoalIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if bound {
		live, err := p.readCampaignConversionGoalByID(ctx, res.Address, id, res.Attributes)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		if err := p.ensureCampaignConversionGoalIdentityMatches(res, live.Identity.ID); err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return live, nil
	}

	campaignID, ok := p.campaignIDFromRef(res.Attributes[AttrCampaign])
	if !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, provider.ErrNotFound)
	}
	category, origin, err := configuredCampaignConversionGoalKey(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	live, err := p.readCampaignConversionGoalByID(ctx, res.Address, campaignConversionGoalID(campaignID, category, origin), res.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
	}
	return live, nil
}

func (p *Provider) createCampaignConversionGoal(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateCampaignConversionGoal(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundCampaignConversionGoalIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}

	campaignID, ok := p.campaignIDFromRef(res.Attributes[AttrCampaign])
	if !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: campaign reference %s has no provider-native identity", res.Address, logicalRef(res.Attributes[AttrCampaign]).Address)
	}
	category, origin, err := configuredCampaignConversionGoalKey(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	id := campaignConversionGoalID(campaignID, category, origin)
	live, err := p.readCampaignConversionGoalByID(ctx, res.Address, id, res.Attributes)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, missingCampaignConversionGoalError(res.Address, campaignID, category, origin)
		}
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}
	return p.reconcileCampaignConversionGoalBiddable(ctx, res, live)
}

func (p *Provider) updateCampaignConversionGoal(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateCampaignConversionGoal(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: missing remote identity", desired.Address)
	}
	if id, bound, err := boundCampaignConversionGoalIdentity(desired); err != nil {
		return resource.RemoteResource{}, err
	} else if bound && id != actual.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: persisted identity %q does not match planned remote identity %q", desired.Address, id, actual.Identity.ID)
	}
	if err := p.ensureCampaignConversionGoalIdentityMatches(desired, actual.Identity.ID); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}
	return p.reconcileCampaignConversionGoalBiddable(ctx, desired, actual)
}

func (p *Provider) importCampaignConversionGoal(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	id, err := p.canonicalCampaignConversionGoalImportID(addr, rawID)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireCustomerID(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	live, err := p.readCampaignConversionGoalByID(ctx, addr, id, nil)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: remote campaign conversion goal %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	if _, ok := live.Attributes[AttrCampaign]; !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: campaign is not bound in local state; import the googleads.campaign resource first (or apply it), then re-import this campaign conversion goal", addr)
	}
	return live, nil
}

func (p *Provider) normalizeCampaignConversionGoalComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	want, err := comparableCampaignConversionGoal(desired.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if live == nil {
		return want, nil, nil
	}
	if id, bound, err := boundCampaignConversionGoalIdentity(desired); err != nil {
		return nil, nil, err
	} else if bound {
		if live.Identity.IsZero() || live.Identity.ID != id {
			return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, id, live.Identity.ID)
		}
	}
	if err := p.ensureCampaignConversionGoalIdentityMatches(desired, live.Identity.ID); err != nil {
		return nil, nil, err
	}
	got, err := comparableCampaignConversionGoal(live.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	return want, got, nil
}

func (p *Provider) reconcileCampaignConversionGoalBiddable(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	want, err := comparableCampaignConversionGoal(desired.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: %s: %w", desired.Address, err)
	}
	got, err := comparableCampaignConversionGoal(actual.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: %s: %w", desired.Address, err)
	}
	if want[AttrCategory] != got[AttrCategory] || want[AttrOrigin] != got[AttrOrigin] {
		return resource.RemoteResource{}, fmt.Errorf("googleads: %s: category and origin are provider-created identity and cannot be changed from %s/%s to %s/%s", desired.Address, got[AttrCategory], got[AttrOrigin], want[AttrCategory], want[AttrOrigin])
	}
	if !sameRef(want[AttrCampaign], got[AttrCampaign]) {
		return resource.RemoteResource{}, fmt.Errorf("googleads: %s: campaign is provider-created identity and cannot be changed from %s to %s", desired.Address, logicalRef(got[AttrCampaign]).Address, logicalRef(want[AttrCampaign]).Address)
	}
	if want[AttrBiddable] == got[AttrBiddable] {
		live, err := p.readCampaignConversionGoalByID(ctx, desired.Address, actual.Identity.ID, desired.Attributes)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: %s: %w", desired.Address, err)
		}
		return live, nil
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	resourceName := campaignConversionGoalResourceName(c.CustomerID(), actual.Identity.ID)
	_, err = c.Mutate(ctx, campaignConversionGoalsCollection, []map[string]any{
		{
			"updateMask": "biddable",
			"update": map[string]any{
				"resourceName": resourceName,
				"biddable":     want[AttrBiddable],
			},
		},
	})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}
	live, err := p.readCampaignConversionGoalByID(ctx, desired.Address, actual.Identity.ID, desired.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s succeeded but refreshing campaign conversion goal %q failed: %w", desired.Address, actual.Identity.ID, err)
	}
	return live, nil
}

func (p *Provider) readCampaignConversionGoalByID(ctx context.Context, addr resource.Address, id string, desired resource.Attributes) (resource.RemoteResource, error) {
	campaignID, category, origin, err := parseCampaignConversionGoalID(addr, id)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	matches, err := p.queryCampaignConversionGoals(ctx, campaignConversionGoalWhere(c.CustomerID(), campaignID, category, origin))
	if err != nil {
		return resource.RemoteResource{}, err
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, provider.ErrNotFound
	case 1:
		return p.remoteCampaignConversionGoal(addr, matches[0], desired)
	default:
		return resource.RemoteResource{}, fmt.Errorf("multiple remote campaign conversion goals returned for campaign %s %s/%s", campaignID, category, origin)
	}
}

func (p *Provider) queryCampaignConversionGoals(ctx context.Context, where string) ([]campaignConversionGoalData, error) {
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	query := campaignConversionGoalSelect
	if strings.TrimSpace(where) != "" {
		query += " WHERE " + where
	}
	rows, err := c.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]campaignConversionGoalData, 0, len(rows))
	for _, row := range rows {
		item, err := decodeCampaignConversionGoalRow(row, c.CustomerID())
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

type campaignConversionGoalData struct {
	ResourceName string
	Campaign     string
	CampaignID   string
	Category     string
	Origin       string
	Biddable     bool
}

type campaignConversionGoalJSON struct {
	ResourceName string `json:"resourceName"`
	Campaign     string `json:"campaign"`
	Category     string `json:"category"`
	Origin       string `json:"origin"`
	Biddable     *bool  `json:"biddable"`
}

func decodeCampaignConversionGoalRow(raw json.RawMessage, configuredCustomerID string) (campaignConversionGoalData, error) {
	malformed := func(detail string) (campaignConversionGoalData, error) {
		if detail == "" {
			return campaignConversionGoalData{}, fmt.Errorf("malformed campaign conversion goal result")
		}
		return campaignConversionGoalData{}, fmt.Errorf("malformed campaign conversion goal result: %s", detail)
	}

	var envelope struct {
		CampaignConversionGoal json.RawMessage `json:"campaignConversionGoal"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.CampaignConversionGoal) == 0 {
		return malformed("")
	}
	var body campaignConversionGoalJSON
	if err := json.Unmarshal(envelope.CampaignConversionGoal, &body); err != nil {
		return malformed("")
	}

	resourceName := strings.TrimSpace(body.ResourceName)
	resourceCustomerID, id, ok := splitCampaignConversionGoalResourceName(resourceName)
	if !ok {
		return malformed("invalid resourceName")
	}
	if configuredCustomerID != "" && resourceCustomerID != configuredCustomerID {
		return malformed("resourceName belongs to a different customer")
	}

	campaignResourceName := strings.TrimSpace(body.Campaign)
	campaignCustomerID, campaignID, ok := splitCampaignResourceName(campaignResourceName)
	if !ok {
		return malformed("invalid campaign")
	}
	if configuredCustomerID != "" && campaignCustomerID != configuredCustomerID {
		return malformed("campaign belongs to a different customer")
	}

	category := normalizeEnum(body.Category)
	origin := normalizeEnum(body.Origin)
	if category == "" {
		return malformed("missing category")
	}
	if origin == "" {
		return malformed("missing origin")
	}
	if body.Biddable == nil {
		return malformed("missing biddable")
	}
	wantID := campaignConversionGoalID(campaignID, category, origin)
	if wantID != id {
		return malformed("resourceName does not match campaign, category, and origin")
	}
	if _, ok := conversionActionCategories[category]; !ok {
		return campaignConversionGoalData{}, fmt.Errorf("campaign conversion goal %s has unsupported category %s; googleads.campaign_conversion_goal only manages website conversion categories", id, category)
	}
	if origin != campaignConversionGoalOriginWebsite {
		return campaignConversionGoalData{}, fmt.Errorf("campaign conversion goal %s has origin %s; googleads.campaign_conversion_goal only manages website (WEBSITE) conversion goals", id, origin)
	}

	return campaignConversionGoalData{
		ResourceName: resourceName,
		Campaign:     campaignResourceName,
		CampaignID:   campaignID,
		Category:     category,
		Origin:       origin,
		Biddable:     *body.Biddable,
	}, nil
}

func (p *Provider) remoteCampaignConversionGoal(addr resource.Address, item campaignConversionGoalData, desired resource.Attributes) (resource.RemoteResource, error) {
	id := campaignConversionGoalID(item.CampaignID, item.Category, item.Origin)
	attrs := resource.Attributes{
		AttrCategory: item.Category,
		AttrOrigin:   item.Origin,
		AttrBiddable: item.Biddable,
	}
	campaign, err := p.liveCampaignGoalAttr(addr, item.Campaign, desired[AttrCampaign])
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if campaign != nil {
		attrs[AttrCampaign] = campaign
	}

	computed := resource.Attributes{}
	setComputed(computed, "id", id)
	setComputed(computed, "resourceName", item.ResourceName)
	return resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: id},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func (p *Provider) liveCampaignGoalAttr(addr resource.Address, campaignResourceName string, desired any) (any, error) {
	want := logicalRef(desired)
	_, campaignID, ok := splitCampaignResourceName(campaignResourceName)
	if !ok {
		if campaignResourceName == "" {
			return nil, nil
		}
		return nil, fmt.Errorf("resource %s: remote campaign resource name is invalid", addr)
	}
	if !want.IsZero() {
		wantID := ""
		if resolved, ok := resource.AsResolved(desired); ok {
			wantID = resolved.Identity.ID
		}
		if wantID == "" {
			wantID = p.lookupID(want.Address)
		}
		if wantID != "" && wantID == campaignID {
			return want, nil
		}
	}
	managed, found, err := p.lookupManagedAddress(TypeCampaign, campaignID)
	if err != nil {
		return nil, err
	}
	if found {
		return resource.Ref{Address: managed}, nil
	}
	if !want.IsZero() {
		return campaignID, nil
	}
	return nil, nil
}

func comparableCampaignConversionGoal(attrs resource.Attributes) (resource.Attributes, error) {
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	campaign, err := comparableCampaignAttr(attrs[AttrCampaign])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrCampaign, err)
	}
	category, err := coerceString(attrs[AttrCategory])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrCategory, err)
	}
	origin, err := coerceString(attrs[AttrOrigin])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrOrigin, err)
	}
	biddable, err := coerceBool(attrs[AttrBiddable])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrBiddable, err)
	}
	return resource.Attributes{
		AttrCampaign: campaign,
		AttrCategory: normalizeEnum(category),
		AttrOrigin:   normalizeEnum(origin),
		AttrBiddable: biddable,
	}, nil
}

func requiredCampaignRef(res resource.Resource) (resource.Ref, error) {
	v, ok := res.Attributes[AttrCampaign]
	if !ok {
		return resource.Ref{}, fmt.Errorf("resource %s: missing required attribute %q", res.Address, AttrCampaign)
	}
	ref, err := campaignRefValue(v)
	if err != nil {
		return resource.Ref{}, fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrCampaign, err)
	}
	if ref.Address.Provider != Name || ref.Address.Type != TypeCampaign {
		return resource.Ref{}, fmt.Errorf("resource %s: attribute %q must reference a %s.%s resource", res.Address, AttrCampaign, Name, TypeCampaign)
	}
	return ref, nil
}

func campaignRefValue(v any) (resource.Ref, error) {
	if resolved, ok := resource.AsResolved(v); ok {
		return resource.Ref{Address: resolved.Address}, nil
	}
	ref, ok := resource.AsRef(v)
	if !ok {
		return resource.Ref{}, fmt.Errorf("must be a resource reference ($ref) to a %s.%s resource", Name, TypeCampaign)
	}
	return ref, nil
}

func comparableCampaignAttr(v any) (resource.Ref, error) {
	return campaignRefValue(v)
}

func requiredCampaignConversionGoalCategory(res resource.Resource) (string, error) {
	category, err := requiredString(res, AttrCategory)
	if err != nil {
		return "", err
	}
	category = normalizeEnum(category)
	if _, ok := conversionActionCategories[category]; !ok {
		return "", fmt.Errorf("resource %s: attribute %q must be one of %s", res.Address, AttrCategory, joinSorted(keys(conversionActionCategories)))
	}
	return category, nil
}

func requiredCampaignConversionGoalOrigin(res resource.Resource) (string, error) {
	origin, err := requiredString(res, AttrOrigin)
	if err != nil {
		return "", err
	}
	origin = normalizeEnum(origin)
	if origin != campaignConversionGoalOriginWebsite {
		return "", fmt.Errorf("resource %s: attribute %q must be %s; googleads.campaign_conversion_goal only manages website conversion goals", res.Address, AttrOrigin, campaignConversionGoalOriginWebsite)
	}
	return origin, nil
}

func configuredCampaignConversionGoalKey(res resource.Resource) (category, origin string, err error) {
	category, err = requiredCampaignConversionGoalCategory(res)
	if err != nil {
		return "", "", err
	}
	origin, err = requiredCampaignConversionGoalOrigin(res)
	if err != nil {
		return "", "", err
	}
	return category, origin, nil
}

func (p *Provider) campaignIDFromRef(v any) (string, bool) {
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

func (p *Provider) ensureCampaignConversionGoalIdentityMatches(res resource.Resource, id string) error {
	campaignID, category, origin, err := parseCampaignConversionGoalID(res.Address, id)
	if err != nil {
		return err
	}
	wantCategory, wantOrigin, err := configuredCampaignConversionGoalKey(res)
	if err != nil {
		return err
	}
	if category != wantCategory || origin != wantOrigin {
		return fmt.Errorf("persisted identity %q does not match configured %s/%s", id, wantCategory, wantOrigin)
	}
	if gotCampaignID, ok := p.campaignIDFromRef(res.Attributes[AttrCampaign]); ok && gotCampaignID != campaignID {
		return fmt.Errorf("persisted identity %q does not match referenced campaign %s", id, gotCampaignID)
	}
	return nil
}

func boundCampaignConversionGoalIdentity(res resource.Resource) (string, bool, error) {
	if res.Identity.IsZero() {
		return "", false, nil
	}
	id, err := parseCampaignConversionGoalIdentity(res.Address, res.Identity.ID)
	if err != nil {
		return "", true, err
	}
	return id, true, nil
}

func parseCampaignConversionGoalIdentity(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("resource %s: persisted identity is empty; a Google Ads campaign conversion goal id of the form CAMPAIGN_ID~CATEGORY~ORIGIN is required", addr)
	}
	if _, id, ok := splitCampaignConversionGoalResourceName(raw); ok {
		raw = id
	}
	if _, _, _, err := parseCampaignConversionGoalID(addr, raw); err != nil {
		return "", err
	}
	return raw, nil
}

func (p *Provider) canonicalCampaignConversionGoalImportID(addr resource.Address, raw string) (string, error) {
	id, err := parseImportCampaignConversionGoalID(addr, raw)
	if err != nil {
		return "", err
	}
	if customerID, restID, ok := splitCampaignConversionGoalResourceName(id); ok {
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

func parseImportCampaignConversionGoalID(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("googleads: import %s: remote identifier is empty; expected CAMPAIGN_ID~CATEGORY~ORIGIN or resource name customers/{customerId}/campaignConversionGoals/{campaignId}~{category}~{origin}", addr)
	}
	if _, id, ok := splitCampaignConversionGoalResourceName(raw); ok {
		if _, _, _, err := parseCampaignConversionGoalID(addr, id); err != nil {
			return "", err
		}
		return raw, nil
	}
	campaignID, category, origin, err := parseCampaignConversionGoalID(addr, raw)
	if err != nil {
		return "", err
	}
	return campaignConversionGoalID(campaignID, category, origin), nil
}

func parseCampaignConversionGoalID(addr resource.Address, raw string) (campaignID, category, origin string, err error) {
	raw = strings.TrimSpace(raw)
	parts := strings.Split(raw, "~")
	if len(parts) != 3 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" || strings.TrimSpace(parts[2]) == "" {
		return "", "", "", fmt.Errorf("resource %s: %q is not a valid Google Ads campaign conversion goal id; expected CAMPAIGN_ID~CATEGORY~ORIGIN", addr, raw)
	}
	campaignID = strings.TrimSpace(parts[0])
	if n, parseErr := strconv.ParseInt(campaignID, 10, 64); parseErr != nil || n <= 0 {
		return "", "", "", fmt.Errorf("resource %s: %q is not a valid Google Ads campaign conversion goal id; expected CAMPAIGN_ID~CATEGORY~ORIGIN", addr, raw)
	}
	category = normalizeEnum(parts[1])
	origin = normalizeEnum(parts[2])
	if _, ok := conversionActionCategories[category]; !ok {
		return "", "", "", fmt.Errorf("resource %s: %q has unsupported category %s; googleads.campaign_conversion_goal only manages website conversion categories", addr, raw, category)
	}
	if origin != campaignConversionGoalOriginWebsite {
		return "", "", "", fmt.Errorf("resource %s: %q has origin %s; googleads.campaign_conversion_goal only manages website (WEBSITE) conversion goals", addr, raw, origin)
	}
	return campaignID, category, origin, nil
}

func splitCampaignConversionGoalResourceName(name string) (customerID, goalID string, ok bool) {
	name = strings.TrimSpace(name)
	parts := strings.Split(name, "/")
	if len(parts) != 4 || parts[0] != "customers" || parts[2] != campaignConversionGoalsCollection {
		return "", "", false
	}
	if _, err := NormalizeCustomerID(parts[1]); err != nil {
		return "", "", false
	}
	idParts := strings.Split(parts[3], "~")
	if len(idParts) != 3 || strings.TrimSpace(idParts[0]) == "" || strings.TrimSpace(idParts[1]) == "" || strings.TrimSpace(idParts[2]) == "" {
		return "", "", false
	}
	if n, err := strconv.ParseInt(strings.TrimSpace(idParts[0]), 10, 64); err != nil || n <= 0 {
		return "", "", false
	}
	return parts[1], strings.TrimSpace(idParts[0]) + "~" + normalizeEnum(idParts[1]) + "~" + normalizeEnum(idParts[2]), true
}

func campaignConversionGoalResourceName(customerID, id string) string {
	return "customers/" + customerID + "/" + campaignConversionGoalsCollection + "/" + id
}

func campaignConversionGoalID(campaignID, category, origin string) string {
	return strings.TrimSpace(campaignID) + "~" + normalizeEnum(category) + "~" + normalizeEnum(origin)
}

func campaignConversionGoalWhere(customerID, campaignID, category, origin string) string {
	return "campaign_conversion_goal.campaign = " + gaqlString(campaignResourceName(customerID, campaignID)) +
		" AND campaign_conversion_goal.category = " + gaqlString(category) +
		" AND campaign_conversion_goal.origin = " + gaqlString(origin)
}

func missingCampaignConversionGoalError(addr resource.Address, campaignID, category, origin string) error {
	return fmt.Errorf("googleads: %s: campaign conversion goal %s/%s/%s was not found; Google Ads creates this object automatically after the campaign and a matching conversion action exist, and Agoraform cannot create or delete it. Apply the referenced googleads.campaign and a googleads.conversion_action with category %s and origin %s first, then retry", addr, campaignID, category, origin, category, origin)
}
