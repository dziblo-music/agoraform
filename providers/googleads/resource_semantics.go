package googleads

import (
	"context"
	"fmt"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

// PlanMissingResource describes provider-native lifecycle semantics for
// resources that are absent during planning. Customer conversion goals are
// created by Google Ads itself, so Agoraform adopts/reconciles them rather than
// claiming to provision a remote object.
func (p *Provider) PlanMissingResource(res resource.Resource) (provider.MissingResourceMode, error) {
	if res.Address.Provider == Name && res.Address.Type == TypeCustomerConversionGoal {
		return provider.MissingResourceAdopt, nil
	}
	return provider.MissingResourceCreate, nil
}

// ValidateResourceSet validates relationships that cannot be checked from one
// resource in isolation. In particular, an explicitly referenced conversion
// action must have the same category as the customer conversion goal it is
// intended to make available.
func (p *Provider) ValidateResourceSet(_ context.Context, resources []resource.Resource) error {
	byAddress := make(map[string]resource.Resource, len(resources))
	for _, res := range resources {
		byAddress[res.Address.String()] = res
	}

	for _, goal := range resources {
		if goal.Address.Provider != Name || goal.Address.Type != TypeCustomerConversionGoal {
			continue
		}
		ref, set, err := optionalConversionActionRef(goal)
		if err != nil {
			return err
		}
		if !set {
			continue
		}

		action, ok := byAddress[ref.Address.String()]
		if !ok {
			return fmt.Errorf("resource %s: attribute %q references %s, which is not declared", goal.Address, AttrConversionAction, ref.Address)
		}
		goalCategory, err := requiredCustomerConversionGoalCategory(goal)
		if err != nil {
			return err
		}
		actionCategory, err := requiredString(action, AttrCategory)
		if err != nil {
			return err
		}
		actionCategory = normalizeEnum(actionCategory)
		if _, ok := conversionActionCategories[actionCategory]; !ok {
			return fmt.Errorf("resource %s: attribute %q must be one of %s", action.Address, AttrCategory, joinSorted(keys(conversionActionCategories)))
		}
		if actionCategory != goalCategory {
			return fmt.Errorf("resource %s: attribute %q references %s with category %s, but the goal category is %s; referenced conversion-action and customer-goal categories must match", goal.Address, AttrConversionAction, action.Address, actionCategory, goalCategory)
		}
	}
	return nil
}
