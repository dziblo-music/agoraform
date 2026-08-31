package cli_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/providers/googleads"
	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

const validManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: Homepage banner
`

const emptyResourcesManifest = `apiVersion: agoraform.io/v1alpha1
resources: []
`

const matomoGoalManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.goal.trial_started
    attributes:
      name: Trial Started
      matchAttribute: event_action
      pattern: trialStarted
`

const matomoGoalIncompleteManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.goal.trial_started
    attributes:
      name: Trial Started
      matchAttribute: event_action
`

const matomoVariableManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.variable.user_id
    attributes:
      type: dataLayer
      key: userId
`

const matomoGoalAndVariableManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.goal.trial_started
    attributes:
      name: Trial Started
      matchAttribute: event_action
      pattern: trialStarted
  - address: matomo.variable.user_id
    attributes:
      type: dataLayer
      key: userId
`

const matomoTriggerManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.trigger.trial_started
    attributes:
      type: customEvent
      event: trialStarted
`

const matomoTagManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.trigger.trial_started
    attributes:
      type: customEvent
      event: trialStarted
  - address: matomo.tag.trial_started
    attributes:
      type: matomoAnalytics
      trigger:
        $ref: matomo.trigger.trial_started
      eventCategory: signup
      eventAction: trialStarted
`

const matomoGoogleAdsConversionTagManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: googleads.conversion_action.trial_started
    attributes:
      name: Trial Started
      category: SIGNUP
  - address: matomo.trigger.trial_started
    attributes:
      type: customEvent
      event: trialStarted
  - address: matomo.tag.google_ads_trial_started
    attributes:
      type: googleAdsConversion
      trigger:
        $ref: matomo.trigger.trial_started
      conversionId:
        $ref: googleads.conversion_action.trial_started
        output: conversionId
      conversionLabel:
        $ref: googleads.conversion_action.trial_started
        output: conversionLabel
`

const invalidManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: not-an-address
    attributes:
      title: Homepage banner
`

const googleAdsProviderManifest = `apiVersion: agoraform.io/v1alpha1
providers:
  googleads: {}
resources: []
`

const googleAdsConversionActionManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: googleads.conversion_action.trial_started
    attributes:
      name: Trial Started
      category: SIGNUP
      value: 0
      count: ONE
      primaryForGoal: true
`

const googleAdsConversionActionIncompleteManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: googleads.conversion_action.trial_started
    attributes:
      name: Trial Started
`

const googleAdsCustomerConversionGoalManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: googleads.customer_conversion_goal.signup
    attributes:
      category: SIGNUP
      origin: WEBSITE
      biddable: true
`

const googleAdsCustomerConversionGoalIncompleteManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: googleads.customer_conversion_goal.signup
    attributes:
      category: SIGNUP
      origin: WEBSITE
`

const googleAdsUnknownResourceManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: googleads.ad.brand
    attributes:
      text: brand
`

const googleAdsCampaignBudgetManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: googleads.campaign_budget.brand
    attributes:
      name: Brand daily budget
      amount: 50
      explicitlyShared: false
      deliveryMethod: STANDARD
`

const googleAdsCampaignBudgetIncompleteManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: googleads.campaign_budget.brand
    attributes:
      name: Brand daily budget
      explicitlyShared: false
`

const googleAdsSearchCampaignManifest = `apiVersion: agoraform.io/v1alpha1
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
`

const googleAdsSearchCampaignIncompleteManifest = `apiVersion: agoraform.io/v1alpha1
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
`

const googleAdsSearchCampaignDisplayManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: googleads.campaign_budget.brand
    attributes:
      name: Brand daily budget
      amount: 50
      explicitlyShared: false
  - address: googleads.campaign.brand
    attributes:
      name: Brand
      advertisingChannelType: DISPLAY
      budget:
        $ref: googleads.campaign_budget.brand
      bidding:
        strategy: MANUAL_CPC
`

const googleAdsCampaignConversionGoalManifest = `apiVersion: agoraform.io/v1alpha1
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
  - address: googleads.campaign_conversion_goal.trial_signup
    attributes:
      campaign:
        $ref: googleads.campaign.brand
      category: SIGNUP
      origin: WEBSITE
      biddable: true
`

const googleAdsCampaignConversionGoalIncompleteManifest = `apiVersion: agoraform.io/v1alpha1
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
  - address: googleads.campaign_conversion_goal.trial_signup
    attributes:
      campaign:
        $ref: googleads.campaign.brand
      category: SIGNUP
      origin: WEBSITE
`

const googleAdsAdGroupManifest = `apiVersion: agoraform.io/v1alpha1
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
      status: PAUSED
      type: SEARCH_STANDARD
      cpcBid: 1.5
`

const googleAdsAdGroupIncompleteManifest = `apiVersion: agoraform.io/v1alpha1
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
`

const googleAdsAdGroupShoppingManifest = `apiVersion: agoraform.io/v1alpha1
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
      type: SHOPPING_PRODUCT_ADS
`

const googleAdsKeywordManifest = `apiVersion: agoraform.io/v1alpha1
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
      text: brand
      matchType: EXACT
      status: PAUSED
      cpcBid: 1.5
`

const googleAdsKeywordIncompleteManifest = `apiVersion: agoraform.io/v1alpha1
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
      text: brand
      matchType: EXACT
`

const googleAdsKeywordInvalidMatchManifest = `apiVersion: agoraform.io/v1alpha1
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
      text: brand
      matchType: BROAD_MATCH_MODIFIED
`

const googleAdsResponsiveSearchAdManifest = `apiVersion: agoraform.io/v1alpha1
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
  - address: googleads.responsive_search_ad.brand
    attributes:
      adGroup:
        $ref: googleads.ad_group.brand
      finalUrls:
        - https://example.com/
      headlines:
        - Buy shoes online
        - Free shipping today
        - Shop the collection
      descriptions:
        - Find shoes that fit your style.
        - Free returns on every order.
`

const googleAdsResponsiveSearchAdIncompleteManifest = `apiVersion: agoraform.io/v1alpha1
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
  - address: googleads.responsive_search_ad.brand
    attributes:
      headlines:
        - Buy shoes online
        - Free shipping today
        - Shop the collection
      descriptions:
        - Find shoes that fit your style.
        - Free returns on every order.
`

const googleAdsResponsiveSearchAdTooFewHeadlinesManifest = `apiVersion: agoraform.io/v1alpha1
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
  - address: googleads.responsive_search_ad.brand
    attributes:
      adGroup:
        $ref: googleads.ad_group.brand
      finalUrls:
        - https://example.com/
      headlines:
        - Buy shoes online
        - Free shipping today
      descriptions:
        - Find shoes that fit your style.
        - Free returns on every order.
`

const googleAdsTargetingManifest = `apiVersion: agoraform.io/v1alpha1
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
      locationTargeting:
        positive: PRESENCE
        negative: PRESENCE
  - address: googleads.campaign_location.united_states
    attributes:
      campaign:
        $ref: googleads.campaign.brand
      location: United States
  - address: googleads.campaign_location.exclude_canada
    attributes:
      campaign:
        $ref: googleads.campaign.brand
      location: Canada
      negative: true
  - address: googleads.campaign_language.english
    attributes:
      campaign:
        $ref: googleads.campaign.brand
      language: en
`

const googleAdsLocationIncompleteManifest = `apiVersion: agoraform.io/v1alpha1
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
  - address: googleads.campaign_location.united_states
    attributes:
      location: United States
`

const googleAdsLocationInvalidModeManifest = `apiVersion: agoraform.io/v1alpha1
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
      locationTargeting:
        positive: UNKNOWN
`

func TestValidateSuccess(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", emptyResourcesManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Validated") {
		t.Fatalf("stdout = %q, want Validated message", stdout.String())
	}
	if !strings.Contains(stdout.String(), "0 resources") {
		t.Fatalf("stdout = %q, want resource count", stdout.String())
	}
}

func TestValidateGoogleAdsMissingCredentials(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", googleAdsProviderManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	errOut := stderr.String()
	if !strings.Contains(errOut, googleads.EnvDeveloperToken) && !strings.Contains(errOut, "required") {
		t.Fatalf("stderr = %q, want missing %s", errOut, googleads.EnvDeveloperToken)
	}
}

func TestValidateGoogleAdsUnknownResourceType(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", googleAdsUnknownResourceManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	errOut := stderr.String()
	if !strings.Contains(errOut, "unknown resource type") {
		t.Fatalf("stderr = %q, want unknown resource type", errOut)
	}
}

func TestValidateGoogleAdsConversionActionMissingCredentials(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", googleAdsConversionActionManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	errOut := stderr.String()
	if strings.Contains(errOut, "unknown resource type") {
		t.Fatalf("stderr = %q, googleads.conversion_action should be registered", errOut)
	}
	if !strings.Contains(errOut, googleads.EnvDeveloperToken) && !strings.Contains(errOut, "required") {
		t.Fatalf("stderr = %q, want missing credential error", errOut)
	}
}

func TestValidateGoogleAdsConversionActionWithProvider(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsConversionActionManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 resource") {
		t.Fatalf("stdout = %q, want 1 resource", stdout.String())
	}
}

func TestValidateGoogleAdsConversionActionMissingCategory(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsConversionActionIncompleteManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "category") {
		t.Fatalf("stderr = %q, want category validation error", stderr.String())
	}
}

func TestValidateGoogleAdsCustomerConversionGoalMissingCredentials(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", googleAdsCustomerConversionGoalManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	errOut := stderr.String()
	if strings.Contains(errOut, "unknown resource type") {
		t.Fatalf("stderr = %q, googleads.customer_conversion_goal should be registered", errOut)
	}
	if !strings.Contains(errOut, googleads.EnvDeveloperToken) && !strings.Contains(errOut, "required") {
		t.Fatalf("stderr = %q, want missing credential error", errOut)
	}
}

func TestValidateGoogleAdsCustomerConversionGoalWithProvider(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsCustomerConversionGoalManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 resource") {
		t.Fatalf("stdout = %q, want 1 resource", stdout.String())
	}
}

func TestValidateGoogleAdsCustomerConversionGoalMissingBiddable(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsCustomerConversionGoalIncompleteManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "biddable") {
		t.Fatalf("stderr = %q, want biddable validation error", stderr.String())
	}
}

func TestValidateGoogleAdsCampaignBudgetMissingCredentials(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", googleAdsCampaignBudgetManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	errOut := stderr.String()
	if strings.Contains(errOut, "unknown resource type") {
		t.Fatalf("stderr = %q, googleads.campaign_budget should be registered", errOut)
	}
	if !strings.Contains(errOut, googleads.EnvDeveloperToken) && !strings.Contains(errOut, "required") {
		t.Fatalf("stderr = %q, want missing credential error", errOut)
	}
}

func TestValidateGoogleAdsCampaignBudgetWithProvider(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsCampaignBudgetManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 resource") {
		t.Fatalf("stdout = %q, want 1 resource", stdout.String())
	}
}

func TestValidateGoogleAdsCampaignBudgetMissingAmount(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsCampaignBudgetIncompleteManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "amount") {
		t.Fatalf("stderr = %q, want amount validation error", stderr.String())
	}
}

func TestValidateGoogleAdsSearchCampaignWithProvider(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsSearchCampaignManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "2 resources") {
		t.Fatalf("stdout = %q, want 2 resources", stdout.String())
	}
}

func TestValidateGoogleAdsSearchCampaignMissingBidding(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsSearchCampaignIncompleteManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "bidding") {
		t.Fatalf("stderr = %q, want bidding validation error", stderr.String())
	}
}

func TestValidateGoogleAdsSearchCampaignRejectsDisplay(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsSearchCampaignDisplayManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "SEARCH") {
		t.Fatalf("stderr = %q, want SEARCH guidance", stderr.String())
	}
}

func TestValidateGoogleAdsCampaignConversionGoalWithProvider(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsCampaignConversionGoalManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "3 resources") {
		t.Fatalf("stdout = %q, want 3 resources", stdout.String())
	}
}

func TestValidateGoogleAdsCampaignConversionGoalMissingBiddable(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsCampaignConversionGoalIncompleteManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "biddable") {
		t.Fatalf("stderr = %q, want biddable validation error", stderr.String())
	}
}

func TestValidateGoogleAdsAdGroupMissingCredentials(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", googleAdsAdGroupManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	errOut := stderr.String()
	if strings.Contains(errOut, "unknown resource type") {
		t.Fatalf("stderr = %q, googleads.ad_group should be registered", errOut)
	}
	if !strings.Contains(errOut, googleads.EnvDeveloperToken) && !strings.Contains(errOut, "required") {
		t.Fatalf("stderr = %q, want missing credential error", errOut)
	}
}

func TestValidateGoogleAdsAdGroupWithProvider(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsAdGroupManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "3 resources") {
		t.Fatalf("stdout = %q, want 3 resources", stdout.String())
	}
}

func TestValidateGoogleAdsAdGroupMissingCampaign(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsAdGroupIncompleteManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "campaign") {
		t.Fatalf("stderr = %q, want campaign validation error", stderr.String())
	}
}

func TestValidateGoogleAdsAdGroupRejectsShoppingType(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsAdGroupShoppingManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "SEARCH_STANDARD") {
		t.Fatalf("stderr = %q, want SEARCH_STANDARD guidance", stderr.String())
	}
}

func TestValidateGoogleAdsKeywordMissingCredentials(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", googleAdsKeywordManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	errOut := stderr.String()
	if strings.Contains(errOut, "unknown resource type") {
		t.Fatalf("stderr = %q, googleads.keyword should be registered", errOut)
	}
	if !strings.Contains(errOut, googleads.EnvDeveloperToken) && !strings.Contains(errOut, "required") {
		t.Fatalf("stderr = %q, want missing credential error", errOut)
	}
}

func TestValidateGoogleAdsKeywordWithProvider(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsKeywordManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "4 resources") {
		t.Fatalf("stdout = %q, want 4 resources", stdout.String())
	}
}

func TestValidateGoogleAdsKeywordMissingAdGroup(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsKeywordIncompleteManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "adGroup") {
		t.Fatalf("stderr = %q, want adGroup validation error", stderr.String())
	}
}

func TestValidateGoogleAdsKeywordRejectsInvalidMatchType(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsKeywordInvalidMatchManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "matchType") {
		t.Fatalf("stderr = %q, want matchType validation error", stderr.String())
	}
}

func TestValidateGoogleAdsResponsiveSearchAdMissingCredentials(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", googleAdsResponsiveSearchAdManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	errOut := stderr.String()
	if strings.Contains(errOut, "unknown resource type") {
		t.Fatalf("stderr = %q, googleads.responsive_search_ad should be registered", errOut)
	}
	if !strings.Contains(errOut, googleads.EnvDeveloperToken) && !strings.Contains(errOut, "required") {
		t.Fatalf("stderr = %q, want missing credential error", errOut)
	}
}

func TestValidateGoogleAdsResponsiveSearchAdWithProvider(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsResponsiveSearchAdManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "4 resources") {
		t.Fatalf("stdout = %q, want 4 resources", stdout.String())
	}
}

func TestValidateGoogleAdsResponsiveSearchAdMissingAdGroup(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsResponsiveSearchAdIncompleteManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "adGroup") {
		t.Fatalf("stderr = %q, want adGroup validation error", stderr.String())
	}
}

func TestValidateGoogleAdsResponsiveSearchAdRejectsTooFewHeadlines(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsResponsiveSearchAdTooFewHeadlinesManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "headline") {
		t.Fatalf("stderr = %q, want headline validation error", stderr.String())
	}
}

func TestValidateGoogleAdsTargetingMissingCredentials(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", googleAdsTargetingManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	errOut := stderr.String()
	if strings.Contains(errOut, "unknown resource type") {
		t.Fatalf("stderr = %q, googleads.campaign_location should be registered", errOut)
	}
	if !strings.Contains(errOut, googleads.EnvDeveloperToken) && !strings.Contains(errOut, "required") {
		t.Fatalf("stderr = %q, want missing credential error", errOut)
	}
}

func TestValidateGoogleAdsTargetingWithProvider(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsTargetingManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "5 resources") {
		t.Fatalf("stdout = %q, want 5 resources", stdout.String())
	}
}

func TestValidateGoogleAdsLocationMissingCampaign(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsLocationIncompleteManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "campaign") {
		t.Fatalf("stderr = %q, want campaign validation error", stderr.String())
	}
}

func TestValidateGoogleAdsLocationTargetingRejectsUnknownMode(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsLocationInvalidModeManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "positive") {
		t.Fatalf("stderr = %q, want positive geo target guidance", stderr.String())
	}
}

func TestValidatePositionalFile(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "site.yaml", emptyResourcesManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", path})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
}

func TestValidateInvalidManifest(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "bad.yaml", invalidManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	errOut := stderr.String()
	if !strings.Contains(errOut, "invalid") && !strings.Contains(errOut, "provider.type.name") {
		t.Fatalf("stderr = %q, want address error", errOut)
	}
}

func TestValidateMissingFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing.yaml")
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d", code, cli.ExitError)
	}
	if !strings.Contains(stderr.String(), "read manifest") {
		t.Fatalf("stderr = %q, want read manifest error", stderr.String())
	}
}

func TestValidateConflictingFileArgs(t *testing.T) {
	t.Parallel()

	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", "a.yaml", "b.yaml"})
	if code != cli.ExitUsage {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "not both") {
		t.Fatalf("stderr = %q, want conflicting path message", stderr.String())
	}
}

func TestValidateDefaultFilename(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agoraform.yaml"), []byte(emptyResourcesManifest), 0o600); err != nil {
		t.Fatal(err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate"})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "agoraform.yaml") {
		t.Fatalf("stdout = %q, want default filename", stdout.String())
	}
}

func TestValidateMatomoGoalMissingCredentials(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", matomoGoalManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	errOut := stderr.String()
	if strings.Contains(errOut, "unknown resource type") {
		t.Fatalf("stderr = %q, matomo.goal should be registered", errOut)
	}
	if !strings.Contains(errOut, "MATOMO_URL") && !strings.Contains(errOut, "required") {
		t.Fatalf("stderr = %q, want missing credential error", errOut)
	}
}

func TestValidateWithFakeProvider(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", validManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 resource") {
		t.Fatalf("stdout = %q, want 1 resource", stdout.String())
	}
}

func TestValidateHelp(t *testing.T) {
	t.Parallel()

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "--help"})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "validate") || !strings.Contains(out, "agoraform.yaml") {
		t.Fatalf("help output missing expected text:\n%s", out)
	}
}

func TestValidateMatomoGoalWithProvider(t *testing.T) {
	t.Parallel()

	p, _ := matomoGoalTestProvider(t, `[]`)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoGoalManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 resource") {
		t.Fatalf("stdout = %q, want 1 resource", stdout.String())
	}
}

func TestValidateMatomoGoalMissingPattern(t *testing.T) {
	t.Parallel()

	p, _ := matomoGoalTestProvider(t, `[]`)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoGoalIncompleteManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "pattern") {
		t.Fatalf("stderr = %q, want pattern validation error", stderr.String())
	}
}

func TestValidateMatomoVariableMissingCredentials(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", matomoVariableManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	errOut := stderr.String()
	if strings.Contains(errOut, "unknown resource type") {
		t.Fatalf("stderr = %q, matomo.variable should be registered", errOut)
	}
	if !strings.Contains(errOut, "MATOMO_URL") && !strings.Contains(errOut, "required") {
		t.Fatalf("stderr = %q, want missing credential error", errOut)
	}
}

func TestValidateMatomoVariableMissingContainer(t *testing.T) {
	t.Parallel()

	p, _ := matomoGoalTestProvider(t, `[]`)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoVariableManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), matomo.EnvContainerID) {
		t.Fatalf("stderr = %q, want %s", stderr.String(), matomo.EnvContainerID)
	}
}

func TestValidateMatomoVariableWithProvider(t *testing.T) {
	t.Parallel()

	p, _ := matomoVariableTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoVariableManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 resource") {
		t.Fatalf("stdout = %q, want 1 resource", stdout.String())
	}
}

func TestValidateMatomoTriggerMissingCredentials(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", matomoTriggerManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	errOut := stderr.String()
	if strings.Contains(errOut, "unknown resource type") {
		t.Fatalf("stderr = %q, matomo.trigger should be registered", errOut)
	}
	if !strings.Contains(errOut, "MATOMO_URL") && !strings.Contains(errOut, "required") {
		t.Fatalf("stderr = %q, want missing credential error", errOut)
	}
}

func TestValidateMatomoTriggerMissingContainer(t *testing.T) {
	t.Parallel()

	p, _ := matomoGoalTestProvider(t, `[]`)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoTriggerManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), matomo.EnvContainerID) {
		t.Fatalf("stderr = %q, want %s", stderr.String(), matomo.EnvContainerID)
	}
}

func TestValidateMatomoTriggerWithProvider(t *testing.T) {
	t.Parallel()

	p, _ := matomoVariableTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoTriggerManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 resource") {
		t.Fatalf("stdout = %q, want 1 resource", stdout.String())
	}
}

func TestValidateMatomoTagMissingCredentials(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", matomoTagManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	errOut := stderr.String()
	if strings.Contains(errOut, "unknown resource type") {
		t.Fatalf("stderr = %q, matomo.tag should be registered", errOut)
	}
	if !strings.Contains(errOut, "MATOMO_URL") && !strings.Contains(errOut, "required") {
		t.Fatalf("stderr = %q, want missing credential error", errOut)
	}
}

func TestValidateMatomoTagWithProvider(t *testing.T) {
	t.Parallel()

	p, _ := matomoVariableTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoTagManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "2 resources") {
		t.Fatalf("stdout = %q, want 2 resources", stdout.String())
	}
}

func TestValidateMatomoGoogleAdsConversionTag(t *testing.T) {
	t.Parallel()

	matomoP, _ := matomoVariableTestProvider(t)
	adsP, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(matomoP); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(adsP); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoGoogleAdsConversionTagManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "3 resources") {
		t.Fatalf("stdout = %q, want 3 resources", stdout.String())
	}
}

func TestValidateMixedGoalAndVariableManifest(t *testing.T) {
	t.Parallel()

	p, _ := matomoVariableTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoGoalAndVariableManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "2 resources") {
		t.Fatalf("stdout = %q, want 2 resources", stdout.String())
	}
}

func matomoGoalTestProvider(t *testing.T, getGoalsBody string) (*matomo.Provider, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "API.getMatomoVersion") {
			_, _ = io.WriteString(w, `"5.2.0"`)
			return
		}
		_, _ = io.WriteString(w, getGoalsBody)
	}))
	t.Cleanup(srv.Close)
	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:    srv.URL,
		TokenAuth:  "cli-test-token",
		SiteID:     "3",
		HTTPClient: srv.Client(),
	}, srv.Client())
	return p, srv
}

func matomoVariableTestProvider(t *testing.T) (*matomo.Provider, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case strings.Contains(string(body), "API.getMatomoVersion"):
			_, _ = io.WriteString(w, `"5.2.0"`)
		case strings.Contains(string(body), "TagManager.getContainerVariables"):
			_, _ = io.WriteString(w, `[]`)
		case strings.Contains(string(body), "TagManager.getContainerTriggers"):
			_, _ = io.WriteString(w, `[]`)
		case strings.Contains(string(body), "TagManager.getContainerTags"):
			_, _ = io.WriteString(w, `[]`)
		case strings.Contains(string(body), "TagManager.getContainer"):
			_, _ = io.WriteString(w, `{"idcontainer":"6OMh6taM","idsite":3,"draft":{"idcontainerversion":9}}`)
		case strings.Contains(string(body), "Goals.getGoals"):
			_, _ = io.WriteString(w, `[]`)
		default:
			_, _ = io.WriteString(w, `{"result":"error","message":"unknown method"}`)
		}
	}))
	t.Cleanup(srv.Close)
	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:     srv.URL,
		TokenAuth:   "cli-test-token",
		SiteID:      "3",
		ContainerID: "6OMh6taM",
		HTTPClient:  srv.Client(),
	}, srv.Client())
	return p, srv
}

func googleAdsTestProvider(t *testing.T) (*googleads.Provider, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/oauth/token") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"cli-test-access-token","expires_in":3600,"token_type":"Bearer"}`)
			return
		}
		_, _ = io.WriteString(w, `{"results":[{"customer":{"id":"1234567890"}}]}`)
	}))
	t.Cleanup(srv.Close)
	p := googleads.NewWithHTTPClient(googleads.Config{
		DeveloperToken: "cli-test-developer-token",
		ClientID:       "cli-test-client-id",
		ClientSecret:   "cli-test-client-secret",
		RefreshToken:   "cli-test-refresh-token",
		CustomerID:     "1234567890",
		BaseURL:        srv.URL,
		TokenURL:       srv.URL + "/oauth/token",
		HTTPClient:     srv.Client(),
	}, srv.Client())
	return p, srv
}

func writeManifest(t *testing.T, name, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
