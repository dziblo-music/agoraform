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
	// TypeAdGroup is the Google Ads Search ad-group type used in addresses
	// such as googleads.ad_group.brand.
	TypeAdGroup = "ad_group"

	// AttrAdGroupType is SEARCH_STANDARD for the supported Search workflow.
	AttrAdGroupType = "type"
	// AttrCpcBid is an optional max CPC bid in account-currency units.
	AttrCpcBid = "cpcBid"

	adGroupTypeSearchStandard = "SEARCH_STANDARD"
	adGroupStatusPaused       = "PAUSED"
	adGroupStatusEnabled      = "ENABLED"
	adGroupsCollection        = "adGroups"
)

var (
	supportedAdGroupAttrs = map[string]struct{}{
		AttrName:        {},
		AttrCampaign:    {},
		AttrStatus:      {},
		AttrAdGroupType: {},
		AttrCpcBid:      {},
	}

	computedAdGroupAttrs = map[string]struct{}{
		"id":                       {},
		"resourceName":             {},
		"resource_name":            {},
		"cpcBidMicros":             {},
		"cpc_bid_micros":           {},
		"effectiveCpcBidMicros":    {},
		"effective_cpc_bid_micros": {},
		"effectiveTargetCpaMicros": {},
		"effectiveTargetRoas":      {},
		"primaryStatus":            {},
		"primary_status":           {},
		"primaryStatusReasons":     {},
		"baseAdGroup":              {},
		"base_ad_group":            {},
		"adRotationMode":           {},
		"ad_rotation_mode":         {},
	}

	adGroupStatuses = map[string]struct{}{
		adGroupStatusPaused:  {},
		adGroupStatusEnabled: {},
	}

	adGroupSelect = strings.Join([]string{
		"SELECT",
		"ad_group.resource_name,",
		"ad_group.id,",
		"ad_group.name,",
		"ad_group.status,",
		"ad_group.type,",
		"ad_group.campaign,",
		"ad_group.cpc_bid_micros,",
		"ad_group.primary_status",
		"FROM ad_group",
	}, " ")
)

func (p *Provider) validateAdGroup(res resource.Resource) error {
	if err := p.requireCustomerID(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}

	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedAdGroupAttrs[key]; ok {
			continue
		}
		if _, computed := computedAdGroupAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; googleads.ad_group supports %s", res.Address, key, joinSorted(keys(supportedAdGroupAttrs)))
	}

	name, err := requiredString(res, AttrName)
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("resource %s: attribute %q must be a non-empty string", res.Address, AttrName)
	}

	if _, err := requiredCampaignRef(res); err != nil {
		return err
	}

	if _, _, err := optionalEnum(res, AttrStatus, adGroupStatuses); err != nil {
		return err
	}

	if groupType, set, err := optionalString(res, AttrAdGroupType); err != nil {
		return err
	} else if set {
		if normalizeEnum(groupType) != adGroupTypeSearchStandard {
			return fmt.Errorf("resource %s: attribute %q must be %s; googleads.ad_group only manages Search standard ad groups", res.Address, AttrAdGroupType, adGroupTypeSearchStandard)
		}
	}

	if _, _, err := optionalAdGroupCpcBidMicros(res); err != nil {
		return err
	}
	if _, _, err := boundAdGroupIdentity(res); err != nil {
		return err
	}
	return nil
}

func (p *Provider) readAdGroup(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateAdGroup(res); err != nil {
		return resource.RemoteResource{}, err
	}

	id, bound, err := boundAdGroupIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if bound {
		live, err := p.readAdGroupByID(ctx, res.Address, id, res.Attributes)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return p.rememberLive(live), nil
	}

	campaignID, ok := p.campaignIDFromRef(res.Attributes[AttrCampaign])
	if !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, provider.ErrNotFound)
	}
	name, _, _ := optionalString(res, AttrName)
	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
	}
	where := "ad_group.campaign = " + gaqlString(campaignResourceName(c.CustomerID(), campaignID)) +
		" AND ad_group.name = " + gaqlString(name)
	matches, err := p.queryAdGroups(ctx, where)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, provider.ErrNotFound)
	case 1:
		live, err := p.remoteAdGroup(res.Address, matches[0], res.Attributes)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return p.rememberLive(live), nil
	default:
		ids := make([]string, 0, len(matches))
		for _, item := range matches {
			ids = append(ids, item.ID)
		}
		sort.Strings(ids)
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: multiple remote ad groups named %q in campaign %s (ids %s); names must be unique within a campaign", res.Address, name, campaignID, strings.Join(ids, ", "))
	}
}

func (p *Provider) createAdGroup(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateAdGroup(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundAdGroupIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}

	body, _, err := p.adGroupMutateBody(res, "")
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}
	body["type"] = adGroupTypeSearchStandard
	if _, ok := body["status"]; !ok {
		body["status"] = adGroupStatusPaused
	}

	raw, err := c.Mutate(ctx, adGroupsCollection, []map[string]any{
		{"create": body},
	})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}
	id, err := parseAdGroupMutateID(raw, c.CustomerID())
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}

	live, err := p.readAdGroupByID(ctx, res.Address, id, res.Attributes)
	if err == nil {
		return p.rememberLive(live), nil
	}
	fallback, ferr := p.remoteAdGroupFromDesired(res, id, c.CustomerID())
	if ferr != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s succeeded but refreshing ad group %q failed: %w", res.Address, id, err)
	}
	return p.rememberLive(fallback), nil
}

func (p *Provider) updateAdGroup(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateAdGroup(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: missing remote identity", desired.Address)
	}
	if id, bound, err := boundAdGroupIdentity(desired); err != nil {
		return resource.RemoteResource{}, err
	} else if bound && id != actual.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: persisted identity %q does not match planned remote identity %q", desired.Address, id, actual.Identity.ID)
	}

	live, err := p.readAdGroupByID(ctx, desired.Address, actual.Identity.ID, desired.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: refreshing current ad group: %w", desired.Address, err)
	}

	want, got, err := p.normalizeAdGroupComparable(desired, &live)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if !sameRef(want[AttrCampaign], got[AttrCampaign]) {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: campaign is immutable and cannot be changed from %s to %s", desired.Address, logicalRef(got[AttrCampaign]).Address, logicalRef(want[AttrCampaign]).Address)
	}
	if reflect.DeepEqual(want, got) {
		return p.rememberLive(live), nil
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	resourceName := adGroupResourceName(c.CustomerID(), actual.Identity.ID)
	full, fullMask, err := p.adGroupMutateBody(desired, resourceName)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}

	changed := changedAdGroupAPIFields(want, got)
	body := map[string]any{"resourceName": resourceName}
	mask := make([]string, 0, len(changed))
	for _, field := range fullMask {
		if _, ok := changed[field]; !ok {
			continue
		}
		if value, ok := nestedMutateValue(full, field); ok {
			setNestedMutateValue(body, field, value)
		}
		mask = append(mask, field)
	}
	sort.Strings(mask)
	if len(mask) == 0 {
		return p.rememberLive(live), nil
	}

	_, err = c.Mutate(ctx, adGroupsCollection, []map[string]any{
		{
			"updateMask": strings.Join(mask, ","),
			"update":     body,
		},
	})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}

	refreshed, err := p.readAdGroupByID(ctx, desired.Address, actual.Identity.ID, desired.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s succeeded but refreshing ad group %q failed: %w", desired.Address, actual.Identity.ID, err)
	}
	return p.rememberLive(refreshed), nil
}

func (p *Provider) importAdGroup(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	id, err := p.canonicalAdGroupImportID(addr, rawID)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireCustomerID(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	live, err := p.readAdGroupByID(ctx, addr, id, nil)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: remote ad group %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	if _, ok := live.Attributes[AttrCampaign]; !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: campaign is not bound in local state; import the googleads.campaign resource first (or apply it), then re-import this ad group", addr)
	}
	return p.rememberLive(live), nil
}

func (p *Provider) normalizeAdGroupComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	if err := p.validateAdGroup(desired); err != nil {
		return nil, nil, err
	}
	want, err := comparableAdGroup(desired.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if live == nil {
		return want, nil, nil
	}
	if id, bound, err := boundAdGroupIdentity(desired); err != nil {
		return nil, nil, err
	} else if bound {
		if live.Identity.IsZero() || live.Identity.ID != id {
			return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, id, live.Identity.ID)
		}
	}
	gotFull, err := comparableAdGroup(live.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	got := resource.Attributes{}
	for key := range want {
		if v, ok := gotFull[key]; ok {
			got[key] = intersectComparableValue(want[key], v)
			continue
		}
		if v, ok := live.Attributes[key]; ok {
			got[key] = intersectComparableValue(want[key], v)
		}
	}
	return want, got, nil
}

func (p *Provider) readAdGroupByID(ctx context.Context, addr resource.Address, id string, desired resource.Attributes) (resource.RemoteResource, error) {
	matches, err := p.queryAdGroups(ctx, "ad_group.id = "+id)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, provider.ErrNotFound
	case 1:
		return p.remoteAdGroup(addr, matches[0], desired)
	default:
		return resource.RemoteResource{}, fmt.Errorf("multiple remote ad groups returned for id %s", id)
	}
}

func (p *Provider) queryAdGroups(ctx context.Context, where string) ([]adGroupData, error) {
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	query := adGroupSelect
	if strings.TrimSpace(where) != "" {
		query += " WHERE " + where
	}
	query += " ORDER BY ad_group.id"
	rows, err := c.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]adGroupData, 0, len(rows))
	for _, row := range rows {
		item, err := decodeAdGroupRow(row, c.CustomerID())
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

type adGroupData struct {
	ResourceName  string
	ID            string
	Name          string
	Status        string
	Type          string
	Campaign      string
	CampaignID    string
	CpcBidMicros  *int64
	PrimaryStatus string
}

type adGroupJSON struct {
	ResourceName  string          `json:"resourceName"`
	ID            json.Number     `json:"id"`
	Name          string          `json:"name"`
	Status        string          `json:"status"`
	Type          string          `json:"type"`
	Campaign      string          `json:"campaign"`
	CpcBidMicros  json.RawMessage `json:"cpcBidMicros"`
	PrimaryStatus string          `json:"primaryStatus"`
}

func decodeAdGroupRow(raw json.RawMessage, configuredCustomerID string) (adGroupData, error) {
	malformed := func(detail string) (adGroupData, error) {
		if detail == "" {
			return adGroupData{}, fmt.Errorf("malformed ad group result")
		}
		return adGroupData{}, fmt.Errorf("malformed ad group result: %s", detail)
	}

	var envelope struct {
		AdGroup json.RawMessage `json:"adGroup"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.AdGroup) == 0 {
		return malformed("")
	}
	var body adGroupJSON
	if err := json.Unmarshal(envelope.AdGroup, &body); err != nil {
		return malformed("")
	}

	resourceName := strings.TrimSpace(body.ResourceName)
	resourceCustomerID, resourceID, ok := splitAdGroupResourceName(resourceName)
	if !ok {
		return malformed("invalid resourceName")
	}
	if configuredCustomerID != "" && resourceCustomerID != configuredCustomerID {
		return malformed("resourceName belongs to a different customer")
	}
	id := strings.TrimSpace(body.ID.String())
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		return malformed("invalid id")
	}
	if id != resourceID {
		return malformed("id does not match resourceName")
	}
	if strings.TrimSpace(body.Name) == "" {
		return malformed("missing name")
	}

	campaignResourceName := strings.TrimSpace(body.Campaign)
	campaignCustomerID, campaignID, ok := splitCampaignResourceName(campaignResourceName)
	if !ok {
		return malformed("invalid campaign")
	}
	if configuredCustomerID != "" && campaignCustomerID != configuredCustomerID {
		return malformed("campaign belongs to a different customer")
	}

	groupType := normalizeEnum(body.Type)
	if groupType == "" {
		return malformed("missing type")
	}
	if groupType != adGroupTypeSearchStandard {
		return adGroupData{}, fmt.Errorf("ad group %s has type %s; googleads.ad_group only manages SEARCH_STANDARD Search ad groups", id, groupType)
	}

	status := normalizeEnum(body.Status)
	if status == "REMOVED" {
		return adGroupData{}, fmt.Errorf("ad group %s has status REMOVED; googleads.ad_group does not manage removed ad groups", id)
	}
	if status != "" {
		if _, ok := adGroupStatuses[status]; !ok {
			return adGroupData{}, fmt.Errorf("ad group %s has status %s; googleads.ad_group supports %s", id, status, joinSorted(keys(adGroupStatuses)))
		}
	}

	item := adGroupData{
		ResourceName:  resourceName,
		ID:            id,
		Name:          body.Name,
		Status:        status,
		Type:          groupType,
		Campaign:      campaignResourceName,
		CampaignID:    campaignID,
		PrimaryStatus: normalizeEnum(body.PrimaryStatus),
	}
	if n, err := parseOptionalMicros(body.CpcBidMicros); err == nil {
		item.CpcBidMicros = n
	}
	return item, nil
}

func (p *Provider) remoteAdGroup(addr resource.Address, item adGroupData, desired resource.Attributes) (resource.RemoteResource, error) {
	attrs := resource.Attributes{
		AttrName: item.Name,
	}
	if item.Status != "" {
		attrs[AttrStatus] = item.Status
	}
	if groupType := item.Type; groupType != "" {
		if desired != nil {
			if _, ok := desired[AttrAdGroupType]; ok {
				attrs[AttrAdGroupType] = groupType
			}
		} else {
			attrs[AttrAdGroupType] = groupType
		}
	}

	campaign, err := p.liveCampaignGoalAttr(addr, item.Campaign, desired[AttrCampaign])
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if campaign != nil {
		attrs[AttrCampaign] = campaign
	}

	if item.CpcBidMicros != nil && *item.CpcBidMicros > 0 {
		if desired != nil {
			if _, ok := desired[AttrCpcBid]; ok {
				attrs[AttrCpcBid] = amountFromMicros(*item.CpcBidMicros)
			}
		} else {
			attrs[AttrCpcBid] = amountFromMicros(*item.CpcBidMicros)
		}
	}

	computed := resource.Attributes{}
	setComputed(computed, "id", item.ID)
	setComputed(computed, "resourceName", item.ResourceName)
	if item.CpcBidMicros != nil {
		setComputed(computed, "cpcBidMicros", strconv.FormatInt(*item.CpcBidMicros, 10))
	}
	setComputed(computed, "primaryStatus", item.PrimaryStatus)

	return resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: item.ID},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func (p *Provider) remoteAdGroupFromDesired(res resource.Resource, id, customerID string) (resource.RemoteResource, error) {
	attrs, err := comparableAdGroup(res.Attributes)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	computed := resource.Attributes{
		"id":           id,
		"resourceName": adGroupResourceName(customerID, id),
		"type":         adGroupTypeSearchStandard,
	}
	return resource.RemoteResource{
		Address:    res.Address,
		Identity:   resource.Identity{ID: id},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func comparableAdGroup(attrs resource.Attributes) (resource.Attributes, error) {
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	name, err := coerceString(attrs[AttrName])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrName, err)
	}
	out := resource.Attributes{AttrName: name}

	campaign, err := comparableCampaignAttr(attrs[AttrCampaign])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrCampaign, err)
	}
	out[AttrCampaign] = campaign

	if _, ok := attrs[AttrAdGroupType]; ok {
		groupType, err := coerceString(attrs[AttrAdGroupType])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrAdGroupType, err)
		}
		out[AttrAdGroupType] = normalizeEnum(groupType)
	}

	status := adGroupStatusPaused
	if _, ok := attrs[AttrStatus]; ok {
		raw, err := coerceString(attrs[AttrStatus])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrStatus, err)
		}
		status = normalizeEnum(raw)
	}
	out[AttrStatus] = status

	if _, ok := attrs[AttrCpcBid]; ok {
		micros, err := adGroupCpcBidToMicros(attrs[AttrCpcBid])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrCpcBid, err)
		}
		out[AttrCpcBid] = amountFromMicros(micros)
	}
	return out, nil
}

func (p *Provider) adGroupMutateBody(res resource.Resource, resourceName string) (map[string]any, []string, error) {
	comparable, err := comparableAdGroup(res.Attributes)
	if err != nil {
		return nil, nil, err
	}
	c, err := p.Client()
	if err != nil {
		return nil, nil, err
	}

	body := map[string]any{}
	var mask []string
	if resourceName != "" {
		body["resourceName"] = resourceName
	}
	if name, ok := comparable[AttrName].(string); ok {
		body["name"] = name
		mask = append(mask, "name")
	}
	if status, ok := comparable[AttrStatus].(string); ok {
		body["status"] = status
		mask = append(mask, "status")
	}
	if resourceName == "" {
		campaignName, err := p.campaignResourceNameFromRef(res.Attributes[AttrCampaign], c.CustomerID())
		if err != nil {
			return nil, nil, err
		}
		body["campaign"] = campaignName
	}
	if _, ok := comparable[AttrCpcBid]; ok {
		micros, err := adGroupCpcBidToMicros(comparable[AttrCpcBid])
		if err != nil {
			return nil, nil, err
		}
		body["cpcBidMicros"] = strconv.FormatInt(micros, 10)
		mask = append(mask, "cpcBidMicros")
	}
	sort.Strings(mask)
	return body, mask, nil
}

func (p *Provider) campaignResourceNameFromRef(v any, customerID string) (string, error) {
	if resolved, ok := resource.AsResolved(v); ok {
		if name, err := coerceString(resolved.Outputs["resourceName"]); err == nil && strings.TrimSpace(name) != "" {
			return name, nil
		}
		if resolved.Identity.ID != "" {
			return campaignResourceName(customerID, resolved.Identity.ID), nil
		}
		return "", fmt.Errorf("campaign reference %s has no provider-native identity", resolved.Address)
	}
	ref, ok := resource.AsRef(v)
	if !ok {
		return "", fmt.Errorf("must be a resource reference ($ref) to a %s.%s resource", Name, TypeCampaign)
	}
	if id := p.lookupID(ref.Address); id != "" {
		return campaignResourceName(customerID, id), nil
	}
	return "", fmt.Errorf("campaign reference %s has no provider-native identity", ref.Address)
}

func optionalAdGroupCpcBidMicros(res resource.Resource) (int64, bool, error) {
	v, ok := res.Attributes[AttrCpcBid]
	if !ok {
		return 0, false, nil
	}
	micros, err := adGroupCpcBidToMicros(v)
	if err != nil {
		return 0, true, fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrCpcBid, err)
	}
	return micros, true, nil
}

func adGroupCpcBidToMicros(v any) (int64, error) {
	micros, err := amountToMicros(v)
	if err != nil {
		return 0, fmt.Errorf("must be a positive CPC bid in account currency units")
	}
	return micros, nil
}

func changedAdGroupAPIFields(want, got resource.Attributes) map[string]struct{} {
	changed := map[string]struct{}{}
	if !reflect.DeepEqual(want[AttrName], got[AttrName]) {
		changed["name"] = struct{}{}
	}
	if !reflect.DeepEqual(want[AttrStatus], got[AttrStatus]) {
		changed["status"] = struct{}{}
	}
	if !reflect.DeepEqual(want[AttrCpcBid], got[AttrCpcBid]) {
		changed["cpcBidMicros"] = struct{}{}
	}
	return changed
}

func boundAdGroupIdentity(res resource.Resource) (string, bool, error) {
	if res.Identity.IsZero() {
		return "", false, nil
	}
	id, err := parseAdGroupIdentity(res.Address, res.Identity.ID)
	if err != nil {
		return "", true, err
	}
	return id, true, nil
}

func parseAdGroupIdentity(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("resource %s: persisted identity is empty; a Google Ads ad group id is required", addr)
	}
	if err := adGroupIdentityIDError(addr, raw); err != nil {
		return "", err
	}
	return raw, nil
}

func (p *Provider) canonicalAdGroupImportID(addr resource.Address, raw string) (string, error) {
	id, err := parseImportAdGroupID(addr, raw)
	if err != nil {
		return "", err
	}
	if customerID, restID, ok := splitAdGroupResourceName(id); ok {
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

func parseImportAdGroupID(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("googleads: import %s: remote identifier is empty; expected a numeric ad group id or resource name customers/{customerId}/adGroups/{id}", addr)
	}
	if _, id, ok := splitAdGroupResourceName(raw); ok {
		if err := importAdGroupIDError(addr, id); err != nil {
			return "", err
		}
		return raw, nil
	}
	if err := importAdGroupIDError(addr, raw); err != nil {
		return "", err
	}
	return raw, nil
}

func importAdGroupIDError(addr resource.Address, id string) error {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("googleads: import %s: %q is not a valid Google Ads ad group id; expected a positive numeric id or resource name customers/{customerId}/adGroups/{id}", addr, id)
	}
	return nil
}

func adGroupIdentityIDError(addr resource.Address, id string) error {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("resource %s: persisted identity %q is not a valid Google Ads ad group id", addr, id)
	}
	return nil
}

func splitAdGroupResourceName(name string) (customerID, adGroupID string, ok bool) {
	name = strings.TrimSpace(name)
	parts := strings.Split(name, "/")
	if len(parts) != 4 || parts[0] != "customers" || parts[2] != adGroupsCollection {
		return "", "", false
	}
	if _, err := NormalizeCustomerID(parts[1]); err != nil {
		return "", "", false
	}
	if n, err := strconv.ParseInt(parts[3], 10, 64); err != nil || n <= 0 {
		return "", "", false
	}
	return parts[1], parts[3], true
}

func adGroupResourceName(customerID, id string) string {
	return "customers/" + customerID + "/" + adGroupsCollection + "/" + id
}

func parseAdGroupMutateID(raw json.RawMessage, customerID string) (string, error) {
	var resp struct {
		Results []struct {
			ResourceName string `json:"resourceName"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || len(resp.Results) == 0 {
		return "", fmt.Errorf("malformed mutate response")
	}
	resourceName := strings.TrimSpace(resp.Results[0].ResourceName)
	gotCustomer, id, ok := splitAdGroupResourceName(resourceName)
	if !ok {
		return "", fmt.Errorf("malformed mutate response")
	}
	if customerID != "" && gotCustomer != customerID {
		return "", fmt.Errorf("mutate returned ad group %s for a different customer", id)
	}
	return id, nil
}
