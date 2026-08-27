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
