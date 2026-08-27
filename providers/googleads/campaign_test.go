package googleads_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/internal/importer"
	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestValidateCampaignValid(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignProvider(t, nil)
	res := campaignResource(t, "brand", defaultCampaignAttrs(t))
	if err := p.Validate(context.Background(), res); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateCampaignErrors(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignProvider(t, nil)
	addr := mustCampaignAddress(t, "brand")
	budget := resource.Ref{Address: mustCampaignBudgetAddress(t, "brand")}
	bidding := map[string]any{"strategy": "MANUAL_CPC"}

	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name:  "missing name",
			attrs: resource.Attributes{googleads.AttrBudget: budget, googleads.AttrBidding: bidding},
			want:  "missing required attribute \"name\"",
		},
		{
			name:  "missing budget",
			attrs: resource.Attributes{googleads.AttrName: "Brand", googleads.AttrBidding: bidding},
			want:  "missing required attribute \"budget\"",
		},
		{
			name: "budget not a ref",
			attrs: resource.Attributes{
				googleads.AttrName:    "Brand",
				googleads.AttrBudget:  "customers/" + testCustomerID + "/campaignBudgets/11",
				googleads.AttrBidding: bidding,
			},
			want: "$ref",
		},
		{
			name: "budget wrong type",
			attrs: resource.Attributes{
				googleads.AttrName:    "Brand",
				googleads.AttrBudget:  resource.Ref{Address: mustConversionActionAddress(t, "trial_started")},
				googleads.AttrBidding: bidding,
			},
			want: "campaign_budget",
		},
		{
			name:  "missing bidding",
			attrs: resource.Attributes{googleads.AttrName: "Brand", googleads.AttrBudget: budget},
			want:  "missing required attribute \"bidding\"",
		},
		{
			name: "unsupported channel type",
			attrs: resource.Attributes{
				googleads.AttrName:                   "Brand",
				googleads.AttrBudget:                 budget,
				googleads.AttrBidding:                bidding,
				googleads.AttrAdvertisingChannelType: "DISPLAY",
			},
			want: "SEARCH",
		},
		{
			name: "unsupported bidding strategy",
			attrs: resource.Attributes{
				googleads.AttrName:    "Brand",
				googleads.AttrBudget:  budget,
				googleads.AttrBidding: map[string]any{"strategy": "TARGET_IMPRESSION_SHARE"},
			},
			want: "strategy",
		},
		{
			name: "target cpa required",
			attrs: resource.Attributes{
				googleads.AttrName:    "Brand",
				googleads.AttrBudget:  budget,
				googleads.AttrBidding: map[string]any{"strategy": "TARGET_CPA"},
			},
			want: "targetCpa",
		},
		{
			name: "enabled status accepted",
			attrs: resource.Attributes{
				googleads.AttrName:    "Brand",
				googleads.AttrStatus:  "ENABLED",
				googleads.AttrBudget:  budget,
				googleads.AttrBidding: bidding,
			},
			want: "",
		},
		{
			name: "removed status rejected",
			attrs: resource.Attributes{
				googleads.AttrName:    "Brand",
				googleads.AttrStatus:  "REMOVED",
				googleads.AttrBudget:  budget,
				googleads.AttrBidding: bidding,
			},
			want: "status",
		},
		{
			name: "computed id",
			attrs: resource.Attributes{
				googleads.AttrName:    "Brand",
				googleads.AttrBudget:  budget,
				googleads.AttrBidding: bidding,
				"id":                  "1",
			},
			want: "computed",
		},
		{
			name: "end before start",
			attrs: resource.Attributes{
				googleads.AttrName:      "Brand",
				googleads.AttrBudget:    budget,
				googleads.AttrBidding:   bidding,
				googleads.AttrStartDate: "2026-12-01",
				googleads.AttrEndDate:   "2026-01-01",
			},
			want: "on or after",
		},
		{
			name: "google search must be true",
			attrs: resource.Attributes{
				googleads.AttrName:    "Brand",
				googleads.AttrBudget:  budget,
				googleads.AttrBidding: bidding,
				googleads.AttrNetwork: map[string]any{"googleSearch": false},
			},
			want: "googleSearch",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := p.Validate(context.Background(), resource.Resource{Address: addr, Attributes: tc.attrs})
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
			assertNoProviderSecret(t, err.Error())
		})
	}
}

func TestReadCampaignSuccess(t *testing.T) {
	t.Parallel()

	fake := newCampaignFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	p, _ := testCampaignProvider(t, fake)

	_, err := p.Read(context.Background(), campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	}))
	if err != nil {
		t.Fatalf("Read budget: %v", err)
	}

	live, err := p.Read(context.Background(), campaignResource(t, "brand", defaultCampaignAttrs(t)))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "21" {
		t.Fatalf("identity = %q, want 21", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrName] != "Brand" {
		t.Fatalf("name = %v", live.Attributes[googleads.AttrName])
	}
	if live.Attributes[googleads.AttrStatus] != "PAUSED" {
		t.Fatalf("status = %v, want PAUSED", live.Attributes[googleads.AttrStatus])
	}
	ref, ok := resource.AsRef(live.Attributes[googleads.AttrBudget])
	if !ok || ref.Address != mustCampaignBudgetAddress(t, "brand") {
		t.Fatalf("budget = %#v, want logical $ref", live.Attributes[googleads.AttrBudget])
	}
	if live.Computed["advertisingChannelType"] != "SEARCH" {
		t.Fatalf("computed channel = %v", live.Computed["advertisingChannelType"])
	}
	if _, ok := live.Attributes["id"]; ok {
		t.Fatal("id must not appear in comparable attributes")
	}
	if _, ok := live.Attributes["campaignBudget"]; ok {
		t.Fatal("native campaignBudget must not appear in comparable attributes")
	}
}

func TestReadCampaignNotFound(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignProvider(t, newCampaignFake())
	_, err := p.Read(context.Background(), campaignResource(t, "brand", defaultCampaignAttrs(t)))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadCampaignRejectsDisplay(t *testing.T) {
	t.Parallel()

	fake := newCampaignFake()
	campaign := sampleSearchCampaign("9", "Display", "11")
	campaign["advertisingChannelType"] = "DISPLAY"
	fake.seedCampaign(campaign)
	p, _ := testCampaignProvider(t, fake)

	_, err := p.Read(context.Background(), campaignResource(t, "display", resource.Attributes{
		googleads.AttrName:    "Display",
		googleads.AttrBudget:  resource.Ref{Address: mustCampaignBudgetAddress(t, "brand")},
		googleads.AttrBidding: map[string]any{"strategy": "MANUAL_CPC"},
	}))
	if err == nil {
		t.Fatal("expected unsupported channel error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported channel must not look like not found")
	}
	if !strings.Contains(err.Error(), "SEARCH") {
		t.Fatalf("error = %q, want SEARCH guidance", err)
	}
}

func TestReadCampaignRejectsUnsupportedBidding(t *testing.T) {
	t.Parallel()

	fake := newCampaignFake()
	campaign := sampleSearchCampaign("9", "Brand", "11")
	campaign["biddingStrategyType"] = "TARGET_IMPRESSION_SHARE"
	delete(campaign, "manualCpc")
	fake.seedCampaign(campaign)
	p, _ := testCampaignProvider(t, fake)

	_, err := p.Read(context.Background(), campaignResource(t, "brand", defaultCampaignAttrs(t)))
	if err == nil {
		t.Fatal("expected unsupported bidding error")
	}
	if !strings.Contains(err.Error(), "TARGET_IMPRESSION_SHARE") {
		t.Fatalf("error = %q, want bidding guidance", err)
	}
}

func TestReadCampaignAPIError(t *testing.T) {
	t.Parallel()

	fake := newCampaignFake()
	fake.searchStatus = http.StatusForbidden
	p, _ := testCampaignProvider(t, fake)
	_, err := p.Read(context.Background(), campaignResource(t, "brand", defaultCampaignAttrs(t)))
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestCreateCampaignDefaultsPausedSearch(t *testing.T) {
	t.Parallel()

	fake := newCampaignFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false})
	p, _ := testCampaignProvider(t, fake)

	attrs := defaultCampaignAttrs(t)
	attrs[googleads.AttrBudget] = resource.Resolved{
		Address:  mustCampaignBudgetAddress(t, "brand"),
		Identity: resource.Identity{ID: "11"},
		Outputs:  resource.Attributes{"resourceName": "customers/" + testCustomerID + "/campaignBudgets/11"},
	}
	live, err := p.Create(context.Background(), campaignResource(t, "brand", attrs))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID == "" {
		t.Fatal("create returned empty identity")
	}
	if live.Attributes[googleads.AttrStatus] != "PAUSED" {
		t.Fatalf("status = %v, want PAUSED", live.Attributes[googleads.AttrStatus])
	}
	if live.Computed["advertisingChannelType"] != "SEARCH" {
		t.Fatalf("channel = %v, want SEARCH", live.Computed["advertisingChannelType"])
	}
	if !strings.Contains(fake.lastMutate, `"status":"PAUSED"`) && !strings.Contains(fake.lastMutate, `"status": "PAUSED"`) {
		t.Fatalf("create mutate missing PAUSED: %s", fake.lastMutate)
	}
	if !strings.Contains(fake.lastMutate, `"advertisingChannelType":"SEARCH"`) && !strings.Contains(fake.lastMutate, `"advertisingChannelType": "SEARCH"`) {
		t.Fatalf("create mutate missing SEARCH: %s", fake.lastMutate)
	}
	if !strings.Contains(fake.lastMutate, "campaignBudgets/11") {
		t.Fatalf("create mutate missing resolved budget: %s", fake.lastMutate)
	}
	if !strings.Contains(fake.lastMutate, "DOES_NOT_CONTAIN_EU_POLITICAL_ADVERTISING") {
		t.Fatalf("create mutate missing EU political advertising declaration: %s", fake.lastMutate)
	}
}

func TestCreateCampaignAPIError(t *testing.T) {
	t.Parallel()

	fake := newCampaignFake()
	fake.mutateStatus = http.StatusBadRequest
	p, _ := testCampaignProvider(t, fake)
	attrs := defaultCampaignAttrs(t)
	attrs[googleads.AttrBudget] = resource.Resolved{
		Address:  mustCampaignBudgetAddress(t, "brand"),
		Identity: resource.Identity{ID: "11"},
		Outputs:  resource.Attributes{"resourceName": "customers/" + testCustomerID + "/campaignBudgets/11"},
	}
	_, err := p.Create(context.Background(), campaignResource(t, "brand", attrs))
	if err == nil {
		t.Fatal("expected API error")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestUpdateCampaign(t *testing.T) {
	t.Parallel()

	fake := newCampaignFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	p, _ := testCampaignProvider(t, fake)

	desired := campaignResource(t, "brand", resource.Attributes{
		googleads.AttrName:   "Brand Search",
		googleads.AttrStatus: "ENABLED",
		googleads.AttrBudget: resource.Resolved{
			Address:  mustCampaignBudgetAddress(t, "brand"),
			Identity: resource.Identity{ID: "11"},
			Outputs:  resource.Attributes{"resourceName": "customers/" + testCustomerID + "/campaignBudgets/11"},
		},
		googleads.AttrBidding: map[string]any{"strategy": "MANUAL_CPC", "enhancedCpc": true},
	})
	live, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "21"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if live.Identity.ID != "21" {
		t.Fatalf("identity = %q, want 21", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrName] != "Brand Search" {
		t.Fatalf("name = %v", live.Attributes[googleads.AttrName])
	}
	if live.Attributes[googleads.AttrStatus] != "ENABLED" {
		t.Fatalf("status = %v, want ENABLED", live.Attributes[googleads.AttrStatus])
	}
	if !strings.Contains(fake.lastMutate, "updateMask") {
		t.Fatalf("update missing updateMask: %s", fake.lastMutate)
	}
}

func TestImportCampaignRequiresBoundBudget(t *testing.T) {
	t.Parallel()

	fake := newCampaignFake()
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	p, _ := testCampaignProvider(t, fake)
	_, err := p.Import(context.Background(), mustCampaignAddress(t, "brand"), "21")
	if err == nil {
		t.Fatal("expected missing budget binding error")
	}
	if !strings.Contains(err.Error(), "campaign_budget") {
		t.Fatalf("error = %q, want budget import guidance", err)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("import mutated remote: %v", fake.mutates)
	}
}

func TestImportCampaignThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	fake := newCampaignFake()
	fake.seedBudget(map[string]any{
		"id":               "11",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
	})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	p, _ := testCampaignProvider(t, fake)

	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignBudgetAddress(t, "brand"), resource.Identity{ID: "11"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	got, err := importer.Run(context.Background(), mustCampaignAddress(t, "brand"), "21", lookupGoogleAds(p), st)
	if err != nil {
		t.Fatalf("importer.Run: %v", err)
	}
	if got.Identity.ID != "21" {
		t.Fatalf("identity = %q, want 21", got.Identity.ID)
	}
	assertNoProviderSecret(t, got.YAML)
	for _, leak := range []string{"resourceName", "campaignBudget:", "id:", "servingStatus", testAccessToken} {
		if strings.Contains(got.YAML, leak) {
			t.Fatalf("generated YAML leaked %q:\n%s", leak, got.YAML)
		}
	}
	if !strings.Contains(got.YAML, "$ref: googleads.campaign_budget.brand") {
		t.Fatalf("generated YAML missing budget $ref:\n%s", got.YAML)
	}

	combined := `apiVersion: agoraform.io/v1alpha1
resources:
  - address: googleads.campaign_budget.brand
    attributes:
      name: Brand daily budget
      amount: 50
      explicitlyShared: false
      deliveryMethod: STANDARD
` + strings.TrimPrefix(got.YAML, "apiVersion: agoraform.io/v1alpha1\nresources:\n")
	parsed, err := manifest.Parse([]byte(combined), "generated")
	if err != nil {
		t.Fatalf("generated YAML: %v\n%s", err, combined)
	}
	planned, err := plan.BuildWithState(context.Background(), parsed.Resources, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("plan after import: %v", err)
	}
	if planned.HasChanges() {
		t.Fatalf("plan after import has changes: %+v", planned.Changes)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("import/plan mutated remote: %v", fake.mutates)
	}
}

func TestNormalizeCampaignImportID(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignProvider(t, nil)
	addr := mustCampaignAddress(t, "brand")
	got, err := p.NormalizeImportID(addr, "21")
	if err != nil || got != "21" {
		t.Fatalf("numeric = (%q, %v), want 21", got, err)
	}
	got, err = p.NormalizeImportID(addr, "customers/"+testCustomerID+"/campaigns/21")
	if err != nil || got != "21" {
		t.Fatalf("resource name = (%q, %v), want 21", got, err)
	}
	_, err = p.NormalizeImportID(addr, "abc")
	if err == nil || !strings.Contains(err.Error(), "not a valid Google Ads campaign id") {
		t.Fatalf("invalid id error = %v", err)
	}
}

func TestPlanCampaignCreateWhenMissing(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignProvider(t, newCampaignFake())
	budget := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	})
	campaign := campaignResource(t, "brand", defaultCampaignAttrs(t))
	got := mustPlanCampaign(t, p, budget, campaign)
	if len(got.Changes) != 2 {
		t.Fatalf("changes = %+v, want 2", got.Changes)
	}
	byAddr := map[string]plan.Action{}
	for _, change := range got.Changes {
		byAddr[change.Address.String()] = change.Action
	}
	if byAddr[budget.Address.String()] != plan.ActionCreate || byAddr[campaign.Address.String()] != plan.ActionCreate {
		t.Fatalf("actions = %+v, want create/create", byAddr)
	}
}

func TestPlanCampaignUnchangedEquivalentRemote(t *testing.T) {
	t.Parallel()

	fake := newCampaignFake()
	fake.seedBudget(map[string]any{
		"id":               "11",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
	})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	p, _ := testCampaignProvider(t, fake)

	budget := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
		googleads.AttrDeliveryMethod:   "standard",
	})
	campaign := campaignResource(t, "brand", resource.Attributes{
		googleads.AttrName:    "Brand",
		googleads.AttrBudget:  resource.Ref{Address: budget.Address},
		googleads.AttrBidding: map[string]any{"strategy": "manual_cpc"},
	})
	got := mustPlanCampaign(t, p, budget, campaign)
	if got.HasChanges() {
		t.Fatalf("equivalent remote produced changes: %+v", got.Changes)
	}
}

func TestPlanCampaignOmittedStatusDefaultsPaused(t *testing.T) {
	t.Parallel()

	fake := newCampaignFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false, "period": "DAILY", "type": "STANDARD"})
	live := sampleSearchCampaign("21", "Brand", "11")
	live["status"] = "ENABLED"
	fake.seedCampaign(live)
	p, _ := testCampaignProvider(t, fake)

	budget := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	})
	campaign := campaignResource(t, "brand", defaultCampaignAttrs(t))
	got := mustPlanCampaign(t, p, budget, campaign)
	var campaignChange *plan.Change
	for i := range got.Changes {
		if got.Changes[i].Address == campaign.Address {
			campaignChange = &got.Changes[i]
		}
	}
	if campaignChange == nil || campaignChange.Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want campaign update to PAUSED", got.Changes)
	}
	found := false
	for _, diff := range campaignChange.Diffs {
		if diff.Path == googleads.AttrStatus {
			found = true
			if diff.After != "PAUSED" {
				t.Fatalf("status after = %v, want PAUSED", diff.After)
			}
		}
	}
	if !found {
		t.Fatalf("missing status diff: %+v", campaignChange.Diffs)
	}
}

func TestPlanCampaignUpdateBidding(t *testing.T) {
	t.Parallel()

	fake := newCampaignFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false, "period": "DAILY", "type": "STANDARD"})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	p, _ := testCampaignProvider(t, fake)

	budget := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	})
	campaign := campaignResource(t, "brand", resource.Attributes{
		googleads.AttrName:   "Brand",
		googleads.AttrBudget: resource.Ref{Address: budget.Address},
		googleads.AttrBidding: map[string]any{
			"strategy":  "MAXIMIZE_CONVERSIONS",
			"targetCpa": 25,
		},
	})
	got := mustPlanCampaign(t, p, budget, campaign)
	var campaignChange *plan.Change
	for i := range got.Changes {
		if got.Changes[i].Address == campaign.Address {
			campaignChange = &got.Changes[i]
		}
	}
	if campaignChange == nil || campaignChange.Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want campaign bidding update", got.Changes)
	}
}

func TestPlanCampaignIgnoresComputedFields(t *testing.T) {
	t.Parallel()

	fake := newCampaignFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false, "period": "DAILY", "type": "STANDARD"})
	campaign := sampleSearchCampaign("21", "Brand", "11")
	campaign["servingStatus"] = "SERVING"
	campaign["optimizationScore"] = 0.8
	fake.seedCampaign(campaign)
	p, _ := testCampaignProvider(t, fake)

	budget := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	})
	res := campaignResource(t, "brand", defaultCampaignAttrs(t))
	got := mustPlanCampaign(t, p, budget, res)
	if got.HasChanges() {
		t.Fatalf("computed fields produced changes: %+v", got.Changes)
	}
}

func TestPlanCampaignOmittedNetworkIsNoOp(t *testing.T) {
	t.Parallel()

	fake := newCampaignFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false, "period": "DAILY", "type": "STANDARD"})
	live := sampleSearchCampaign("21", "Brand", "11")
	live["networkSettings"] = map[string]any{
		"targetGoogleSearch":         true,
		"targetSearchNetwork":        true,
		"targetContentNetwork":       true,
		"targetPartnerSearchNetwork": false,
	}
	fake.seedCampaign(live)
	p, _ := testCampaignProvider(t, fake)

	budget := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	})
	res := campaignResource(t, "brand", defaultCampaignAttrs(t))
	got := mustPlanCampaign(t, p, budget, res)
	if got.HasChanges() {
		t.Fatalf("omitted network produced changes: %+v", got.Changes)
	}
}

func mustPlanCampaign(t *testing.T, p *googleads.Provider, resources ...resource.Resource) *plan.Plan {
	t.Helper()
	got, err := plan.Build(context.Background(), resources, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	return got
}

func defaultCampaignAttrs(t *testing.T) resource.Attributes {
	t.Helper()
	return resource.Attributes{
		googleads.AttrName:    "Brand",
		googleads.AttrBudget:  resource.Ref{Address: mustCampaignBudgetAddress(t, "brand")},
		googleads.AttrBidding: map[string]any{"strategy": "MANUAL_CPC"},
	}
}

func campaignResource(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{Address: mustCampaignAddress(t, name), Attributes: attrs}
}

func mustCampaignAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("googleads.campaign." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func testCampaignProvider(t *testing.T, fake *campaignFake) (*googleads.Provider, *httptest.Server) {
	t.Helper()
	if fake == nil {
		fake = newCampaignFake()
	}
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	t.Cleanup(srv.Close)
	cfg := validConfig(srv.URL)
	cfg.TokenURL = srv.URL + "/oauth/token"
	p := googleads.NewWithHTTPClient(cfg, srv.Client())
	return p, srv
}

func sampleSearchCampaign(id, name, budgetID string) map[string]any {
	return map[string]any{
		"id":                     id,
		"name":                   name,
		"status":                 "PAUSED",
		"advertisingChannelType": "SEARCH",
		"campaignBudget":         "customers/" + testCustomerID + "/campaignBudgets/" + budgetID,
		"biddingStrategyType":    "MANUAL_CPC",
		"manualCpc":              map[string]any{"enhancedCpcEnabled": false},
		"networkSettings": map[string]any{
			"targetGoogleSearch":         true,
			"targetSearchNetwork":        true,
			"targetContentNetwork":       false,
			"targetPartnerSearchNetwork": false,
		},
		"containsEuPoliticalAdvertising": "DOES_NOT_CONTAIN_EU_POLITICAL_ADVERTISING",
		"servingStatus":                  "PENDING",
	}
}

type campaignFake struct {
	mu sync.Mutex

	nextID    int64
	campaigns map[string]map[string]any
	budgets   map[string]map[string]any

	searchStatus int
	searchBody   string
	mutateStatus int
	mutateBody   string

	lastQuery  string
	lastMutate string
	mutates    []string
}

func newCampaignFake() *campaignFake {
	return &campaignFake{
		campaigns: map[string]map[string]any{},
		budgets:   map[string]map[string]any{},
	}
}

func (f *campaignFake) seedCampaign(campaign map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storeCampaignLocked(cloneMap(campaign))
}

func (f *campaignFake) seedBudget(budget map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cloned := cloneMap(budget)
	id := stringify(cloned["id"])
	if stringify(cloned["resourceName"]) == "" {
		cloned["resourceName"] = "customers/" + testCustomerID + "/campaignBudgets/" + id
	}
	if stringify(cloned["period"]) == "" {
		cloned["period"] = "DAILY"
	}
	if stringify(cloned["type"]) == "" {
		cloned["type"] = "STANDARD"
	}
	f.budgets[id] = cloned
}

func (f *campaignFake) storeCampaignLocked(campaign map[string]any) {
	id := stringify(campaign["id"])
	if stringify(campaign["resourceName"]) == "" {
		campaign["resourceName"] = "customers/" + testCustomerID + "/campaigns/" + id
	}
	if stringify(campaign["advertisingChannelType"]) == "" {
		campaign["advertisingChannelType"] = "SEARCH"
	}
	if stringify(campaign["status"]) == "" {
		campaign["status"] = "PAUSED"
	}
	f.campaigns[id] = campaign
}

func (f *campaignFake) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, _ := io.ReadAll(r.Body)

	if strings.HasSuffix(r.URL.Path, "/oauth/token") {
		writeToken(w)
		return
	}
	if strings.Contains(r.URL.Path, "/googleAds:search") {
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(body, &req)
		f.lastQuery = req.Query
		if f.searchStatus >= 400 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(f.searchStatus)
			_, _ = io.WriteString(w, `{"error":{"code":`+strconv.Itoa(f.searchStatus)+`,"message":"query failed `+testAccessToken+`","status":"PERMISSION_DENIED"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if f.searchBody != "" {
			_, _ = io.WriteString(w, f.searchBody)
			return
		}
		query := strings.ToLower(req.Query)
		if strings.Contains(query, "from customer") && !strings.Contains(query, "from campaign") {
			_, _ = io.WriteString(w, `{"results":[{"customer":{"id":"`+testCustomerID+`"}}]}`)
			return
		}
		if strings.Contains(query, "from campaign_budget") {
			_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchBudgetsLocked(req.Query)})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchCampaignsLocked(req.Query)})
		return
	}
	if strings.Contains(r.URL.Path, ":mutate") {
		f.lastMutate = string(body)
		collection := "campaigns"
		if strings.Contains(r.URL.Path, "/campaignBudgets:mutate") {
			collection = "campaignBudgets"
		}
		f.mutates = append(f.mutates, collection)
		if f.mutateStatus >= 400 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(f.mutateStatus)
			if f.mutateBody != "" {
				_, _ = io.WriteString(w, f.mutateBody)
				return
			}
			_, _ = io.WriteString(w, `{"error":{"code":400,"message":"mutate failed `+testDeveloperToken+`","status":"INVALID_ARGUMENT"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if f.mutateBody != "" {
			_, _ = io.WriteString(w, f.mutateBody)
			return
		}
		resourceName, err := f.mutateLocked(collection, body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"resourceName": resourceName}}})
		return
	}
	http.NotFound(w, r)
}

func (f *campaignFake) searchCampaignsLocked(query string) []any {
	var out []any
	for _, campaign := range f.campaigns {
		if matchesCampaignQuery(query, campaign) {
			out = append(out, map[string]any{"campaign": cloneMap(campaign)})
		}
	}
	return out
}

func (f *campaignFake) searchBudgetsLocked(query string) []any {
	var out []any
	for _, budget := range f.budgets {
		if matchesCampaignBudgetQuery(query, budget) {
			out = append(out, map[string]any{"campaignBudget": cloneMap(budget)})
		}
	}
	return out
}

func (f *campaignFake) mutateLocked(collection string, body []byte) (string, error) {
	var req struct {
		Operations []map[string]any `json:"operations"`
	}
	if err := json.Unmarshal(body, &req); err != nil || len(req.Operations) == 0 {
		return "", errors.New("malformed mutate")
	}
	op := req.Operations[0]
	if collection == "campaignBudgets" {
		return "", errors.New("unexpected campaign budget mutate")
	}
	if raw, ok := op["create"]; ok {
		campaign, _ := raw.(map[string]any)
		created := cloneMap(campaign)
		f.nextID++
		id := strconv.FormatInt(f.nextID, 10)
		created["id"] = id
		if stringify(created["advertisingChannelType"]) == "" {
			created["advertisingChannelType"] = "SEARCH"
		}
		if stringify(created["status"]) == "" {
			created["status"] = "PAUSED"
		}
		if stringify(created["biddingStrategyType"]) == "" {
			if _, ok := created["manualCpc"]; ok {
				created["biddingStrategyType"] = "MANUAL_CPC"
			}
			if _, ok := created["maximizeConversions"]; ok {
				created["biddingStrategyType"] = "MAXIMIZE_CONVERSIONS"
			}
			if _, ok := created["targetSpend"]; ok {
				created["biddingStrategyType"] = "TARGET_SPEND"
			}
		}
		f.storeCampaignLocked(created)
		return stringify(created["resourceName"]), nil
	}
	if raw, ok := op["update"]; ok {
		campaign, _ := raw.(map[string]any)
		resourceName := stringify(campaign["resourceName"])
		id := strings.TrimPrefix(resourceName, "customers/"+testCustomerID+"/campaigns/")
		current, ok := f.campaigns[id]
		if !ok {
			return "", errors.New("missing campaign")
		}
		merged := cloneMap(current)
		mergeCampaignUpdate(merged, campaign)
		f.storeCampaignLocked(merged)
		return resourceName, nil
	}
	return "", errors.New("unsupported mutate")
}

func mergeCampaignUpdate(dst, src map[string]any) {
	for k, v := range src {
		if k == "resourceName" {
			continue
		}
		if nested, ok := v.(map[string]any); ok {
			current, _ := dst[k].(map[string]any)
			if current == nil {
				current = map[string]any{}
			}
			for nk, nv := range nested {
				current[nk] = nv
			}
			dst[k] = current
			continue
		}
		dst[k] = v
	}
	if _, ok := src["manualCpc"]; ok {
		dst["biddingStrategyType"] = "MANUAL_CPC"
	}
	if _, ok := src["maximizeConversions"]; ok {
		dst["biddingStrategyType"] = "MAXIMIZE_CONVERSIONS"
	}
	if _, ok := src["targetSpend"]; ok {
		dst["biddingStrategyType"] = "TARGET_SPEND"
	}
	if _, ok := src["targetCpa"]; ok {
		dst["biddingStrategyType"] = "TARGET_CPA"
	}
	if _, ok := src["targetRoas"]; ok {
		dst["biddingStrategyType"] = "TARGET_ROAS"
	}
	if _, ok := src["maximizeConversionValue"]; ok {
		dst["biddingStrategyType"] = "MAXIMIZE_CONVERSION_VALUE"
	}
}

func matchesCampaignQuery(query string, campaign map[string]any) bool {
	id := stringify(campaign["id"])
	name := stringify(campaign["name"])
	if strings.Contains(query, "campaign.id = ") {
		want := strings.TrimSpace(query[strings.Index(query, "campaign.id = ")+len("campaign.id = "):])
		if i := strings.IndexAny(want, " \n"); i >= 0 {
			want = want[:i]
		}
		return want == id
	}
	if strings.Contains(query, "campaign.name = ") {
		start := strings.Index(query, "campaign.name = ") + len("campaign.name = ")
		rest := strings.TrimSpace(query[start:])
		rest = strings.Trim(rest, "'")
		return rest == name
	}
	return true
}
