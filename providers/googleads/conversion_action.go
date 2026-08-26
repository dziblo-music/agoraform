package googleads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

const (
	// TypeConversionAction is the Google Ads website conversion-action type
	// used in addresses such as googleads.conversion_action.trial_started.
	TypeConversionAction = "conversion_action"

	// Configurable attributes. Names follow the v0.3 SaaS acquisition
	// workflow rather than a cross-provider abstraction.
	AttrName                           = "name"
	AttrCategory                       = "category"
	AttrStatus                         = "status"
	AttrValue                          = "value"
	AttrCurrency                       = "currency"
	AttrAlwaysUseDefaultValue          = "alwaysUseDefaultValue"
	AttrCount                          = "count"
	AttrPrimaryForGoal                 = "primaryForGoal"
	AttrClickThroughLookbackWindowDays = "clickThroughLookbackWindowDays"
	AttrViewThroughLookbackWindowDays  = "viewThroughLookbackWindowDays"

	conversionActionTypeWebpage = "WEBPAGE"
	defaultStatus               = "ENABLED"
	countOne                    = "ONE"
	countMany                   = "MANY"
	countOnePerClick            = "ONE_PER_CLICK"
	countManyPerClick           = "MANY_PER_CLICK"

	clickThroughLookbackMin = 1
	clickThroughLookbackMax = 90
	viewThroughLookbackMin  = 1
	viewThroughLookbackMax  = 30

	conversionActionsCollection = "conversionActions"
)

var (
	supportedConversionActionAttrs = map[string]struct{}{
		AttrName:                           {},
		AttrCategory:                       {},
		AttrStatus:                         {},
		AttrValue:                          {},
		AttrCurrency:                       {},
		AttrAlwaysUseDefaultValue:          {},
		AttrCount:                          {},
		AttrPrimaryForGoal:                 {},
		AttrClickThroughLookbackWindowDays: {},
		AttrViewThroughLookbackWindowDays:  {},
	}

	computedConversionActionAttrs = map[string]struct{}{
		"id":                                 {},
		"resourceName":                       {},
		"resource_name":                      {},
		"type":                               {},
		"origin":                             {},
		"ownerCustomer":                      {},
		"owner_customer":                     {},
		"includeInConversionsMetric":         {},
		"tagSnippets":                        {},
		"tag_snippets":                       {},
		"conversionId":                       {},
		"conversionLabel":                    {},
		"countingType":                       {},
		"counting_type":                      {},
		"attributionModelSettings":           {},
		"click_through_lookback_window_days": {},
		"view_through_lookback_window_days":  {},
		"primary_for_goal":                   {},
		"always_use_default_value":           {},
		"defaultValue":                       {},
		"default_value":                      {},
	}

	conversionActionCategories = map[string]struct{}{
		"DEFAULT":          {},
		"PAGE_VIEW":        {},
		"PURCHASE":         {},
		"SIGNUP":           {},
		"DOWNLOAD":         {},
		"ADD_TO_CART":      {},
		"BEGIN_CHECKOUT":   {},
		"SUBSCRIBE_PAID":   {},
		"SUBMIT_LEAD_FORM": {},
		"BOOK_APPOINTMENT": {},
		"REQUEST_QUOTE":    {},
		"GET_DIRECTIONS":   {},
		"OUTBOUND_CLICK":   {},
		"CONTACT":          {},
		"ENGAGEMENT":       {},
		"QUALIFIED_LEAD":   {},
		"CONVERTED_LEAD":   {},
	}

	conversionActionStatuses = map[string]struct{}{
		"ENABLED": {},
		"HIDDEN":  {},
		"REMOVED": {},
	}

	sendToPattern = regexp.MustCompile(`AW-(\d+)/([A-Za-z0-9_-]+)`)

	conversionActionSelect = strings.Join([]string{
		"SELECT",
		"conversion_action.resource_name,",
		"conversion_action.id,",
		"conversion_action.name,",
		"conversion_action.status,",
		"conversion_action.type,",
		"conversion_action.origin,",
		"conversion_action.category,",
		"conversion_action.primary_for_goal,",
		"conversion_action.value_settings.default_value,",
		"conversion_action.value_settings.default_currency_code,",
		"conversion_action.value_settings.always_use_default_value,",
		"conversion_action.counting_type,",
		"conversion_action.click_through_lookback_window_days,",
		"conversion_action.view_through_lookback_window_days,",
		"conversion_action.include_in_conversions_metric,",
		"conversion_action.owner_customer,",
		"conversion_action.tag_snippets",
		"FROM conversion_action",
	}, " ")
)

func (p *Provider) validateConversionAction(res resource.Resource) error {
	if err := p.requireCustomerID(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}

	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedConversionActionAttrs[key]; ok {
			continue
		}
		if _, computed := computedConversionActionAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; googleads.conversion_action supports %s", res.Address, key, joinSorted(keys(supportedConversionActionAttrs)))
	}

	name, err := requiredString(res, AttrName)
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("resource %s: attribute %q must be a non-empty string", res.Address, AttrName)
	}

	category, err := requiredString(res, AttrCategory)
	if err != nil {
		return err
	}
	category = normalizeEnum(category)
	if _, ok := conversionActionCategories[category]; !ok {
		return fmt.Errorf("resource %s: attribute %q must be one of %s", res.Address, AttrCategory, joinSorted(keys(conversionActionCategories)))
	}

	if _, _, err := optionalEnum(res, AttrStatus, conversionActionStatuses); err != nil {
		return err
	}

	if _, set, err := optionalFloat(res, AttrValue); err != nil {
		return err
	} else if set {
		v := attrs[AttrValue]
		n, _ := coerceFloat(v)
		if n < 0 {
			return fmt.Errorf("resource %s: attribute %q must be greater than or equal to 0", res.Address, AttrValue)
		}
	}

	if currency, set, err := optionalString(res, AttrCurrency); err != nil {
		return err
	} else if set {
		if err := validateCurrency(res.Address, currency); err != nil {
			return err
		}
	}

	if _, _, err := optionalBool(res, AttrAlwaysUseDefaultValue); err != nil {
		return err
	}

	if count, set, err := optionalString(res, AttrCount); err != nil {
		return err
	} else if set {
		if _, err := normalizeCount(count); err != nil {
			return fmt.Errorf("resource %s: attribute %q must be ONE or MANY", res.Address, AttrCount)
		}
	}

	if _, _, err := optionalBool(res, AttrPrimaryForGoal); err != nil {
		return err
	}

	if days, set, err := optionalInt64(res, AttrClickThroughLookbackWindowDays); err != nil {
		return err
	} else if set {
		if days < clickThroughLookbackMin || days > clickThroughLookbackMax {
			return fmt.Errorf("resource %s: attribute %q must be between %d and %d", res.Address, AttrClickThroughLookbackWindowDays, clickThroughLookbackMin, clickThroughLookbackMax)
		}
	}

	if days, set, err := optionalInt64(res, AttrViewThroughLookbackWindowDays); err != nil {
		return err
	} else if set {
		if days < viewThroughLookbackMin || days > viewThroughLookbackMax {
			return fmt.Errorf("resource %s: attribute %q must be between %d and %d", res.Address, AttrViewThroughLookbackWindowDays, viewThroughLookbackMin, viewThroughLookbackMax)
		}
	}

	if _, _, err := boundConversionActionIdentity(res); err != nil {
		return err
	}

	return nil
}

func (p *Provider) readConversionAction(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateConversionAction(res); err != nil {
		return resource.RemoteResource{}, err
	}

	id, bound, err := boundConversionActionIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if bound {
		live, err := p.readConversionActionByID(ctx, res.Address, id)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return live, nil
	}

	name, _, _ := optionalString(res, AttrName)
	matches, err := p.queryConversionActions(ctx, "conversion_action.name = "+gaqlString(name))
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, provider.ErrNotFound)
	case 1:
		live, err := remoteConversionAction(res.Address, matches[0])
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
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: multiple remote conversion actions named %q (ids %s); names must be unique", res.Address, name, strings.Join(ids, ", "))
	}
}

func (p *Provider) createConversionAction(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateConversionAction(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundConversionActionIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}

	action, _, err := conversionActionMutateBody(res.Attributes, "")
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}
	action["type"] = conversionActionTypeWebpage

	raw, err := c.Mutate(ctx, conversionActionsCollection, []map[string]any{
		{"create": action},
	})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}
	id, err := parseMutateResourceID(raw, c.CustomerID())
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}

	live, err := p.readConversionActionByID(ctx, res.Address, id)
	if err == nil {
		return live, nil
	}
	fallback, ferr := remoteFromDesired(res, id, c.CustomerID())
	if ferr != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s succeeded but refreshing conversion action %q failed: %w", res.Address, id, err)
	}
	return fallback, nil
}

func (p *Provider) updateConversionAction(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateConversionAction(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: missing remote identity", desired.Address)
	}
	if id, bound, err := boundConversionActionIdentity(desired); err != nil {
		return resource.RemoteResource{}, err
	} else if bound && id != actual.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: persisted identity %q does not match planned remote identity %q", desired.Address, id, actual.Identity.ID)
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}

	resourceName := conversionActionResourceName(c.CustomerID(), actual.Identity.ID)
	action, mask, err := conversionActionMutateBody(desired.Attributes, resourceName)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}
	if len(mask) == 0 {
		live, err := p.readConversionActionByID(ctx, desired.Address, actual.Identity.ID)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
		}
		return live, nil
	}

	_, err = c.Mutate(ctx, conversionActionsCollection, []map[string]any{
		{
			"updateMask": strings.Join(mask, ","),
			"update":     action,
		},
	})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}

	live, err := p.readConversionActionByID(ctx, desired.Address, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s succeeded but refreshing conversion action %q failed: %w", desired.Address, actual.Identity.ID, err)
	}
	return live, nil
}

func (p *Provider) importConversionAction(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	id, err := parseImportConversionActionID(addr, id)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireCustomerID(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	if customerID, restID, ok := splitConversionActionResourceName(id); ok {
		configured, err := p.configuredCustomerID()
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
		}
		if customerID != configured {
			return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: resource name customer %s does not match configured %s", addr, customerID, configured)
		}
		id = restID
	}
	live, err := p.readConversionActionByID(ctx, addr, id)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: remote conversion action %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	return live, nil
}

func (p *Provider) normalizeConversionActionComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	want, err := comparableConversionAction(desired.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if live == nil {
		return want, nil, nil
	}
	if id, bound, err := boundConversionActionIdentity(desired); err != nil {
		return nil, nil, err
	} else if bound {
		if live.Identity.IsZero() || live.Identity.ID != id {
			return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, id, live.Identity.ID)
		}
	}
	gotFull, err := comparableConversionAction(live.Attributes)
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

func (p *Provider) readConversionActionByID(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	matches, err := p.queryConversionActions(ctx, "conversion_action.id = "+id)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, provider.ErrNotFound
	case 1:
		return remoteConversionAction(addr, matches[0])
	default:
		return resource.RemoteResource{}, fmt.Errorf("multiple remote conversion actions returned for id %s", id)
	}
}

func (p *Provider) queryConversionActions(ctx context.Context, where string) ([]conversionActionData, error) {
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	query := conversionActionSelect
	if strings.TrimSpace(where) != "" {
		query += " WHERE " + where
	}
	rows, err := c.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]conversionActionData, 0, len(rows))
	for _, row := range rows {
		item, err := decodeConversionActionRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (p *Provider) requireCustomerID() error {
	if p == nil || strings.TrimSpace(p.cfg.CustomerID) == "" {
		return fmt.Errorf("%s is required to manage conversion actions", EnvCustomerID)
	}
	return nil
}

func (p *Provider) configuredCustomerID() (string, error) {
	if err := p.requireCustomerID(); err != nil {
		return "", err
	}
	return NormalizeCustomerID(p.cfg.CustomerID)
}

type conversionActionData struct {
	ResourceName                   string
	ID                             string
	Name                           string
	Status                         string
	Type                           string
	Origin                         string
	Category                       string
	PrimaryForGoal                 *bool
	OwnerCustomer                  string
	IncludeInConversionsMetric     *bool
	ClickThroughLookbackWindowDays *int64
	ViewThroughLookbackWindowDays  *int64
	DefaultValue                   *float64
	DefaultCurrencyCode            string
	AlwaysUseDefaultValue          *bool
	CountingType                   string
	TagSnippets                    []map[string]any
}

type conversionActionJSON struct {
	ResourceName                   string      `json:"resourceName"`
	ID                             json.Number `json:"id"`
	Name                           string      `json:"name"`
	Status                         string      `json:"status"`
	Type                           string      `json:"type"`
	Origin                         string      `json:"origin"`
	Category                       string      `json:"category"`
	PrimaryForGoal                 *bool       `json:"primaryForGoal"`
	OwnerCustomer                  string      `json:"ownerCustomer"`
	IncludeInConversionsMetric     *bool       `json:"includeInConversionsMetric"`
	ClickThroughLookbackWindowDays json.Number `json:"clickThroughLookbackWindowDays"`
	ViewThroughLookbackWindowDays  json.Number `json:"viewThroughLookbackWindowDays"`
	CountingType                   string      `json:"countingType"`
	ValueSettings                  *struct {
		DefaultValue          *float64 `json:"defaultValue"`
		DefaultCurrencyCode   string   `json:"defaultCurrencyCode"`
		AlwaysUseDefaultValue *bool    `json:"alwaysUseDefaultValue"`
	} `json:"valueSettings"`
	TagSnippets []map[string]any `json:"tagSnippets"`
}

func decodeConversionActionRow(raw json.RawMessage) (conversionActionData, error) {
	var envelope struct {
		ConversionAction json.RawMessage `json:"conversionAction"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return conversionActionData{}, fmt.Errorf("malformed conversion action result")
	}
	if len(envelope.ConversionAction) == 0 {
		return conversionActionData{}, fmt.Errorf("malformed conversion action result")
	}
	var body conversionActionJSON
	if err := json.Unmarshal(envelope.ConversionAction, &body); err != nil {
		return conversionActionData{}, fmt.Errorf("malformed conversion action result")
	}
	id := strings.TrimSpace(body.ID.String())
	if id == "" {
		if _, parsed, ok := splitConversionActionResourceName(body.ResourceName); ok {
			id = parsed
		}
	}
	if id == "" || body.Name == "" {
		return conversionActionData{}, fmt.Errorf("malformed conversion action result")
	}
	item := conversionActionData{
		ResourceName:               strings.TrimSpace(body.ResourceName),
		ID:                         id,
		Name:                       body.Name,
		Status:                     normalizeEnum(body.Status),
		Type:                       normalizeEnum(body.Type),
		Origin:                     normalizeEnum(body.Origin),
		Category:                   normalizeEnum(body.Category),
		PrimaryForGoal:             body.PrimaryForGoal,
		OwnerCustomer:              strings.TrimSpace(body.OwnerCustomer),
		IncludeInConversionsMetric: body.IncludeInConversionsMetric,
		CountingType:               normalizeEnum(body.CountingType),
		TagSnippets:                body.TagSnippets,
	}
	if n, err := parseOptionalInt64(body.ClickThroughLookbackWindowDays); err == nil {
		item.ClickThroughLookbackWindowDays = n
	}
	if n, err := parseOptionalInt64(body.ViewThroughLookbackWindowDays); err == nil {
		item.ViewThroughLookbackWindowDays = n
	}
	if body.ValueSettings != nil {
		item.DefaultValue = body.ValueSettings.DefaultValue
		item.DefaultCurrencyCode = strings.TrimSpace(body.ValueSettings.DefaultCurrencyCode)
		item.AlwaysUseDefaultValue = body.ValueSettings.AlwaysUseDefaultValue
	}
	return item, nil
}

func remoteConversionAction(addr resource.Address, item conversionActionData) (resource.RemoteResource, error) {
	if item.Type != "" && item.Type != conversionActionTypeWebpage {
		return resource.RemoteResource{}, fmt.Errorf("conversion action %s has type %s; googleads.conversion_action only manages website (WEBPAGE) conversion actions", item.ID, item.Type)
	}

	attrs := resource.Attributes{
		AttrName:     item.Name,
		AttrCategory: item.Category,
	}
	if status := comparableStatus(item.Status); status != "" {
		attrs[AttrStatus] = status
	}
	if count := comparableCount(item.CountingType); count != "" {
		attrs[AttrCount] = count
	}
	if item.PrimaryForGoal != nil {
		attrs[AttrPrimaryForGoal] = *item.PrimaryForGoal
	}
	if item.DefaultValue != nil {
		attrs[AttrValue] = *item.DefaultValue
	}
	if item.DefaultCurrencyCode != "" {
		attrs[AttrCurrency] = item.DefaultCurrencyCode
	}
	if item.AlwaysUseDefaultValue != nil {
		attrs[AttrAlwaysUseDefaultValue] = *item.AlwaysUseDefaultValue
	}
	if item.ClickThroughLookbackWindowDays != nil {
		attrs[AttrClickThroughLookbackWindowDays] = *item.ClickThroughLookbackWindowDays
	}
	if item.ViewThroughLookbackWindowDays != nil {
		attrs[AttrViewThroughLookbackWindowDays] = *item.ViewThroughLookbackWindowDays
	}

	computed := resource.Attributes{}
	setComputed(computed, "id", item.ID)
	setComputed(computed, "resourceName", item.ResourceName)
	setComputed(computed, "type", item.Type)
	setComputed(computed, "origin", item.Origin)
	setComputed(computed, "ownerCustomer", item.OwnerCustomer)
	if item.IncludeInConversionsMetric != nil {
		computed["includeInConversionsMetric"] = *item.IncludeInConversionsMetric
	}
	if len(item.TagSnippets) > 0 {
		snippets := make([]any, 0, len(item.TagSnippets))
		for _, snippet := range item.TagSnippets {
			if len(snippet) == 0 {
				continue
			}
			snippets = append(snippets, snippet)
		}
		if len(snippets) > 0 {
			computed["tagSnippets"] = snippets
		}
		if conversionID, label := conversionIdentityFromSnippets(item.TagSnippets); conversionID != "" {
			computed["conversionId"] = conversionID
			if label != "" {
				computed["conversionLabel"] = label
			}
		}
	}

	return resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: item.ID},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func remoteFromDesired(res resource.Resource, id, customerID string) (resource.RemoteResource, error) {
	attrs, err := comparableConversionAction(res.Attributes)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	computed := resource.Attributes{
		"id":           id,
		"resourceName": conversionActionResourceName(customerID, id),
		"type":         conversionActionTypeWebpage,
	}
	return resource.RemoteResource{
		Address:    res.Address,
		Identity:   resource.Identity{ID: id},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func comparableConversionAction(attrs resource.Attributes) (resource.Attributes, error) {
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	name, err := coerceString(attrs[AttrName])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrName, err)
	}
	category, err := coerceString(attrs[AttrCategory])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrCategory, err)
	}
	out := resource.Attributes{
		AttrName:     name,
		AttrCategory: normalizeEnum(category),
	}

	if _, ok := attrs[AttrStatus]; ok {
		status, err := coerceString(attrs[AttrStatus])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrStatus, err)
		}
		out[AttrStatus] = normalizeEnum(status)
	}
	if _, ok := attrs[AttrCount]; ok {
		count, err := coerceString(attrs[AttrCount])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrCount, err)
		}
		normalized, err := normalizeCount(count)
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrCount, err)
		}
		out[AttrCount] = normalized
	}
	if _, ok := attrs[AttrPrimaryForGoal]; ok {
		v, err := coerceBool(attrs[AttrPrimaryForGoal])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrPrimaryForGoal, err)
		}
		out[AttrPrimaryForGoal] = v
	}
	if _, ok := attrs[AttrValue]; ok {
		v, err := coerceFloat(attrs[AttrValue])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrValue, err)
		}
		out[AttrValue] = v
		if _, alwaysSet := attrs[AttrAlwaysUseDefaultValue]; !alwaysSet {
			out[AttrAlwaysUseDefaultValue] = true
		}
	}
	if _, ok := attrs[AttrCurrency]; ok {
		v, err := coerceString(attrs[AttrCurrency])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrCurrency, err)
		}
		out[AttrCurrency] = strings.ToUpper(strings.TrimSpace(v))
	}
	if _, ok := attrs[AttrAlwaysUseDefaultValue]; ok {
		v, err := coerceBool(attrs[AttrAlwaysUseDefaultValue])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrAlwaysUseDefaultValue, err)
		}
		out[AttrAlwaysUseDefaultValue] = v
	}
	if _, ok := attrs[AttrClickThroughLookbackWindowDays]; ok {
		v, err := coerceInt64(attrs[AttrClickThroughLookbackWindowDays])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrClickThroughLookbackWindowDays, err)
		}
		out[AttrClickThroughLookbackWindowDays] = v
	}
	if _, ok := attrs[AttrViewThroughLookbackWindowDays]; ok {
		v, err := coerceInt64(attrs[AttrViewThroughLookbackWindowDays])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrViewThroughLookbackWindowDays, err)
		}
		out[AttrViewThroughLookbackWindowDays] = v
	}
	return out, nil
}

func conversionActionMutateBody(attrs resource.Attributes, resourceName string) (map[string]any, []string, error) {
	comparable, err := comparableConversionAction(attrs)
	if err != nil {
		return nil, nil, err
	}
	action := map[string]any{}
	var mask []string
	if resourceName != "" {
		action["resourceName"] = resourceName
	}

	if name, ok := comparable[AttrName].(string); ok {
		action["name"] = name
		mask = append(mask, "name")
	}
	if category, ok := comparable[AttrCategory].(string); ok {
		action["category"] = category
		mask = append(mask, "category")
	}
	if status, ok := comparable[AttrStatus].(string); ok {
		action["status"] = status
		mask = append(mask, "status")
	} else if resourceName == "" {
		action["status"] = defaultStatus
	}
	if count, ok := comparable[AttrCount].(string); ok {
		action["countingType"] = apiCount(count)
		mask = append(mask, "countingType")
	}
	if v, ok := comparable[AttrPrimaryForGoal]; ok {
		action["primaryForGoal"] = v
		mask = append(mask, "primaryForGoal")
	}
	if v, ok := comparable[AttrClickThroughLookbackWindowDays]; ok {
		action["clickThroughLookbackWindowDays"] = v
		mask = append(mask, "clickThroughLookbackWindowDays")
	}
	if v, ok := comparable[AttrViewThroughLookbackWindowDays]; ok {
		action["viewThroughLookbackWindowDays"] = v
		mask = append(mask, "viewThroughLookbackWindowDays")
	}

	valueSettings := map[string]any{}
	if v, ok := comparable[AttrValue]; ok {
		valueSettings["defaultValue"] = v
		mask = append(mask, "valueSettings.defaultValue")
	}
	if v, ok := comparable[AttrCurrency].(string); ok {
		valueSettings["defaultCurrencyCode"] = v
		mask = append(mask, "valueSettings.defaultCurrencyCode")
	}
	if v, ok := comparable[AttrAlwaysUseDefaultValue]; ok {
		valueSettings["alwaysUseDefaultValue"] = v
		mask = append(mask, "valueSettings.alwaysUseDefaultValue")
	}
	if len(valueSettings) > 0 {
		action["valueSettings"] = valueSettings
	}

	sort.Strings(mask)
	return action, mask, nil
}

func parseMutateResourceID(raw json.RawMessage, customerID string) (string, error) {
	var resp struct {
		Results []struct {
			ResourceName string `json:"resourceName"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || len(resp.Results) == 0 {
		return "", fmt.Errorf("malformed mutate response")
	}
	resourceName := strings.TrimSpace(resp.Results[0].ResourceName)
	gotCustomer, id, ok := splitConversionActionResourceName(resourceName)
	if !ok {
		return "", fmt.Errorf("malformed mutate response")
	}
	if customerID != "" && gotCustomer != customerID {
		return "", fmt.Errorf("mutate returned conversion action %s for a different customer", id)
	}
	return id, nil
}

func boundConversionActionIdentity(res resource.Resource) (string, bool, error) {
	if res.Identity.IsZero() {
		return "", false, nil
	}
	id, err := parseConversionActionIdentity(res.Address, res.Identity.ID)
	if err != nil {
		return "", true, err
	}
	return id, true, nil
}

func parseConversionActionIdentity(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("resource %s: persisted identity is empty; a Google Ads conversion action id is required", addr)
	}
	if err := identityIDError(addr, raw); err != nil {
		return "", err
	}
	return raw, nil
}

func parseImportConversionActionID(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("resource %s: persisted identity is empty; a Google Ads conversion action id is required", addr)
	}
	if _, id, ok := splitConversionActionResourceName(raw); ok {
		if err := identityIDError(addr, id); err != nil {
			return "", err
		}
		return raw, nil
	}
	if err := identityIDError(addr, raw); err != nil {
		return "", err
	}
	return raw, nil
}

func identityIDError(addr resource.Address, id string) error {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("resource %s: persisted identity %q is not a valid Google Ads conversion action id", addr, id)
	}
	return nil
}

func splitConversionActionResourceName(name string) (customerID, actionID string, ok bool) {
	name = strings.TrimSpace(name)
	parts := strings.Split(name, "/")
	if len(parts) != 4 || parts[0] != "customers" || parts[2] != conversionActionsCollection {
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

func conversionActionResourceName(customerID, id string) string {
	return "customers/" + customerID + "/" + conversionActionsCollection + "/" + id
}

func comparableStatus(status string) string {
	status = normalizeEnum(status)
	if _, ok := conversionActionStatuses[status]; ok {
		return status
	}
	return ""
}

func comparableCount(countingType string) string {
	normalized, err := normalizeCount(countingType)
	if err != nil {
		return ""
	}
	return normalized
}

func normalizeCount(raw string) (string, error) {
	switch normalizeEnum(raw) {
	case countOne, countOnePerClick:
		return countOne, nil
	case countMany, countManyPerClick:
		return countMany, nil
	default:
		return "", fmt.Errorf("must be ONE or MANY")
	}
}

func apiCount(count string) string {
	if count == countOne {
		return countOnePerClick
	}
	return countManyPerClick
}

func validateCurrency(addr resource.Address, currency string) error {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if len(currency) != 3 {
		return fmt.Errorf("resource %s: attribute %q must be a 3-letter currency code", addr, AttrCurrency)
	}
	for _, r := range currency {
		if r < 'A' || r > 'Z' {
			return fmt.Errorf("resource %s: attribute %q must be a 3-letter currency code", addr, AttrCurrency)
		}
	}
	return nil
}

func conversionIdentityFromSnippets(snippets []map[string]any) (conversionID, label string) {
	for _, snippet := range snippets {
		for _, key := range []string{"eventSnippet", "globalSiteTag"} {
			s, err := coerceString(snippet[key])
			if err != nil || s == "" {
				continue
			}
			if match := sendToPattern.FindStringSubmatch(s); len(match) == 3 {
				return match[1], match[2]
			}
		}
	}
	return "", ""
}

func gaqlString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return "'" + s + "'"
}

func parseOptionalInt64(n json.Number) (*int64, error) {
	if strings.TrimSpace(n.String()) == "" {
		return nil, nil
	}
	v, err := n.Int64()
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func setComputed(attrs resource.Attributes, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	attrs[key] = value
}

func requiredString(res resource.Resource, key string) (string, error) {
	v, ok := res.Attributes[key]
	if !ok {
		return "", fmt.Errorf("resource %s: missing required attribute %q", res.Address, key)
	}
	s, err := coerceString(v)
	if err != nil {
		return "", fmt.Errorf("resource %s: attribute %q %w", res.Address, key, err)
	}
	return s, nil
}

func optionalString(res resource.Resource, key string) (string, bool, error) {
	v, ok := res.Attributes[key]
	if !ok {
		return "", false, nil
	}
	s, err := coerceString(v)
	if err != nil {
		return "", true, fmt.Errorf("resource %s: attribute %q %w", res.Address, key, err)
	}
	return s, true, nil
}

func optionalEnum(res resource.Resource, key string, allowed map[string]struct{}) (string, bool, error) {
	s, set, err := optionalString(res, key)
	if err != nil || !set {
		return s, set, err
	}
	s = normalizeEnum(s)
	if _, ok := allowed[s]; !ok {
		return "", true, fmt.Errorf("resource %s: attribute %q must be one of %s", res.Address, key, joinSorted(keys(allowed)))
	}
	return s, true, nil
}

func optionalBool(res resource.Resource, key string) (bool, bool, error) {
	v, ok := res.Attributes[key]
	if !ok {
		return false, false, nil
	}
	b, err := coerceBool(v)
	if err != nil {
		return false, true, fmt.Errorf("resource %s: attribute %q %w", res.Address, key, err)
	}
	return b, true, nil
}

func optionalFloat(res resource.Resource, key string) (float64, bool, error) {
	v, ok := res.Attributes[key]
	if !ok {
		return 0, false, nil
	}
	n, err := coerceFloat(v)
	if err != nil {
		return 0, true, fmt.Errorf("resource %s: attribute %q %w", res.Address, key, err)
	}
	return n, true, nil
}

func optionalInt64(res resource.Resource, key string) (int64, bool, error) {
	v, ok := res.Attributes[key]
	if !ok {
		return 0, false, nil
	}
	n, err := coerceInt64(v)
	if err != nil {
		return 0, true, fmt.Errorf("resource %s: attribute %q %w", res.Address, key, err)
	}
	return n, true, nil
}

func coerceString(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	switch x := v.(type) {
	case string:
		return x, nil
	case json.Number:
		return x.String(), nil
	case int:
		return strconv.Itoa(x), nil
	case int8:
		return strconv.FormatInt(int64(x), 10), nil
	case int16:
		return strconv.FormatInt(int64(x), 10), nil
	case int32:
		return strconv.FormatInt(int64(x), 10), nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case uint:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint8:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint64:
		return strconv.FormatUint(x, 10), nil
	case float32:
		return formatFloat(float64(x)), nil
	case float64:
		return formatFloat(x), nil
	default:
		return "", fmt.Errorf("must be a string")
	}
}

func coerceBool(v any) (bool, error) {
	switch x := v.(type) {
	case bool:
		return x, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "1", "yes":
			return true, nil
		case "false", "0", "no":
			return false, nil
		default:
			return false, fmt.Errorf("must be a boolean")
		}
	default:
		return false, fmt.Errorf("must be a boolean")
	}
}

func coerceFloat(v any) (float64, error) {
	switch x := v.(type) {
	case int:
		return float64(x), nil
	case int8:
		return float64(x), nil
	case int16:
		return float64(x), nil
	case int32:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case uint:
		return float64(x), nil
	case uint8:
		return float64(x), nil
	case uint16:
		return float64(x), nil
	case uint32:
		return float64(x), nil
	case uint64:
		return float64(x), nil
	case float32:
		return float64(x), nil
	case float64:
		return x, nil
	case json.Number:
		return x.Float64()
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil {
			return 0, fmt.Errorf("must be a number")
		}
		return n, nil
	default:
		return 0, fmt.Errorf("must be a number")
	}
}

func coerceInt64(v any) (int64, error) {
	n, err := coerceFloat(v)
	if err != nil {
		return 0, fmt.Errorf("must be an integer")
	}
	if n != float64(int64(n)) {
		return 0, fmt.Errorf("must be an integer")
	}
	return int64(n), nil
}

func formatFloat(n float64) string {
	if n == float64(int64(n)) && n >= -1e15 && n <= 1e15 {
		return strconv.FormatInt(int64(n), 10)
	}
	return strconv.FormatFloat(n, 'g', -1, 64)
}

func normalizeEnum(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

func keys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func joinSorted(values []string) string {
	sort.Strings(values)
	return strings.Join(values, ", ")
}
