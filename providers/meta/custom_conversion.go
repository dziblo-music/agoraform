package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/meta/client"
)

const (
	customConversionFields          = "id,account_id,name,custom_event_type,rule,pixel,event_source_type,default_conversion_value,is_archived,is_unavailable"
	customConversionActionSource    = "website"
	customConversionEventSourceType = "pixel"
)

var (
	supportedCustomConversionAttrs = map[string]struct{}{
		AttrName:         {},
		AttrPixel:        {},
		AttrRule:         {},
		AttrEventType:    {},
		AttrDefaultValue: {},
	}

	computedCustomConversionAttrs = map[string]struct{}{
		"id":                       {},
		"customConversionId":       {},
		"account_id":               {},
		"accountId":                {},
		"custom_event_type":        {},
		"event_source_id":          {},
		"eventSourceId":            {},
		"event_source_type":        {},
		"eventSourceType":          {},
		"default_conversion_value": {},
		"is_archived":              {},
		"isArchived":               {},
		"is_unavailable":           {},
		"isUnavailable":            {},
		"description":              {},
		"advanced_rule":            {},
		"advancedRule":             {},
		"action_source_type":       {},
		"actionSourceType":         {},
		"currency":                 {},
	}

	customEventTypes = map[string]struct{}{
		"ADD_PAYMENT_INFO":      {},
		"ADD_TO_CART":           {},
		"ADD_TO_WISHLIST":       {},
		"COMPLETE_REGISTRATION": {},
		"CONTENT_VIEW":          {},
		"INITIATED_CHECKOUT":    {},
		"LEAD":                  {},
		"PURCHASE":              {},
		"SEARCH":                {},
		"CONTACT":               {},
		"CUSTOMIZE_PRODUCT":     {},
		"DONATE":                {},
		"FIND_LOCATION":         {},
		"SCHEDULE":              {},
		"START_TRIAL":           {},
		"SUBMIT_APPLICATION":    {},
		"SUBSCRIBE":             {},
		"LISTING_INTERACTION":   {},
		"FACEBOOK_SELECTED":     {},
		"OTHER":                 {},
	}
)

type customConversion struct {
	ID                     string          `json:"id"`
	AccountID              string          `json:"account_id"`
	Name                   string          `json:"name"`
	CustomEventType        string          `json:"custom_event_type"`
	Rule                   string          `json:"rule"`
	Pixel                  json.RawMessage `json:"pixel"`
	EventSourceType        string          `json:"event_source_type"`
	DefaultConversionValue any             `json:"default_conversion_value"`
	IsArchived             bool            `json:"is_archived"`
	IsUnavailable          bool            `json:"is_unavailable"`
}

func (p *Provider) validateCustomConversion(res resource.Resource) error {
	if err := p.requireAdAccount(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}
	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedCustomConversionAttrs[key]; ok {
			continue
		}
		if _, computed := computedCustomConversionAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; meta.custom_conversion supports %s", res.Address, key, joinSorted(keys(supportedCustomConversionAttrs)))
	}
	if _, err := requiredString(res, AttrName); err != nil {
		return err
	}
	if _, err := requiredPixelRef(res); err != nil {
		return err
	}
	if _, err := requiredRule(res); err != nil {
		return err
	}
	if _, err := requiredEventType(res); err != nil {
		return err
	}
	if _, _, err := optionalDefaultValue(res); err != nil {
		return err
	}
	if _, _, err := boundIdentity(res); err != nil {
		return err
	}
	return nil
}

func (p *Provider) readCustomConversion(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateCustomConversion(res); err != nil {
		return resource.RemoteResource{}, err
	}
	id, bound, err := boundIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if bound {
		live, err := p.readCustomConversionByID(ctx, res, id)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("meta: read %s: %w", res.Address, err)
		}
		return live, nil
	}

	// Custom Conversions are managed resources, not name-adopted bindings.
	// Returning a discovered identity for an equivalent unbound object would
	// produce an unchanged plan, and apply intentionally does not persist
	// identities for unchanged resources. Existing objects must therefore be
	// imported explicitly; otherwise an unbound declaration plans a create.
	return resource.RemoteResource{}, fmt.Errorf("meta: read %s: %w", res.Address, provider.ErrNotFound)
}

func (p *Provider) createCustomConversion(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateCustomConversion(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	form, err := p.customConversionCreateForm(res)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: %w", res.Address, err)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := c.Post(ctx, c.AdAccountID()+"/customconversions", form, &created); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: %w", res.Address, err)
	}
	id, err := normalizeObjectID(created.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: API returned an invalid id: %w", res.Address, err)
	}
	live, err := p.readCustomConversionByID(ctx, res, id)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s succeeded but refreshing custom conversion %q failed: %w", res.Address, id, err)
	}
	return live, nil
}

func (p *Provider) updateCustomConversion(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateCustomConversion(desired); err != nil {
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

	want, err := p.comparableCustomConversion(desired)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: %w", desired.Address, err)
	}
	got, err := p.comparableCustomConversion(resource.Resource{Address: desired.Address, Attributes: actual.Attributes})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: %w", desired.Address, err)
	}
	if !rulesEqual(want[AttrRule], got[AttrRule]) {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: rule is immutable after create; create a new meta.custom_conversion instead of emulating a rule change", desired.Address)
	}
	if !p.samePixelIdentity(want[AttrPixel], got[AttrPixel]) {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: pixel is immutable after create; create a new meta.custom_conversion instead of moving the event source", desired.Address)
	}
	if want[AttrEventType] != got[AttrEventType] {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: eventType is immutable after create; the Marketing API update contract only accepts name and defaultValue", desired.Address)
	}

	form := url.Values{}
	if want[AttrName] != got[AttrName] {
		form.Set("name", want[AttrName].(string))
	}
	if !defaultValuesEqual(want[AttrDefaultValue], got[AttrDefaultValue]) {
		if want[AttrDefaultValue] == nil {
			return resource.RemoteResource{}, fmt.Errorf("meta: update %s: defaultValue cannot be cleared through the Marketing API", desired.Address)
		}
		form.Set("default_conversion_value", formatDefaultValue(want[AttrDefaultValue]))
	}
	if len(form) == 0 {
		live, err := p.readCustomConversionByID(ctx, desired, actual.Identity.ID)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("meta: update %s: %w", desired.Address, err)
		}
		return live, nil
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
	live, err := p.readCustomConversionByID(ctx, desired, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s succeeded but refreshing custom conversion %q failed: %w", desired.Address, actual.Identity.ID, err)
	}
	return live, nil
}

func (p *Provider) importCustomConversion(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	id, err := p.canonicalCustomConversionImportID(addr, rawID)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireConfig(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: import %s: %w", addr, err)
	}
	res := resource.Resource{Address: addr}
	live, err := p.readCustomConversionByID(ctx, res, id)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("meta: import %s: remote custom conversion %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("meta: import %s: %w", addr, err)
	}
	if _, ok := resource.AsRef(live.Attributes[AttrPixel]); !ok {
		return resource.RemoteResource{}, fmt.Errorf("meta: import %s: Pixel/Dataset %s is not a uniquely bound meta.pixel; import that event source first so the relationship can be reconstructed as a logical $ref", addr, pixelIDFromAttr(live.Attributes[AttrPixel]))
	}
	return live, nil
}

func (p *Provider) canonicalCustomConversionImportID(_ resource.Address, raw string) (string, error) {
	id, err := normalizeObjectID(raw)
	if err != nil {
		return "", fmt.Errorf("meta: import identity is invalid: %w", err)
	}
	return id, nil
}

func (p *Provider) normalizeCustomConversionComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	want, err := p.comparableCustomConversion(desired)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if live == nil {
		return want, nil, nil
	}
	if id, bound, err := boundIdentity(desired); err != nil {
		return nil, nil, err
	} else if bound {
		if live.Identity.IsZero() || live.Identity.ID != id {
			return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, id, live.Identity.ID)
		}
	}
	got, err := p.comparableCustomConversion(resource.Resource{Address: desired.Address, Attributes: live.Attributes})
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if p.samePixelIdentity(want[AttrPixel], got[AttrPixel]) {
		got[AttrPixel] = want[AttrPixel]
	}
	return want, got, nil
}

func (p *Provider) readCustomConversionByID(ctx context.Context, res resource.Resource, id string) (resource.RemoteResource, error) {
	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	var item customConversion
	if err := c.Get(ctx, id, url.Values{"fields": {customConversionFields}}, &item); err != nil {
		if client.IsNotFound(err) {
			return resource.RemoteResource{}, provider.ErrNotFound
		}
		return resource.RemoteResource{}, err
	}
	if item.IsArchived {
		return resource.RemoteResource{}, provider.ErrNotFound
	}
	live, err := p.remoteCustomConversion(res, item)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	return p.rememberLive(live), nil
}

func (p *Provider) remoteCustomConversion(res resource.Resource, item customConversion) (resource.RemoteResource, error) {
	id, err := normalizeObjectID(item.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote custom conversion id is invalid: %w", err)
	}
	if err := p.ensureCustomConversionAccount(item.AccountID); err != nil {
		return resource.RemoteResource{}, err
	}
	eventSourceType := strings.ToLower(strings.TrimSpace(item.EventSourceType))
	if eventSourceType != "" && eventSourceType != customConversionEventSourceType {
		return resource.RemoteResource{}, fmt.Errorf("remote custom conversion %s uses event source type %s; Agoraform manages website Pixel/Dataset conversions only", id, item.EventSourceType)
	}
	name := strings.TrimSpace(item.Name)
	if name == "" {
		return resource.RemoteResource{}, fmt.Errorf("remote custom conversion %s is missing a name", id)
	}
	eventType := strings.ToUpper(strings.TrimSpace(item.CustomEventType))
	if _, ok := customEventTypes[eventType]; !ok {
		return resource.RemoteResource{}, fmt.Errorf("remote custom conversion %s has unsupported custom_event_type %q", id, item.CustomEventType)
	}
	rule, err := parseRule(item.Rule)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote custom conversion %s has an invalid rule: %w", id, err)
	}
	pixelID, err := pixelIDFromJSON(item.Pixel)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote custom conversion %s: %w", id, err)
	}
	pixelAttr, err := p.livePixelAttr(res.Address, pixelID, res.Attributes[AttrPixel])
	if err != nil {
		return resource.RemoteResource{}, err
	}
	attrs := resource.Attributes{
		AttrName:      name,
		AttrEventType: eventType,
		AttrRule:      rule,
		AttrPixel:     pixelAttr,
	}
	if item.DefaultConversionValue != nil && item.DefaultConversionValue != "" {
		n, err := coerceFloat(item.DefaultConversionValue)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("remote custom conversion %s has an invalid default_conversion_value", id)
		}
		attrs[AttrDefaultValue] = normalizeDefaultValue(n)
	}
	computed := resource.Attributes{
		OutputCustomConversionID: id,
	}
	if item.IsUnavailable {
		computed["isUnavailable"] = true
	}
	return resource.RemoteResource{
		Address:    res.Address,
		Identity:   resource.Identity{ID: id},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func (p *Provider) livePixelAttr(addr resource.Address, pixelID string, desired any) (any, error) {
	want := logicalRef(desired)
	if !want.IsZero() {
		wantID := ""
		if resolved, ok := resource.AsResolved(desired); ok {
			wantID = resolved.Identity.ID
			if wantID == "" {
				if id, err := coerceString(resolved.Outputs[OutputPixelID]); err == nil {
					wantID = id
				}
			}
		}
		if wantID == "" {
			wantID = p.lookupID(want.Address)
		}
		if normalized, err := normalizeObjectID(wantID); err == nil && normalized == pixelID {
			return resource.Ref{Address: want.Address}, nil
		}
	}
	managed, found, err := p.lookupManagedAddress(TypePixel, pixelID)
	if err != nil {
		return nil, err
	}
	if found {
		return resource.Ref{Address: managed}, nil
	}
	return pixelID, nil
}

func (p *Provider) customConversionCreateForm(res resource.Resource) (url.Values, error) {
	name, err := requiredString(res, AttrName)
	if err != nil {
		return nil, err
	}
	eventType, err := requiredEventType(res)
	if err != nil {
		return nil, err
	}
	rule, err := requiredRule(res)
	if err != nil {
		return nil, err
	}
	ruleJSON, err := encodeRule(rule)
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrRule, err)
	}
	pixelID, err := p.pixelIDFromRef(res.Attributes[AttrPixel])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrPixel, err)
	}
	form := url.Values{}
	form.Set("name", name)
	form.Set("custom_event_type", eventType)
	form.Set("rule", ruleJSON)
	form.Set("event_source_id", pixelID)
	form.Set("action_source_type", customConversionActionSource)
	if value, set, err := optionalDefaultValue(res); err != nil {
		return nil, err
	} else if set {
		form.Set("default_conversion_value", formatDefaultValue(value))
	}
	return form, nil
}

func (p *Provider) pixelIDFromRef(v any) (string, error) {
	if resolved, ok := resource.AsResolved(v); ok {
		if resolved.Identity.ID != "" {
			return normalizeObjectID(resolved.Identity.ID)
		}
		if id, err := coerceString(resolved.Outputs[OutputPixelID]); err == nil && strings.TrimSpace(id) != "" {
			return normalizeObjectID(id)
		}
		return "", fmt.Errorf("pixel reference %s has no provider-native identity", resolved.Address)
	}
	ref, ok := resource.AsRef(v)
	if !ok {
		return "", fmt.Errorf("must be a resource reference ($ref) to a %s.%s resource", Name, TypePixel)
	}
	if id := p.lookupID(ref.Address); id != "" {
		return normalizeObjectID(id)
	}
	return "", fmt.Errorf("pixel reference %s has no provider-native identity", ref.Address)
}

func (p *Provider) comparableCustomConversion(res resource.Resource) (resource.Attributes, error) {
	name, err := requiredString(res, AttrName)
	if err != nil {
		return nil, err
	}
	rule, err := requiredRule(res)
	if err != nil {
		return nil, err
	}
	eventType, err := requiredEventType(res)
	if err != nil {
		return nil, err
	}
	ref, refErr := requiredPixelRef(res)
	if refErr != nil {
		id, ok := objectIDFromAny(res.Attributes[AttrPixel])
		if !ok {
			return nil, refErr
		}
		out := resource.Attributes{
			AttrName:      name,
			AttrPixel:     id,
			AttrRule:      rule,
			AttrEventType: eventType,
		}
		if value, set, err := optionalDefaultValue(res); err != nil {
			return nil, err
		} else if set {
			out[AttrDefaultValue] = value
		}
		return out, nil
	}
	out := resource.Attributes{
		AttrName:      name,
		AttrPixel:     resource.Ref{Address: ref.Address},
		AttrRule:      rule,
		AttrEventType: eventType,
	}
	if value, set, err := optionalDefaultValue(res); err != nil {
		return nil, err
	} else if set {
		out[AttrDefaultValue] = value
	}
	return out, nil
}

func (p *Provider) ensureCustomConversionAccount(accountID string) error {
	c, err := p.Client()
	if err != nil {
		return err
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil
	}
	want := strings.TrimPrefix(c.AdAccountID(), "act_")
	got := strings.TrimPrefix(accountID, "act_")
	if got != want {
		return fmt.Errorf("custom conversion belongs to ad account %s, not the configured %s", accountID, c.AdAccountID())
	}
	return nil
}

func requiredPixelRef(res resource.Resource) (resource.Ref, error) {
	v, ok := res.Attributes[AttrPixel]
	if !ok {
		return resource.Ref{}, fmt.Errorf("resource %s: missing required attribute %q", res.Address, AttrPixel)
	}
	if resolved, ok := resource.AsResolved(v); ok {
		ref := resource.Ref{Address: resolved.Address}
		if ref.Address.Provider != Name || ref.Address.Type != TypePixel {
			return resource.Ref{}, fmt.Errorf("resource %s: attribute %q must reference a %s.%s resource", res.Address, AttrPixel, Name, TypePixel)
		}
		return ref, nil
	}
	ref, ok := resource.AsRef(v)
	if !ok {
		return resource.Ref{}, fmt.Errorf("resource %s: attribute %q must be a resource reference ($ref) to a %s.%s resource", res.Address, AttrPixel, Name, TypePixel)
	}
	if ref.Address.Provider != Name || ref.Address.Type != TypePixel {
		return resource.Ref{}, fmt.Errorf("resource %s: attribute %q must reference a %s.%s resource", res.Address, AttrPixel, Name, TypePixel)
	}
	return ref, nil
}

func requiredRule(res resource.Resource) (any, error) {
	v, ok := res.Attributes[AttrRule]
	if !ok {
		return nil, fmt.Errorf("resource %s: missing required attribute %q", res.Address, AttrRule)
	}
	rule, err := parseRule(v)
	if err != nil {
		return nil, fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrRule, err)
	}
	return rule, nil
}

func requiredEventType(res resource.Resource) (string, error) {
	eventType, err := requiredString(res, AttrEventType)
	if err != nil {
		return "", err
	}
	eventType = strings.ToUpper(strings.TrimSpace(eventType))
	if _, ok := customEventTypes[eventType]; !ok {
		return "", fmt.Errorf("resource %s: attribute %q must be one of %s", res.Address, AttrEventType, joinSorted(keys(customEventTypes)))
	}
	return eventType, nil
}

func optionalDefaultValue(res resource.Resource) (any, bool, error) {
	n, set, err := optionalFloat(res, AttrDefaultValue)
	if err != nil || !set {
		return nil, set, err
	}
	if n < 0 {
		return nil, true, fmt.Errorf("resource %s: attribute %q must be greater than or equal to 0", res.Address, AttrDefaultValue)
	}
	return normalizeDefaultValue(n), true, nil
}

func normalizeDefaultValue(n float64) any {
	if n == float64(int64(n)) {
		return int64(n)
	}
	return n
}

func formatDefaultValue(v any) string {
	switch x := v.(type) {
	case int64:
		return fmt.Sprintf("%d", x)
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", x), "0"), ".")
	default:
		return fmt.Sprint(v)
	}
}

func defaultValuesEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	left, err := coerceFloat(a)
	if err != nil {
		return false
	}
	right, err := coerceFloat(b)
	if err != nil {
		return false
	}
	return left == right
}

func (p *Provider) samePixelIdentity(a, b any) bool {
	idA, okA := p.pixelIdentity(a)
	idB, okB := p.pixelIdentity(b)
	if okA && okB {
		return idA == idB
	}
	refA := logicalRef(a)
	refB := logicalRef(b)
	return !refA.IsZero() && refA.Address == refB.Address
}

func (p *Provider) pixelIdentity(v any) (string, bool) {
	if id, ok := objectIDFromAny(v); ok {
		return id, true
	}
	if resolved, ok := resource.AsResolved(v); ok {
		if id, err := normalizeObjectID(resolved.Identity.ID); err == nil {
			return id, true
		}
		if id, err := coerceString(resolved.Outputs[OutputPixelID]); err == nil {
			if normalized, nerr := normalizeObjectID(id); nerr == nil {
				return normalized, true
			}
		}
	}
	ref := logicalRef(v)
	if !ref.IsZero() {
		if id := p.lookupID(ref.Address); id != "" {
			return id, true
		}
	}
	return "", false
}

func pixelIDFromJSON(raw json.RawMessage) (string, error) {
	if len(bytesTrimSpace(raw)) == 0 {
		return "", fmt.Errorf("missing Pixel/Dataset event source")
	}
	var asObject map[string]any
	if err := json.Unmarshal(raw, &asObject); err == nil {
		if id, ok := objectIDFromAny(asObject); ok {
			return id, nil
		}
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		if id, err := normalizeObjectID(asString); err == nil {
			return id, nil
		}
	}
	return "", fmt.Errorf("Pixel/Dataset event source is missing a numeric id")
}

func pixelIDFromAttr(v any) string {
	if id, ok := objectIDFromAny(v); ok {
		return id
	}
	return "(unknown)"
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
