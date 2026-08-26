package googleads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

const websiteClickThroughLookbackMax = 30

// validateWebsiteConversionAction applies the API constraints for the WEBPAGE
// conversion actions managed by Agoraform. The shared conversion-action
// validator intentionally covers the broader Google Ads field shape; website
// actions have a stricter 30-day click-through maximum.
func (p *Provider) validateWebsiteConversionAction(res resource.Resource) error {
	if err := p.validateConversionAction(res); err != nil {
		return err
	}
	if days, set, err := optionalInt64(res, AttrClickThroughLookbackWindowDays); err != nil {
		return err
	} else if set && (days < clickThroughLookbackMin || days > websiteClickThroughLookbackMax) {
		return fmt.Errorf("resource %s: attribute %q must be between %d and %d for WEBPAGE conversion actions", res.Address, AttrClickThroughLookbackWindowDays, clickThroughLookbackMin, websiteClickThroughLookbackMax)
	}
	return nil
}

func (p *Provider) readWebsiteConversionAction(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateWebsiteConversionAction(res); err != nil {
		return resource.RemoteResource{}, err
	}

	id, bound, err := boundConversionActionIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if bound {
		live, err := p.readWebsiteConversionActionByID(ctx, res.Address, id)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return live, nil
	}

	name, _, _ := optionalString(res, AttrName)
	matches, err := p.queryWebsiteConversionActions(ctx, "conversion_action.name = "+gaqlString(name))
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, provider.ErrNotFound)
	case 1:
		return remoteConversionAction(res.Address, matches[0])
	default:
		ids := make([]string, 0, len(matches))
		for _, item := range matches {
			ids = append(ids, item.ID)
		}
		sort.Strings(ids)
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: multiple remote conversion actions named %q (ids %s); names must be unique", res.Address, name, strings.Join(ids, ", "))
	}
}

func (p *Provider) createWebsiteConversionAction(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateWebsiteConversionAction(res); err != nil {
		return resource.RemoteResource{}, err
	}
	live, err := p.createConversionAction(ctx, res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if live.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s succeeded but returned no remote identity", res.Address)
	}
	strict, err := p.readWebsiteConversionActionByID(ctx, res.Address, live.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s succeeded but strict refresh of conversion action %q failed: %w", res.Address, live.Identity.ID, err)
	}
	return strict, nil
}

func (p *Provider) updateWebsiteConversionAction(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateWebsiteConversionAction(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	live, err := p.updateConversionAction(ctx, desired, actual)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if live.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s succeeded but returned no remote identity", desired.Address)
	}
	strict, err := p.readWebsiteConversionActionByID(ctx, desired.Address, live.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s succeeded but strict refresh of conversion action %q failed: %w", desired.Address, live.Identity.ID, err)
	}
	return strict, nil
}

func (p *Provider) importWebsiteConversionAction(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	id, err := p.canonicalConversionActionImportID(addr, rawID)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireCustomerID(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	live, err := p.readWebsiteConversionActionByID(ctx, addr, id)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: remote conversion action %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	return live, nil
}

func (p *Provider) readWebsiteConversionActionByID(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	matches, err := p.queryWebsiteConversionActions(ctx, "conversion_action.id = "+id)
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

func (p *Provider) queryWebsiteConversionActions(ctx context.Context, where string) ([]conversionActionData, error) {
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
		item, err := decodeWebsiteConversionActionRow(row, c.CustomerID())
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func decodeWebsiteConversionActionRow(raw json.RawMessage, configuredCustomerID string) (conversionActionData, error) {
	malformed := func(detail string) (conversionActionData, error) {
		if detail == "" {
			return conversionActionData{}, fmt.Errorf("malformed conversion action result")
		}
		return conversionActionData{}, fmt.Errorf("malformed conversion action result: %s", detail)
	}

	var envelope struct {
		ConversionAction json.RawMessage `json:"conversionAction"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.ConversionAction) == 0 {
		return malformed("")
	}

	var body conversionActionJSON
	if err := json.Unmarshal(envelope.ConversionAction, &body); err != nil {
		return malformed("")
	}

	resourceName := strings.TrimSpace(body.ResourceName)
	resourceCustomerID, resourceID, ok := splitConversionActionResourceName(resourceName)
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

	typeName := normalizeEnum(body.Type)
	if typeName == "" {
		return malformed("missing type")
	}
	if typeName != conversionActionTypeWebpage {
		return conversionActionData{}, fmt.Errorf("conversion action %s has type %s; googleads.conversion_action only manages website (WEBPAGE) conversion actions", id, typeName)
	}

	category := normalizeEnum(body.Category)
	if category == "" {
		return malformed("missing category")
	}
	if _, ok := conversionActionCategories[category]; !ok {
		return malformed("unsupported category " + category)
	}

	status := normalizeEnum(body.Status)
	if status != "" {
		if _, ok := conversionActionStatuses[status]; !ok {
			return conversionActionData{}, fmt.Errorf("conversion action %s has status %s; googleads.conversion_action supports statuses %s", id, status, joinSorted(keys(conversionActionStatuses)))
		}
	}

	countingType := normalizeEnum(body.CountingType)
	if countingType != "" {
		if _, err := normalizeCount(countingType); err != nil {
			return conversionActionData{}, fmt.Errorf("conversion action %s has counting type %s; googleads.conversion_action only manages ONE_PER_CLICK or MANY_PER_CLICK counting", id, countingType)
		}
	}

	clickWindow, err := parseOptionalInt64(body.ClickThroughLookbackWindowDays)
	if err != nil {
		return malformed("invalid clickThroughLookbackWindowDays")
	}
	if clickWindow != nil && (*clickWindow < clickThroughLookbackMin || *clickWindow > websiteClickThroughLookbackMax) {
		return malformed("clickThroughLookbackWindowDays is outside the WEBPAGE range")
	}

	viewWindow, err := parseOptionalInt64(body.ViewThroughLookbackWindowDays)
	if err != nil {
		return malformed("invalid viewThroughLookbackWindowDays")
	}
	if viewWindow != nil && (*viewWindow < viewThroughLookbackMin || *viewWindow > viewThroughLookbackMax) {
		return malformed("viewThroughLookbackWindowDays is outside the supported range")
	}

	item := conversionActionData{
		ResourceName:                   resourceName,
		ID:                             id,
		Name:                           body.Name,
		Status:                         status,
		Type:                           typeName,
		Origin:                         normalizeEnum(body.Origin),
		Category:                       category,
		PrimaryForGoal:                 body.PrimaryForGoal,
		OwnerCustomer:                  strings.TrimSpace(body.OwnerCustomer),
		IncludeInConversionsMetric:     body.IncludeInConversionsMetric,
		ClickThroughLookbackWindowDays: clickWindow,
		ViewThroughLookbackWindowDays:  viewWindow,
		CountingType:                   countingType,
		TagSnippets:                    body.TagSnippets,
	}
	if body.ValueSettings != nil {
		item.DefaultValue = body.ValueSettings.DefaultValue
		item.DefaultCurrencyCode = strings.TrimSpace(body.ValueSettings.DefaultCurrencyCode)
		item.AlwaysUseDefaultValue = body.ValueSettings.AlwaysUseDefaultValue
	}
	return item, nil
}
