package googleads

import (
	"context"
	"fmt"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

// PlanMissingResource describes provider-native lifecycle semantics for
// resources that are absent during planning. Customer and campaign conversion
// goals are created by Google Ads itself, so Agoraform adopts/reconciles them
// rather than claiming to provision a remote object.
func (p *Provider) PlanMissingResource(res resource.Resource) (provider.MissingResourceMode, error) {
	if res.Address.Provider == Name && (res.Address.Type == TypeCustomerConversionGoal || res.Address.Type == TypeCampaignConversionGoal) {
		return provider.MissingResourceAdopt, nil
	}
	return provider.MissingResourceCreate, nil
}

// ValidateResourceSet validates relationships that cannot be checked from one
// resource in isolation. In particular, an explicitly referenced conversion
// action must have the same category as the customer or campaign conversion
// goal it is intended to make available.
func (p *Provider) ValidateResourceSet(ctx context.Context, resources []resource.Resource) error {
	byAddress := make(map[string]resource.Resource, len(resources))
	for _, res := range resources {
		byAddress[res.Address.String()] = res
	}

	for _, goal := range resources {
		if goal.Address.Provider != Name {
			continue
		}
		var goalKind string
		var goalCategory string
		var err error
		switch goal.Address.Type {
		case TypeCustomerConversionGoal:
			goalKind = "customer-goal"
			goalCategory, err = requiredCustomerConversionGoalCategory(goal)
		case TypeCampaignConversionGoal:
			goalKind = "campaign-goal"
			goalCategory, err = requiredCampaignConversionGoalCategory(goal)
		default:
			continue
		}
		if err != nil {
			return err
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
		actionCategory, err := requiredString(action, AttrCategory)
		if err != nil {
			return err
		}
		actionCategory = normalizeEnum(actionCategory)
		if _, ok := conversionActionCategories[actionCategory]; !ok {
			return fmt.Errorf("resource %s: attribute %q must be one of %s", action.Address, AttrCategory, joinSorted(keys(conversionActionCategories)))
		}
		if actionCategory != goalCategory {
			return fmt.Errorf("resource %s: attribute %q references %s with category %s, but the goal category is %s; referenced conversion-action and %s categories must match", goal.Address, AttrConversionAction, action.Address, actionCategory, goalCategory, goalKind)
		}
	}

	seenKeywords := map[string]resource.Address{}
	for _, res := range resources {
		if res.Address.Provider != Name || res.Address.Type != TypeKeyword {
			continue
		}
		key, err := keywordNaturalKey(res)
		if err != nil {
			continue
		}
		if other, ok := seenKeywords[key]; ok {
			text, _ := requiredKeywordText(res)
			matchType, _ := requiredKeywordMatchType(res)
			ref, _ := requiredAdGroupRef(res)
			return fmt.Errorf("resource %s: duplicates %s; keyword text %q with match type %s is already declared for ad group %s", res.Address, other, text, matchType, ref.Address)
		}
		seenKeywords[key] = res.Address
	}

	seenLocations := map[string]resource.Address{}
	seenCanonicalLocations := map[string]resource.Address{}
	for _, res := range resources {
		if res.Address.Provider != Name || res.Address.Type != TypeCampaignLocation {
			continue
		}
		key, err := campaignLocationNaturalKey(res)
		if err != nil {
			continue
		}
		if other, ok := seenLocations[key]; ok {
			location, _ := requiredLocationValue(res)
			ref, _ := requiredCampaignRef(res)
			return fmt.Errorf("resource %s: duplicates %s; location %q is already declared for campaign %s", res.Address, other, location, ref.Address)
		}
		seenLocations[key] = res.Address

		ref, err := requiredCampaignRef(res)
		if err != nil {
			continue
		}
		target, err := p.resolveGeoTarget(ctx, res.Address, res.Attributes[AttrLocation])
		if err != nil {
			// Resource-set validation must remain useful without live API access.
			// Planning/read still resolves every target before any mutation.
			continue
		}
		canonicalKey := ref.Address.String() + "\x00" + target.ResourceName
		if other, ok := seenCanonicalLocations[canonicalKey]; ok {
			location, _ := requiredLocationValue(res)
			return fmt.Errorf("resource %s: duplicates %s; location %q resolves to %s, which is already declared for campaign %s", res.Address, other, location, target.ResourceName, ref.Address)
		}
		seenCanonicalLocations[canonicalKey] = res.Address
	}

	seenLanguages := map[string]resource.Address{}
	seenCanonicalLanguages := map[string]resource.Address{}
	for _, res := range resources {
		if res.Address.Provider != Name || res.Address.Type != TypeCampaignLanguage {
			continue
		}
		key, err := campaignLanguageNaturalKey(res)
		if err != nil {
			continue
		}
		if other, ok := seenLanguages[key]; ok {
			language, _ := requiredLanguageValue(res)
			ref, _ := requiredCampaignRef(res)
			return fmt.Errorf("resource %s: duplicates %s; language %q is already declared for campaign %s", res.Address, other, language, ref.Address)
		}
		seenLanguages[key] = res.Address

		ref, err := requiredCampaignRef(res)
		if err != nil {
			continue
		}
		language, err := p.resolveLanguage(ctx, res.Address, res.Attributes[AttrLanguage])
		if err != nil {
			// Resource-set validation must remain useful without live API access.
			// Planning/read still resolves every target before any mutation.
			continue
		}
		canonicalKey := ref.Address.String() + "\x00" + language.ResourceName
		if other, ok := seenCanonicalLanguages[canonicalKey]; ok {
			value, _ := requiredLanguageValue(res)
			return fmt.Errorf("resource %s: duplicates %s; language %q resolves to %s, which is already declared for campaign %s", res.Address, other, value, language.ResourceName, ref.Address)
		}
		seenCanonicalLanguages[canonicalKey] = res.Address
	}
	return nil
}
