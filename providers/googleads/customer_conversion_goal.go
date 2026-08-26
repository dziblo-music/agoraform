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
	// TypeCustomerConversionGoal is the Google Ads customer conversion-goal
	// type used in addresses such as googleads.customer_conversion_goal.signup.
	TypeCustomerConversionGoal = "customer_conversion_goal"

	// AttrOrigin is the conversion origin of a customer conversion goal.
	AttrOrigin = "origin"
	// AttrBiddable controls whether the goal is an account-default
	// optimization goal.
	AttrBiddable = "biddable"
	// AttrConversionAction is an optional $ref to a conversion action that
	// must exist before the provider-created goal can be reconciled.
	AttrConversionAction = "conversionAction"

	customerConversionGoalOriginWebsite = "WEBSITE"
	customerConversionGoalsCollection   = "customerConversionGoals"
)

var (
	supportedCustomerConversionGoalAttrs = map[string]struct{}{
		AttrCategory:         {},
		AttrOrigin:           {},
		AttrBiddable:         {},
		AttrConversionAction: {},
	}

	computedCustomerConversionGoalAttrs = map[string]struct{}{
		"id":            {},
		"resourceName":  {},
		"resource_name": {},
	}

	customerConversionGoalSelect = strings.Join([]string{
		"SELECT",
		"customer_conversion_goal.resource_name,",
		"customer_conversion_goal.category,",
		"customer_conversion_goal.origin,",
		"customer_conversion_goal.biddable",
		"FROM customer_conversion_goal",
	}, " ")
)

func (p *Provider) validateCustomerConversionGoal(res resource.Resource) error {
	if err := p.requireCustomerID(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}

	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedCustomerConversionGoalAttrs[key]; ok {
			continue
		}
		if _, computed := computedCustomerConversionGoalAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; googleads.customer_conversion_goal supports %s", res.Address, key, joinSorted(keys(supportedCustomerConversionGoalAttrs)))
	}

	if _, err := requiredCustomerConversionGoalCategory(res); err != nil {
		return err
	}
	if _, err := requiredCustomerConversionGoalOrigin(res); err != nil {
		return err
	}
	if _, err := requiredBool(res, AttrBiddable); err != nil {
		return err
	}
	if _, _, err := optionalConversionActionRef(res); err != nil {
		return err
	}
	if _, _, err := boundCustomerConversionGoalIdentity(res); err != nil {
		return err
	}
	return nil
}

func (p *Provider) readCustomerConversionGoal(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateCustomerConversionGoal(res); err != nil {
		return resource.RemoteResource{}, err
	}

	id, bound, err := boundCustomerConversionGoalIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if bound {
		live, err := p.readCustomerConversionGoalByID(ctx, res.Address, id)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		if err := ensureCustomerConversionGoalIdentityMatches(res, live.Identity.ID); err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return live, nil
	}

	category, origin, err := configuredCustomerConversionGoalKey(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	live, err := p.readCustomerConversionGoalByID(ctx, res.Address, customerConversionGoalID(category, origin))
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
	}
	return live, nil
}

func (p *Provider) createCustomerConversionGoal(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateCustomerConversionGoal(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundCustomerConversionGoalIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}

	category, origin, err := configuredCustomerConversionGoalKey(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	live, err := p.readCustomerConversionGoalByID(ctx, res.Address, customerConversionGoalID(category, origin))
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, missingCustomerConversionGoalError(res.Address, category, origin)
		}
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}
	return p.reconcileCustomerConversionGoalBiddable(ctx, res, live)
}

func (p *Provider) updateCustomerConversionGoal(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateCustomerConversionGoal(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: missing remote identity", desired.Address)
	}
	if id, bound, err := boundCustomerConversionGoalIdentity(desired); err != nil {
		return resource.RemoteResource{}, err
	} else if bound && id != actual.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: persisted identity %q does not match planned remote identity %q", desired.Address, id, actual.Identity.ID)
	}
	if err := ensureCustomerConversionGoalIdentityMatches(desired, actual.Identity.ID); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}
	return p.reconcileCustomerConversionGoalBiddable(ctx, desired, actual)
}

func (p *Provider) importCustomerConversionGoal(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	id, err := parseImportCustomerConversionGoalID(addr, rawID)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireCustomerID(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	if customerID, restID, ok := splitCustomerConversionGoalResourceName(id); ok {
		configured, err := p.configuredCustomerID()
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
		}
		if customerID != configured {
			return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: resource name customer %s does not match configured %s", addr, customerID, configured)
		}
		id = restID
	}
	live, err := p.readCustomerConversionGoalByID(ctx, addr, id)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: remote customer conversion goal %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	return live, nil
}

func (p *Provider) normalizeCustomerConversionGoalComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	want, err := comparableCustomerConversionGoal(desired.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if live == nil {
		return want, nil, nil
	}
	if id, bound, err := boundCustomerConversionGoalIdentity(desired); err != nil {
		return nil, nil, err
	} else if bound {
		if live.Identity.IsZero() || live.Identity.ID != id {
			return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, id, live.Identity.ID)
		}
	}
	if err := ensureCustomerConversionGoalIdentityMatches(desired, live.Identity.ID); err != nil {
		return nil, nil, err
	}
	got, err := comparableCustomerConversionGoal(live.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	return want, got, nil
}

func (p *Provider) reconcileCustomerConversionGoalBiddable(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	want, err := comparableCustomerConversionGoal(desired.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: %s: %w", desired.Address, err)
	}
	got, err := comparableCustomerConversionGoal(actual.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: %s: %w", desired.Address, err)
	}
	if want[AttrCategory] != got[AttrCategory] || want[AttrOrigin] != got[AttrOrigin] {
		return resource.RemoteResource{}, fmt.Errorf("googleads: %s: category and origin are provider-created identity and cannot be changed from %s/%s to %s/%s", desired.Address, got[AttrCategory], got[AttrOrigin], want[AttrCategory], want[AttrOrigin])
	}
	if want[AttrBiddable] == got[AttrBiddable] {
		live, err := p.readCustomerConversionGoalByID(ctx, desired.Address, actual.Identity.ID)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: %s: %w", desired.Address, err)
		}
		return live, nil
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	resourceName := customerConversionGoalResourceName(c.CustomerID(), actual.Identity.ID)
	_, err = c.Mutate(ctx, customerConversionGoalsCollection, []map[string]any{
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
	live, err := p.readCustomerConversionGoalByID(ctx, desired.Address, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s succeeded but refreshing customer conversion goal %q failed: %w", desired.Address, actual.Identity.ID, err)
	}
	return live, nil
}

func (p *Provider) readCustomerConversionGoalByID(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	category, origin, err := parseCustomerConversionGoalID(addr, id)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	matches, err := p.queryCustomerConversionGoals(ctx, customerConversionGoalWhere(category, origin))
	if err != nil {
		return resource.RemoteResource{}, err
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, provider.ErrNotFound
	case 1:
		return remoteCustomerConversionGoal(addr, matches[0])
	default:
		return resource.RemoteResource{}, fmt.Errorf("multiple remote customer conversion goals returned for %s/%s", category, origin)
	}
}

func (p *Provider) queryCustomerConversionGoals(ctx context.Context, where string) ([]customerConversionGoalData, error) {
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	query := customerConversionGoalSelect
	if strings.TrimSpace(where) != "" {
		query += " WHERE " + where
	}
	rows, err := c.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]customerConversionGoalData, 0, len(rows))
	for _, row := range rows {
		item, err := decodeCustomerConversionGoalRow(row, c.CustomerID())
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

type customerConversionGoalData struct {
	ResourceName string
	Category     string
	Origin       string
	Biddable     bool
}

type customerConversionGoalJSON struct {
	ResourceName string `json:"resourceName"`
	Category     string `json:"category"`
	Origin       string `json:"origin"`
	Biddable     *bool  `json:"biddable"`
}

func decodeCustomerConversionGoalRow(raw json.RawMessage, configuredCustomerID string) (customerConversionGoalData, error) {
	malformed := func(detail string) (customerConversionGoalData, error) {
		if detail == "" {
			return customerConversionGoalData{}, fmt.Errorf("malformed customer conversion goal result")
		}
		return customerConversionGoalData{}, fmt.Errorf("malformed customer conversion goal result: %s", detail)
	}

	var envelope struct {
		CustomerConversionGoal json.RawMessage `json:"customerConversionGoal"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.CustomerConversionGoal) == 0 {
		return malformed("")
	}
	var body customerConversionGoalJSON
	if err := json.Unmarshal(envelope.CustomerConversionGoal, &body); err != nil {
		return malformed("")
	}

	resourceName := strings.TrimSpace(body.ResourceName)
	resourceCustomerID, id, ok := splitCustomerConversionGoalResourceName(resourceName)
	if !ok {
		return malformed("invalid resourceName")
	}
	if configuredCustomerID != "" && resourceCustomerID != configuredCustomerID {
		return malformed("resourceName belongs to a different customer")
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
	if customerConversionGoalID(category, origin) != id {
		return malformed("resourceName does not match category and origin")
	}
	if _, ok := conversionActionCategories[category]; !ok {
		return customerConversionGoalData{}, fmt.Errorf("customer conversion goal %s has unsupported category %s; googleads.customer_conversion_goal only manages website conversion categories", id, category)
	}
	if origin != customerConversionGoalOriginWebsite {
		return customerConversionGoalData{}, fmt.Errorf("customer conversion goal %s has origin %s; googleads.customer_conversion_goal only manages website (WEBSITE) conversion goals", id, origin)
	}

	return customerConversionGoalData{
		ResourceName: resourceName,
		Category:     category,
		Origin:       origin,
		Biddable:     *body.Biddable,
	}, nil
}

func remoteCustomerConversionGoal(addr resource.Address, item customerConversionGoalData) (resource.RemoteResource, error) {
	id := customerConversionGoalID(item.Category, item.Origin)
	attrs := resource.Attributes{
		AttrCategory: item.Category,
		AttrOrigin:   item.Origin,
		AttrBiddable: item.Biddable,
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

func comparableCustomerConversionGoal(attrs resource.Attributes) (resource.Attributes, error) {
	if attrs == nil {
		attrs = resource.Attributes{}
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
		AttrCategory: normalizeEnum(category),
		AttrOrigin:   normalizeEnum(origin),
		AttrBiddable: biddable,
	}, nil
}

func requiredCustomerConversionGoalCategory(res resource.Resource) (string, error) {
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

func requiredCustomerConversionGoalOrigin(res resource.Resource) (string, error) {
	origin, err := requiredString(res, AttrOrigin)
	if err != nil {
		return "", err
	}
	origin = normalizeEnum(origin)
	if origin != customerConversionGoalOriginWebsite {
		return "", fmt.Errorf("resource %s: attribute %q must be %s; googleads.customer_conversion_goal only manages website conversion goals", res.Address, AttrOrigin, customerConversionGoalOriginWebsite)
	}
	return origin, nil
}

func requiredBool(res resource.Resource, key string) (bool, error) {
	v, ok := res.Attributes[key]
	if !ok {
		return false, fmt.Errorf("resource %s: missing required attribute %q", res.Address, key)
	}
	b, err := coerceBool(v)
	if err != nil {
		return false, fmt.Errorf("resource %s: attribute %q %w", res.Address, key, err)
	}
	return b, nil
}

func optionalConversionActionRef(res resource.Resource) (resource.Ref, bool, error) {
	v, ok := res.Attributes[AttrConversionAction]
	if !ok {
		return resource.Ref{}, false, nil
	}
	ref, err := conversionActionRefValue(v)
	if err != nil {
		return resource.Ref{}, true, fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrConversionAction, err)
	}
	if ref.Address.Provider != Name || ref.Address.Type != TypeConversionAction {
		return resource.Ref{}, true, fmt.Errorf("resource %s: attribute %q must reference a %s.%s resource", res.Address, AttrConversionAction, Name, TypeConversionAction)
	}
	return ref, true, nil
}

func conversionActionRefValue(v any) (resource.Ref, error) {
	if resolved, ok := resource.AsResolved(v); ok {
		return resource.Ref{Address: resolved.Address}, nil
	}
	ref, ok := resource.AsRef(v)
	if !ok {
		return resource.Ref{}, fmt.Errorf("must be a resource reference ($ref) to a %s.%s resource", Name, TypeConversionAction)
	}
	return ref, nil
}

func configuredCustomerConversionGoalKey(res resource.Resource) (category, origin string, err error) {
	category, err = requiredCustomerConversionGoalCategory(res)
	if err != nil {
		return "", "", err
	}
	origin, err = requiredCustomerConversionGoalOrigin(res)
	if err != nil {
		return "", "", err
	}
	return category, origin, nil
}

func ensureCustomerConversionGoalIdentityMatches(res resource.Resource, id string) error {
	category, origin, err := configuredCustomerConversionGoalKey(res)
	if err != nil {
		return err
	}
	want := customerConversionGoalID(category, origin)
	if id != want {
		return fmt.Errorf("persisted identity %q does not match configured %s/%s", id, category, origin)
	}
	return nil
}

func boundCustomerConversionGoalIdentity(res resource.Resource) (string, bool, error) {
	if res.Identity.IsZero() {
		return "", false, nil
	}
	id, err := parseCustomerConversionGoalIdentity(res.Address, res.Identity.ID)
	if err != nil {
		return "", true, err
	}
	return id, true, nil
}

func parseCustomerConversionGoalIdentity(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("resource %s: persisted identity is empty; a Google Ads customer conversion goal id of the form CATEGORY~ORIGIN is required", addr)
	}
	if _, id, ok := splitCustomerConversionGoalResourceName(raw); ok {
		raw = id
	}
	if _, _, err := parseCustomerConversionGoalID(addr, raw); err != nil {
		return "", err
	}
	return raw, nil
}

func parseImportCustomerConversionGoalID(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("resource %s: persisted identity is empty; a Google Ads customer conversion goal id of the form CATEGORY~ORIGIN is required", addr)
	}
	if _, id, ok := splitCustomerConversionGoalResourceName(raw); ok {
		if _, _, err := parseCustomerConversionGoalID(addr, id); err != nil {
			return "", err
		}
		return raw, nil
	}
	category, origin, err := parseCustomerConversionGoalID(addr, raw)
	if err != nil {
		return "", err
	}
	return customerConversionGoalID(category, origin), nil
}

func parseCustomerConversionGoalID(addr resource.Address, raw string) (category, origin string, err error) {
	raw = strings.TrimSpace(raw)
	parts := strings.Split(raw, "~")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("resource %s: persisted identity %q is not a valid Google Ads customer conversion goal id; expected CATEGORY~ORIGIN", addr, raw)
	}
	category = normalizeEnum(parts[0])
	origin = normalizeEnum(parts[1])
	if _, ok := conversionActionCategories[category]; !ok {
		return "", "", fmt.Errorf("resource %s: persisted identity %q has unsupported category %s", addr, raw, category)
	}
	if origin != customerConversionGoalOriginWebsite {
		return "", "", fmt.Errorf("resource %s: persisted identity %q has origin %s; googleads.customer_conversion_goal only manages website (WEBSITE) conversion goals", addr, raw, origin)
	}
	return category, origin, nil
}

func splitCustomerConversionGoalResourceName(name string) (customerID, goalID string, ok bool) {
	name = strings.TrimSpace(name)
	parts := strings.Split(name, "/")
	if len(parts) != 4 || parts[0] != "customers" || parts[2] != customerConversionGoalsCollection {
		return "", "", false
	}
	if _, err := NormalizeCustomerID(parts[1]); err != nil {
		return "", "", false
	}
	idParts := strings.Split(parts[3], "~")
	if len(idParts) != 2 || strings.TrimSpace(idParts[0]) == "" || strings.TrimSpace(idParts[1]) == "" {
		return "", "", false
	}
	return parts[1], normalizeEnum(idParts[0]) + "~" + normalizeEnum(idParts[1]), true
}

func customerConversionGoalResourceName(customerID, id string) string {
	return "customers/" + customerID + "/" + customerConversionGoalsCollection + "/" + id
}

func customerConversionGoalID(category, origin string) string {
	return normalizeEnum(category) + "~" + normalizeEnum(origin)
}

func customerConversionGoalWhere(category, origin string) string {
	return "customer_conversion_goal.category = " + gaqlString(category) + " AND customer_conversion_goal.origin = " + gaqlString(origin)
}

func missingCustomerConversionGoalError(addr resource.Address, category, origin string) error {
	return fmt.Errorf("googleads: %s: customer conversion goal %s/%s was not found; Google Ads creates this object automatically after a matching conversion action exists, and Agoraform cannot create or delete it. Apply a googleads.conversion_action with category %s and origin %s first, then retry", addr, category, origin, category, origin)
}
