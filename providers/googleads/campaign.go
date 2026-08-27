package googleads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

const (
	// TypeCampaign is the Google Ads Search campaign type used in addresses
	// such as googleads.campaign.brand.
	TypeCampaign = "campaign"

	// AttrBudget is a $ref to a googleads.campaign_budget.
	AttrBudget = "budget"
	// AttrBidding is the campaign bidding strategy and optional settings.
	AttrBidding = "bidding"
	// AttrNetwork is Search network / partner targeting.
	AttrNetwork = "network"
	// AttrStartDate is an optional campaign start date (YYYY-MM-DD).
	AttrStartDate = "startDate"
	// AttrEndDate is an optional campaign end date (YYYY-MM-DD).
	AttrEndDate = "endDate"
	// AttrTrackingUrlTemplate is an optional tracking URL template.
	AttrTrackingUrlTemplate = "trackingUrlTemplate"
	// AttrFinalUrlSuffix is an optional final URL suffix.
	AttrFinalUrlSuffix = "finalUrlSuffix"
	// AttrAdvertisingChannelType may be set to SEARCH; other channel types
	// are rejected before mutation.
	AttrAdvertisingChannelType = "advertisingChannelType"

	campaignChannelSearch          = "SEARCH"
	campaignStatusPaused           = "PAUSED"
	campaignStatusEnabled          = "ENABLED"
	campaignsCollection            = "campaigns"
	euPoliticalAdvertisingNone     = "DOES_NOT_CONTAIN_EU_POLITICAL_ADVERTISING"
	biddingStrategyManualCPC       = "MANUAL_CPC"
	biddingStrategyMaximizeClicks  = "MAXIMIZE_CLICKS"
	biddingStrategyMaximizeConv    = "MAXIMIZE_CONVERSIONS"
	biddingStrategyTargetCPA       = "TARGET_CPA"
	biddingStrategyTargetROAS      = "TARGET_ROAS"
	biddingStrategyMaximizeConvVal = "MAXIMIZE_CONVERSION_VALUE"
	apiBiddingTargetSpend          = "TARGET_SPEND"

	biddingKeyStrategy      = "strategy"
	biddingKeyEnhancedCPC   = "enhancedCpc"
	biddingKeyTargetCPA     = "targetCpa"
	biddingKeyTargetROAS    = "targetRoas"
	biddingKeyCPCBidCeiling = "cpcBidCeiling"

	networkKeyGoogleSearch         = "googleSearch"
	networkKeySearchNetwork        = "searchNetwork"
	networkKeyContentNetwork       = "contentNetwork"
	networkKeyPartnerSearchNetwork = "partnerSearchNetwork"
)

var (
	supportedCampaignAttrs = map[string]struct{}{
		AttrName:                   {},
		AttrStatus:                 {},
		AttrBudget:                 {},
		AttrBidding:                {},
		AttrNetwork:                {},
		AttrStartDate:              {},
		AttrEndDate:                {},
		AttrTrackingUrlTemplate:    {},
		AttrFinalUrlSuffix:         {},
		AttrAdvertisingChannelType: {},
	}

	computedCampaignAttrs = map[string]struct{}{
		"id":                             {},
		"resourceName":                   {},
		"resource_name":                  {},
		"servingStatus":                  {},
		"serving_status":                 {},
		"advertisingChannelSubType":      {},
		"advertising_channel_sub_type":   {},
		"biddingStrategyType":            {},
		"bidding_strategy_type":          {},
		"biddingStrategy":                {},
		"bidding_strategy":               {},
		"campaignBudget":                 {},
		"campaign_budget":                {},
		"containsEuPoliticalAdvertising": {},
		"optimizationScore":              {},
		"paymentMode":                    {},
		"startDateTime":                  {},
		"endDateTime":                    {},
	}

	campaignStatuses = map[string]struct{}{
		campaignStatusPaused:  {},
		campaignStatusEnabled: {},
	}

	campaignBiddingStrategies = map[string]struct{}{
		biddingStrategyManualCPC:       {},
		biddingStrategyMaximizeClicks:  {},
		biddingStrategyMaximizeConv:    {},
		biddingStrategyTargetCPA:       {},
		biddingStrategyTargetROAS:      {},
		biddingStrategyMaximizeConvVal: {},
	}

	datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

	campaignSelect = strings.Join([]string{
		"SELECT",
		"campaign.resource_name,",
		"campaign.id,",
		"campaign.name,",
		"campaign.status,",
		"campaign.advertising_channel_type,",
		"campaign.advertising_channel_sub_type,",
		"campaign.campaign_budget,",
		"campaign.bidding_strategy_type,",
		"campaign.bidding_strategy,",
		"campaign.manual_cpc.enhanced_cpc_enabled,",
		"campaign.maximize_conversions.target_cpa_micros,",
		"campaign.maximize_conversion_value.target_roas,",
		"campaign.target_cpa.target_cpa_micros,",
		"campaign.target_roas.target_roas,",
		"campaign.target_spend.cpc_bid_ceiling_micros,",
		"campaign.network_settings.target_google_search,",
		"campaign.network_settings.target_search_network,",
		"campaign.network_settings.target_content_network,",
		"campaign.network_settings.target_partner_search_network,",
		"campaign.start_date_time,",
		"campaign.end_date_time,",
		"campaign.tracking_url_template,",
		"campaign.final_url_suffix,",
		"campaign.contains_eu_political_advertising,",
		"campaign.serving_status",
		"FROM campaign",
	}, " ")
)

func (p *Provider) validateCampaign(res resource.Resource) error {
	if err := p.requireCustomerID(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}

	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedCampaignAttrs[key]; ok {
			continue
		}
		if _, computed := computedCampaignAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; googleads.campaign supports %s", res.Address, key, joinSorted(keys(supportedCampaignAttrs)))
	}

	name, err := requiredString(res, AttrName)
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("resource %s: attribute %q must be a non-empty string", res.Address, AttrName)
	}

	if channel, set, err := optionalString(res, AttrAdvertisingChannelType); err != nil {
		return err
	} else if set {
		if normalizeEnum(channel) != campaignChannelSearch {
			return fmt.Errorf("resource %s: attribute %q must be %s; googleads.campaign only manages Search campaigns", res.Address, AttrAdvertisingChannelType, campaignChannelSearch)
		}
	}

	if _, _, err := optionalEnum(res, AttrStatus, campaignStatuses); err != nil {
		return err
	}

	if _, err := requiredCampaignBudgetRef(res); err != nil {
		return err
	}
	if _, err := requiredCampaignBidding(res); err != nil {
		return err
	}
	if _, _, err := optionalCampaignNetwork(res); err != nil {
		return err
	}

	start, startSet, err := optionalCampaignDate(res, AttrStartDate)
	if err != nil {
		return err
	}
	end, endSet, err := optionalCampaignDate(res, AttrEndDate)
	if err != nil {
		return err
	}
	if startSet && endSet && start > end {
		return fmt.Errorf("resource %s: attribute %q must be on or after %s", res.Address, AttrEndDate, AttrStartDate)
	}

	if s, set, err := optionalString(res, AttrTrackingUrlTemplate); err != nil {
		return err
	} else if set && strings.TrimSpace(s) == "" {
		return fmt.Errorf("resource %s: attribute %q must be a non-empty string", res.Address, AttrTrackingUrlTemplate)
	}
	if s, set, err := optionalString(res, AttrFinalUrlSuffix); err != nil {
		return err
	} else if set && strings.TrimSpace(s) == "" {
		return fmt.Errorf("resource %s: attribute %q must be a non-empty string", res.Address, AttrFinalUrlSuffix)
	}

	if _, _, err := boundCampaignIdentity(res); err != nil {
		return err
	}
	return nil
}

func (p *Provider) readCampaign(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateCampaign(res); err != nil {
		return resource.RemoteResource{}, err
	}

	id, bound, err := boundCampaignIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if bound {
		live, err := p.readCampaignByID(ctx, res.Address, id, res.Attributes)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return p.rememberLive(live), nil
	}

	name, _, _ := optionalString(res, AttrName)
	matches, err := p.queryCampaigns(ctx, "campaign.name = "+gaqlString(name))
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, provider.ErrNotFound)
	case 1:
		live, err := p.remoteCampaign(res.Address, matches[0], res.Attributes)
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
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: multiple remote campaigns named %q (ids %s); names must be unique", res.Address, name, strings.Join(ids, ", "))
	}
}

func (p *Provider) createCampaign(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateCampaign(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundCampaignIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}

	body, _, err := p.campaignMutateBody(res, "")
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}
	body["advertisingChannelType"] = campaignChannelSearch
	body["containsEuPoliticalAdvertising"] = euPoliticalAdvertisingNone
	if _, ok := body["status"]; !ok {
		body["status"] = campaignStatusPaused
	}
	if _, ok := body["networkSettings"]; !ok {
		body["networkSettings"] = defaultNetworkSettings()
	}

	raw, err := c.Mutate(ctx, campaignsCollection, []map[string]any{
		{"create": body},
	})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}
	id, err := parseCampaignMutateID(raw, c.CustomerID())
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}

	live, err := p.readCampaignByID(ctx, res.Address, id, res.Attributes)
	if err == nil {
		return p.rememberLive(live), nil
	}
	fallback, ferr := p.remoteCampaignFromDesired(res, id, c.CustomerID())
	if ferr != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s succeeded but refreshing campaign %q failed: %w", res.Address, id, err)
	}
	return p.rememberLive(fallback), nil
}

func (p *Provider) updateCampaign(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateCampaign(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: missing remote identity", desired.Address)
	}
	if id, bound, err := boundCampaignIdentity(desired); err != nil {
		return resource.RemoteResource{}, err
	} else if bound && id != actual.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: persisted identity %q does not match planned remote identity %q", desired.Address, id, actual.Identity.ID)
	}

	live, err := p.readCampaignByID(ctx, desired.Address, actual.Identity.ID, desired.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: refreshing current campaign: %w", desired.Address, err)
	}

	want, got, err := p.normalizeCampaignComparable(desired, &live)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if reflect.DeepEqual(want, got) {
		return p.rememberLive(live), nil
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	resourceName := campaignResourceName(c.CustomerID(), actual.Identity.ID)
	full, fullMask, err := p.campaignMutateBody(desired, resourceName)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}

	changed := changedCampaignAPIFields(want, got)
	body := map[string]any{"resourceName": resourceName}
	mask := make([]string, 0, len(changed))
	for _, field := range fullMask {
		if _, ok := changed[field]; !ok {
			continue
		}
		if value, ok := nestedMutateValue(full, field); ok {
			setNestedMutateValue(body, field, value)
			mask = append(mask, field)
		}
	}
	sort.Strings(mask)
	if len(mask) == 0 {
		return p.rememberLive(live), nil
	}

	_, err = c.Mutate(ctx, campaignsCollection, []map[string]any{
		{
			"updateMask": strings.Join(mask, ","),
			"update":     body,
		},
	})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}

	refreshed, err := p.readCampaignByID(ctx, desired.Address, actual.Identity.ID, desired.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s succeeded but refreshing campaign %q failed: %w", desired.Address, actual.Identity.ID, err)
	}
	return p.rememberLive(refreshed), nil
}

func (p *Provider) importCampaign(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	id, err := p.canonicalCampaignImportID(addr, rawID)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireCustomerID(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	live, err := p.readCampaignByID(ctx, addr, id, nil)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: remote campaign %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	if _, ok := live.Attributes[AttrBudget]; !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: campaign budget is not bound in local state; import the googleads.campaign_budget resource first (or apply it), then re-import this campaign", addr)
	}
	return p.rememberLive(live), nil
}

func (p *Provider) normalizeCampaignComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	if err := p.validateCampaign(desired); err != nil {
		return nil, nil, err
	}
	want, err := p.comparableCampaign(desired.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if live == nil {
		return want, nil, nil
	}
	if id, bound, err := boundCampaignIdentity(desired); err != nil {
		return nil, nil, err
	} else if bound {
		if live.Identity.IsZero() || live.Identity.ID != id {
			return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, id, live.Identity.ID)
		}
	}
	gotFull, err := p.comparableCampaign(live.Attributes)
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

func (p *Provider) readCampaignByID(ctx context.Context, addr resource.Address, id string, desired resource.Attributes) (resource.RemoteResource, error) {
	matches, err := p.queryCampaigns(ctx, "campaign.id = "+id)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, provider.ErrNotFound
	case 1:
		return p.remoteCampaign(addr, matches[0], desired)
	default:
		return resource.RemoteResource{}, fmt.Errorf("multiple remote campaigns returned for id %s", id)
	}
}

func (p *Provider) queryCampaigns(ctx context.Context, where string) ([]campaignData, error) {
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	query := campaignSelect
	if strings.TrimSpace(where) != "" {
		query += " WHERE " + where
	}
	rows, err := c.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]campaignData, 0, len(rows))
	for _, row := range rows {
		item, err := decodeCampaignRow(row, c.CustomerID())
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

type campaignData struct {
	ResourceName                   string
	ID                             string
	Name                           string
	Status                         string
	AdvertisingChannelType         string
	AdvertisingChannelSubType      string
	CampaignBudget                 string
	BiddingStrategyType            string
	BiddingStrategy                string
	EnhancedCPCEnabled             *bool
	MaximizeConversionsTargetCPA   *int64
	MaximizeConversionValueROAS    *float64
	TargetCPAMicros                *int64
	TargetROAS                     *float64
	CPCBidCeilingMicros            *int64
	TargetGoogleSearch             *bool
	TargetSearchNetwork            *bool
	TargetContentNetwork           *bool
	TargetPartnerSearchNetwork     *bool
	StartDateTime                  string
	EndDateTime                    string
	TrackingURLTemplate            string
	FinalURLSuffix                 string
	ContainsEuPoliticalAdvertising string
	ServingStatus                  string
}

type campaignJSON struct {
	ResourceName                   string      `json:"resourceName"`
	ID                             json.Number `json:"id"`
	Name                           string      `json:"name"`
	Status                         string      `json:"status"`
	AdvertisingChannelType         string      `json:"advertisingChannelType"`
	AdvertisingChannelSubType      string      `json:"advertisingChannelSubType"`
	CampaignBudget                 string      `json:"campaignBudget"`
	BiddingStrategyType            string      `json:"biddingStrategyType"`
	BiddingStrategy                string      `json:"biddingStrategy"`
	TrackingURLTemplate            string      `json:"trackingUrlTemplate"`
	FinalURLSuffix                 string      `json:"finalUrlSuffix"`
	StartDateTime                  string      `json:"startDateTime"`
	EndDateTime                    string      `json:"endDateTime"`
	ContainsEuPoliticalAdvertising string      `json:"containsEuPoliticalAdvertising"`
	ServingStatus                  string      `json:"servingStatus"`
	ManualCpc                      *struct {
		EnhancedCpcEnabled *bool `json:"enhancedCpcEnabled"`
	} `json:"manualCpc"`
	MaximizeConversions *struct {
		TargetCpaMicros json.RawMessage `json:"targetCpaMicros"`
	} `json:"maximizeConversions"`
	MaximizeConversionValue *struct {
		TargetRoas *float64 `json:"targetRoas"`
	} `json:"maximizeConversionValue"`
	TargetCpa *struct {
		TargetCpaMicros json.RawMessage `json:"targetCpaMicros"`
	} `json:"targetCpa"`
	TargetRoas *struct {
		TargetRoas *float64 `json:"targetRoas"`
	} `json:"targetRoas"`
	TargetSpend *struct {
		CpcBidCeilingMicros json.RawMessage `json:"cpcBidCeilingMicros"`
	} `json:"targetSpend"`
	NetworkSettings *struct {
		TargetGoogleSearch         *bool `json:"targetGoogleSearch"`
		TargetSearchNetwork        *bool `json:"targetSearchNetwork"`
		TargetContentNetwork       *bool `json:"targetContentNetwork"`
		TargetPartnerSearchNetwork *bool `json:"targetPartnerSearchNetwork"`
	} `json:"networkSettings"`
}

func decodeCampaignRow(raw json.RawMessage, configuredCustomerID string) (campaignData, error) {
	malformed := func(detail string) (campaignData, error) {
		if detail == "" {
			return campaignData{}, fmt.Errorf("malformed campaign result")
		}
		return campaignData{}, fmt.Errorf("malformed campaign result: %s", detail)
	}

	var envelope struct {
		Campaign json.RawMessage `json:"campaign"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.Campaign) == 0 {
		return malformed("")
	}
	var body campaignJSON
	if err := json.Unmarshal(envelope.Campaign, &body); err != nil {
		return malformed("")
	}

	resourceName := strings.TrimSpace(body.ResourceName)
	resourceCustomerID, resourceID, ok := splitCampaignResourceName(resourceName)
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

	channel := normalizeEnum(body.AdvertisingChannelType)
	if channel == "" {
		return malformed("missing advertisingChannelType")
	}
	if channel != campaignChannelSearch {
		return campaignData{}, fmt.Errorf("campaign %s has advertising channel type %s; googleads.campaign only manages SEARCH campaigns", id, channel)
	}
	subType := normalizeEnum(body.AdvertisingChannelSubType)
	if subType != "" && subType != "UNSPECIFIED" && subType != "UNKNOWN" {
		return campaignData{}, fmt.Errorf("campaign %s has advertising channel subtype %s; googleads.campaign only manages standard SEARCH campaigns", id, subType)
	}

	status := normalizeEnum(body.Status)
	if status == "REMOVED" {
		return campaignData{}, fmt.Errorf("campaign %s has status REMOVED; googleads.campaign does not manage removed campaigns", id)
	}
	if status != "" {
		if _, ok := campaignStatuses[status]; !ok {
			return campaignData{}, fmt.Errorf("campaign %s has status %s; googleads.campaign supports %s", id, status, joinSorted(keys(campaignStatuses)))
		}
	}

	eu := normalizeEnum(body.ContainsEuPoliticalAdvertising)
	if eu != "" && eu != euPoliticalAdvertisingNone && eu != "UNSPECIFIED" && eu != "UNKNOWN" {
		return campaignData{}, fmt.Errorf("campaign %s contains EU political advertising; googleads.campaign does not manage political advertising campaigns", id)
	}

	item := campaignData{
		ResourceName:                   resourceName,
		ID:                             id,
		Name:                           body.Name,
		Status:                         status,
		AdvertisingChannelType:         channel,
		AdvertisingChannelSubType:      subType,
		CampaignBudget:                 strings.TrimSpace(body.CampaignBudget),
		BiddingStrategyType:            normalizeEnum(body.BiddingStrategyType),
		BiddingStrategy:                strings.TrimSpace(body.BiddingStrategy),
		StartDateTime:                  strings.TrimSpace(body.StartDateTime),
		EndDateTime:                    strings.TrimSpace(body.EndDateTime),
		TrackingURLTemplate:            strings.TrimSpace(body.TrackingURLTemplate),
		FinalURLSuffix:                 strings.TrimSpace(body.FinalURLSuffix),
		ContainsEuPoliticalAdvertising: eu,
		ServingStatus:                  normalizeEnum(body.ServingStatus),
	}
	if item.BiddingStrategy != "" {
		return campaignData{}, fmt.Errorf("campaign %s uses a portfolio bidding strategy; googleads.campaign only manages standard Search campaign bidding settings", id)
	}
	if comparableBiddingStrategy(item.BiddingStrategyType) == "" && item.BiddingStrategyType != "" && item.BiddingStrategyType != "UNSPECIFIED" && item.BiddingStrategyType != "UNKNOWN" {
		return campaignData{}, fmt.Errorf("campaign %s has bidding strategy %s; googleads.campaign supports %s", id, item.BiddingStrategyType, joinSorted(keys(campaignBiddingStrategies)))
	}
	if body.ManualCpc != nil {
		item.EnhancedCPCEnabled = body.ManualCpc.EnhancedCpcEnabled
	}
	if body.MaximizeConversions != nil {
		if n, err := parseOptionalMicros(body.MaximizeConversions.TargetCpaMicros); err == nil {
			item.MaximizeConversionsTargetCPA = n
		}
	}
	if body.MaximizeConversionValue != nil {
		item.MaximizeConversionValueROAS = body.MaximizeConversionValue.TargetRoas
	}
	if body.TargetCpa != nil {
		if n, err := parseOptionalMicros(body.TargetCpa.TargetCpaMicros); err == nil {
			item.TargetCPAMicros = n
		}
	}
	if body.TargetRoas != nil {
		item.TargetROAS = body.TargetRoas.TargetRoas
	}
	if body.TargetSpend != nil {
		if n, err := parseOptionalMicros(body.TargetSpend.CpcBidCeilingMicros); err == nil {
			item.CPCBidCeilingMicros = n
		}
	}
	if body.NetworkSettings != nil {
		item.TargetGoogleSearch = body.NetworkSettings.TargetGoogleSearch
		item.TargetSearchNetwork = body.NetworkSettings.TargetSearchNetwork
		item.TargetContentNetwork = body.NetworkSettings.TargetContentNetwork
		item.TargetPartnerSearchNetwork = body.NetworkSettings.TargetPartnerSearchNetwork
	}
	return item, nil
}

func (p *Provider) remoteCampaign(addr resource.Address, item campaignData, desired resource.Attributes) (resource.RemoteResource, error) {
	attrs := resource.Attributes{
		AttrName: item.Name,
	}
	if item.Status != "" {
		attrs[AttrStatus] = item.Status
	}
	if channel := item.AdvertisingChannelType; channel != "" {
		if desired != nil {
			if _, ok := desired[AttrAdvertisingChannelType]; ok {
				attrs[AttrAdvertisingChannelType] = channel
			}
		} else {
			attrs[AttrAdvertisingChannelType] = channel
		}
	}

	budget, err := p.liveBudgetAttr(addr, item.CampaignBudget, desired[AttrBudget])
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if budget != nil {
		attrs[AttrBudget] = budget
	}

	if bidding := liveBiddingAttr(item, desired[AttrBidding]); bidding != nil {
		attrs[AttrBidding] = bidding
	}
	if network := liveNetworkAttr(item, desired[AttrNetwork]); network != nil {
		attrs[AttrNetwork] = network
	}
	if date := campaignDateFromDateTime(item.StartDateTime); date != "" {
		attrs[AttrStartDate] = date
	}
	if date := campaignDateFromDateTime(item.EndDateTime); date != "" {
		attrs[AttrEndDate] = date
	}
	if item.TrackingURLTemplate != "" {
		attrs[AttrTrackingUrlTemplate] = item.TrackingURLTemplate
	}
	if item.FinalURLSuffix != "" {
		attrs[AttrFinalUrlSuffix] = item.FinalURLSuffix
	}

	computed := resource.Attributes{}
	setComputed(computed, "id", item.ID)
	setComputed(computed, "resourceName", item.ResourceName)
	setComputed(computed, "advertisingChannelType", item.AdvertisingChannelType)
	setComputed(computed, "advertisingChannelSubType", item.AdvertisingChannelSubType)
	setComputed(computed, "campaignBudget", item.CampaignBudget)
	setComputed(computed, "biddingStrategyType", item.BiddingStrategyType)
	setComputed(computed, "servingStatus", item.ServingStatus)
	setComputed(computed, "containsEuPoliticalAdvertising", item.ContainsEuPoliticalAdvertising)
	setComputed(computed, "startDateTime", item.StartDateTime)
	setComputed(computed, "endDateTime", item.EndDateTime)

	return resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: item.ID},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func (p *Provider) remoteCampaignFromDesired(res resource.Resource, id, customerID string) (resource.RemoteResource, error) {
	attrs, err := p.comparableCampaign(res.Attributes)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	computed := resource.Attributes{
		"id":                     id,
		"resourceName":           campaignResourceName(customerID, id),
		"advertisingChannelType": campaignChannelSearch,
	}
	return resource.RemoteResource{
		Address:    res.Address,
		Identity:   resource.Identity{ID: id},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func (p *Provider) comparableCampaign(attrs resource.Attributes) (resource.Attributes, error) {
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	name, err := coerceString(attrs[AttrName])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrName, err)
	}
	out := resource.Attributes{AttrName: name}

	if _, ok := attrs[AttrAdvertisingChannelType]; ok {
		channel, err := coerceString(attrs[AttrAdvertisingChannelType])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrAdvertisingChannelType, err)
		}
		out[AttrAdvertisingChannelType] = normalizeEnum(channel)
	}

	status := campaignStatusPaused
	if _, ok := attrs[AttrStatus]; ok {
		raw, err := coerceString(attrs[AttrStatus])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrStatus, err)
		}
		status = normalizeEnum(raw)
	}
	out[AttrStatus] = status

	budget, err := comparableBudgetAttr(attrs[AttrBudget])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrBudget, err)
	}
	out[AttrBudget] = budget

	bidding, err := comparableBiddingAttr(attrs[AttrBidding])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrBidding, err)
	}
	out[AttrBidding] = bidding

	if _, ok := attrs[AttrNetwork]; ok {
		network, err := comparableNetworkAttr(attrs[AttrNetwork], true)
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrNetwork, err)
		}
		out[AttrNetwork] = network
	}
	if _, ok := attrs[AttrStartDate]; ok {
		date, err := coerceCampaignDate(attrs[AttrStartDate])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrStartDate, err)
		}
		out[AttrStartDate] = date
	}
	if _, ok := attrs[AttrEndDate]; ok {
		date, err := coerceCampaignDate(attrs[AttrEndDate])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrEndDate, err)
		}
		out[AttrEndDate] = date
	}
	if _, ok := attrs[AttrTrackingUrlTemplate]; ok {
		s, err := coerceString(attrs[AttrTrackingUrlTemplate])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrTrackingUrlTemplate, err)
		}
		out[AttrTrackingUrlTemplate] = s
	}
	if _, ok := attrs[AttrFinalUrlSuffix]; ok {
		s, err := coerceString(attrs[AttrFinalUrlSuffix])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrFinalUrlSuffix, err)
		}
		out[AttrFinalUrlSuffix] = s
	}
	return out, nil
}

func (p *Provider) campaignMutateBody(res resource.Resource, resourceName string) (map[string]any, []string, error) {
	comparable, err := p.comparableCampaign(res.Attributes)
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
	if _, ok := comparable[AttrBudget]; ok {
		budgetName, err := p.campaignBudgetResourceName(res.Attributes[AttrBudget], c.CustomerID())
		if err != nil {
			return nil, nil, err
		}
		body["campaignBudget"] = budgetName
		mask = append(mask, "campaignBudget")
	}
	if bidding, ok := comparable[AttrBidding].(map[string]any); ok {
		applyBiddingMutate(body, &mask, bidding)
	}
	if network, ok := comparable[AttrNetwork].(map[string]any); ok {
		settings := map[string]any{}
		if v, ok := network[networkKeyGoogleSearch]; ok {
			settings["targetGoogleSearch"] = v
			mask = append(mask, "networkSettings.targetGoogleSearch")
		}
		if v, ok := network[networkKeySearchNetwork]; ok {
			settings["targetSearchNetwork"] = v
			mask = append(mask, "networkSettings.targetSearchNetwork")
		}
		if v, ok := network[networkKeyContentNetwork]; ok {
			settings["targetContentNetwork"] = v
			mask = append(mask, "networkSettings.targetContentNetwork")
		}
		if v, ok := network[networkKeyPartnerSearchNetwork]; ok {
			settings["targetPartnerSearchNetwork"] = v
			mask = append(mask, "networkSettings.targetPartnerSearchNetwork")
		}
		if len(settings) > 0 {
			body["networkSettings"] = settings
		}
	}
	if date, ok := comparable[AttrStartDate].(string); ok {
		body["startDateTime"] = date + " 00:00:00"
		mask = append(mask, "startDateTime")
	}
	if date, ok := comparable[AttrEndDate].(string); ok {
		body["endDateTime"] = date + " 23:59:59"
		mask = append(mask, "endDateTime")
	}
	if s, ok := comparable[AttrTrackingUrlTemplate].(string); ok {
		body["trackingUrlTemplate"] = s
		mask = append(mask, "trackingUrlTemplate")
	}
	if s, ok := comparable[AttrFinalUrlSuffix].(string); ok {
		body["finalUrlSuffix"] = s
		mask = append(mask, "finalUrlSuffix")
	}
	sort.Strings(mask)
	return body, mask, nil
}

func applyBiddingMutate(body map[string]any, mask *[]string, bidding map[string]any) {
	strategy, _ := bidding[biddingKeyStrategy].(string)
	switch strategy {
	case biddingStrategyManualCPC:
		manual := map[string]any{}
		if v, ok := bidding[biddingKeyEnhancedCPC]; ok {
			manual["enhancedCpcEnabled"] = v
			*mask = append(*mask, "manualCpc.enhancedCpcEnabled")
		} else {
			manual["enhancedCpcEnabled"] = false
			*mask = append(*mask, "manualCpc")
		}
		body["manualCpc"] = manual
	case biddingStrategyMaximizeClicks:
		spend := map[string]any{}
		if v, ok := bidding[biddingKeyCPCBidCeiling]; ok {
			if micros, err := amountToMicros(v); err == nil {
				spend["cpcBidCeilingMicros"] = strconv.FormatInt(micros, 10)
				*mask = append(*mask, "targetSpend.cpcBidCeilingMicros")
			}
		}
		body["targetSpend"] = spend
		if len(spend) == 0 {
			*mask = append(*mask, "targetSpend")
		}
	case biddingStrategyMaximizeConv:
		max := map[string]any{}
		if v, ok := bidding[biddingKeyTargetCPA]; ok {
			if micros, err := amountToMicros(v); err == nil {
				max["targetCpaMicros"] = strconv.FormatInt(micros, 10)
				*mask = append(*mask, "maximizeConversions.targetCpaMicros")
			}
		}
		body["maximizeConversions"] = max
		if len(max) == 0 {
			*mask = append(*mask, "maximizeConversions")
		}
	case biddingStrategyTargetCPA:
		if v, ok := bidding[biddingKeyTargetCPA]; ok {
			if micros, err := amountToMicros(v); err == nil {
				body["targetCpa"] = map[string]any{"targetCpaMicros": strconv.FormatInt(micros, 10)}
				*mask = append(*mask, "targetCpa.targetCpaMicros")
			}
		}
	case biddingStrategyTargetROAS:
		if v, ok := bidding[biddingKeyTargetROAS]; ok {
			if n, err := coerceFloat(v); err == nil {
				body["targetRoas"] = map[string]any{"targetRoas": n}
				*mask = append(*mask, "targetRoas.targetRoas")
			}
		}
	case biddingStrategyMaximizeConvVal:
		max := map[string]any{}
		if v, ok := bidding[biddingKeyTargetROAS]; ok {
			if n, err := coerceFloat(v); err == nil {
				max["targetRoas"] = n
				*mask = append(*mask, "maximizeConversionValue.targetRoas")
			}
		}
		body["maximizeConversionValue"] = max
		if len(max) == 0 {
			*mask = append(*mask, "maximizeConversionValue")
		}
	}
}

func (p *Provider) liveBudgetAttr(addr resource.Address, budgetResourceName string, desired any) (any, error) {
	want := logicalRef(desired)
	_, budgetID, ok := splitCampaignBudgetResourceName(budgetResourceName)
	if !ok {
		if budgetResourceName == "" {
			return nil, nil
		}
		return nil, fmt.Errorf("resource %s: remote campaign budget resource name is invalid", addr)
	}
	if !want.IsZero() {
		wantID := ""
		if resolved, ok := resource.AsResolved(desired); ok {
			wantID = resolved.Identity.ID
		}
		if wantID == "" {
			wantID = p.lookupID(want.Address)
		}
		if wantID != "" && wantID == budgetID {
			return want, nil
		}
	}
	managed, found, err := p.lookupManagedAddress(TypeCampaignBudget, budgetID)
	if err != nil {
		return nil, err
	}
	if found {
		return resource.Ref{Address: managed}, nil
	}
	if !want.IsZero() {
		return budgetID, nil
	}
	return nil, nil
}

func (p *Provider) campaignBudgetResourceName(v any, customerID string) (string, error) {
	if resolved, ok := resource.AsResolved(v); ok {
		if name, err := coerceString(resolved.Outputs["resourceName"]); err == nil && strings.TrimSpace(name) != "" {
			return name, nil
		}
		if resolved.Identity.ID != "" {
			return campaignBudgetResourceName(customerID, resolved.Identity.ID), nil
		}
		return "", fmt.Errorf("budget reference %s has no provider-native identity", resolved.Address)
	}
	ref, ok := resource.AsRef(v)
	if !ok {
		return "", fmt.Errorf("must be a resource reference ($ref) to a %s.%s resource", Name, TypeCampaignBudget)
	}
	if id := p.lookupID(ref.Address); id != "" {
		return campaignBudgetResourceName(customerID, id), nil
	}
	return "", fmt.Errorf("budget reference %s has no provider-native identity", ref.Address)
}

func requiredCampaignBudgetRef(res resource.Resource) (resource.Ref, error) {
	v, ok := res.Attributes[AttrBudget]
	if !ok {
		return resource.Ref{}, fmt.Errorf("resource %s: missing required attribute %q", res.Address, AttrBudget)
	}
	ref, err := campaignBudgetRefValue(v)
	if err != nil {
		return resource.Ref{}, fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrBudget, err)
	}
	if ref.Address.Provider != Name || ref.Address.Type != TypeCampaignBudget {
		return resource.Ref{}, fmt.Errorf("resource %s: attribute %q must reference a %s.%s resource", res.Address, AttrBudget, Name, TypeCampaignBudget)
	}
	return ref, nil
}

func campaignBudgetRefValue(v any) (resource.Ref, error) {
	if resolved, ok := resource.AsResolved(v); ok {
		return resource.Ref{Address: resolved.Address}, nil
	}
	ref, ok := resource.AsRef(v)
	if !ok {
		return resource.Ref{}, fmt.Errorf("must be a resource reference ($ref) to a %s.%s resource", Name, TypeCampaignBudget)
	}
	return ref, nil
}

func comparableBudgetAttr(v any) (resource.Ref, error) {
	return campaignBudgetRefValue(v)
}

func requiredCampaignBidding(res resource.Resource) (map[string]any, error) {
	v, ok := res.Attributes[AttrBidding]
	if !ok {
		return nil, fmt.Errorf("resource %s: missing required attribute %q", res.Address, AttrBidding)
	}
	bidding, err := comparableBiddingAttr(v)
	if err != nil {
		return nil, fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrBidding, err)
	}
	return bidding, nil
}

func comparableBiddingAttr(v any) (map[string]any, error) {
	raw, err := asStringMap(v)
	if err != nil {
		return nil, fmt.Errorf("must be an object with a supported Search bidding strategy")
	}
	strategy, err := coerceString(raw[biddingKeyStrategy])
	if err != nil || strings.TrimSpace(strategy) == "" {
		return nil, fmt.Errorf("must include strategy; supported values are %s", joinSorted(keys(campaignBiddingStrategies)))
	}
	strategy = normalizeEnum(strategy)
	if _, ok := campaignBiddingStrategies[strategy]; !ok {
		return nil, fmt.Errorf("strategy must be one of %s", joinSorted(keys(campaignBiddingStrategies)))
	}
	out := map[string]any{biddingKeyStrategy: strategy}

	switch strategy {
	case biddingStrategyManualCPC:
		if _, ok := raw[biddingKeyEnhancedCPC]; ok {
			b, err := coerceBool(raw[biddingKeyEnhancedCPC])
			if err != nil {
				return nil, fmt.Errorf("%s must be a boolean", biddingKeyEnhancedCPC)
			}
			out[biddingKeyEnhancedCPC] = b
		}
	case biddingStrategyMaximizeClicks:
		if _, ok := raw[biddingKeyCPCBidCeiling]; ok {
			micros, err := amountToMicros(raw[biddingKeyCPCBidCeiling])
			if err != nil {
				return nil, fmt.Errorf("%s %w", biddingKeyCPCBidCeiling, err)
			}
			out[biddingKeyCPCBidCeiling] = amountFromMicros(micros)
		}
	case biddingStrategyMaximizeConv:
		if _, ok := raw[biddingKeyTargetCPA]; ok {
			micros, err := amountToMicros(raw[biddingKeyTargetCPA])
			if err != nil {
				return nil, fmt.Errorf("%s %w", biddingKeyTargetCPA, err)
			}
			out[biddingKeyTargetCPA] = amountFromMicros(micros)
		}
	case biddingStrategyTargetCPA:
		if _, ok := raw[biddingKeyTargetCPA]; !ok {
			return nil, fmt.Errorf("%s is required when strategy is %s", biddingKeyTargetCPA, biddingStrategyTargetCPA)
		}
		micros, err := amountToMicros(raw[biddingKeyTargetCPA])
		if err != nil {
			return nil, fmt.Errorf("%s %w", biddingKeyTargetCPA, err)
		}
		out[biddingKeyTargetCPA] = amountFromMicros(micros)
	case biddingStrategyTargetROAS:
		if _, ok := raw[biddingKeyTargetROAS]; !ok {
			return nil, fmt.Errorf("%s is required when strategy is %s", biddingKeyTargetROAS, biddingStrategyTargetROAS)
		}
		n, err := coerceFloat(raw[biddingKeyTargetROAS])
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("%s must be a positive number", biddingKeyTargetROAS)
		}
		out[biddingKeyTargetROAS] = n
	case biddingStrategyMaximizeConvVal:
		if _, ok := raw[biddingKeyTargetROAS]; ok {
			n, err := coerceFloat(raw[biddingKeyTargetROAS])
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("%s must be a positive number", biddingKeyTargetROAS)
			}
			out[biddingKeyTargetROAS] = n
		}
	}

	allowed := map[string]struct{}{biddingKeyStrategy: {}}
	switch strategy {
	case biddingStrategyManualCPC:
		allowed[biddingKeyEnhancedCPC] = struct{}{}
	case biddingStrategyMaximizeClicks:
		allowed[biddingKeyCPCBidCeiling] = struct{}{}
	case biddingStrategyMaximizeConv, biddingStrategyTargetCPA:
		allowed[biddingKeyTargetCPA] = struct{}{}
	case biddingStrategyTargetROAS, biddingStrategyMaximizeConvVal:
		allowed[biddingKeyTargetROAS] = struct{}{}
	}
	for key := range raw {
		if _, ok := allowed[key]; !ok {
			return nil, fmt.Errorf("unsupported bidding field %q for strategy %s", key, strategy)
		}
	}
	return out, nil
}

func liveBiddingAttr(item campaignData, desired any) any {
	strategy := comparableBiddingStrategy(item.BiddingStrategyType)
	if strategy == "" {
		if desired != nil {
			return map[string]any{biddingKeyStrategy: item.BiddingStrategyType}
		}
		return nil
	}
	out := map[string]any{biddingKeyStrategy: strategy}
	switch strategy {
	case biddingStrategyManualCPC:
		if item.EnhancedCPCEnabled != nil {
			out[biddingKeyEnhancedCPC] = *item.EnhancedCPCEnabled
		}
	case biddingStrategyMaximizeClicks:
		if item.CPCBidCeilingMicros != nil {
			out[biddingKeyCPCBidCeiling] = amountFromMicros(*item.CPCBidCeilingMicros)
		}
	case biddingStrategyMaximizeConv:
		if item.MaximizeConversionsTargetCPA != nil {
			out[biddingKeyTargetCPA] = amountFromMicros(*item.MaximizeConversionsTargetCPA)
		}
	case biddingStrategyTargetCPA:
		if item.TargetCPAMicros != nil {
			out[biddingKeyTargetCPA] = amountFromMicros(*item.TargetCPAMicros)
		}
	case biddingStrategyTargetROAS:
		if item.TargetROAS != nil {
			out[biddingKeyTargetROAS] = *item.TargetROAS
		}
	case biddingStrategyMaximizeConvVal:
		if item.MaximizeConversionValueROAS != nil {
			out[biddingKeyTargetROAS] = *item.MaximizeConversionValueROAS
		}
	}
	return out
}

func comparableBiddingStrategy(apiType string) string {
	switch normalizeEnum(apiType) {
	case biddingStrategyManualCPC:
		return biddingStrategyManualCPC
	case apiBiddingTargetSpend, biddingStrategyMaximizeClicks:
		return biddingStrategyMaximizeClicks
	case biddingStrategyMaximizeConv:
		return biddingStrategyMaximizeConv
	case biddingStrategyTargetCPA:
		return biddingStrategyTargetCPA
	case biddingStrategyTargetROAS:
		return biddingStrategyTargetROAS
	case biddingStrategyMaximizeConvVal:
		return biddingStrategyMaximizeConvVal
	default:
		return ""
	}
}

func optionalCampaignNetwork(res resource.Resource) (map[string]any, bool, error) {
	v, ok := res.Attributes[AttrNetwork]
	if !ok {
		return nil, false, nil
	}
	network, err := comparableNetworkAttr(v, true)
	if err != nil {
		return nil, true, fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrNetwork, err)
	}
	return network, true, nil
}

func comparableNetworkAttr(v any, fillDefaults bool) (map[string]any, error) {
	raw, err := asStringMap(v)
	if err != nil {
		return nil, fmt.Errorf("must be an object with Search network settings")
	}
	allowed := map[string]struct{}{
		networkKeyGoogleSearch:         {},
		networkKeySearchNetwork:        {},
		networkKeyContentNetwork:       {},
		networkKeyPartnerSearchNetwork: {},
	}
	for key := range raw {
		if _, ok := allowed[key]; !ok {
			return nil, fmt.Errorf("unsupported network field %q", key)
		}
	}
	out := map[string]any{}
	defaults := defaultNetworkSettingsComparable()
	for _, key := range []string{networkKeyGoogleSearch, networkKeySearchNetwork, networkKeyContentNetwork, networkKeyPartnerSearchNetwork} {
		if _, ok := raw[key]; ok {
			b, err := coerceBool(raw[key])
			if err != nil {
				return nil, fmt.Errorf("%s must be a boolean", key)
			}
			out[key] = b
			continue
		}
		if fillDefaults {
			out[key] = defaults[key]
		}
	}
	if google, ok := out[networkKeyGoogleSearch].(bool); ok && !google {
		return nil, fmt.Errorf("%s must be true for SEARCH campaigns", networkKeyGoogleSearch)
	}
	return out, nil
}

func liveNetworkAttr(item campaignData, desired any) any {
	if desired == nil && item.TargetGoogleSearch == nil && item.TargetSearchNetwork == nil && item.TargetContentNetwork == nil && item.TargetPartnerSearchNetwork == nil {
		return nil
	}
	out := map[string]any{}
	if item.TargetGoogleSearch != nil {
		out[networkKeyGoogleSearch] = *item.TargetGoogleSearch
	}
	if item.TargetSearchNetwork != nil {
		out[networkKeySearchNetwork] = *item.TargetSearchNetwork
	}
	if item.TargetContentNetwork != nil {
		out[networkKeyContentNetwork] = *item.TargetContentNetwork
	}
	if item.TargetPartnerSearchNetwork != nil {
		out[networkKeyPartnerSearchNetwork] = *item.TargetPartnerSearchNetwork
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func defaultNetworkSettings() map[string]any {
	return map[string]any{
		"targetGoogleSearch":         true,
		"targetSearchNetwork":        true,
		"targetContentNetwork":       false,
		"targetPartnerSearchNetwork": false,
	}
}

func defaultNetworkSettingsComparable() map[string]any {
	return map[string]any{
		networkKeyGoogleSearch:         true,
		networkKeySearchNetwork:        true,
		networkKeyContentNetwork:       false,
		networkKeyPartnerSearchNetwork: false,
	}
}

func optionalCampaignDate(res resource.Resource, key string) (string, bool, error) {
	v, ok := res.Attributes[key]
	if !ok {
		return "", false, nil
	}
	date, err := coerceCampaignDate(v)
	if err != nil {
		return "", true, fmt.Errorf("resource %s: attribute %q %w", res.Address, key, err)
	}
	return date, true, nil
}

func coerceCampaignDate(v any) (string, error) {
	s, err := coerceString(v)
	if err != nil {
		return "", fmt.Errorf("must be a date YYYY-MM-DD")
	}
	s = strings.TrimSpace(s)
	if date := campaignDateFromDateTime(s); date != "" {
		if !datePattern.MatchString(date) {
			return "", fmt.Errorf("must be a date YYYY-MM-DD")
		}
		return date, nil
	}
	return "", fmt.Errorf("must be a date YYYY-MM-DD")
}

func campaignDateFromDateTime(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) >= 10 && raw[4] == '-' && raw[7] == '-' {
		return raw[:10]
	}
	compact := strings.ReplaceAll(raw, "-", "")
	if len(compact) >= 8 && isAllDigits(compact[:8]) {
		return compact[:4] + "-" + compact[4:6] + "-" + compact[6:8]
	}
	return ""
}

func asStringMap(v any) (map[string]any, error) {
	switch x := v.(type) {
	case resource.Attributes:
		return map[string]any(x), nil
	case map[string]any:
		return x, nil
	default:
		return nil, fmt.Errorf("must be an object")
	}
}

func logicalRef(v any) resource.Ref {
	if resolved, ok := resource.AsResolved(v); ok {
		return resource.Ref{Address: resolved.Address}
	}
	ref, _ := resource.AsRef(v)
	return ref
}

func intersectComparableValue(want, got any) any {
	wantMap, wantOK := asMapValue(want)
	gotMap, gotOK := asMapValue(got)
	if wantOK && gotOK {
		out := map[string]any{}
		for key := range wantMap {
			if v, ok := gotMap[key]; ok {
				out[key] = v
			}
		}
		return out
	}
	return got
}

func asMapValue(v any) (map[string]any, bool) {
	switch x := v.(type) {
	case map[string]any:
		return x, true
	case resource.Attributes:
		return map[string]any(x), true
	default:
		return nil, false
	}
}

func changedCampaignAPIFields(want, got resource.Attributes) map[string]struct{} {
	changed := map[string]struct{}{}
	if !reflect.DeepEqual(want[AttrName], got[AttrName]) {
		changed["name"] = struct{}{}
	}
	if !reflect.DeepEqual(want[AttrStatus], got[AttrStatus]) {
		changed["status"] = struct{}{}
	}
	if !sameRef(want[AttrBudget], got[AttrBudget]) {
		changed["campaignBudget"] = struct{}{}
	}
	wantBid, _ := asMapValue(want[AttrBidding])
	gotBid, _ := asMapValue(got[AttrBidding])
	if !reflect.DeepEqual(wantBid[biddingKeyStrategy], gotBid[biddingKeyStrategy]) || !reflect.DeepEqual(wantBid, gotBid) {
		for _, field := range biddingAPIFields(wantBid) {
			changed[field] = struct{}{}
		}
	}
	if !reflect.DeepEqual(want[AttrNetwork], got[AttrNetwork]) {
		changed["networkSettings.targetGoogleSearch"] = struct{}{}
		changed["networkSettings.targetSearchNetwork"] = struct{}{}
		changed["networkSettings.targetContentNetwork"] = struct{}{}
		changed["networkSettings.targetPartnerSearchNetwork"] = struct{}{}
	}
	if !reflect.DeepEqual(want[AttrStartDate], got[AttrStartDate]) {
		changed["startDateTime"] = struct{}{}
	}
	if !reflect.DeepEqual(want[AttrEndDate], got[AttrEndDate]) {
		changed["endDateTime"] = struct{}{}
	}
	if !reflect.DeepEqual(want[AttrTrackingUrlTemplate], got[AttrTrackingUrlTemplate]) {
		changed["trackingUrlTemplate"] = struct{}{}
	}
	if !reflect.DeepEqual(want[AttrFinalUrlSuffix], got[AttrFinalUrlSuffix]) {
		changed["finalUrlSuffix"] = struct{}{}
	}
	return changed
}

func biddingAPIFields(bidding map[string]any) []string {
	strategy, _ := bidding[biddingKeyStrategy].(string)
	switch strategy {
	case biddingStrategyManualCPC:
		return []string{"manualCpc", "manualCpc.enhancedCpcEnabled"}
	case biddingStrategyMaximizeClicks:
		return []string{"targetSpend", "targetSpend.cpcBidCeilingMicros"}
	case biddingStrategyMaximizeConv:
		return []string{"maximizeConversions", "maximizeConversions.targetCpaMicros"}
	case biddingStrategyTargetCPA:
		return []string{"targetCpa", "targetCpa.targetCpaMicros"}
	case biddingStrategyTargetROAS:
		return []string{"targetRoas", "targetRoas.targetRoas"}
	case biddingStrategyMaximizeConvVal:
		return []string{"maximizeConversionValue", "maximizeConversionValue.targetRoas"}
	default:
		return nil
	}
}

func sameRef(a, b any) bool {
	ra, ok := resource.AsRef(a)
	if !ok {
		if resolved, ok := resource.AsResolved(a); ok {
			ra = resource.Ref{Address: resolved.Address}
		} else {
			return reflect.DeepEqual(a, b)
		}
	}
	rb, ok := resource.AsRef(b)
	if !ok {
		if resolved, ok := resource.AsResolved(b); ok {
			rb = resource.Ref{Address: resolved.Address}
		} else {
			return false
		}
	}
	return ra.Address == rb.Address
}

func nestedMutateValue(body map[string]any, field string) (any, bool) {
	parts := strings.Split(field, ".")
	cur := any(body)
	for _, part := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func setNestedMutateValue(body map[string]any, field string, value any) {
	parts := strings.Split(field, ".")
	cur := body
	for i, part := range parts {
		if i == len(parts)-1 {
			cur[part] = value
			return
		}
		next, ok := cur[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[part] = next
		}
		cur = next
	}
}

func boundCampaignIdentity(res resource.Resource) (string, bool, error) {
	if res.Identity.IsZero() {
		return "", false, nil
	}
	id, err := parseCampaignIdentity(res.Address, res.Identity.ID)
	if err != nil {
		return "", true, err
	}
	return id, true, nil
}

func parseCampaignIdentity(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("resource %s: persisted identity is empty; a Google Ads campaign id is required", addr)
	}
	if err := campaignIdentityIDError(addr, raw); err != nil {
		return "", err
	}
	return raw, nil
}

func (p *Provider) canonicalCampaignImportID(addr resource.Address, raw string) (string, error) {
	id, err := parseImportCampaignID(addr, raw)
	if err != nil {
		return "", err
	}
	if customerID, restID, ok := splitCampaignResourceName(id); ok {
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

func parseImportCampaignID(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("googleads: import %s: remote identifier is empty; expected a numeric campaign id or resource name customers/{customerId}/campaigns/{id}", addr)
	}
	if _, id, ok := splitCampaignResourceName(raw); ok {
		if err := importCampaignIDError(addr, id); err != nil {
			return "", err
		}
		return raw, nil
	}
	if err := importCampaignIDError(addr, raw); err != nil {
		return "", err
	}
	return raw, nil
}

func importCampaignIDError(addr resource.Address, id string) error {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("googleads: import %s: %q is not a valid Google Ads campaign id; expected a positive numeric id or resource name customers/{customerId}/campaigns/{id}", addr, id)
	}
	return nil
}

func campaignIdentityIDError(addr resource.Address, id string) error {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("resource %s: persisted identity %q is not a valid Google Ads campaign id", addr, id)
	}
	return nil
}

func splitCampaignResourceName(name string) (customerID, campaignID string, ok bool) {
	name = strings.TrimSpace(name)
	parts := strings.Split(name, "/")
	if len(parts) != 4 || parts[0] != "customers" || parts[2] != campaignsCollection {
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

func campaignResourceName(customerID, id string) string {
	return "customers/" + customerID + "/" + campaignsCollection + "/" + id
}

func parseCampaignMutateID(raw json.RawMessage, customerID string) (string, error) {
	var resp struct {
		Results []struct {
			ResourceName string `json:"resourceName"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || len(resp.Results) == 0 {
		return "", fmt.Errorf("malformed mutate response")
	}
	resourceName := strings.TrimSpace(resp.Results[0].ResourceName)
	gotCustomer, id, ok := splitCampaignResourceName(resourceName)
	if !ok {
		return "", fmt.Errorf("malformed mutate response")
	}
	if customerID != "" && gotCustomer != customerID {
		return "", fmt.Errorf("mutate returned campaign %s for a different customer", id)
	}
	return id, nil
}

func parseOptionalMicros(raw json.RawMessage) (*int64, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	s := strings.TrimSpace(string(raw))
	s = strings.Trim(s, `"`)
	if s == "" || s == "null" {
		return nil, nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil, err
	}
	return &n, nil
}
