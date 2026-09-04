package meta

import (
	"context"
	"fmt"

	"github.com/dziblo-music/agoraform/internal/resource"
)

// ValidateResourceSet checks Meta relationships whose validity depends on the
// referenced resource's configuration. It never contacts or mutates Meta.
func (p *Provider) ValidateResourceSet(_ context.Context, resources []resource.Resource) error {
	byAddress := make(map[string]resource.Resource, len(resources))
	for _, res := range resources {
		byAddress[res.Address.String()] = res
	}
	for _, res := range resources {
		if res.Address.Provider != Name || res.Address.Type != TypeAdSet {
			continue
		}
		ad, err := normalizeAdSet(res)
		if err != nil {
			return err
		}
		campaign, ok := byAddress[ad.Campaign.Address.String()]
		if !ok {
			return fmt.Errorf("resource %s: campaign reference %s is not declared", res.Address, ad.Campaign.Address)
		}
		managedCampaign, err := normalizeCampaign(campaign)
		if err != nil {
			return fmt.Errorf("resource %s: referenced campaign %s is invalid: %w", res.Address, campaign.Address, err)
		}
		campaignOwnsBudget := managedCampaign.HasDailyBudget || managedCampaign.HasLifetimeBudget
		adSetOwnsBudget := ad.HasDailyBudget || ad.HasLifetimeBudget
		switch {
		case campaignOwnsBudget && adSetOwnsBudget:
			return fmt.Errorf("resource %s: budget ownership conflicts with campaign %s; remove dailyBudget/lifetimeBudget from the ad set", res.Address, campaign.Address)
		case !campaignOwnsBudget && !adSetOwnsBudget:
			return fmt.Errorf("resource %s: campaign %s has no campaign-level budget, so the ad set must declare exactly one of dailyBudget or lifetimeBudget", res.Address, campaign.Address)
		case campaignOwnsBudget && res.Attributes[AttrBidAmount] != nil:
			return fmt.Errorf("resource %s: campaign %s owns budget and bidding; ad-set bidAmount is not allowed", res.Address, campaign.Address)
		case campaignOwnsBudget && res.Attributes[AttrBidStrategy] != nil && ad.BidStrategy != "LOWEST_COST_WITHOUT_CAP":
			return fmt.Errorf("resource %s: campaign %s owns bidding; only the canonical inherited LOWEST_COST_WITHOUT_CAP value is allowed on the ad set", res.Address, campaign.Address)
		}
		if ad.OptimizationGoal == "OFFSITE_CONVERSIONS" && managedCampaign.Objective != "OUTCOME_SALES" {
			return fmt.Errorf("resource %s: OFFSITE_CONVERSIONS requires referenced campaign %s to use objective OUTCOME_SALES", res.Address, campaign.Address)
		}
		if ad.OptimizationGoal == "LINK_CLICKS" && managedCampaign.Objective != "OUTCOME_TRAFFIC" && managedCampaign.Objective != "OUTCOME_SALES" {
			return fmt.Errorf("resource %s: LINK_CLICKS requires referenced campaign %s to use OUTCOME_TRAFFIC or OUTCOME_SALES", res.Address, campaign.Address)
		}
		if ad.OptimizationGoal == "OFFSITE_CONVERSIONS" {
			conversion, ok := byAddress[ad.CustomConversion.Address.String()]
			if !ok {
				return fmt.Errorf("resource %s: customConversion reference %s is not declared", res.Address, ad.CustomConversion.Address)
			}
			conversionPixel, err := requiredPixelRef(conversion)
			if err != nil {
				return fmt.Errorf("resource %s: referenced custom conversion %s is invalid: %w", res.Address, conversion.Address, err)
			}
			if conversionPixel.Address != ad.Pixel.Address {
				return fmt.Errorf("resource %s: pixel %s does not match custom conversion %s pixel %s", res.Address, ad.Pixel.Address, conversion.Address, conversionPixel.Address)
			}
		}
	}
	return nil
}
