package googleads

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/dziblo-music/agoraform/internal/resource"
)

// validateCampaignBudgetSafe applies Search-campaign lifecycle restrictions
// that are stricter than the generic CampaignBudget API surface.
func (p *Provider) validateCampaignBudgetSafe(res resource.Resource) error {
	if err := p.validateCampaignBudget(res); err != nil {
		return err
	}
	method, configured, err := optionalEnum(res, AttrDeliveryMethod, campaignBudgetDeliveryMethods)
	if err != nil {
		return err
	}
	if configured && method == deliveryMethodAccelerated {
		return fmt.Errorf("resource %s: attribute %q cannot be ACCELERATED for Search campaign budgets; use STANDARD or omit the attribute", res.Address, AttrDeliveryMethod)
	}
	return nil
}

// normalizeCampaignBudgetComparableSafe keeps planning aligned with Google Ads
// lifecycle rules. Shared budgets cannot become non-shared, and Google owns the
// name of a non-shared budget once it is attached to its single campaign.
func (p *Provider) normalizeCampaignBudgetComparableSafe(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	if err := p.validateCampaignBudgetSafe(desired); err != nil {
		return nil, nil, err
	}

	want, got, err := p.normalizeCampaignBudgetComparable(desired, live)
	if err != nil || live == nil {
		return want, got, err
	}

	desiredShared, err := requiredCampaignBudgetExplicitlyShared(desired)
	if err != nil {
		return nil, nil, err
	}
	liveShared, err := campaignBudgetRemoteShared(desired.Address, *live)
	if err != nil {
		return nil, nil, err
	}
	if liveShared && !desiredShared {
		return nil, nil, fmt.Errorf("resource %s: explicitlyShared cannot change from true to false; Google Ads shared campaign budgets can never become non-shared", desired.Address)
	}

	if !liveShared && !desiredShared {
		// For a non-shared budget Google keeps the budget name synchronized with
		// the attached campaign name. Treat the live name as authoritative after
		// creation so campaign-driven name synchronization does not create drift.
		if liveName, ok := got[AttrName]; ok {
			want[AttrName] = liveName
		}
	}

	return want, got, nil
}

// updateCampaignBudgetSafe performs a fresh read before mutation, validates
// one-way sharing semantics, and sends only fields that actually changed.
// Sparse updates are important for non-shared budgets because Google manages
// their name together with the attached campaign.
func (p *Provider) updateCampaignBudgetSafe(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	// Keep the primitive validator here so direct provider-level tests can still
	// exercise the raw API enum surface. Normal Agoraform validate/plan/create
	// paths use validateCampaignBudgetSafe and reject ACCELERATED for Search.
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

	live, err := p.readCampaignBudgetByID(ctx, desired.Address, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: refreshing current campaign budget: %w", desired.Address, err)
	}

	desiredShared, err := requiredCampaignBudgetExplicitlyShared(desired)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	liveShared, err := campaignBudgetRemoteShared(desired.Address, live)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if liveShared && !desiredShared {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: explicitlyShared cannot change from true to false; Google Ads shared campaign budgets can never become non-shared", desired.Address)
	}

	want, err := comparableCampaignBudget(desired.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}
	got, err := comparableCampaignBudget(live.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: current campaign budget: %w", desired.Address, err)
	}

	changedAttrs := map[string]struct{}{}
	for key, value := range want {
		if key == AttrName && !liveShared && !desiredShared {
			continue
		}
		if !reflect.DeepEqual(value, got[key]) {
			changedAttrs[key] = struct{}{}
		}
	}
	if !liveShared && desiredShared {
		// Google requires a name in the same operation that converts a
		// non-shared budget to explicitly shared, even when the value is
		// unchanged.
		changedAttrs[AttrName] = struct{}{}
	}
	if len(changedAttrs) == 0 {
		return live, nil
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	resourceName := campaignBudgetResourceName(c.CustomerID(), actual.Identity.ID)
	fullBudget, fullMask, err := campaignBudgetMutateBody(desired.Attributes, resourceName)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}

	apiFieldByAttr := map[string]string{
		AttrName:             "name",
		AttrAmount:           "amountMicros",
		AttrDeliveryMethod:   "deliveryMethod",
		AttrExplicitlyShared: "explicitlyShared",
	}
	allowed := map[string]struct{}{}
	for attr := range changedAttrs {
		allowed[apiFieldByAttr[attr]] = struct{}{}
	}

	budget := map[string]any{"resourceName": resourceName}
	mask := make([]string, 0, len(allowed))
	for _, field := range fullMask {
		if _, ok := allowed[field]; !ok {
			continue
		}
		if value, ok := fullBudget[field]; ok {
			budget[field] = value
			mask = append(mask, field)
		}
	}
	sort.Strings(mask)
	if len(mask) == 0 {
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

	refreshed, err := p.readCampaignBudgetByID(ctx, desired.Address, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s succeeded but refreshing campaign budget %q failed: %w", desired.Address, actual.Identity.ID, err)
	}
	return refreshed, nil
}

func campaignBudgetRemoteShared(addr resource.Address, live resource.RemoteResource) (bool, error) {
	value, ok := live.Attributes[AttrExplicitlyShared]
	if !ok {
		return false, fmt.Errorf("resource %s: remote campaign budget is missing %q", addr, AttrExplicitlyShared)
	}
	shared, err := coerceBool(value)
	if err != nil {
		return false, fmt.Errorf("resource %s: remote attribute %q %w", addr, AttrExplicitlyShared, err)
	}
	return shared, nil
}
