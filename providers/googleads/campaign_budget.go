package googleads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

const (
	// TypeCampaignBudget is the Google Ads campaign-budget type used in
	// addresses such as googleads.campaign_budget.brand.
	TypeCampaignBudget = "campaign_budget"

	// AttrAmount is the daily budget in account-currency units. Agoraform
	// converts it to Google Ads amount_micros (1 unit = 1_000_000 micros).
	AttrAmount = "amount"
	// AttrDeliveryMethod is STANDARD or ACCELERATED.
	AttrDeliveryMethod = "deliveryMethod"
	// AttrExplicitlyShared distinguishes shared portfolio budgets from
	// dedicated single-campaign budgets.
	AttrExplicitlyShared = "explicitlyShared"

	campaignBudgetPeriodDaily    = "DAILY"
	campaignBudgetTypeStandard   = "STANDARD"
	deliveryMethodStandard       = "STANDARD"
	deliveryMethodAccelerated    = "ACCELERATED"
	microsPerCurrencyUnit        = int64(1_000_000)
	campaignBudgetsCollection    = "campaignBudgets"
	maxAmountFractionalDigits    = 6
	floatMicrosEqualityTolerance = 1e-3
)

var (
	supportedCampaignBudgetAttrs = map[string]struct{}{
		AttrName:             {},
		AttrAmount:           {},
		AttrDeliveryMethod:   {},
		AttrExplicitlyShared: {},
	}

	computedCampaignBudgetAttrs = map[string]struct{}{
		"id":                  {},
		"resourceName":        {},
		"resource_name":       {},
		"amountMicros":        {},
		"amount_micros":       {},
		"period":              {},
		"type":                {},
		"status":              {},
		"referenceCount":      {},
		"reference_count":     {},
		"totalAmount":         {},
		"total_amount":        {},
		"totalAmountMicros":   {},
		"total_amount_micros": {},
	}

	campaignBudgetDeliveryMethods = map[string]struct{}{
		deliveryMethodStandard:    {},
		deliveryMethodAccelerated: {},
	}

	campaignBudgetSelect = strings.Join([]string{
		"SELECT",
		"campaign_budget.resource_name,",
		"campaign_budget.id,",
		"campaign_budget.name,",
		"campaign_budget.amount_micros,",
		"campaign_budget.delivery_method,",
		"campaign_budget.explicitly_shared,",
		"campaign_budget.period,",
		"campaign_budget.type,",
		"campaign_budget.status,",
		"campaign_budget.reference_count",
		"FROM campaign_budget",
	}, " ")
)

func (p *Provider) validateCampaignBudget(res resource.Resource) error {
	if err := p.requireCustomerID(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}

	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedCampaignBudgetAttrs[key]; ok {
			continue
		}
		if _, computed := computedCampaignBudgetAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; googleads.campaign_budget supports %s", res.Address, key, joinSorted(keys(supportedCampaignBudgetAttrs)))
	}

	name, err := requiredString(res, AttrName)
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("resource %s: attribute %q must be a non-empty string", res.Address, AttrName)
	}

	if _, err := requiredCampaignBudgetAmountMicros(res); err != nil {
		return err
	}

	if _, err := requiredCampaignBudgetExplicitlyShared(res); err != nil {
		return err
	}

	if _, _, err := optionalEnum(res, AttrDeliveryMethod, campaignBudgetDeliveryMethods); err != nil {
		return err
	}

	if _, _, err := boundCampaignBudgetIdentity(res); err != nil {
		return err
	}

	return nil
}

func (p *Provider) readCampaignBudget(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateCampaignBudget(res); err != nil {
		return resource.RemoteResource{}, err
	}

	id, bound, err := boundCampaignBudgetIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if bound {
		live, err := p.readCampaignBudgetByID(ctx, res.Address, id)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return live, nil
	}

	name, _, _ := optionalString(res, AttrName)
	matches, err := p.queryCampaignBudgets(ctx, "campaign_budget.name = "+gaqlString(name))
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, provider.ErrNotFound)
	case 1:
		live, err := remoteCampaignBudget(res.Address, matches[0])
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return live, nil
	default:
		ids := make([]string, 0, len(matches))
		for _, item := range matches {
			ids = append(ids, item.ID)
		}
		sort.Strings(ids)
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: multiple remote campaign budgets named %q (ids %s); names must be unique", res.Address, name, strings.Join(ids, ", "))
	}
}

func (p *Provider) createCampaignBudget(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateCampaignBudget(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundCampaignBudgetIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}

	budget, _, err := campaignBudgetMutateBody(res.Attributes, "")
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}
	budget["period"] = campaignBudgetPeriodDaily
	budget["type"] = campaignBudgetTypeStandard

	raw, err := c.Mutate(ctx, campaignBudgetsCollection, []map[string]any{
		{"create": budget},
	})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}
	id, err := parseCampaignBudgetMutateID(raw, c.CustomerID())
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}

	live, err := p.readCampaignBudgetByID(ctx, res.Address, id)
	if err == nil {
		return live, nil
	}
	fallback, ferr := remoteCampaignBudgetFromDesired(res, id, c.CustomerID())
	if ferr != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s succeeded but refreshing campaign budget %q failed: %w", res.Address, id, err)
	}
	return fallback, nil
}

func (p *Provider) updateCampaignBudget(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateCampaignBudget(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: missing remote identity", desired.Address)
	}
	if id, bound, err := boundCampaignBudgetIdentity(desired); err != nil {
		return resource.RemoteResource{}, err
	} else if bound && id != actual.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: persisted identity %q does not match planned remote identity %q", desired.Address, id, actual.Identity.ID)
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}

	resourceName := campaignBudgetResourceName(c.CustomerID(), actual.Identity.ID)
	budget, mask, err := campaignBudgetMutateBody(desired.Attributes, resourceName)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}
	if len(mask) == 0 {
		live, err := p.readCampaignBudgetByID(ctx, desired.Address, actual.Identity.ID)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
		}
		return live, nil
	}

	_, err = c.Mutate(ctx, campaignBudgetsCollection, []map[string]any{
		{
			"updateMask": strings.Join(mask, ","),
			"update":     budget,
		},
	})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}

	live, err := p.readCampaignBudgetByID(ctx, desired.Address, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s succeeded but refreshing campaign budget %q failed: %w", desired.Address, actual.Identity.ID, err)
	}
	return live, nil
}

func (p *Provider) importCampaignBudget(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	id, err := p.canonicalCampaignBudgetImportID(addr, rawID)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireCustomerID(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	live, err := p.readCampaignBudgetByID(ctx, addr, id)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: remote campaign budget %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	return live, nil
}

func (p *Provider) normalizeCampaignBudgetComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	want, err := comparableCampaignBudget(desired.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if live == nil {
		return want, nil, nil
	}
	if id, bound, err := boundCampaignBudgetIdentity(desired); err != nil {
		return nil, nil, err
	} else if bound {
		if live.Identity.IsZero() || live.Identity.ID != id {
			return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, id, live.Identity.ID)
		}
	}
	gotFull, err := comparableCampaignBudget(live.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	got := resource.Attributes{}
	for key := range want {
		if v, ok := gotFull[key]; ok {
			got[key] = v
			continue
		}
		if v, ok := live.Attributes[key]; ok {
			got[key] = v
		}
	}
	return want, got, nil
}

func (p *Provider) readCampaignBudgetByID(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	matches, err := p.queryCampaignBudgets(ctx, "campaign_budget.id = "+id)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, provider.ErrNotFound
	case 1:
		return remoteCampaignBudget(addr, matches[0])
	default:
		return resource.RemoteResource{}, fmt.Errorf("multiple remote campaign budgets returned for id %s", id)
	}
}

func (p *Provider) queryCampaignBudgets(ctx context.Context, where string) ([]campaignBudgetData, error) {
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	query := campaignBudgetSelect
	if strings.TrimSpace(where) != "" {
		query += " WHERE " + where
	}
	rows, err := c.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]campaignBudgetData, 0, len(rows))
	for _, row := range rows {
		item, err := decodeCampaignBudgetRow(row, c.CustomerID())
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

type campaignBudgetData struct {
	ResourceName     string
	ID               string
	Name             string
	AmountMicros     int64
	DeliveryMethod   string
	ExplicitlyShared *bool
	Period           string
	Type             string
	Status           string
	ReferenceCount   *int64
}

type campaignBudgetJSON struct {
	ResourceName     string          `json:"resourceName"`
	ID               json.Number     `json:"id"`
	Name             string          `json:"name"`
	AmountMicros     json.RawMessage `json:"amountMicros"`
	DeliveryMethod   string          `json:"deliveryMethod"`
	ExplicitlyShared *bool           `json:"explicitlyShared"`
	Period           string          `json:"period"`
	Type             string          `json:"type"`
	Status           string          `json:"status"`
	ReferenceCount   json.Number     `json:"referenceCount"`
}

func decodeCampaignBudgetRow(raw json.RawMessage, configuredCustomerID string) (campaignBudgetData, error) {
	malformed := func(detail string) (campaignBudgetData, error) {
		if detail == "" {
			return campaignBudgetData{}, fmt.Errorf("malformed campaign budget result")
		}
		return campaignBudgetData{}, fmt.Errorf("malformed campaign budget result: %s", detail)
	}

	var envelope struct {
		CampaignBudget json.RawMessage `json:"campaignBudget"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.CampaignBudget) == 0 {
		return malformed("")
	}

	var body campaignBudgetJSON
	if err := json.Unmarshal(envelope.CampaignBudget, &body); err != nil {
		return malformed("")
	}

	resourceName := strings.TrimSpace(body.ResourceName)
	resourceCustomerID, resourceID, ok := splitCampaignBudgetResourceName(resourceName)
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

	micros, err := parseMicrosJSON(body.AmountMicros)
	if err != nil || micros <= 0 {
		return malformed("invalid amountMicros")
	}

	period := normalizeEnum(body.Period)
	if period == "" {
		period = campaignBudgetPeriodDaily
	}
	if period != campaignBudgetPeriodDaily {
		return campaignBudgetData{}, fmt.Errorf("campaign budget %s has period %s; googleads.campaign_budget only manages DAILY Search campaign budgets", id, period)
	}

	typeName := normalizeEnum(body.Type)
	if typeName == "" {
		typeName = campaignBudgetTypeStandard
	}
	if typeName != campaignBudgetTypeStandard {
		return campaignBudgetData{}, fmt.Errorf("campaign budget %s has type %s; googleads.campaign_budget only manages STANDARD daily Search campaign budgets", id, typeName)
	}

	delivery := normalizeEnum(body.DeliveryMethod)
	if delivery != "" {
		if _, ok := campaignBudgetDeliveryMethods[delivery]; !ok {
			return campaignBudgetData{}, fmt.Errorf("campaign budget %s has delivery method %s; googleads.campaign_budget supports %s", id, delivery, joinSorted(keys(campaignBudgetDeliveryMethods)))
		}
	}

	item := campaignBudgetData{
		ResourceName:     resourceName,
		ID:               id,
		Name:             body.Name,
		AmountMicros:     micros,
		DeliveryMethod:   delivery,
		ExplicitlyShared: body.ExplicitlyShared,
		Period:           period,
		Type:             typeName,
		Status:           normalizeEnum(body.Status),
	}
	if n, err := parseOptionalInt64(body.ReferenceCount); err == nil {
		item.ReferenceCount = n
	}
	return item, nil
}

func remoteCampaignBudget(addr resource.Address, item campaignBudgetData) (resource.RemoteResource, error) {
	attrs := resource.Attributes{
		AttrName:   item.Name,
		AttrAmount: amountFromMicros(item.AmountMicros),
	}
	if item.DeliveryMethod != "" {
		attrs[AttrDeliveryMethod] = item.DeliveryMethod
	}
	if item.ExplicitlyShared != nil {
		attrs[AttrExplicitlyShared] = *item.ExplicitlyShared
	} else {
		attrs[AttrExplicitlyShared] = true
	}

	computed := resource.Attributes{}
	setComputed(computed, "id", item.ID)
	setComputed(computed, "resourceName", item.ResourceName)
	computed["amountMicros"] = item.AmountMicros
	setComputed(computed, "period", item.Period)
	setComputed(computed, "type", item.Type)
	setComputed(computed, "status", item.Status)
	if item.ReferenceCount != nil {
		computed["referenceCount"] = *item.ReferenceCount
	}

	return resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: item.ID},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func remoteCampaignBudgetFromDesired(res resource.Resource, id, customerID string) (resource.RemoteResource, error) {
	attrs, err := comparableCampaignBudget(res.Attributes)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	micros, err := amountToMicros(attrs[AttrAmount])
	if err != nil {
		return resource.RemoteResource{}, err
	}
	computed := resource.Attributes{
		"id":           id,
		"resourceName": campaignBudgetResourceName(customerID, id),
		"amountMicros": micros,
		"period":       campaignBudgetPeriodDaily,
		"type":         campaignBudgetTypeStandard,
	}
	return resource.RemoteResource{
		Address:    res.Address,
		Identity:   resource.Identity{ID: id},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func comparableCampaignBudget(attrs resource.Attributes) (resource.Attributes, error) {
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	name, err := coerceString(attrs[AttrName])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrName, err)
	}
	micros, err := amountToMicros(attrs[AttrAmount])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrAmount, err)
	}
	shared, err := coerceBool(attrs[AttrExplicitlyShared])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrExplicitlyShared, err)
	}
	out := resource.Attributes{
		AttrName:             name,
		AttrAmount:           amountFromMicros(micros),
		AttrExplicitlyShared: shared,
	}
	if _, ok := attrs[AttrDeliveryMethod]; ok {
		method, err := coerceString(attrs[AttrDeliveryMethod])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrDeliveryMethod, err)
		}
		out[AttrDeliveryMethod] = normalizeEnum(method)
	}
	return out, nil
}

func campaignBudgetMutateBody(attrs resource.Attributes, resourceName string) (map[string]any, []string, error) {
	comparable, err := comparableCampaignBudget(attrs)
	if err != nil {
		return nil, nil, err
	}
	budget := map[string]any{}
	var mask []string
	if resourceName != "" {
		budget["resourceName"] = resourceName
	}

	if name, ok := comparable[AttrName].(string); ok {
		budget["name"] = name
		mask = append(mask, "name")
	}
	if amount, ok := comparable[AttrAmount]; ok {
		micros, err := amountToMicros(amount)
		if err != nil {
			return nil, nil, err
		}
		budget["amountMicros"] = strconv.FormatInt(micros, 10)
		mask = append(mask, "amountMicros")
	}
	if method, ok := comparable[AttrDeliveryMethod].(string); ok {
		budget["deliveryMethod"] = method
		mask = append(mask, "deliveryMethod")
	}
	if shared, ok := comparable[AttrExplicitlyShared]; ok {
		budget["explicitlyShared"] = shared
		mask = append(mask, "explicitlyShared")
	}

	sort.Strings(mask)
	return budget, mask, nil
}

func requiredCampaignBudgetAmountMicros(res resource.Resource) (int64, error) {
	v, ok := res.Attributes[AttrAmount]
	if !ok {
		return 0, fmt.Errorf("resource %s: missing required attribute %q", res.Address, AttrAmount)
	}
	micros, err := amountToMicros(v)
	if err != nil {
		return 0, fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrAmount, err)
	}
	return micros, nil
}

func requiredCampaignBudgetExplicitlyShared(res resource.Resource) (bool, error) {
	v, ok := res.Attributes[AttrExplicitlyShared]
	if !ok {
		return false, fmt.Errorf("resource %s: missing required attribute %q", res.Address, AttrExplicitlyShared)
	}
	shared, err := coerceBool(v)
	if err != nil {
		return false, fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrExplicitlyShared, err)
	}
	return shared, nil
}

func amountToMicros(v any) (int64, error) {
	if v == nil {
		return 0, fmt.Errorf("must be a positive daily amount in account currency units")
	}
	switch x := v.(type) {
	case string:
		return parseDecimalToMicros(strings.TrimSpace(x))
	case json.Number:
		return parseDecimalToMicros(strings.TrimSpace(x.String()))
	case int:
		return intAmountToMicros(int64(x))
	case int8:
		return intAmountToMicros(int64(x))
	case int16:
		return intAmountToMicros(int64(x))
	case int32:
		return intAmountToMicros(int64(x))
	case int64:
		return intAmountToMicros(x)
	case uint:
		return intAmountToMicros(int64(x))
	case uint8:
		return intAmountToMicros(int64(x))
	case uint16:
		return intAmountToMicros(int64(x))
	case uint32:
		return intAmountToMicros(int64(x))
	case uint64:
		if x > uint64(math.MaxInt64/microsPerCurrencyUnit) {
			return 0, fmt.Errorf("must be a positive daily amount in account currency units")
		}
		return intAmountToMicros(int64(x))
	case float32:
		return floatAmountToMicros(float64(x))
	case float64:
		return floatAmountToMicros(x)
	default:
		return 0, fmt.Errorf("must be a positive daily amount in account currency units")
	}
}

func intAmountToMicros(n int64) (int64, error) {
	if n <= 0 {
		return 0, fmt.Errorf("must be greater than 0")
	}
	if n > math.MaxInt64/microsPerCurrencyUnit {
		return 0, fmt.Errorf("is too large")
	}
	return n * microsPerCurrencyUnit, nil
}

func floatAmountToMicros(n float64) (int64, error) {
	if math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 {
		return 0, fmt.Errorf("must be greater than 0")
	}
	scaled := n * float64(microsPerCurrencyUnit)
	rounded := math.Round(scaled)
	if math.Abs(scaled-rounded) > floatMicrosEqualityTolerance {
		return 0, fmt.Errorf("must have at most %d decimal places", maxAmountFractionalDigits)
	}
	if rounded <= 0 || rounded > float64(math.MaxInt64) {
		return 0, fmt.Errorf("must be a positive daily amount in account currency units")
	}
	return int64(rounded), nil
}

func parseDecimalToMicros(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("must be a positive daily amount in account currency units")
	}
	if strings.HasPrefix(s, "+") {
		s = s[1:]
	}
	if strings.HasPrefix(s, "-") {
		return 0, fmt.Errorf("must be greater than 0")
	}
	whole, frac, hasFrac := strings.Cut(s, ".")
	if whole == "" {
		whole = "0"
	}
	if !isAllDigits(whole) || (hasFrac && !isAllDigits(frac)) {
		return 0, fmt.Errorf("must be a positive daily amount in account currency units")
	}
	if hasFrac {
		frac = strings.TrimRight(frac, "0")
		if len(frac) > maxAmountFractionalDigits {
			return 0, fmt.Errorf("must have at most %d decimal places", maxAmountFractionalDigits)
		}
		frac = frac + strings.Repeat("0", maxAmountFractionalDigits-len(frac))
	} else {
		frac = strings.Repeat("0", maxAmountFractionalDigits)
	}
	units, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("is too large")
	}
	fracMicros, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("must be a positive daily amount in account currency units")
	}
	if units > math.MaxInt64/microsPerCurrencyUnit {
		return 0, fmt.Errorf("is too large")
	}
	micros := units*microsPerCurrencyUnit + fracMicros
	if micros <= 0 {
		return 0, fmt.Errorf("must be greater than 0")
	}
	return micros, nil
}

func amountFromMicros(micros int64) any {
	if micros%microsPerCurrencyUnit == 0 {
		return micros / microsPerCurrencyUnit
	}
	return float64(micros) / float64(microsPerCurrencyUnit)
}

func parseMicrosJSON(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 {
		return 0, fmt.Errorf("missing amountMicros")
	}
	s := strings.TrimSpace(string(raw))
	s = strings.Trim(s, `"`)
	if s == "" || s == "null" {
		return 0, fmt.Errorf("missing amountMicros")
	}
	return strconv.ParseInt(s, 10, 64)
}

func boundCampaignBudgetIdentity(res resource.Resource) (string, bool, error) {
	if res.Identity.IsZero() {
		return "", false, nil
	}
	id, err := parseCampaignBudgetIdentity(res.Address, res.Identity.ID)
	if err != nil {
		return "", true, err
	}
	return id, true, nil
}

func parseCampaignBudgetIdentity(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("resource %s: persisted identity is empty; a Google Ads campaign budget id is required", addr)
	}
	if err := campaignBudgetIdentityIDError(addr, raw); err != nil {
		return "", err
	}
	return raw, nil
}

func (p *Provider) canonicalCampaignBudgetImportID(addr resource.Address, raw string) (string, error) {
	id, err := parseImportCampaignBudgetID(addr, raw)
	if err != nil {
		return "", err
	}
	if customerID, restID, ok := splitCampaignBudgetResourceName(id); ok {
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

func parseImportCampaignBudgetID(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("googleads: import %s: remote identifier is empty; expected a numeric campaign budget id or resource name customers/{customerId}/campaignBudgets/{id}", addr)
	}
	if _, id, ok := splitCampaignBudgetResourceName(raw); ok {
		if err := importCampaignBudgetIDError(addr, id); err != nil {
			return "", err
		}
		return raw, nil
	}
	if err := importCampaignBudgetIDError(addr, raw); err != nil {
		return "", err
	}
	return raw, nil
}

func importCampaignBudgetIDError(addr resource.Address, id string) error {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("googleads: import %s: %q is not a valid Google Ads campaign budget id; expected a positive numeric id or resource name customers/{customerId}/campaignBudgets/{id}", addr, id)
	}
	return nil
}

func campaignBudgetIdentityIDError(addr resource.Address, id string) error {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("resource %s: persisted identity %q is not a valid Google Ads campaign budget id", addr, id)
	}
	return nil
}

func splitCampaignBudgetResourceName(name string) (customerID, budgetID string, ok bool) {
	name = strings.TrimSpace(name)
	parts := strings.Split(name, "/")
	if len(parts) != 4 || parts[0] != "customers" || parts[2] != campaignBudgetsCollection {
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

func campaignBudgetResourceName(customerID, id string) string {
	return "customers/" + customerID + "/" + campaignBudgetsCollection + "/" + id
}

func parseCampaignBudgetMutateID(raw json.RawMessage, customerID string) (string, error) {
	var resp struct {
		Results []struct {
			ResourceName string `json:"resourceName"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || len(resp.Results) == 0 {
		return "", fmt.Errorf("malformed mutate response")
	}
	resourceName := strings.TrimSpace(resp.Results[0].ResourceName)
	gotCustomer, id, ok := splitCampaignBudgetResourceName(resourceName)
	if !ok {
		return "", fmt.Errorf("malformed mutate response")
	}
	if customerID != "" && gotCustomer != customerID {
		return "", fmt.Errorf("mutate returned campaign budget %s for a different customer", id)
	}
	return id, nil
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
