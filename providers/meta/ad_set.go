package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/meta/client"
)

const (
	adSetFields = "id,account_id,campaign_id,name,status,configured_status,effective_status,daily_budget,lifetime_budget,start_time,end_time,billing_event,optimization_goal,bid_strategy,bid_amount,destination_type,promoted_object,targeting"

	adSetStatusActive   = "ACTIVE"
	adSetStatusPaused   = "PAUSED"
	adSetStatusDeleted  = "DELETED"
	adSetStatusArchived = "ARCHIVED"

	targetCountries          = "countries"
	targetRegions            = "regions"
	targetAgeMin             = "ageMin"
	targetAgeMax             = "ageMax"
	targetGenders            = "genders"
	targetLocales            = "locales"
	targetPublisherPlatforms = "publisherPlatforms"
	targetInstagramPositions = "instagramPositions"
	targetDevicePlatforms    = "devicePlatforms"
	defaultTargetAgeMin      = int64(18)
	defaultTargetAgeMax      = int64(65)
)

var (
	supportedAdSetAttrs = map[string]struct{}{
		AttrName: {}, AttrStatus: {}, AttrCampaign: {}, AttrDailyBudget: {}, AttrLifetimeBudget: {},
		AttrStartTime: {}, AttrEndTime: {}, AttrBillingEvent: {}, AttrOptimizationGoal: {},
		AttrBidStrategy: {}, AttrBidAmount: {}, AttrDestinationType: {}, AttrPixel: {},
		AttrCustomConversion: {}, AttrTargeting: {},
	}
	computedAdSetAttrs = map[string]struct{}{
		"id": {}, "adSetId": {}, "account_id": {}, "accountId": {}, "campaign_id": {},
		"configured_status": {}, "configuredStatus": {}, "effective_status": {}, "effectiveStatus": {},
		"created_time": {}, "createdTime": {}, "updated_time": {}, "updatedTime": {},
		"budget_remaining": {}, "budgetRemaining": {}, "issues_info": {}, "issuesInfo": {},
	}
	adSetStatuses          = map[string]struct{}{adSetStatusActive: {}, adSetStatusPaused: {}}
	adSetBillingEvents     = map[string]struct{}{"IMPRESSIONS": {}}
	adSetOptimizationGoals = map[string]struct{}{
		"OFFSITE_CONVERSIONS": {}, "LINK_CLICKS": {},
	}
	adSetBidStrategies = map[string]struct{}{
		"LOWEST_COST_WITHOUT_CAP": {}, "LOWEST_COST_WITH_BID_CAP": {}, "COST_CAP": {},
	}
	adSetDestinationTypes  = map[string]struct{}{"WEBSITE": {}}
	genderCodes            = map[string]int64{"MALE": 1, "FEMALE": 2}
	instagramPositionToAPI = map[string]string{"FEED": "stream", "STORIES": "story", "REELS": "reels"}
)

type adSet struct {
	ID               string          `json:"id"`
	AccountID        string          `json:"account_id"`
	CampaignID       string          `json:"campaign_id"`
	Name             string          `json:"name"`
	Status           string          `json:"status"`
	ConfiguredStatus string          `json:"configured_status"`
	EffectiveStatus  string          `json:"effective_status"`
	DailyBudget      any             `json:"daily_budget"`
	LifetimeBudget   any             `json:"lifetime_budget"`
	StartTime        string          `json:"start_time"`
	EndTime          string          `json:"end_time"`
	BillingEvent     string          `json:"billing_event"`
	OptimizationGoal string          `json:"optimization_goal"`
	BidStrategy      string          `json:"bid_strategy"`
	BidAmount        any             `json:"bid_amount"`
	DestinationType  string          `json:"destination_type"`
	PromotedObject   json.RawMessage `json:"promoted_object"`
	Targeting        json.RawMessage `json:"targeting"`
}

type normalizedAdSet struct {
	Name              string
	Status            string
	Campaign          resource.Ref
	DailyBudget       int64
	HasDailyBudget    bool
	LifetimeBudget    int64
	HasLifetimeBudget bool
	StartTime         string
	HasStartTime      bool
	EndTime           string
	HasEndTime        bool
	BillingEvent      string
	OptimizationGoal  string
	BidStrategy       string
	HasBidStrategy    bool
	BidAmount         int64
	HasBidAmount      bool
	DestinationType   string
	Pixel             resource.Ref
	CustomConversion  resource.Ref
	Targeting         normalizedTargeting
}

type normalizedTargeting struct {
	Countries          []string
	Regions            []string
	AgeMin             int64
	AgeMax             int64
	Genders            []string
	Locales            []int64
	PublisherPlatforms []string
	InstagramPositions []string
	DevicePlatforms    []string
}

func (p *Provider) validateAdSet(res resource.Resource) error {
	if err := p.requireAdAccount(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}
	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedAdSetAttrs[key]; ok {
			continue
		}
		if _, computed := computedAdSetAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; meta.ad_set supports %s", res.Address, key, joinSorted(keys(supportedAdSetAttrs)))
	}
	if _, err := normalizeAdSet(res); err != nil {
		return err
	}
	if _, _, err := boundIdentity(res); err != nil {
		return err
	}
	return nil
}

func (p *Provider) readAdSet(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateAdSet(res); err != nil {
		return resource.RemoteResource{}, err
	}
	id, bound, err := boundIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if !bound {
		return resource.RemoteResource{}, fmt.Errorf("meta: read %s: %w", res.Address, provider.ErrNotFound)
	}
	live, err := p.readAdSetByID(ctx, res, id)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: read %s: %w", res.Address, err)
	}
	return live, nil
}

func (p *Provider) createAdSet(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateAdSet(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}
	normalized, _ := normalizeAdSet(res)
	form, err := p.adSetForm(normalized)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: %w", res.Address, err)
	}
	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := c.Post(ctx, c.AdAccountID()+"/adsets", form, &created); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: %w", res.Address, err)
	}
	id, err := normalizeObjectID(created.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: API returned an invalid id: %w", res.Address, err)
	}
	live, err := p.readAdSetByID(ctx, res, id)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s succeeded but refreshing ad set %q failed: %w", res.Address, id, err)
	}
	return live, nil
}

func (p *Provider) updateAdSet(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateAdSet(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: missing remote identity", desired.Address)
	}
	if id, bound, err := boundIdentity(desired); err != nil {
		return resource.RemoteResource{}, err
	} else if bound && id != actual.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: persisted identity %q does not match planned remote identity %q", desired.Address, id, actual.Identity.ID)
	}
	want, err := normalizeAdSet(desired)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	got, err := normalizeAdSet(resource.Resource{Address: desired.Address, Attributes: actual.Attributes})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: live ad set is invalid: %w", desired.Address, err)
	}
	if err := validateAdSetTransition(desired.Address, want, got); err != nil {
		return resource.RemoteResource{}, err
	}
	form := url.Values{}
	if want.Name != got.Name {
		form.Set("name", want.Name)
	}
	if want.Status != got.Status {
		form.Set("status", want.Status)
	}
	if want.HasDailyBudget && want.DailyBudget != got.DailyBudget {
		form.Set("daily_budget", strconv.FormatInt(want.DailyBudget, 10))
	}
	if want.HasLifetimeBudget && want.LifetimeBudget != got.LifetimeBudget {
		form.Set("lifetime_budget", strconv.FormatInt(want.LifetimeBudget, 10))
	}
	if want.HasEndTime && want.EndTime != got.EndTime {
		form.Set("end_time", want.EndTime)
	}
	if want.BidStrategy != got.BidStrategy {
		form.Set("bid_strategy", want.BidStrategy)
	}
	if want.HasBidAmount && want.BidAmount != got.BidAmount {
		form.Set("bid_amount", strconv.FormatInt(want.BidAmount, 10))
	}
	if !reflect.DeepEqual(want.Targeting, got.Targeting) {
		raw, _ := json.Marshal(targetingAPIObject(want.Targeting))
		form.Set("targeting", string(raw))
	}
	if len(form) == 0 {
		return p.readAdSetByID(ctx, desired, actual.Identity.ID)
	}
	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	var result map[string]any
	if err := c.Post(ctx, actual.Identity.ID, form, &result); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: %w", desired.Address, err)
	}
	if success, ok := result["success"].(bool); ok && !success {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: API did not report success", desired.Address)
	}
	live, err := p.readAdSetByID(ctx, desired, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s succeeded but refreshing ad set %q failed: %w", desired.Address, actual.Identity.ID, err)
	}
	return live, nil
}

func (p *Provider) importAdSet(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	id, err := p.canonicalCustomConversionImportID(addr, rawID)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireConfig(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: import %s: %w", addr, err)
	}
	live, err := p.readAdSetByID(ctx, resource.Resource{Address: addr}, id)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("meta: import %s: remote ad set %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("meta: import %s: %w", addr, err)
	}
	return live, nil
}

func (p *Provider) normalizeAdSetComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	want, err := normalizeAdSet(desired)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	wantAttrs := adSetAttributes(want)
	if live == nil {
		return wantAttrs, nil, nil
	}
	if id, bound, err := boundIdentity(desired); err != nil {
		return nil, nil, err
	} else if bound && (live.Identity.IsZero() || live.Identity.ID != id) {
		return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, id, live.Identity.ID)
	}
	got, err := normalizeAdSet(resource.Resource{Address: desired.Address, Attributes: live.Attributes})
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: live ad set is invalid: %w", desired.Address, err)
	}
	if err := validateAdSetTransition(desired.Address, want, got); err != nil {
		return nil, nil, err
	}
	return wantAttrs, adSetAttributes(got), nil
}

func (p *Provider) readAdSetByID(ctx context.Context, desired resource.Resource, id string) (resource.RemoteResource, error) {
	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	var item adSet
	if err := c.Get(ctx, id, url.Values{"fields": {adSetFields}}, &item); err != nil {
		if client.IsNotFound(err) {
			return resource.RemoteResource{}, provider.ErrNotFound
		}
		return resource.RemoteResource{}, err
	}
	status := strings.ToUpper(strings.TrimSpace(item.ConfiguredStatus))
	if status == "" {
		status = strings.ToUpper(strings.TrimSpace(item.Status))
	}
	if status == adSetStatusDeleted || status == adSetStatusArchived {
		return resource.RemoteResource{}, provider.ErrNotFound
	}
	live, err := p.remoteAdSet(desired, item)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	return p.rememberLive(live), nil
}

func (p *Provider) remoteAdSet(desired resource.Resource, item adSet) (resource.RemoteResource, error) {
	id, err := normalizeObjectID(item.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote ad set id is invalid: %w", err)
	}
	if err := p.ensureAdSetAccount(item.AccountID); err != nil {
		return resource.RemoteResource{}, err
	}
	campaignID, err := normalizeObjectID(item.CampaignID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote ad set %s has invalid campaign_id: %w", id, err)
	}
	campaign, err := p.managedRefAttr(TypeCampaign, OutputCampaignID, campaignID, desired.Attributes[AttrCampaign])
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote ad set %s campaign relationship: %w", id, err)
	}
	optimization := strings.ToUpper(strings.TrimSpace(item.OptimizationGoal))
	var pixel, conversion resource.Ref
	if optimization == "OFFSITE_CONVERSIONS" {
		promoted, err := decodeJSONObject(item.PromotedObject)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("remote ad set %s has invalid promoted_object: %w", id, err)
		}
		pixelID, ok := objectIDFromAny(promoted["pixel_id"])
		if !ok {
			return resource.RemoteResource{}, fmt.Errorf("remote ad set %s promoted_object is missing pixel_id", id)
		}
		conversionID, ok := objectIDFromAny(promoted["custom_conversion_id"])
		if !ok {
			return resource.RemoteResource{}, fmt.Errorf("remote ad set %s promoted_object is missing custom_conversion_id", id)
		}
		pixel, err = p.managedRefAttr(TypePixel, OutputPixelID, pixelID, desired.Attributes[AttrPixel])
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("remote ad set %s pixel relationship: %w", id, err)
		}
		conversion, err = p.managedRefAttr(TypeCustomConversion, OutputCustomConversionID, conversionID, desired.Attributes[AttrCustomConversion])
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("remote ad set %s custom conversion relationship: %w", id, err)
		}
	}
	targeting, err := normalizeRemoteTargeting(desired.Address, item.Targeting)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote ad set %s targeting cannot be represented: %w", id, err)
	}
	status := strings.ToUpper(strings.TrimSpace(item.ConfiguredStatus))
	if status == "" {
		status = strings.ToUpper(strings.TrimSpace(item.Status))
	}
	attrs := resource.Attributes{
		AttrName: strings.TrimSpace(item.Name), AttrStatus: status, AttrCampaign: campaign,
		AttrBillingEvent:     strings.ToUpper(strings.TrimSpace(item.BillingEvent)),
		AttrOptimizationGoal: optimization,
		AttrDestinationType:  strings.ToUpper(strings.TrimSpace(item.DestinationType)),
		AttrTargeting:        targetingAttributes(targeting),
	}
	if !pixel.IsZero() {
		attrs[AttrPixel] = pixel
	}
	if !conversion.IsZero() {
		attrs[AttrCustomConversion] = conversion
	}
	if budget, set, e := remoteBudget(item.DailyBudget); e != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote ad set %s has invalid daily_budget: %w", id, e)
	} else if set {
		attrs[AttrDailyBudget] = budget
	}
	if budget, set, e := remoteBudget(item.LifetimeBudget); e != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote ad set %s has invalid lifetime_budget: %w", id, e)
	} else if set {
		attrs[AttrLifetimeBudget] = budget
	}
	if value, set, e := normalizeScheduleValue(desired.Address, AttrStartTime, item.StartTime); e != nil {
		return resource.RemoteResource{}, e
	} else if set {
		attrs[AttrStartTime] = value
	}
	if value, set, e := normalizeScheduleValue(desired.Address, AttrEndTime, item.EndTime); e != nil {
		return resource.RemoteResource{}, e
	} else if set {
		attrs[AttrEndTime] = value
	}
	if bid := strings.ToUpper(strings.TrimSpace(item.BidStrategy)); bid != "" {
		attrs[AttrBidStrategy] = bid
	}
	if amount, set, e := remoteBudget(item.BidAmount); e != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote ad set %s has invalid bid_amount: %w", id, e)
	} else if set {
		attrs[AttrBidAmount] = amount
	}
	res := resource.Resource{Address: desired.Address, Attributes: attrs}
	if _, err := normalizeAdSet(res); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote ad set %s cannot be represented: %w", id, err)
	}
	computed := resource.Attributes{OutputAdSetID: id}
	if effective := strings.ToUpper(strings.TrimSpace(item.EffectiveStatus)); effective != "" {
		computed["effectiveStatus"] = effective
	}
	return resource.RemoteResource{Address: desired.Address, Identity: resource.Identity{ID: id}, Attributes: attrs, Computed: computed}, nil
}

func normalizeAdSet(res resource.Resource) (normalizedAdSet, error) {
	name, err := requiredString(res, AttrName)
	if err != nil {
		return normalizedAdSet{}, err
	}
	status, err := campaignEnum(res, AttrStatus, adSetStatuses, false, adSetStatusPaused)
	if err != nil {
		return normalizedAdSet{}, err
	}
	campaign, err := requiredTypedRef(res, AttrCampaign, TypeCampaign)
	if err != nil {
		return normalizedAdSet{}, err
	}
	daily, hasDaily, err := optionalBudget(res, AttrDailyBudget)
	if err != nil {
		return normalizedAdSet{}, err
	}
	lifetime, hasLifetime, err := optionalBudget(res, AttrLifetimeBudget)
	if err != nil {
		return normalizedAdSet{}, err
	}
	if hasDaily && hasLifetime {
		return normalizedAdSet{}, fmt.Errorf("resource %s: attributes %q and %q are mutually exclusive", res.Address, AttrDailyBudget, AttrLifetimeBudget)
	}
	start, hasStart, err := optionalSchedule(res, AttrStartTime)
	if err != nil {
		return normalizedAdSet{}, err
	}
	end, hasEnd, err := optionalSchedule(res, AttrEndTime)
	if err != nil {
		return normalizedAdSet{}, err
	}
	if hasStart && hasEnd {
		st, _ := time.Parse(time.RFC3339, start)
		et, _ := time.Parse(time.RFC3339, end)
		if !et.After(st) {
			return normalizedAdSet{}, fmt.Errorf("resource %s: attribute %q must be after %q", res.Address, AttrEndTime, AttrStartTime)
		}
	}
	if hasLifetime && (!hasStart || !hasEnd) {
		return normalizedAdSet{}, fmt.Errorf("resource %s: lifetimeBudget requires both startTime and endTime", res.Address)
	}
	billing, err := campaignEnum(res, AttrBillingEvent, adSetBillingEvents, false, "IMPRESSIONS")
	if err != nil {
		return normalizedAdSet{}, err
	}
	optimization, err := campaignEnum(res, AttrOptimizationGoal, adSetOptimizationGoals, true, "")
	if err != nil {
		return normalizedAdSet{}, err
	}
	destination, err := campaignEnum(res, AttrDestinationType, adSetDestinationTypes, true, "")
	if err != nil {
		return normalizedAdSet{}, err
	}
	bid, err := campaignEnum(res, AttrBidStrategy, adSetBidStrategies, false, "LOWEST_COST_WITHOUT_CAP")
	if err != nil {
		return normalizedAdSet{}, err
	}
	_, hasBidStrategy := res.Attributes[AttrBidStrategy]
	bidAmount, hasBidAmount, err := optionalBudget(res, AttrBidAmount)
	if err != nil {
		return normalizedAdSet{}, err
	}
	needsBid := bid == "LOWEST_COST_WITH_BID_CAP" || bid == "COST_CAP"
	if needsBid != hasBidAmount {
		if needsBid {
			return normalizedAdSet{}, fmt.Errorf("resource %s: bidStrategy %s requires bidAmount", res.Address, bid)
		}
		return normalizedAdSet{}, fmt.Errorf("resource %s: bidAmount is valid only with LOWEST_COST_WITH_BID_CAP or COST_CAP", res.Address)
	}
	var pixel, conversion resource.Ref
	if optimization == "OFFSITE_CONVERSIONS" {
		pixel, err = requiredTypedRef(res, AttrPixel, TypePixel)
		if err != nil {
			return normalizedAdSet{}, err
		}
		conversion, err = requiredTypedRef(res, AttrCustomConversion, TypeCustomConversion)
		if err != nil {
			return normalizedAdSet{}, err
		}
	} else if _, ok := res.Attributes[AttrPixel]; ok {
		return normalizedAdSet{}, fmt.Errorf("resource %s: pixel is valid only with OFFSITE_CONVERSIONS", res.Address)
	} else if _, ok := res.Attributes[AttrCustomConversion]; ok {
		return normalizedAdSet{}, fmt.Errorf("resource %s: customConversion is valid only with OFFSITE_CONVERSIONS", res.Address)
	}
	targeting, err := normalizeTargeting(res)
	if err != nil {
		return normalizedAdSet{}, err
	}
	return normalizedAdSet{Name: name, Status: status, Campaign: campaign, DailyBudget: daily, HasDailyBudget: hasDaily,
		LifetimeBudget: lifetime, HasLifetimeBudget: hasLifetime, StartTime: start, HasStartTime: hasStart, EndTime: end, HasEndTime: hasEnd,
		BillingEvent: billing, OptimizationGoal: optimization, BidStrategy: bid, HasBidStrategy: hasBidStrategy, BidAmount: bidAmount, HasBidAmount: hasBidAmount,
		DestinationType: destination, Pixel: pixel, CustomConversion: conversion, Targeting: targeting}, nil
}

func requiredTypedRef(res resource.Resource, key, resourceType string) (resource.Ref, error) {
	v, ok := res.Attributes[key]
	if !ok {
		return resource.Ref{}, fmt.Errorf("resource %s: missing required attribute %q", res.Address, key)
	}
	ref := logicalRef(v)
	if ref.IsZero() {
		return resource.Ref{}, fmt.Errorf("resource %s: attribute %q must be a resource reference ($ref) to a %s.%s resource", res.Address, key, Name, resourceType)
	}
	if ref.Address.Provider != Name || ref.Address.Type != resourceType {
		return resource.Ref{}, fmt.Errorf("resource %s: attribute %q must reference a %s.%s resource", res.Address, key, Name, resourceType)
	}
	return resource.Ref{Address: ref.Address}, nil
}

func optionalSchedule(res resource.Resource, key string) (string, bool, error) {
	v, ok := res.Attributes[key]
	if !ok {
		return "", false, nil
	}
	s, err := coerceString(v)
	if err != nil || strings.TrimSpace(s) == "" {
		return "", true, fmt.Errorf("resource %s: attribute %q must be an RFC3339 timestamp", res.Address, key)
	}
	return normalizeScheduleValue(res.Address, key, s)
}

func normalizeScheduleValue(addr resource.Address, key, raw string) (string, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false, nil
	}
	var parsed time.Time
	var err error
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05-0700", "2006-01-02T15:04:05Z0700"} {
		parsed, err = time.Parse(layout, raw)
		if err == nil {
			return parsed.UTC().Format(time.RFC3339), true, nil
		}
	}
	return "", true, fmt.Errorf("resource %s: attribute %q must be an RFC3339 timestamp", addr, key)
}

func normalizeTargeting(res resource.Resource) (normalizedTargeting, error) {
	raw, ok := res.Attributes[AttrTargeting]
	if !ok {
		return normalizedTargeting{}, fmt.Errorf("resource %s: missing required attribute %q", res.Address, AttrTargeting)
	}
	m, ok := stringMap(raw)
	if !ok {
		return normalizedTargeting{}, fmt.Errorf("resource %s: attribute %q must be an object", res.Address, AttrTargeting)
	}
	allowed := map[string]struct{}{targetCountries: {}, targetRegions: {}, targetAgeMin: {}, targetAgeMax: {}, targetGenders: {}, targetLocales: {}, targetPublisherPlatforms: {}, targetInstagramPositions: {}, targetDevicePlatforms: {}}
	for key := range m {
		if _, ok := allowed[key]; !ok {
			return normalizedTargeting{}, fmt.Errorf("resource %s: unsupported targeting field %q", res.Address, key)
		}
	}
	countries, err := normalizedStringList(res.Address, AttrTargeting+"."+targetCountries, m[targetCountries], false, normalizeCountry)
	if err != nil {
		return normalizedTargeting{}, err
	}
	regions, err := normalizedStringList(res.Address, AttrTargeting+"."+targetRegions, m[targetRegions], false, func(s string) (string, bool) { id, e := normalizeObjectID(s); return id, e == nil })
	if err != nil {
		return normalizedTargeting{}, err
	}
	if len(countries) == 0 && len(regions) == 0 {
		return normalizedTargeting{}, fmt.Errorf("resource %s: targeting requires at least one country or region", res.Address)
	}
	ageMin, err := optionalWhole(m[targetAgeMin], defaultTargetAgeMin, 18, 65)
	if err != nil {
		return normalizedTargeting{}, fmt.Errorf("resource %s: targeting.ageMin %w", res.Address, err)
	}
	ageMax, err := optionalWhole(m[targetAgeMax], defaultTargetAgeMax, 18, 65)
	if err != nil {
		return normalizedTargeting{}, fmt.Errorf("resource %s: targeting.ageMax %w", res.Address, err)
	}
	if ageMin > ageMax {
		return normalizedTargeting{}, fmt.Errorf("resource %s: targeting.ageMin must not exceed targeting.ageMax", res.Address)
	}
	genders, err := normalizedStringList(res.Address, AttrTargeting+"."+targetGenders, m[targetGenders], false, func(s string) (string, bool) { s = strings.ToUpper(s); _, ok := genderCodes[s]; return s, ok })
	if err != nil {
		return normalizedTargeting{}, err
	}
	locales, err := normalizedIntList(res.Address, AttrTargeting+"."+targetLocales, m[targetLocales])
	if err != nil {
		return normalizedTargeting{}, err
	}
	platforms, err := normalizedStringList(res.Address, AttrTargeting+"."+targetPublisherPlatforms, m[targetPublisherPlatforms], false, func(s string) (string, bool) { s = strings.ToUpper(s); return s, s == "INSTAGRAM" })
	if err != nil {
		return normalizedTargeting{}, err
	}
	positions, err := normalizedStringList(res.Address, AttrTargeting+"."+targetInstagramPositions, m[targetInstagramPositions], false, normalizeInstagramPosition)
	if err != nil {
		return normalizedTargeting{}, err
	}
	if len(positions) > 0 && (len(platforms) != 1 || platforms[0] != "INSTAGRAM") {
		return normalizedTargeting{}, fmt.Errorf("resource %s: instagramPositions requires publisherPlatforms: [INSTAGRAM]", res.Address)
	}
	devices, err := normalizedStringList(res.Address, AttrTargeting+"."+targetDevicePlatforms, m[targetDevicePlatforms], false, func(s string) (string, bool) {
		s = strings.ToUpper(s)
		return s, s == "MOBILE" || s == "DESKTOP"
	})
	if err != nil {
		return normalizedTargeting{}, err
	}
	return normalizedTargeting{Countries: countries, Regions: regions, AgeMin: ageMin, AgeMax: ageMax, Genders: genders, Locales: locales, PublisherPlatforms: platforms, InstagramPositions: positions, DevicePlatforms: devices}, nil
}

func normalizeRemoteTargeting(addr resource.Address, raw json.RawMessage) (normalizedTargeting, error) {
	m, err := decodeJSONObject(raw)
	if err != nil {
		return normalizedTargeting{}, err
	}
	allowed := map[string]struct{}{"geo_locations": {}, "age_min": {}, "age_max": {}, "genders": {}, "locales": {}, "publisher_platforms": {}, "instagram_positions": {}, "device_platforms": {}}
	for key := range m {
		if _, ok := allowed[key]; !ok {
			return normalizedTargeting{}, fmt.Errorf("unsupported provider field %q", key)
		}
	}
	geo, ok := stringMap(m["geo_locations"])
	if !ok {
		return normalizedTargeting{}, fmt.Errorf("geo_locations must be an object")
	}
	for key := range geo {
		if key != "countries" && key != "regions" {
			return normalizedTargeting{}, fmt.Errorf("unsupported geo_locations field %q", key)
		}
	}
	countries, err := remoteStrings(geo["countries"])
	if err != nil {
		return normalizedTargeting{}, fmt.Errorf("countries: %w", err)
	}
	regionsRaw, err := anySlice(geo["regions"])
	if err != nil {
		return normalizedTargeting{}, fmt.Errorf("regions: %w", err)
	}
	regions := make([]string, 0, len(regionsRaw))
	for _, v := range regionsRaw {
		obj, ok := stringMap(v)
		if !ok {
			return normalizedTargeting{}, fmt.Errorf("region must be an object")
		}
		id, ok := objectIDFromAny(obj["key"])
		if !ok {
			return normalizedTargeting{}, fmt.Errorf("region key must be numeric")
		}
		regions = append(regions, id)
	}
	gendersRaw, err := remoteInt64s(m["genders"])
	if err != nil {
		return normalizedTargeting{}, fmt.Errorf("genders: %w", err)
	}
	genders := make([]string, 0, len(gendersRaw))
	for _, code := range gendersRaw {
		switch code {
		case 1:
			genders = append(genders, "MALE")
		case 2:
			genders = append(genders, "FEMALE")
		default:
			return normalizedTargeting{}, fmt.Errorf("unsupported gender code %d", code)
		}
	}
	locales, err := remoteInt64s(m["locales"])
	if err != nil {
		return normalizedTargeting{}, fmt.Errorf("locales: %w", err)
	}
	platformsAPI, err := remoteStrings(m["publisher_platforms"])
	if err != nil {
		return normalizedTargeting{}, fmt.Errorf("publisher_platforms: %w", err)
	}
	platforms := make([]string, 0, len(platformsAPI))
	for _, v := range platformsAPI {
		if strings.ToLower(v) != "instagram" {
			return normalizedTargeting{}, fmt.Errorf("unsupported publisher platform %q", v)
		}
		platforms = append(platforms, "INSTAGRAM")
	}
	positionsAPI, err := remoteStrings(m["instagram_positions"])
	if err != nil {
		return normalizedTargeting{}, fmt.Errorf("instagram_positions: %w", err)
	}
	positions := make([]string, 0, len(positionsAPI))
	for _, v := range positionsAPI {
		canonical, ok := normalizeInstagramPosition(v)
		if !ok {
			return normalizedTargeting{}, fmt.Errorf("unsupported Instagram position %q", v)
		}
		positions = append(positions, canonical)
	}
	devicesAPI, err := remoteStrings(m["device_platforms"])
	if err != nil {
		return normalizedTargeting{}, fmt.Errorf("device_platforms: %w", err)
	}
	devices := make([]string, 0, len(devicesAPI))
	for _, v := range devicesAPI {
		value := strings.ToUpper(strings.TrimSpace(v))
		if value != "MOBILE" && value != "DESKTOP" {
			return normalizedTargeting{}, fmt.Errorf("unsupported device platform %q", v)
		}
		devices = append(devices, value)
	}
	attrs := resource.Attributes{AttrTargeting: map[string]any{targetCountries: stringsToAny(countries), targetRegions: stringsToAny(regions), targetAgeMin: m["age_min"], targetAgeMax: m["age_max"], targetGenders: stringsToAny(genders), targetLocales: int64sToAny(locales), targetPublisherPlatforms: stringsToAny(platforms), targetInstagramPositions: stringsToAny(positions), targetDevicePlatforms: stringsToAny(devices)}}
	for k, v := range attrs[AttrTargeting].(map[string]any) {
		if v == nil {
			delete(attrs[AttrTargeting].(map[string]any), k)
		}
	}
	return normalizeTargeting(resource.Resource{Address: addr, Attributes: attrs})
}

func adSetAttributes(a normalizedAdSet) resource.Attributes {
	out := resource.Attributes{AttrName: a.Name, AttrStatus: a.Status, AttrCampaign: a.Campaign, AttrBillingEvent: a.BillingEvent, AttrOptimizationGoal: a.OptimizationGoal, AttrBidStrategy: a.BidStrategy, AttrDestinationType: a.DestinationType, AttrTargeting: targetingAttributes(a.Targeting)}
	if a.HasDailyBudget {
		out[AttrDailyBudget] = a.DailyBudget
	}
	if a.HasLifetimeBudget {
		out[AttrLifetimeBudget] = a.LifetimeBudget
	}
	if a.HasStartTime {
		out[AttrStartTime] = a.StartTime
	}
	if a.HasEndTime {
		out[AttrEndTime] = a.EndTime
	}
	if a.HasBidAmount {
		out[AttrBidAmount] = a.BidAmount
	}
	if !a.Pixel.IsZero() {
		out[AttrPixel] = a.Pixel
	}
	if !a.CustomConversion.IsZero() {
		out[AttrCustomConversion] = a.CustomConversion
	}
	return out
}

func targetingAttributes(t normalizedTargeting) map[string]any {
	out := map[string]any{targetCountries: stringsToAny(t.Countries), targetAgeMin: t.AgeMin, targetAgeMax: t.AgeMax}
	if len(t.Regions) > 0 {
		out[targetRegions] = stringsToAny(t.Regions)
	}
	if len(t.Genders) > 0 {
		out[targetGenders] = stringsToAny(t.Genders)
	}
	if len(t.Locales) > 0 {
		out[targetLocales] = int64sToAny(t.Locales)
	}
	if len(t.PublisherPlatforms) > 0 {
		out[targetPublisherPlatforms] = stringsToAny(t.PublisherPlatforms)
	}
	if len(t.InstagramPositions) > 0 {
		out[targetInstagramPositions] = stringsToAny(t.InstagramPositions)
	}
	if len(t.DevicePlatforms) > 0 {
		out[targetDevicePlatforms] = stringsToAny(t.DevicePlatforms)
	}
	return out
}

func targetingAPIObject(t normalizedTargeting) map[string]any {
	geo := map[string]any{}
	if len(t.Countries) > 0 {
		geo["countries"] = t.Countries
	}
	if len(t.Regions) > 0 {
		rows := make([]map[string]string, len(t.Regions))
		for i, id := range t.Regions {
			rows[i] = map[string]string{"key": id}
		}
		geo["regions"] = rows
	}
	out := map[string]any{"geo_locations": geo, "age_min": t.AgeMin, "age_max": t.AgeMax}
	if len(t.Genders) > 0 {
		values := make([]int64, len(t.Genders))
		for i, v := range t.Genders {
			values[i] = genderCodes[v]
		}
		out["genders"] = values
	}
	if len(t.Locales) > 0 {
		out["locales"] = t.Locales
	}
	if len(t.PublisherPlatforms) > 0 {
		values := make([]string, len(t.PublisherPlatforms))
		for i, v := range t.PublisherPlatforms {
			values[i] = strings.ToLower(v)
		}
		out["publisher_platforms"] = values
	}
	if len(t.InstagramPositions) > 0 {
		values := make([]string, len(t.InstagramPositions))
		for i, v := range t.InstagramPositions {
			values[i] = instagramPositionToAPI[v]
		}
		out["instagram_positions"] = values
	}
	if len(t.DevicePlatforms) > 0 {
		values := make([]string, len(t.DevicePlatforms))
		for i, v := range t.DevicePlatforms {
			values[i] = strings.ToLower(v)
		}
		out["device_platforms"] = values
	}
	return out
}

func (p *Provider) adSetForm(a normalizedAdSet) (url.Values, error) {
	campaignID, err := p.refID(a.Campaign, OutputCampaignID)
	if err != nil {
		return nil, fmt.Errorf("campaign %w", err)
	}
	form := url.Values{"name": {a.Name}, "status": {a.Status}, "campaign_id": {campaignID}, "billing_event": {a.BillingEvent}, "optimization_goal": {a.OptimizationGoal}, "destination_type": {a.DestinationType}}
	if a.HasBidStrategy {
		form.Set("bid_strategy", a.BidStrategy)
	}
	if a.HasDailyBudget {
		form.Set("daily_budget", strconv.FormatInt(a.DailyBudget, 10))
	}
	if a.HasLifetimeBudget {
		form.Set("lifetime_budget", strconv.FormatInt(a.LifetimeBudget, 10))
	}
	if a.HasStartTime {
		form.Set("start_time", a.StartTime)
	}
	if a.HasEndTime {
		form.Set("end_time", a.EndTime)
	}
	if a.HasBidAmount {
		form.Set("bid_amount", strconv.FormatInt(a.BidAmount, 10))
	}
	raw, _ := json.Marshal(targetingAPIObject(a.Targeting))
	form.Set("targeting", string(raw))
	if a.OptimizationGoal == "OFFSITE_CONVERSIONS" {
		pixelID, e := p.refID(a.Pixel, OutputPixelID)
		if e != nil {
			return nil, fmt.Errorf("pixel %w", e)
		}
		conversionID, e := p.refID(a.CustomConversion, OutputCustomConversionID)
		if e != nil {
			return nil, fmt.Errorf("customConversion %w", e)
		}
		promoted, _ := json.Marshal(map[string]string{"pixel_id": pixelID, "custom_conversion_id": conversionID})
		form.Set("promoted_object", string(promoted))
	}
	return form, nil
}

func (p *Provider) refID(ref resource.Ref, output string) (string, error) {
	if id := p.lookupID(ref.Address); id != "" {
		return normalizeObjectID(id)
	}
	return "", fmt.Errorf("reference %s has no provider-native identity", ref.Address)
}

func (p *Provider) managedRefAttr(resourceType, output, remoteID string, desired any) (resource.Ref, error) {
	ref := logicalRef(desired)
	if !ref.IsZero() {
		id := ""
		if resolved, ok := resource.AsResolved(desired); ok {
			id = resolved.Identity.ID
			if id == "" {
				id, _ = coerceString(resolved.Outputs[output])
			}
		}
		if id == "" {
			id = p.lookupID(ref.Address)
		}
		if normalized, e := normalizeObjectID(id); e == nil && normalized == remoteID {
			return resource.Ref{Address: ref.Address}, nil
		}
	}
	managed, found, err := p.lookupManagedAddress(resourceType, remoteID)
	if err != nil {
		return resource.Ref{}, err
	}
	if found {
		return resource.Ref{Address: managed}, nil
	}
	return resource.Ref{}, fmt.Errorf("remote id %s is not uniquely bound to a meta.%s resource; import that dependency first", remoteID, resourceType)
}

func validateAdSetTransition(addr resource.Address, want, got normalizedAdSet) error {
	if want.Campaign.Address != got.Campaign.Address {
		return fmt.Errorf("resource %s: campaign is immutable after create; create a new meta.ad_set instead", addr)
	}
	if want.BillingEvent != got.BillingEvent {
		return fmt.Errorf("resource %s: billingEvent is immutable after create", addr)
	}
	if want.OptimizationGoal != got.OptimizationGoal {
		return fmt.Errorf("resource %s: optimizationGoal is immutable after create", addr)
	}
	if want.DestinationType != got.DestinationType {
		return fmt.Errorf("resource %s: destinationType is immutable after create", addr)
	}
	if want.Pixel.Address != got.Pixel.Address || want.CustomConversion.Address != got.CustomConversion.Address {
		return fmt.Errorf("resource %s: promoted conversion object is immutable after create; create a new meta.ad_set instead", addr)
	}
	if want.HasDailyBudget != got.HasDailyBudget || want.HasLifetimeBudget != got.HasLifetimeBudget {
		return fmt.Errorf("resource %s: ad-set budget ownership/type cannot be added, removed, or switched in place", addr)
	}
	if want.HasStartTime != got.HasStartTime || want.StartTime != got.StartTime {
		return fmt.Errorf("resource %s: startTime is immutable after create", addr)
	}
	if want.HasEndTime != got.HasEndTime {
		return fmt.Errorf("resource %s: endTime cannot be added or cleared after create", addr)
	}
	if want.HasBidAmount != got.HasBidAmount {
		return fmt.Errorf("resource %s: bidAmount cannot be added or cleared after create", addr)
	}
	return nil
}

func (p *Provider) destroyAdSet(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	id, bound, err := boundIdentity(res)
	if err != nil {
		return provider.DestroyResult{}, err
	}
	if !bound {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: missing persisted identity", res.Address)
	}
	_, err = p.readAdSetByID(ctx, res, id)
	if errors.Is(err, provider.ErrNotFound) {
		return provider.DestroyResult{Status: provider.DestroyStatusAlreadyAbsent}, nil
	}
	if err != nil {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: %w", res.Address, err)
	}
	c, err := p.Client()
	if err != nil {
		return provider.DestroyResult{}, err
	}
	var result map[string]any
	if err := c.Delete(ctx, id, nil, &result); err != nil {
		if client.IsNotFound(err) {
			return provider.DestroyResult{Status: provider.DestroyStatusAlreadyAbsent}, nil
		}
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: %w", res.Address, err)
	}
	if success, ok := result["success"].(bool); ok && !success {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: API did not report success", res.Address)
	}
	_, err = p.readAdSetByID(ctx, res, id)
	if errors.Is(err, provider.ErrNotFound) {
		return provider.DestroyResult{Status: provider.DestroyStatusRemoved}, nil
	}
	if err != nil {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: DELETE succeeded but confirming terminal state failed: %w", res.Address, err)
	}
	return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: ad set %s is still active after DELETE", res.Address, id)
}

func (p *Provider) ensureAdSetAccount(accountID string) error {
	c, err := p.Client()
	if err != nil {
		return err
	}
	got := strings.TrimPrefix(strings.TrimSpace(accountID), "act_")
	if got != "" && got != strings.TrimPrefix(c.AdAccountID(), "act_") {
		return fmt.Errorf("ad set belongs to ad account %s, not the configured %s", accountID, c.AdAccountID())
	}
	return nil
}
func stringMap(v any) (map[string]any, bool) {
	switch x := v.(type) {
	case map[string]any:
		return x, true
	case resource.Attributes:
		return map[string]any(x), true
	default:
		return nil, false
	}
}
func decodeJSONObject(raw json.RawMessage) (map[string]any, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}
func normalizeInstagramPosition(s string) (string, bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch s {
	case "FEED", "STREAM":
		return "FEED", true
	case "STORIES", "STORY":
		return "STORIES", true
	case "REELS":
		return "REELS", true
	default:
		return s, false
	}
}

func normalizeCountry(s string) (string, bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	return s, len(s) == 2 && s[0] >= 'A' && s[0] <= 'Z' && s[1] >= 'A' && s[1] <= 'Z'
}
func normalizedStringList(addr resource.Address, path string, v any, required bool, normalize func(string) (string, bool)) ([]string, error) {
	if v == nil {
		if required {
			return nil, fmt.Errorf("resource %s: %s is required", addr, path)
		}
		return nil, nil
	}
	items, err := anySlice(v)
	if err != nil {
		return nil, fmt.Errorf("resource %s: %s must be a list", addr, path)
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for i, item := range items {
		s, e := coerceString(item)
		if e != nil {
			return nil, fmt.Errorf("resource %s: %s[%d] must be a string", addr, path, i)
		}
		s, ok := normalize(strings.TrimSpace(s))
		if !ok {
			return nil, fmt.Errorf("resource %s: %s[%d] has unsupported value %q", addr, path, i, item)
		}
		if _, dup := seen[s]; dup {
			return nil, fmt.Errorf("resource %s: %s contains duplicate value %q", addr, path, s)
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	if required && len(out) == 0 {
		return nil, fmt.Errorf("resource %s: %s must not be empty", addr, path)
	}
	return out, nil
}
func normalizedIntList(addr resource.Address, path string, v any) ([]int64, error) {
	if v == nil {
		return nil, nil
	}
	items, err := anySlice(v)
	if err != nil {
		return nil, fmt.Errorf("resource %s: %s must be a list", addr, path)
	}
	seen := map[int64]struct{}{}
	out := make([]int64, 0, len(items))
	for i, item := range items {
		n, e := coerceFloat(item)
		if e != nil || n <= 0 || n != math.Trunc(n) || n > math.MaxInt64 {
			return nil, fmt.Errorf("resource %s: %s[%d] must be a positive integer", addr, path, i)
		}
		value := int64(n)
		if _, dup := seen[value]; dup {
			return nil, fmt.Errorf("resource %s: %s contains duplicate value %d", addr, path, value)
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}
func optionalWhole(v any, def, min, max int64) (int64, error) {
	if v == nil {
		return def, nil
	}
	n, err := coerceFloat(v)
	if err != nil || n != math.Trunc(n) || n < float64(min) || n > float64(max) {
		return 0, fmt.Errorf("must be a whole number from %d through %d", min, max)
	}
	return int64(n), nil
}
func anySlice(v any) ([]any, error) {
	if v == nil {
		return nil, nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, fmt.Errorf("must be a list")
	}
	out := make([]any, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		out[i] = rv.Index(i).Interface()
	}
	return out, nil
}
func remoteStrings(v any) ([]string, error) {
	items, err := anySlice(v)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, e := coerceString(item)
		if e != nil {
			return nil, e
		}
		out = append(out, s)
	}
	return out, nil
}
func remoteInt64s(v any) ([]int64, error) {
	if v == nil {
		return nil, nil
	}
	items, err := anySlice(v)
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(items))
	for _, item := range items {
		n, e := coerceFloat(item)
		if e != nil || n != math.Trunc(n) {
			return nil, fmt.Errorf("must contain integers")
		}
		out = append(out, int64(n))
	}
	return out, nil
}
func int64sToAny(in []int64) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}
