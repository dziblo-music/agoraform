package googleads_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestCustomerConversionGoalMissingResourceUsesAdoptSemantics(t *testing.T) {
	p := googleads.New(googleads.Config{})
	goal := semanticResource(t, "googleads.customer_conversion_goal.signup", resource.Attributes{})
	mode, err := p.PlanMissingResource(goal)
	if err != nil {
		t.Fatalf("PlanMissingResource: %v", err)
	}
	if mode != provider.MissingResourceAdopt {
		t.Fatalf("mode = %q, want %q", mode, provider.MissingResourceAdopt)
	}

	action := semanticResource(t, "googleads.conversion_action.trial_started", resource.Attributes{})
	mode, err = p.PlanMissingResource(action)
	if err != nil {
		t.Fatalf("PlanMissingResource conversion action: %v", err)
	}
	if mode != provider.MissingResourceCreate {
		t.Fatalf("conversion-action mode = %q, want %q", mode, provider.MissingResourceCreate)
	}

	budget := semanticResource(t, "googleads.campaign_budget.brand", resource.Attributes{})
	mode, err = p.PlanMissingResource(budget)
	if err != nil {
		t.Fatalf("PlanMissingResource campaign budget: %v", err)
	}
	if mode != provider.MissingResourceCreate {
		t.Fatalf("campaign-budget mode = %q, want %q", mode, provider.MissingResourceCreate)
	}

	campaign := semanticResource(t, "googleads.campaign.brand", resource.Attributes{})
	mode, err = p.PlanMissingResource(campaign)
	if err != nil {
		t.Fatalf("PlanMissingResource campaign: %v", err)
	}
	if mode != provider.MissingResourceCreate {
		t.Fatalf("campaign mode = %q, want %q", mode, provider.MissingResourceCreate)
	}

	campaignGoal := semanticResource(t, "googleads.campaign_conversion_goal.trial_signup", resource.Attributes{})
	mode, err = p.PlanMissingResource(campaignGoal)
	if err != nil {
		t.Fatalf("PlanMissingResource campaign conversion goal: %v", err)
	}
	if mode != provider.MissingResourceAdopt {
		t.Fatalf("campaign-conversion-goal mode = %q, want %q", mode, provider.MissingResourceAdopt)
	}

	adGroup := semanticResource(t, "googleads.ad_group.brand", resource.Attributes{})
	mode, err = p.PlanMissingResource(adGroup)
	if err != nil {
		t.Fatalf("PlanMissingResource ad group: %v", err)
	}
	if mode != provider.MissingResourceCreate {
		t.Fatalf("ad-group mode = %q, want %q", mode, provider.MissingResourceCreate)
	}

	keyword := semanticResource(t, "googleads.keyword.brand_exact", resource.Attributes{})
	mode, err = p.PlanMissingResource(keyword)
	if err != nil {
		t.Fatalf("PlanMissingResource keyword: %v", err)
	}
	if mode != provider.MissingResourceCreate {
		t.Fatalf("keyword mode = %q, want %q", mode, provider.MissingResourceCreate)
	}
}

func TestCustomerConversionGoalReferenceCategoryMustMatch(t *testing.T) {
	p := googleads.New(googleads.Config{})
	actionAddr := semanticAddress(t, "googleads.conversion_action.purchase")
	action := resource.Resource{
		Address: actionAddr,
		Attributes: resource.Attributes{
			googleads.AttrName:     "Purchase",
			googleads.AttrCategory: "PURCHASE",
		},
	}
	goal := semanticResource(t, "googleads.customer_conversion_goal.signup", resource.Attributes{
		googleads.AttrCategory:         "SIGNUP",
		googleads.AttrOrigin:           "WEBSITE",
		googleads.AttrBiddable:         true,
		googleads.AttrConversionAction: resource.Ref{Address: actionAddr},
	})

	err := p.ValidateResourceSet(context.Background(), []resource.Resource{goal, action})
	if err == nil {
		t.Fatal("expected category mismatch")
	}
	if !strings.Contains(err.Error(), "PURCHASE") || !strings.Contains(err.Error(), "SIGNUP") || !strings.Contains(err.Error(), "must match") {
		t.Fatalf("error = %q, want actionable category mismatch", err)
	}
}

func TestCustomerConversionGoalReferenceCategoryMatch(t *testing.T) {
	p := googleads.New(googleads.Config{})
	actionAddr := semanticAddress(t, "googleads.conversion_action.trial_started")
	action := resource.Resource{
		Address: actionAddr,
		Attributes: resource.Attributes{
			googleads.AttrName:     "Trial Started",
			googleads.AttrCategory: "signup",
		},
	}
	goal := semanticResource(t, "googleads.customer_conversion_goal.signup", resource.Attributes{
		googleads.AttrCategory:         "SIGNUP",
		googleads.AttrOrigin:           "WEBSITE",
		googleads.AttrBiddable:         true,
		googleads.AttrConversionAction: resource.Ref{Address: actionAddr},
	})

	if err := p.ValidateResourceSet(context.Background(), []resource.Resource{goal, action}); err != nil {
		t.Fatalf("ValidateResourceSet: %v", err)
	}
}

func TestCampaignConversionGoalReferenceCategoryMustMatch(t *testing.T) {
	p := googleads.New(googleads.Config{})
	actionAddr := semanticAddress(t, "googleads.conversion_action.purchase")
	campaignAddr := semanticAddress(t, "googleads.campaign.brand")
	action := resource.Resource{
		Address: actionAddr,
		Attributes: resource.Attributes{
			googleads.AttrName:     "Purchase",
			googleads.AttrCategory: "PURCHASE",
		},
	}
	goal := semanticResource(t, "googleads.campaign_conversion_goal.trial_signup", resource.Attributes{
		googleads.AttrCampaign:         resource.Ref{Address: campaignAddr},
		googleads.AttrCategory:         "SIGNUP",
		googleads.AttrOrigin:           "WEBSITE",
		googleads.AttrBiddable:         true,
		googleads.AttrConversionAction: resource.Ref{Address: actionAddr},
	})

	err := p.ValidateResourceSet(context.Background(), []resource.Resource{goal, action})
	if err == nil {
		t.Fatal("expected category mismatch")
	}
	if !strings.Contains(err.Error(), "PURCHASE") || !strings.Contains(err.Error(), "SIGNUP") || !strings.Contains(err.Error(), "must match") {
		t.Fatalf("error = %q, want actionable category mismatch", err)
	}
}

func TestCampaignConversionGoalReferenceCategoryMatch(t *testing.T) {
	p := googleads.New(googleads.Config{})
	actionAddr := semanticAddress(t, "googleads.conversion_action.trial_started")
	campaignAddr := semanticAddress(t, "googleads.campaign.brand")
	action := resource.Resource{
		Address: actionAddr,
		Attributes: resource.Attributes{
			googleads.AttrName:     "Trial Started",
			googleads.AttrCategory: "signup",
		},
	}
	goal := semanticResource(t, "googleads.campaign_conversion_goal.trial_signup", resource.Attributes{
		googleads.AttrCampaign:         resource.Ref{Address: campaignAddr},
		googleads.AttrCategory:         "SIGNUP",
		googleads.AttrOrigin:           "WEBSITE",
		googleads.AttrBiddable:         true,
		googleads.AttrConversionAction: resource.Ref{Address: actionAddr},
	})

	if err := p.ValidateResourceSet(context.Background(), []resource.Resource{goal, action}); err != nil {
		t.Fatalf("ValidateResourceSet: %v", err)
	}
}

func TestKeywordDuplicateTextAndMatchTypeRejected(t *testing.T) {
	p := googleads.New(googleads.Config{})
	adGroupAddr := semanticAddress(t, "googleads.ad_group.brand")
	first := semanticResource(t, "googleads.keyword.brand_exact", resource.Attributes{
		googleads.AttrAdGroup:   resource.Ref{Address: adGroupAddr},
		googleads.AttrText:      "Brand",
		googleads.AttrMatchType: "exact",
	})
	second := semanticResource(t, "googleads.keyword.brand_exact_dup", resource.Attributes{
		googleads.AttrAdGroup:   resource.Ref{Address: adGroupAddr},
		googleads.AttrText:      "brand",
		googleads.AttrMatchType: "EXACT",
	})

	err := p.ValidateResourceSet(context.Background(), []resource.Resource{first, second})
	if err == nil {
		t.Fatal("expected duplicate keyword error")
	}
	if !strings.Contains(err.Error(), "duplicates") || !strings.Contains(err.Error(), "brand") || !strings.Contains(err.Error(), "EXACT") {
		t.Fatalf("error = %q, want duplicate keyword diagnostic", err)
	}
}

func TestKeywordSameTextDifferentMatchTypeAllowed(t *testing.T) {
	p := googleads.New(googleads.Config{})
	adGroupAddr := semanticAddress(t, "googleads.ad_group.brand")
	exact := semanticResource(t, "googleads.keyword.brand_exact", resource.Attributes{
		googleads.AttrAdGroup:   resource.Ref{Address: adGroupAddr},
		googleads.AttrText:      "brand",
		googleads.AttrMatchType: "EXACT",
	})
	phrase := semanticResource(t, "googleads.keyword.brand_phrase", resource.Attributes{
		googleads.AttrAdGroup:   resource.Ref{Address: adGroupAddr},
		googleads.AttrText:      "brand",
		googleads.AttrMatchType: "PHRASE",
	})

	if err := p.ValidateResourceSet(context.Background(), []resource.Resource{exact, phrase}); err != nil {
		t.Fatalf("ValidateResourceSet: %v", err)
	}
}

func TestKeywordSameTextDifferentAdGroupAllowed(t *testing.T) {
	p := googleads.New(googleads.Config{})
	brand := semanticAddress(t, "googleads.ad_group.brand")
	generic := semanticAddress(t, "googleads.ad_group.generic")
	first := semanticResource(t, "googleads.keyword.brand_exact", resource.Attributes{
		googleads.AttrAdGroup:   resource.Ref{Address: brand},
		googleads.AttrText:      "brand",
		googleads.AttrMatchType: "EXACT",
	})
	second := semanticResource(t, "googleads.keyword.generic_exact", resource.Attributes{
		googleads.AttrAdGroup:   resource.Ref{Address: generic},
		googleads.AttrText:      "brand",
		googleads.AttrMatchType: "EXACT",
	})

	if err := p.ValidateResourceSet(context.Background(), []resource.Resource{first, second}); err != nil {
		t.Fatalf("ValidateResourceSet: %v", err)
	}
}

func semanticResource(t *testing.T, address string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{Address: semanticAddress(t, address), Attributes: attrs}
}

func semanticAddress(t *testing.T, raw string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress(raw)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}
