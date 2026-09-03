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

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/meta/client"
)

const (
	campaignFields = "id,account_id,name,objective,status,configured_status,effective_status,special_ad_categories,buying_type,daily_budget,lifetime_budget,bid_strategy,is_adset_budget_sharing_enabled"

	campaignStatusActive   = "ACTIVE"
	campaignStatusPaused   = "PAUSED"
	campaignStatusDeleted  = "DELETED"
	campaignStatusArchived = "ARCHIVED"
	campaignBuyingAuction  = "AUCTION"
)

var (
	supportedCampaignAttrs = map[string]struct{}{
		AttrName: {}, AttrObjective: {}, AttrStatus: {}, AttrSpecialAdCategories: {},
		AttrBuyingType: {}, AttrDailyBudget: {}, AttrLifetimeBudget: {}, AttrBidStrategy: {},
		AttrAdSetBudgetSharing: {},
	}
	computedCampaignAttrs = map[string]struct{}{
		"id": {}, "campaignId": {}, "account_id": {}, "accountId": {},
		"configured_status": {}, "configuredStatus": {}, "effective_status": {}, "effectiveStatus": {},
		"budget_remaining": {}, "budgetRemaining": {}, "spend_cap": {}, "spendCap": {},
		"created_time": {}, "createdTime": {}, "updated_time": {}, "updatedTime": {},
		"is_adset_budget_sharing_enabled": {}, "isAdSetBudgetSharingEnabled": {},
	}
	campaignObjectives = map[string]struct{}{
		"OUTCOME_APP_PROMOTION": {}, "OUTCOME_AWARENESS": {}, "OUTCOME_ENGAGEMENT": {},
		"OUTCOME_LEADS": {}, "OUTCOME_SALES": {}, "OUTCOME_TRAFFIC": {},
	}
	campaignStatuses            = map[string]struct{}{campaignStatusActive: {}, campaignStatusPaused: {}}
	campaignSpecialAdCategories = map[string]struct{}{
		"CREDIT": {}, "EMPLOYMENT": {}, "FINANCIAL_PRODUCTS_SERVICES": {}, "HOUSING": {},
		"ISSUES_ELECTIONS_POLITICS": {}, "ONLINE_GAMBLING_AND_GAMING": {},
	}
	campaignBidStrategies = map[string]struct{}{
		"COST_CAP": {}, "LOWEST_COST_WITHOUT_CAP": {}, "LOWEST_COST_WITH_BID_CAP": {},
		"LOWEST_COST_WITH_MIN_ROAS": {},
	}
)

type campaign struct {
	ID                  string   `json:"id"`
	AccountID           string   `json:"account_id"`
	Name                string   `json:"name"`
	Objective           string   `json:"objective"`
	Status              string   `json:"status"`
	ConfiguredStatus    string   `json:"configured_status"`
	EffectiveStatus     string   `json:"effective_status"`
	SpecialAdCategories []string `json:"special_ad_categories"`
	BuyingType          string   `json:"buying_type"`
	DailyBudget         any      `json:"daily_budget"`
	LifetimeBudget      any      `json:"lifetime_budget"`
	BidStrategy         string   `json:"bid_strategy"`
	AdSetBudgetSharing  bool     `json:"is_adset_budget_sharing_enabled"`
}

type normalizedCampaign struct {
	Name                string
	Objective           string
	Status              string
	SpecialAdCategories []string
	BuyingType          string
	DailyBudget         int64
	HasDailyBudget      bool
	LifetimeBudget      int64
	HasLifetimeBudget   bool
	BidStrategy         string
	HasBidStrategy      bool
	AdSetBudgetSharing  bool
}

func (p *Provider) validateCampaign(res resource.Resource) error {
	if err := p.requireAdAccount(); err != nil {
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
		return fmt.Errorf("resource %s: unsupported attribute %q; meta.campaign supports %s", res.Address, key, joinSorted(keys(supportedCampaignAttrs)))
	}
	if _, err := normalizeCampaign(res); err != nil {
		return err
	}
	if _, _, err := boundIdentity(res); err != nil {
		return err
	}
	return nil
}

func (p *Provider) readCampaign(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateCampaign(res); err != nil {
		return resource.RemoteResource{}, err
	}
	id, bound, err := boundIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if !bound {
		return resource.RemoteResource{}, fmt.Errorf("meta: read %s: %w", res.Address, provider.ErrNotFound)
	}
	live, err := p.readCampaignByID(ctx, res.Address, id)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: read %s: %w", res.Address, err)
	}
	return live, nil
}

func (p *Provider) createCampaign(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateCampaign(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}
	normalized, _ := normalizeCampaign(res)
	form, err := campaignForm(normalized)
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
	if err := c.Post(ctx, c.AdAccountID()+"/campaigns", form, &created); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: %w", res.Address, err)
	}
	id, err := normalizeObjectID(created.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: API returned an invalid id: %w", res.Address, err)
	}
	live, err := p.readCampaignByID(ctx, res.Address, id)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s succeeded but refreshing campaign %q failed: %w", res.Address, id, err)
	}
	return live, nil
}

func (p *Provider) updateCampaign(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateCampaign(desired); err != nil {
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
	want, err := normalizeCampaign(desired)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	got, err := normalizeCampaign(resource.Resource{Address: desired.Address, Attributes: actual.Attributes})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: live campaign is invalid: %w", desired.Address, err)
	}
	if err := validateCampaignTransition(desired.Address, want, got); err != nil {
		return resource.RemoteResource{}, err
	}
	form := url.Values{}
	if want.Name != got.Name {
		form.Set("name", want.Name)
	}
	if want.Status != got.Status {
		form.Set("status", want.Status)
	}
	if !reflect.DeepEqual(want.SpecialAdCategories, got.SpecialAdCategories) {
		raw, _ := json.Marshal(want.SpecialAdCategories)
		form.Set("special_ad_categories", string(raw))
	}
	if want.HasDailyBudget && want.DailyBudget != got.DailyBudget {
		form.Set("daily_budget", strconv.FormatInt(want.DailyBudget, 10))
	}
	if want.HasLifetimeBudget && want.LifetimeBudget != got.LifetimeBudget {
		form.Set("lifetime_budget", strconv.FormatInt(want.LifetimeBudget, 10))
	}
	if want.HasBidStrategy && (!got.HasBidStrategy || want.BidStrategy != got.BidStrategy) {
		form.Set("bid_strategy", want.BidStrategy)
	}
	if !want.HasDailyBudget && !want.HasLifetimeBudget && want.AdSetBudgetSharing != got.AdSetBudgetSharing {
		form.Set("is_adset_budget_sharing_enabled", strconv.FormatBool(want.AdSetBudgetSharing))
	}
	if len(form) == 0 {
		return p.readCampaignByID(ctx, desired.Address, actual.Identity.ID)
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
	live, err := p.readCampaignByID(ctx, desired.Address, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s succeeded but refreshing campaign %q failed: %w", desired.Address, actual.Identity.ID, err)
	}
	return live, nil
}

func (p *Provider) importCampaign(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	id, err := p.canonicalCustomConversionImportID(addr, rawID)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireConfig(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: import %s: %w", addr, err)
	}
	live, err := p.readCampaignByID(ctx, addr, id)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("meta: import %s: remote campaign %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("meta: import %s: %w", addr, err)
	}
	return live, nil
}

func (p *Provider) normalizeCampaignComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	want, err := normalizeCampaign(desired)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	wantAttrs := campaignAttributes(want)
	if live == nil {
		return wantAttrs, nil, nil
	}
	if id, bound, err := boundIdentity(desired); err != nil {
		return nil, nil, err
	} else if bound && (live.Identity.IsZero() || live.Identity.ID != id) {
		return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, id, live.Identity.ID)
	}
	got, err := normalizeCampaign(resource.Resource{Address: desired.Address, Attributes: live.Attributes})
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: live campaign is invalid: %w", desired.Address, err)
	}
	if err := validateCampaignTransition(desired.Address, want, got); err != nil {
		return nil, nil, err
	}
	return wantAttrs, campaignAttributes(got), nil
}

func (p *Provider) readCampaignByID(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	var item campaign
	if err := c.Get(ctx, id, url.Values{"fields": {campaignFields}}, &item); err != nil {
		if client.IsNotFound(err) {
			return resource.RemoteResource{}, provider.ErrNotFound
		}
		return resource.RemoteResource{}, err
	}
	status := strings.ToUpper(strings.TrimSpace(item.ConfiguredStatus))
	if status == "" {
		status = strings.ToUpper(strings.TrimSpace(item.Status))
	}
	if status == campaignStatusDeleted || status == campaignStatusArchived {
		return resource.RemoteResource{}, provider.ErrNotFound
	}
	live, err := p.remoteCampaign(addr, item)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	return p.rememberLive(live), nil
}

func (p *Provider) remoteCampaign(addr resource.Address, item campaign) (resource.RemoteResource, error) {
	id, err := normalizeObjectID(item.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote campaign id is invalid: %w", err)
	}
	if err := p.ensureCampaignAccount(item.AccountID); err != nil {
		return resource.RemoteResource{}, err
	}
	status := strings.ToUpper(strings.TrimSpace(item.ConfiguredStatus))
	if status == "" {
		status = strings.ToUpper(strings.TrimSpace(item.Status))
	}
	attrs := resource.Attributes{
		AttrName: strings.TrimSpace(item.Name), AttrObjective: strings.ToUpper(strings.TrimSpace(item.Objective)),
		AttrStatus: status, AttrSpecialAdCategories: stringsToAny(normalizeCategories(item.SpecialAdCategories)),
		AttrBuyingType:         strings.ToUpper(strings.TrimSpace(item.BuyingType)),
		AttrAdSetBudgetSharing: item.AdSetBudgetSharing,
	}
	if attrs[AttrBuyingType] == "" {
		attrs[AttrBuyingType] = campaignBuyingAuction
	}
	if budget, set, err := remoteBudget(item.DailyBudget); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote campaign %s has invalid daily_budget: %w", id, err)
	} else if set {
		attrs[AttrDailyBudget] = budget
	}
	if budget, set, err := remoteBudget(item.LifetimeBudget); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote campaign %s has invalid lifetime_budget: %w", id, err)
	} else if set {
		attrs[AttrLifetimeBudget] = budget
	}
	if bid := strings.ToUpper(strings.TrimSpace(item.BidStrategy)); bid != "" {
		attrs[AttrBidStrategy] = bid
	}
	res := resource.Resource{Address: addr, Attributes: attrs}
	if _, err := normalizeCampaign(res); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote campaign %s cannot be represented: %w", id, err)
	}
	computed := resource.Attributes{OutputCampaignID: id}
	if effective := strings.ToUpper(strings.TrimSpace(item.EffectiveStatus)); effective != "" {
		computed["effectiveStatus"] = effective
	}
	return resource.RemoteResource{Address: addr, Identity: resource.Identity{ID: id}, Attributes: attrs, Computed: computed}, nil
}

func normalizeCampaign(res resource.Resource) (normalizedCampaign, error) {
	name, err := requiredString(res, AttrName)
	if err != nil {
		return normalizedCampaign{}, err
	}
	objective, err := campaignEnum(res, AttrObjective, campaignObjectives, true, "")
	if err != nil {
		return normalizedCampaign{}, err
	}
	status, err := campaignEnum(res, AttrStatus, campaignStatuses, false, campaignStatusPaused)
	if err != nil {
		return normalizedCampaign{}, err
	}
	buyingType, err := campaignEnum(res, AttrBuyingType, map[string]struct{}{campaignBuyingAuction: {}}, false, campaignBuyingAuction)
	if err != nil {
		return normalizedCampaign{}, fmt.Errorf("%w; RESERVED campaigns require a materially different schema and are out of scope", err)
	}
	categories, err := requiredCategories(res)
	if err != nil {
		return normalizedCampaign{}, err
	}
	daily, hasDaily, err := optionalBudget(res, AttrDailyBudget)
	if err != nil {
		return normalizedCampaign{}, err
	}
	lifetime, hasLifetime, err := optionalBudget(res, AttrLifetimeBudget)
	if err != nil {
		return normalizedCampaign{}, err
	}
	if hasDaily && hasLifetime {
		return normalizedCampaign{}, fmt.Errorf("resource %s: attributes %q and %q are mutually exclusive", res.Address, AttrDailyBudget, AttrLifetimeBudget)
	}
	bid, err := campaignEnum(res, AttrBidStrategy, campaignBidStrategies, false, "")
	if err != nil {
		return normalizedCampaign{}, err
	}
	_, hasBid := res.Attributes[AttrBidStrategy]
	if hasBid && !hasDaily && !hasLifetime {
		return normalizedCampaign{}, fmt.Errorf("resource %s: attribute %q requires %q or %q at campaign level", res.Address, AttrBidStrategy, AttrDailyBudget, AttrLifetimeBudget)
	}
	adSetBudgetSharing, err := optionalBoolDefault(res, AttrAdSetBudgetSharing, false)
	if err != nil {
		return normalizedCampaign{}, err
	}
	if adSetBudgetSharing && (hasDaily || hasLifetime) {
		return normalizedCampaign{}, fmt.Errorf("resource %s: attribute %q cannot be true with a campaign-level budget", res.Address, AttrAdSetBudgetSharing)
	}
	return normalizedCampaign{Name: name, Objective: objective, Status: status, SpecialAdCategories: categories, BuyingType: buyingType,
		DailyBudget: daily, HasDailyBudget: hasDaily, LifetimeBudget: lifetime, HasLifetimeBudget: hasLifetime,
		BidStrategy: bid, HasBidStrategy: hasBid, AdSetBudgetSharing: adSetBudgetSharing}, nil
}

func optionalBoolDefault(res resource.Resource, key string, defaultValue bool) (bool, error) {
	v, ok := res.Attributes[key]
	if !ok {
		return defaultValue, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("resource %s: attribute %q must be a boolean", res.Address, key)
	}
	return b, nil
}

func campaignEnum(res resource.Resource, key string, allowed map[string]struct{}, required bool, defaultValue string) (string, error) {
	v, ok := res.Attributes[key]
	if !ok {
		if required {
			return "", fmt.Errorf("resource %s: missing required attribute %q", res.Address, key)
		}
		return defaultValue, nil
	}
	s, err := coerceString(v)
	if err != nil || strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("resource %s: attribute %q must be a non-empty string", res.Address, key)
	}
	s = strings.ToUpper(strings.TrimSpace(s))
	if _, ok := allowed[s]; !ok {
		return "", fmt.Errorf("resource %s: attribute %q must be one of %s", res.Address, key, joinSorted(keys(allowed)))
	}
	return s, nil
}

func requiredCategories(res resource.Resource) ([]string, error) {
	v, ok := res.Attributes[AttrSpecialAdCategories]
	if !ok {
		return nil, fmt.Errorf("resource %s: missing required attribute %q; use an empty list when no special-ad category applies", res.Address, AttrSpecialAdCategories)
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() || (rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array) {
		return nil, fmt.Errorf("resource %s: attribute %q must be a list", res.Address, AttrSpecialAdCategories)
	}
	set := map[string]struct{}{}
	for i := 0; i < rv.Len(); i++ {
		s, err := coerceString(rv.Index(i).Interface())
		if err != nil || strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf("resource %s: attribute %q[%d] must be a non-empty string", res.Address, AttrSpecialAdCategories, i)
		}
		s = strings.ToUpper(strings.TrimSpace(s))
		if s == "NONE" {
			if rv.Len() != 1 {
				return nil, fmt.Errorf("resource %s: NONE cannot be combined with another %q value", res.Address, AttrSpecialAdCategories)
			}
			return []string{}, nil
		}
		if _, ok := campaignSpecialAdCategories[s]; !ok {
			return nil, fmt.Errorf("resource %s: attribute %q[%d] must be one of %s", res.Address, AttrSpecialAdCategories, i, joinSorted(keys(campaignSpecialAdCategories)))
		}
		if _, duplicate := set[s]; duplicate {
			return nil, fmt.Errorf("resource %s: attribute %q contains duplicate value %q", res.Address, AttrSpecialAdCategories, s)
		}
		set[s] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out, nil
}

func optionalBudget(res resource.Resource, key string) (int64, bool, error) {
	v, ok := res.Attributes[key]
	if !ok {
		return 0, false, nil
	}
	n, err := coerceFloat(v)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) || n <= 0 || n > math.MaxInt64 {
		return 0, true, fmt.Errorf("resource %s: attribute %q must be a positive whole number in the ad account currency's smallest unit", res.Address, key)
	}
	return int64(n), true, nil
}

func remoteBudget(v any) (int64, bool, error) {
	if v == nil || v == "" {
		return 0, false, nil
	}
	n, err := coerceFloat(v)
	if err != nil {
		return 0, true, err
	}
	if n == 0 {
		return 0, false, nil
	}
	if n < 0 || n != math.Trunc(n) || n > math.MaxInt64 {
		return 0, true, fmt.Errorf("must be a positive whole number")
	}
	return int64(n), true, nil
}

func campaignAttributes(c normalizedCampaign) resource.Attributes {
	out := resource.Attributes{AttrName: c.Name, AttrObjective: c.Objective, AttrStatus: c.Status,
		AttrSpecialAdCategories: stringsToAny(c.SpecialAdCategories), AttrBuyingType: c.BuyingType,
		AttrAdSetBudgetSharing: c.AdSetBudgetSharing}
	if c.HasDailyBudget {
		out[AttrDailyBudget] = c.DailyBudget
	}
	if c.HasLifetimeBudget {
		out[AttrLifetimeBudget] = c.LifetimeBudget
	}
	if c.HasBidStrategy {
		out[AttrBidStrategy] = c.BidStrategy
	}
	return out
}

func campaignForm(c normalizedCampaign) (url.Values, error) {
	form := url.Values{}
	form.Set("name", c.Name)
	form.Set("objective", c.Objective)
	form.Set("status", c.Status)
	form.Set("buying_type", c.BuyingType)
	raw, err := json.Marshal(c.SpecialAdCategories)
	if err != nil {
		return nil, err
	}
	form.Set("special_ad_categories", string(raw))
	if c.HasDailyBudget {
		form.Set("daily_budget", strconv.FormatInt(c.DailyBudget, 10))
	}
	if c.HasLifetimeBudget {
		form.Set("lifetime_budget", strconv.FormatInt(c.LifetimeBudget, 10))
	}
	if c.HasBidStrategy {
		form.Set("bid_strategy", c.BidStrategy)
	}
	if !c.HasDailyBudget && !c.HasLifetimeBudget {
		form.Set("is_adset_budget_sharing_enabled", strconv.FormatBool(c.AdSetBudgetSharing))
	}
	return form, nil
}

func validateCampaignTransition(addr resource.Address, want, got normalizedCampaign) error {
	if want.Objective != got.Objective {
		return fmt.Errorf("resource %s: objective is immutable/unsafe after create; create a new meta.campaign instead", addr)
	}
	if want.BuyingType != got.BuyingType {
		return fmt.Errorf("resource %s: buyingType is immutable after create; create a new meta.campaign instead", addr)
	}
	if want.HasDailyBudget != got.HasDailyBudget || want.HasLifetimeBudget != got.HasLifetimeBudget {
		return fmt.Errorf("resource %s: campaign budget ownership/type cannot be added, removed, or switched in place; create a new meta.campaign (and move ad sets explicitly) instead", addr)
	}
	if got.HasBidStrategy && !want.HasBidStrategy {
		return fmt.Errorf("resource %s: bidStrategy cannot be cleared deterministically through the Marketing API", addr)
	}
	return nil
}

func (p *Provider) ensureCampaignAccount(accountID string) error {
	c, err := p.Client()
	if err != nil {
		return err
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil
	}
	if strings.TrimPrefix(accountID, "act_") != strings.TrimPrefix(c.AdAccountID(), "act_") {
		return fmt.Errorf("campaign belongs to ad account %s, not the configured %s", accountID, c.AdAccountID())
	}
	return nil
}

func normalizeCategories(in []string) []string {
	set := map[string]struct{}{}
	for _, s := range in {
		s = strings.ToUpper(strings.TrimSpace(s))
		if s != "" && s != "NONE" {
			set[s] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func stringsToAny(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

func (p *Provider) destroyCampaign(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	id, bound, err := boundIdentity(res)
	if err != nil {
		return provider.DestroyResult{}, err
	}
	if !bound {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: missing persisted identity", res.Address)
	}
	_, err = p.readCampaignByID(ctx, res.Address, id)
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
	_, err = p.readCampaignByID(ctx, res.Address, id)
	if errors.Is(err, provider.ErrNotFound) {
		return provider.DestroyResult{Status: provider.DestroyStatusRemoved}, nil
	}
	if err != nil {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: DELETE succeeded but confirming terminal state failed: %w", res.Address, err)
	}
	return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: campaign %s is still active after DELETE", res.Address, id)
}
