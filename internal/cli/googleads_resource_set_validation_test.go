package cli_test

import (
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/internal/provider"
)

const googleAdsMismatchedGoalReferenceManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: googleads.conversion_action.purchase
    attributes:
      name: Purchase
      category: PURCHASE
  - address: googleads.customer_conversion_goal.signup
    attributes:
      category: SIGNUP
      origin: WEBSITE
      biddable: true
      conversionAction:
        $ref: googleads.conversion_action.purchase
`

const googleAdsMismatchedCampaignGoalReferenceManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: googleads.campaign_budget.brand
    attributes:
      name: Brand daily budget
      amount: 50
      explicitlyShared: false
  - address: googleads.campaign.brand
    attributes:
      name: Brand
      budget:
        $ref: googleads.campaign_budget.brand
      bidding:
        strategy: MANUAL_CPC
  - address: googleads.conversion_action.purchase
    attributes:
      name: Purchase
      category: PURCHASE
  - address: googleads.campaign_conversion_goal.trial_signup
    attributes:
      campaign:
        $ref: googleads.campaign.brand
      category: SIGNUP
      origin: WEBSITE
      biddable: true
      conversionAction:
        $ref: googleads.conversion_action.purchase
`

const googleAdsDuplicateKeywordManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: googleads.campaign_budget.brand
    attributes:
      name: Brand daily budget
      amount: 50
      explicitlyShared: false
  - address: googleads.campaign.brand
    attributes:
      name: Brand
      budget:
        $ref: googleads.campaign_budget.brand
      bidding:
        strategy: MANUAL_CPC
  - address: googleads.ad_group.brand
    attributes:
      name: Brand
      campaign:
        $ref: googleads.campaign.brand
  - address: googleads.keyword.brand_exact
    attributes:
      adGroup:
        $ref: googleads.ad_group.brand
      text: Brand
      matchType: exact
  - address: googleads.keyword.brand_exact_dup
    attributes:
      adGroup:
        $ref: googleads.ad_group.brand
      text: brand
      matchType: EXACT
`

func TestValidateGoogleAdsCustomerGoalReferenceCategoryMismatch(t *testing.T) {
	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsMismatchedGoalReferenceManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d", code, cli.ExitError)
	}
	errOut := stderr.String()
	if !strings.Contains(errOut, "PURCHASE") || !strings.Contains(errOut, "SIGNUP") || !strings.Contains(errOut, "must match") {
		t.Fatalf("stderr = %q, want actionable category mismatch", errOut)
	}
}

func TestValidateGoogleAdsCampaignGoalReferenceCategoryMismatch(t *testing.T) {
	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsMismatchedCampaignGoalReferenceManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d", code, cli.ExitError)
	}
	errOut := stderr.String()
	if !strings.Contains(errOut, "PURCHASE") || !strings.Contains(errOut, "SIGNUP") || !strings.Contains(errOut, "must match") {
		t.Fatalf("stderr = %q, want actionable category mismatch", errOut)
	}
}

func TestValidateGoogleAdsDuplicateKeywords(t *testing.T) {
	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsDuplicateKeywordManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d", code, cli.ExitError)
	}
	errOut := stderr.String()
	if !strings.Contains(errOut, "duplicates") || !strings.Contains(errOut, "EXACT") {
		t.Fatalf("stderr = %q, want duplicate keyword diagnostic", errOut)
	}
}
