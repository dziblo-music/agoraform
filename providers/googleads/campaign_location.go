package googleads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

const (
	// TypeCampaignLocation is the Google Ads campaign location criterion type
	// used in addresses such as googleads.campaign_location.united_states.
	TypeCampaignLocation = "campaign_location"

	// AttrLocation is a human-reviewable location name, country code, or
	// geoTargetConstants/{id} identifier.
	AttrLocation = "location"

	campaignCriterionTypeLocation = "LOCATION"
)

var (
	supportedCampaignLocationAttrs = map[string]struct{}{
		AttrCampaign: {},
		AttrLocation: {},
		AttrNegative: {},
	}

	computedCampaignLocationAttrs = map[string]struct{}{
		"id":                  {},
		"resourceName":        {},
		"resource_name":       {},
		"criterionId":         {},
		"criterion_id":        {},
		"type":                {},
		"geoTargetConstant":   {},
		"geo_target_constant": {},
		"status":              {},
	}

	campaignLocationSelect = strings.Join([]string{
		"SELECT",
		"campaign_criterion.resource_name,",
		"campaign_criterion.criterion_id,",
		"campaign_criterion.campaign,",
		"campaign_criterion.negative,",
		"campaign_criterion.type,",
		"campaign_criterion.status,",
		"campaign_criterion.location.geo_target_constant",
		"FROM campaign_criterion",
	}, " ")
)

func (p *Provider) validateCampaignLocation(res resource.Resource) error {
	if err := p.requireCustomerID(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}

	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedCampaignLocationAttrs[key]; ok {
			continue
		}
		if _, computed := computedCampaignLocationAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; googleads.campaign_location supports %s", res.Address, key, joinSorted(keys(supportedCampaignLocationAttrs)))
	}

	if _, err := requiredCampaignRef(res); err != nil {
		return err
	}
	if _, err := requiredLocationValue(res); err != nil {
		return err
	}
	if _, _, err := optionalBool(res, AttrNegative); err != nil {
		return err
	}
	return p.ensureCampaignCriterionIdentityMatches(res)
}

func (p *Provider) readCampaignLocation(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateCampaignLocation(res); err != nil {
		return resource.RemoteResource{}, err
	}

	id, bound, err := boundCampaignCriterionIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if bound {
		live, err := p.readCampaignLocationByID(ctx, res.Address, id, res.Attributes)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return p.rememberLive(live), nil
	}

	campaignID, ok := p.campaignIDFromRef(res.Attributes[AttrCampaign])
	if !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, provider.ErrNotFound)
	}
	target, err := p.resolveGeoTarget(ctx, res.Address, res.Attributes[AttrLocation])
	if err != nil {
		return resource.RemoteResource{}, err
	}
	negative, _, _ := optionalBool(res, AttrNegative)
	where := strings.Join([]string{
		"campaign.id = " + campaignID,
		"campaign_criterion.type = " + gaqlString(campaignCriterionTypeLocation),
		"campaign_criterion.location.geo_target_constant = " + gaqlString(target.ResourceName),
		"campaign_criterion.negative = " + gaqlBool(negative),
		"campaign_criterion.status != " + gaqlString("REMOVED"),
	}, " AND ")
	matches, err := p.queryCampaignLocations(ctx, where)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, provider.ErrNotFound)
	case 1:
		live, err := p.remoteCampaignLocation(ctx, res.Address, matches[0], res.Attributes)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return p.rememberLive(live), nil
	default:
		ids := make([]string, 0, len(matches))
		for _, item := range matches {
			ids = append(ids, campaignCriterionID(item.CampaignID, item.CriterionID))
		}
		sort.Strings(ids)
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: multiple remote location criteria for %s in campaign %s (ids %s); a campaign may target a location at most once", res.Address, target.displayName(), campaignID, strings.Join(ids, ", "))
	}
}

func (p *Provider) createCampaignLocation(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateCampaignLocation(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundCampaignCriterionIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	body, err := p.campaignLocationMutateBody(ctx, res, "")
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}

	raw, err := c.Mutate(ctx, campaignCriteriaCollection, []map[string]any{
		{"create": body},
	})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}
	id, err := parseCampaignCriterionMutateID(raw, c.CustomerID())
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}

	live, err := p.readCampaignLocationByID(ctx, res.Address, id, res.Attributes)
	if err == nil {
		return p.rememberLive(live), nil
	}
	fallback, ferr := p.remoteCampaignLocationFromDesired(ctx, res, id, c.CustomerID())
	if ferr != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s succeeded but refreshing location criterion %q failed: %w", res.Address, id, err)
	}
	return p.rememberLive(fallback), nil
}

func (p *Provider) updateCampaignLocation(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateCampaignLocation(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: missing remote identity", desired.Address)
	}
	if id, bound, err := boundCampaignCriterionIdentity(desired); err != nil {
		return resource.RemoteResource{}, err
	} else if bound && id != actual.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: persisted identity %q does not match planned remote identity %q", desired.Address, id, actual.Identity.ID)
	}

	live, err := p.readCampaignLocationByID(ctx, desired.Address, actual.Identity.ID, desired.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: refreshing current location criterion: %w", desired.Address, err)
	}
	if _, _, err := p.normalizeCampaignLocationComparable(ctx, desired, &live); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}
	return p.rememberLive(live), nil
}

func (p *Provider) importCampaignLocation(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	id, err := p.canonicalCampaignCriterionImportID(addr, rawID)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireCustomerID(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	live, err := p.readCampaignLocationByID(ctx, addr, id, nil)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: remote location criterion %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	if _, ok := live.Attributes[AttrCampaign]; !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: campaign is not bound in local state; import the googleads.campaign resource first (or apply it), then re-import this location criterion", addr)
	}
	return p.rememberLive(live), nil
}

func (p *Provider) normalizeCampaignLocationComparable(ctx context.Context, desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	if err := p.validateCampaignLocation(desired); err != nil {
		return nil, nil, err
	}
	want, err := p.comparableCampaignLocation(ctx, desired.Address, desired.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if live == nil {
		return want, nil, nil
	}
	if id, bound, err := boundCampaignCriterionIdentity(desired); err != nil {
		return nil, nil, err
	} else if bound {
		if live.Identity.IsZero() || live.Identity.ID != id {
			return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, id, live.Identity.ID)
		}
	}
	got, err := p.comparableCampaignLocationFromLive(ctx, desired.Address, live)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if err := rejectImmutableCampaignLocationChanges(want, got); err != nil {
		return nil, nil, err
	}
	return want, got, nil
}

func (p *Provider) readCampaignLocationByID(ctx context.Context, addr resource.Address, id string, desired resource.Attributes) (resource.RemoteResource, error) {
	campaignID, criterionID, err := parseCampaignCriterionID(addr, id)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	where := "campaign.id = " + campaignID + " AND campaign_criterion.criterion_id = " + criterionID
	matches, err := p.queryCampaignLocations(ctx, where)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, provider.ErrNotFound
	case 1:
		return p.remoteCampaignLocation(ctx, addr, matches[0], desired)
	default:
		return resource.RemoteResource{}, fmt.Errorf("multiple remote location criteria returned for id %s", id)
	}
}

func (p *Provider) queryCampaignLocations(ctx context.Context, where string) ([]campaignLocationData, error) {
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	query := campaignLocationSelect
	if strings.TrimSpace(where) != "" {
		query += " WHERE " + where
	}
	query += " ORDER BY campaign_criterion.criterion_id"
	rows, err := c.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]campaignLocationData, 0, len(rows))
	for _, row := range rows {
		item, err := decodeCampaignLocationRow(row, c.CustomerID())
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CampaignID != out[j].CampaignID {
			return out[i].CampaignID < out[j].CampaignID
		}
		return out[i].CriterionID < out[j].CriterionID
	})
	return out, nil
}

type campaignLocationData struct {
	ResourceName      string
	Campaign          string
	CampaignID        string
	CriterionID       string
	Negative          bool
	Type              string
	Status            string
	GeoTargetConstant string
}

type campaignLocationJSON struct {
	ResourceName string      `json:"resourceName"`
	CriterionID  json.Number `json:"criterionId"`
	Campaign     string      `json:"campaign"`
	Negative     *bool       `json:"negative"`
	Type         string      `json:"type"`
	Status       string      `json:"status"`
	Location     *struct {
		GeoTargetConstant string `json:"geoTargetConstant"`
	} `json:"location"`
}

func decodeCampaignLocationRow(raw json.RawMessage, configuredCustomerID string) (campaignLocationData, error) {
	malformed := func(detail string) (campaignLocationData, error) {
		if detail == "" {
			return campaignLocationData{}, fmt.Errorf("malformed location criterion result")
		}
		return campaignLocationData{}, fmt.Errorf("malformed location criterion result: %s", detail)
	}

	var envelope struct {
		CampaignCriterion json.RawMessage `json:"campaignCriterion"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.CampaignCriterion) == 0 {
		return malformed("")
	}
	var body campaignLocationJSON
	if err := json.Unmarshal(envelope.CampaignCriterion, &body); err != nil {
		return malformed("")
	}

	resourceName := strings.TrimSpace(body.ResourceName)
	resourceCustomerID, campaignID, criterionID, ok := splitCampaignCriterionResourceName(resourceName)
	if !ok {
		return malformed("invalid resourceName")
	}
	if configuredCustomerID != "" && resourceCustomerID != configuredCustomerID {
		return malformed("resourceName belongs to a different customer")
	}
	id := strings.TrimSpace(body.CriterionID.String())
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		return malformed("invalid criterionId")
	}
	if id != criterionID {
		return malformed("criterionId does not match resourceName")
	}

	campaignResourceName := strings.TrimSpace(body.Campaign)
	campaignCustomerID, parsedCampaignID, ok := splitCampaignResourceName(campaignResourceName)
	if !ok {
		return malformed("invalid campaign")
	}
	if configuredCustomerID != "" && campaignCustomerID != configuredCustomerID {
		return malformed("campaign belongs to a different customer")
	}
	if parsedCampaignID != campaignID {
		return malformed("campaign does not match resourceName")
	}

	criterionType := normalizeEnum(body.Type)
	if criterionType == "" {
		return malformed("missing type")
	}
	if criterionType != campaignCriterionTypeLocation {
		return campaignLocationData{}, fmt.Errorf("campaign criterion %s has type %s; googleads.campaign_location only manages LOCATION criteria", campaignCriterionID(campaignID, id), criterionType)
	}
	if body.Location == nil || strings.TrimSpace(body.Location.GeoTargetConstant) == "" {
		return malformed("missing location")
	}

	status := normalizeEnum(body.Status)
	if status == "REMOVED" {
		return campaignLocationData{}, fmt.Errorf("location criterion %s has status REMOVED; googleads.campaign_location does not manage removed location criteria", campaignCriterionID(campaignID, id))
	}

	negative := false
	if body.Negative != nil {
		negative = *body.Negative
	}

	return campaignLocationData{
		ResourceName:      resourceName,
		Campaign:          campaignResourceName,
		CampaignID:        campaignID,
		CriterionID:       id,
		Negative:          negative,
		Type:              criterionType,
		Status:            status,
		GeoTargetConstant: strings.TrimSpace(body.Location.GeoTargetConstant),
	}, nil
}

func (p *Provider) remoteCampaignLocation(ctx context.Context, addr resource.Address, item campaignLocationData, desired resource.Attributes) (resource.RemoteResource, error) {
	target, err := p.resolveGeoTarget(ctx, addr, item.GeoTargetConstant)
	if err != nil {
		target = geoTargetConstant{ResourceName: item.GeoTargetConstant}
		if _, id, ok := splitGeoTargetConstantResourceName(item.GeoTargetConstant); ok {
			target.ID = id
		}
	}

	location := target.displayName()
	if desired != nil {
		if raw, ok := desired[AttrLocation]; ok {
			if query, nerr := coerceString(raw); nerr == nil {
				if resolved, rerr := p.resolveGeoTarget(ctx, addr, query); rerr == nil && resolved.ResourceName == target.ResourceName {
					location = strings.Join(strings.Fields(strings.TrimSpace(query)), " ")
				}
			}
		}
	}

	attrs := resource.Attributes{
		AttrLocation: location,
		AttrNegative: item.Negative,
	}
	campaign, err := p.liveCampaignGoalAttr(addr, item.Campaign, desired[AttrCampaign])
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if campaign != nil {
		attrs[AttrCampaign] = campaign
	}

	id := campaignCriterionID(item.CampaignID, item.CriterionID)
	computed := resource.Attributes{}
	setComputed(computed, "id", id)
	setComputed(computed, "resourceName", item.ResourceName)
	setComputed(computed, "type", item.Type)
	setComputed(computed, "geoTargetConstant", item.GeoTargetConstant)
	setComputed(computed, "status", item.Status)

	return resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: id},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func (p *Provider) remoteCampaignLocationFromDesired(ctx context.Context, res resource.Resource, id, customerID string) (resource.RemoteResource, error) {
	attrs, err := p.comparableCampaignLocation(ctx, res.Address, res.Attributes)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	target, err := p.resolveGeoTarget(ctx, res.Address, res.Attributes[AttrLocation])
	if err != nil {
		return resource.RemoteResource{}, err
	}
	computed := resource.Attributes{
		"id":                id,
		"resourceName":      campaignCriterionResourceName(customerID, id),
		"type":              campaignCriterionTypeLocation,
		"geoTargetConstant": target.ResourceName,
	}
	return resource.RemoteResource{
		Address:    res.Address,
		Identity:   resource.Identity{ID: id},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func (p *Provider) comparableCampaignLocation(ctx context.Context, addr resource.Address, attrs resource.Attributes) (resource.Attributes, error) {
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	campaign, err := comparableCampaignAttr(attrs[AttrCampaign])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrCampaign, err)
	}
	raw, err := coerceString(attrs[AttrLocation])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrLocation, err)
	}
	target, err := p.resolveGeoTarget(ctx, addr, raw)
	if err != nil {
		return nil, err
	}
	negative := false
	if _, ok := attrs[AttrNegative]; ok {
		negative, err = coerceBool(attrs[AttrNegative])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrNegative, err)
		}
	}
	return resource.Attributes{
		AttrCampaign: campaign,
		AttrLocation: target.displayName(),
		AttrNegative: negative,
	}, nil
}

func (p *Provider) comparableCampaignLocationFromLive(ctx context.Context, addr resource.Address, live *resource.RemoteResource) (resource.Attributes, error) {
	if live == nil {
		return nil, nil
	}
	got, err := p.comparableCampaignLocation(ctx, addr, live.Attributes)
	if err != nil {
		return nil, err
	}
	if constant, err := coerceString(live.Computed["geoTargetConstant"]); err == nil && strings.TrimSpace(constant) != "" {
		if target, rerr := p.resolveGeoTarget(ctx, addr, constant); rerr == nil {
			got[AttrLocation] = target.displayName()
		}
	}
	return got, nil
}

func (p *Provider) campaignLocationMutateBody(ctx context.Context, res resource.Resource, resourceName string) (map[string]any, error) {
	comparable, err := p.comparableCampaignLocation(ctx, res.Address, res.Attributes)
	if err != nil {
		return nil, err
	}
	target, err := p.resolveGeoTarget(ctx, res.Address, res.Attributes[AttrLocation])
	if err != nil {
		return nil, err
	}
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	if resourceName != "" {
		body["resourceName"] = resourceName
		return body, nil
	}
	campaignName, err := p.campaignResourceNameFromRef(res.Attributes[AttrCampaign], c.CustomerID())
	if err != nil {
		return nil, err
	}
	body["campaign"] = campaignName
	body["location"] = map[string]any{"geoTargetConstant": target.ResourceName}
	if negative, ok := comparable[AttrNegative].(bool); ok {
		body["negative"] = negative
	}
	return body, nil
}

func rejectImmutableCampaignLocationChanges(want, got resource.Attributes) error {
	if !sameRef(want[AttrCampaign], got[AttrCampaign]) {
		return fmt.Errorf("campaign is immutable and cannot be changed from %s to %s; create a new googleads.campaign_location resource instead of mutating this criterion", logicalRef(got[AttrCampaign]).Address, logicalRef(want[AttrCampaign]).Address)
	}
	if !reflect.DeepEqual(want[AttrLocation], got[AttrLocation]) {
		return fmt.Errorf("location is immutable and cannot be changed from %q to %q; create a new googleads.campaign_location resource instead of mutating this criterion", got[AttrLocation], want[AttrLocation])
	}
	if !reflect.DeepEqual(want[AttrNegative], got[AttrNegative]) {
		return fmt.Errorf("negative is immutable and cannot be changed from %v to %v; create a new googleads.campaign_location resource instead of mutating this criterion", got[AttrNegative], want[AttrNegative])
	}
	return nil
}

func campaignLocationNaturalKey(res resource.Resource) (string, error) {
	ref, err := requiredCampaignRef(res)
	if err != nil {
		return "", err
	}
	location, err := requiredLocationValue(res)
	if err != nil {
		return "", err
	}
	return ref.Address.String() + "\x00" + strings.ToLower(location), nil
}
