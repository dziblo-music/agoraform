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

func TestValidateAdGroupValid(t *testing.T) {
	t.Parallel()

	p, _ := testAdGroupProvider(t, nil)
	res := adGroupResource(t, "brand", defaultAdGroupAttrs(t))
	if err := p.Validate(context.Background(), res); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateAdGroupErrors(t *testing.T) {
	t.Parallel()

	p, _ := testAdGroupProvider(t, nil)
	addr := mustAdGroupAddress(t, "brand")
	campaign := campaignRef(t, "brand")

	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name:  "missing name",
			attrs: resource.Attributes{googleads.AttrCampaign: campaign},
			want:  "missing required attribute \"name\"",
		},
		{
			name:  "missing campaign",
			attrs: resource.Attributes{googleads.AttrName: "Brand"},
			want:  "missing required attribute \"campaign\"",
		},
		{
			name: "campaign not a ref",
			attrs: resource.Attributes{
				googleads.AttrName:     "Brand",
				googleads.AttrCampaign: "customers/" + testCustomerID + "/campaigns/21",
			},
			want: "$ref",
		},
		{
			name: "campaign wrong type",
			attrs: resource.Attributes{
				googleads.AttrName:     "Brand",
				googleads.AttrCampaign: resource.Ref{Address: mustCampaignBudgetAddress(t, "brand")},
			},
			want: "googleads.campaign",
		},
		{
			name: "unsupported type",
			attrs: resource.Attributes{
				googleads.AttrName:        "Brand",
				googleads.AttrCampaign:    campaign,
				googleads.AttrAdGroupType: "SHOPPING_PRODUCT_ADS",
			},
			want: "SEARCH_STANDARD",
		},
		{
			name: "dsa type rejected",
			attrs: resource.Attributes{
				googleads.AttrName:        "Brand",
				googleads.AttrCampaign:    campaign,
				googleads.AttrAdGroupType: "SEARCH_DYNAMIC_ADS",
			},
			want: "SEARCH_STANDARD",
		},
		{
			name: "enabled status accepted",
			attrs: resource.Attributes{
				googleads.AttrName:     "Brand",
				googleads.AttrStatus:   "ENABLED",
				googleads.AttrCampaign: campaign,
			},
			want: "",
		},
		{
			name: "removed status rejected",
			attrs: resource.Attributes{
				googleads.AttrName:     "Brand",
				googleads.AttrStatus:   "REMOVED",
				googleads.AttrCampaign: campaign,
			},
			want: "status",
		},
		{
			name: "computed id",
			attrs: resource.Attributes{
				googleads.AttrName:     "Brand",
				googleads.AttrCampaign: campaign,
				"id":                   "1",
			},
			want: "computed",
		},
		{
			name: "unsupported keywords attribute",
			attrs: resource.Attributes{
				googleads.AttrName:     "Brand",
				googleads.AttrCampaign: campaign,
				"keywords":             []any{"brand"},
			},
			want: "unsupported attribute",
		},
		{
			name: "zero cpc bid",
			attrs: resource.Attributes{
				googleads.AttrName:     "Brand",
				googleads.AttrCampaign: campaign,
				googleads.AttrCpcBid:   0,
			},
			want: "CPC bid",
		},
		{
			name: "search standard type accepted",
			attrs: resource.Attributes{
				googleads.AttrName:        "Brand",
				googleads.AttrCampaign:    campaign,
				googleads.AttrAdGroupType: "search_standard",
				googleads.AttrCpcBid:      1.5,
			},
			want: "",
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

func TestReadAdGroupSuccess(t *testing.T) {
	t.Parallel()

	fake := newAdGroupFake()
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	p, _ := testAdGroupProvider(t, fake)
	bindCampaignIdentity(t, p, "21")

	res := adGroupResource(t, "brand", defaultAdGroupAttrs(t))
	res.Identity = resource.Identity{ID: "31"}
	live, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "31" {
		t.Fatalf("identity = %q, want 31", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrName] != "Brand" {
		t.Fatalf("name = %v", live.Attributes[googleads.AttrName])
	}
	if live.Attributes[googleads.AttrStatus] != "PAUSED" {
		t.Fatalf("status = %v, want PAUSED", live.Attributes[googleads.AttrStatus])
	}
	ref, ok := resource.AsRef(live.Attributes[googleads.AttrCampaign])
	if !ok || ref.Address != mustCampaignAddress(t, "brand") {
		t.Fatalf("campaign = %#v, want logical $ref", live.Attributes[googleads.AttrCampaign])
	}
	if live.Computed["id"] != "31" {
		t.Fatalf("computed id = %v", live.Computed["id"])
	}
	if _, ok := live.Attributes["id"]; ok {
		t.Fatal("id must not appear in comparable attributes")
	}
	if _, ok := live.Attributes["cpcBidMicros"]; ok {
		t.Fatal("native cpcBidMicros must not appear in comparable attributes")
	}
}

func TestReadAdGroupNotFound(t *testing.T) {
	t.Parallel()

	p, _ := testAdGroupProvider(t, newAdGroupFake())
	_, err := p.Read(context.Background(), adGroupResource(t, "brand", resolvedAdGroupAttrs(t, "21")))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadAdGroupUnboundWithoutCampaignIdentityIsNotFound(t *testing.T) {
	t.Parallel()

	fake := newAdGroupFake()
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	p, _ := testAdGroupProvider(t, fake)

	_, err := p.Read(context.Background(), adGroupResource(t, "brand", defaultAdGroupAttrs(t)))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound when campaign identity is unknown", err)
	}
}

func TestReadAdGroupSameNameDifferentCampaigns(t *testing.T) {
	t.Parallel()

	fake := newAdGroupFake()
	fake.seedAdGroup(sampleSearchAdGroup("99", "Brand", "88"))
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	p, _ := testAdGroupProvider(t, fake)

	live, err := p.Read(context.Background(), adGroupResource(t, "brand", resolvedAdGroupAttrs(t, "21")))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "31" {
		t.Fatalf("identity = %q, want 31 from the referenced campaign", live.Identity.ID)
	}
}

func TestReadAdGroupMultipleNamesAreDeterministic(t *testing.T) {
	t.Parallel()

	fake := newAdGroupFake()
	fake.seedAdGroup(sampleSearchAdGroup("41", "Brand", "21"))
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	p, _ := testAdGroupProvider(t, fake)

	_, err := p.Read(context.Background(), adGroupResource(t, "brand", resolvedAdGroupAttrs(t, "21")))
	if err == nil {
		t.Fatal("expected unique-name error")
	}
	if !strings.Contains(err.Error(), "ids 31, 41") {
		t.Fatalf("error = %q, want deterministic sorted ids 31, 41", err)
	}
}

func TestReadAdGroupRejectsShoppingType(t *testing.T) {
	t.Parallel()

	fake := newAdGroupFake()
	group := sampleSearchAdGroup("9", "Shopping", "21")
	group["type"] = "SHOPPING_PRODUCT_ADS"
	fake.seedAdGroup(group)
	p, _ := testAdGroupProvider(t, fake)

	res := adGroupResource(t, "shopping", resource.Attributes{
		googleads.AttrName:     "Shopping",
		googleads.AttrCampaign: campaignRef(t, "brand"),
	})
	res.Identity = resource.Identity{ID: "9"}
	_, err := p.Read(context.Background(), res)
	if err == nil {
		t.Fatal("expected unsupported type error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported type must not look like not found")
	}
	if !strings.Contains(err.Error(), "SEARCH_STANDARD") {
		t.Fatalf("error = %q, want SEARCH_STANDARD guidance", err)
	}
}

func TestReadAdGroupAPIError(t *testing.T) {
	t.Parallel()

	fake := newAdGroupFake()
	fake.searchStatus = http.StatusForbidden
	p, _ := testAdGroupProvider(t, fake)
	_, err := p.Read(context.Background(), adGroupResource(t, "brand", resolvedAdGroupAttrs(t, "21")))
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestCreateAdGroupDefaultsPausedSearchStandard(t *testing.T) {
	t.Parallel()

	fake := newAdGroupFake()
	p, _ := testAdGroupProvider(t, fake)

	live, err := p.Create(context.Background(), adGroupResource(t, "brand", resolvedAdGroupAttrs(t, "21")))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID == "" {
		t.Fatal("create returned empty identity")
	}
	if live.Attributes[googleads.AttrStatus] != "PAUSED" {
		t.Fatalf("status = %v, want PAUSED", live.Attributes[googleads.AttrStatus])
	}
	if !strings.Contains(fake.lastMutate, `"status":"PAUSED"`) && !strings.Contains(fake.lastMutate, `"status": "PAUSED"`) {
		t.Fatalf("create mutate missing PAUSED: %s", fake.lastMutate)
	}
	if !strings.Contains(fake.lastMutate, `"type":"SEARCH_STANDARD"`) && !strings.Contains(fake.lastMutate, `"type": "SEARCH_STANDARD"`) {
		t.Fatalf("create mutate missing SEARCH_STANDARD: %s", fake.lastMutate)
	}
	if !strings.Contains(fake.lastMutate, "campaigns/21") {
		t.Fatalf("create mutate missing resolved campaign: %s", fake.lastMutate)
	}
}

func TestCreateAdGroupResolvesCampaignReference(t *testing.T) {
	t.Parallel()

	fake := newAdGroupFake()
	p, _ := testAdGroupProvider(t, fake)
	attrs := defaultAdGroupAttrs(t)
	attrs[googleads.AttrCampaign] = resource.Resolved{
		Address:  mustCampaignAddress(t, "brand"),
		Identity: resource.Identity{ID: "21"},
		Outputs:  resource.Attributes{"resourceName": "customers/" + testCustomerID + "/campaigns/21"},
	}
	attrs[googleads.AttrCpcBid] = 1.25
	if _, err := p.Create(context.Background(), adGroupResource(t, "brand", attrs)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.Contains(fake.lastMutate, `"cpcBidMicros":"1250000"`) && !strings.Contains(fake.lastMutate, `"cpcBidMicros": "1250000"`) {
		t.Fatalf("create mutate missing cpc bid micros: %s", fake.lastMutate)
	}
}

func TestCreateAdGroupMissingCampaignIdentity(t *testing.T) {
	t.Parallel()

	p, _ := testAdGroupProvider(t, newAdGroupFake())
	_, err := p.Create(context.Background(), adGroupResource(t, "brand", defaultAdGroupAttrs(t)))
	if err == nil {
		t.Fatal("expected missing campaign identity error")
	}
	if !strings.Contains(err.Error(), "googleads.campaign.brand") {
		t.Fatalf("error = %q, want campaign address", err)
	}
}

func TestCreateAdGroupAPIError(t *testing.T) {
	t.Parallel()

	fake := newAdGroupFake()
	fake.mutateStatus = http.StatusBadRequest
	p, _ := testAdGroupProvider(t, fake)
	_, err := p.Create(context.Background(), adGroupResource(t, "brand", resolvedAdGroupAttrs(t, "21")))
	if err == nil {
		t.Fatal("expected API error")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestUpdateAdGroup(t *testing.T) {
	t.Parallel()

	fake := newAdGroupFake()
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	p, _ := testAdGroupProvider(t, fake)
	bindCampaignIdentity(t, p, "21")

	desired := adGroupResource(t, "brand", resource.Attributes{
		googleads.AttrName:   "Brand Search",
		googleads.AttrStatus: "ENABLED",
		googleads.AttrCampaign: resource.Resolved{
			Address:  mustCampaignAddress(t, "brand"),
			Identity: resource.Identity{ID: "21"},
			Outputs:  resource.Attributes{"resourceName": "customers/" + testCustomerID + "/campaigns/21"},
		},
		googleads.AttrCpcBid: 2,
	})
	live, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "31"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if live.Identity.ID != "31" {
		t.Fatalf("identity = %q, want 31", live.Identity.ID)
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
	if strings.Contains(fake.lastMutate, `"campaign"`) && strings.Contains(fake.lastMutate, "updateMask") {
		if strings.Contains(fake.lastMutate, "campaign,") || strings.Contains(fake.lastMutate, ",campaign") || strings.Contains(fake.lastMutate, `"updateMask":"campaign"`) {
			t.Fatalf("update must not mutate immutable campaign: %s", fake.lastMutate)
		}
	}
}

func TestUpdateAdGroupNoOp(t *testing.T) {
	t.Parallel()

	fake := newAdGroupFake()
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	p, _ := testAdGroupProvider(t, fake)
	bindCampaignIdentity(t, p, "21")

	desired := adGroupResource(t, "brand", resolvedAdGroupAttrs(t, "21"))
	if _, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "31"},
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("no-op update mutated remote: %v", fake.mutates)
	}
}

func TestUpdateAdGroupRejectsCampaignChange(t *testing.T) {
	t.Parallel()

	fake := newAdGroupFake()
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	p, _ := testAdGroupProvider(t, fake)
	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustCampaignAddress(t, "other"), resource.Identity{ID: "88"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	desired := adGroupResource(t, "brand", resource.Attributes{
		googleads.AttrName: "Brand",
		googleads.AttrCampaign: resource.Resolved{
			Address:  mustCampaignAddress(t, "other"),
			Identity: resource.Identity{ID: "88"},
		},
	})
	_, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "31"},
	})
	if err == nil {
		t.Fatal("expected immutable campaign error")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("error = %q, want immutable campaign", err)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("rejected campaign change mutated remote: %v", fake.mutates)
	}
}

func TestUpdateAdGroupAPIError(t *testing.T) {
	t.Parallel()

	fake := newAdGroupFake()
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	fake.mutateStatus = http.StatusBadRequest
	p, _ := testAdGroupProvider(t, fake)
	bindCampaignIdentity(t, p, "21")

	desired := adGroupResource(t, "brand", resource.Attributes{
		googleads.AttrName:     "Brand Search",
		googleads.AttrCampaign: resolvedAdGroupAttrs(t, "21")[googleads.AttrCampaign],
	})
	_, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "31"},
	})
	if err == nil {
		t.Fatal("expected API error")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestImportAdGroupRequiresBoundCampaign(t *testing.T) {
	t.Parallel()

	fake := newAdGroupFake()
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	p, _ := testAdGroupProvider(t, fake)
	_, err := p.Import(context.Background(), mustAdGroupAddress(t, "brand"), "31")
	if err == nil {
		t.Fatal("expected missing campaign binding error")
	}
	if !strings.Contains(err.Error(), "campaign") {
		t.Fatalf("error = %q, want campaign import guidance", err)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("import mutated remote: %v", fake.mutates)
	}
}

func TestImportAdGroupThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	fake := newAdGroupFake()
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
	group := sampleSearchAdGroup("31", "Brand", "21")
	group["cpcBidMicros"] = "1500000"
	fake.seedAdGroup(group)
	p, _ := testAdGroupProvider(t, fake)

	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignBudgetAddress(t, "brand"), resource.Identity{ID: "11"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	got, err := importer.Run(context.Background(), mustAdGroupAddress(t, "brand"), "31", lookupGoogleAds(p), st)
	if err != nil {
		t.Fatalf("importer.Run: %v", err)
	}
	if got.Identity.ID != "31" {
		t.Fatalf("identity = %q, want 31", got.Identity.ID)
	}
	assertNoProviderSecret(t, got.YAML)
	for _, leak := range []string{"resourceName", "cpcBidMicros", "primaryStatus", testAccessToken} {
		if strings.Contains(got.YAML, leak) {
			t.Fatalf("generated YAML leaked %q:\n%s", leak, got.YAML)
		}
	}
	if !strings.Contains(got.YAML, "$ref: googleads.campaign.brand") {
		t.Fatalf("generated YAML missing campaign $ref:\n%s", got.YAML)
	}

	combined := `apiVersion: agoraform.io/v1alpha1
resources:
  - address: googleads.campaign_budget.brand
    attributes:
      name: Brand daily budget
      amount: 50
      explicitlyShared: false
      deliveryMethod: STANDARD
  - address: googleads.campaign.brand
    attributes:
      name: Brand
      budget:
        $ref: googleads.campaign_budget.brand
      bidding:
        strategy: MANUAL_CPC
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

func TestNormalizeAdGroupImportID(t *testing.T) {
	t.Parallel()

	p, _ := testAdGroupProvider(t, nil)
	addr := mustAdGroupAddress(t, "brand")
	got, err := p.NormalizeImportID(addr, "31")
	if err != nil || got != "31" {
		t.Fatalf("numeric = (%q, %v), want 31", got, err)
	}
	got, err = p.NormalizeImportID(addr, "customers/"+testCustomerID+"/adGroups/31")
	if err != nil || got != "31" {
		t.Fatalf("resource name = (%q, %v), want 31", got, err)
	}
	_, err = p.NormalizeImportID(addr, "abc")
	if err == nil || !strings.Contains(err.Error(), "not a valid Google Ads ad group id") {
		t.Fatalf("invalid id error = %v", err)
	}
}

func TestPlanAdGroupCreateWhenMissing(t *testing.T) {
	t.Parallel()

	p, _ := testAdGroupProvider(t, newAdGroupFake())
	got := mustPlanAdGroup(t, p, adGroupStack(t, defaultAdGroupAttrs(t))...)
	if len(got.Changes) != 3 {
		t.Fatalf("changes = %+v, want 3", got.Changes)
	}
	byAddr := map[string]plan.Action{}
	for _, change := range got.Changes {
		byAddr[change.Address.String()] = change.Action
	}
	if byAddr["googleads.campaign_budget.brand"] != plan.ActionCreate || byAddr["googleads.campaign.brand"] != plan.ActionCreate || byAddr["googleads.ad_group.brand"] != plan.ActionCreate {
		t.Fatalf("actions = %+v, want create/create/create", byAddr)
	}
}

func TestPlanAdGroupUnchangedEquivalentRemote(t *testing.T) {
	t.Parallel()

	fake := newAdGroupFake()
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
	group := sampleSearchAdGroup("31", "Brand", "21")
	group["cpcBidMicros"] = "1500000"
	fake.seedAdGroup(group)
	p, _ := testAdGroupProvider(t, fake)

	attrs := defaultAdGroupAttrs(t)
	attrs[googleads.AttrAdGroupType] = "search_standard"
	attrs[googleads.AttrCpcBid] = "1.50"
	got := mustPlanAdGroup(t, p, adGroupStack(t, attrs)...)
	if got.HasChanges() {
		t.Fatalf("equivalent remote produced changes: %+v", got.Changes)
	}
}

func TestPlanAdGroupOmittedStatusDefaultsPaused(t *testing.T) {
	t.Parallel()

	fake := newAdGroupFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false, "period": "DAILY", "type": "STANDARD"})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	live := sampleSearchAdGroup("31", "Brand", "21")
	live["status"] = "ENABLED"
	fake.seedAdGroup(live)
	p, _ := testAdGroupProvider(t, fake)

	got := mustPlanAdGroup(t, p, adGroupStack(t, defaultAdGroupAttrs(t))...)
	change := adGroupChange(t, got)
	if change.Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want ad group update to PAUSED", got.Changes)
	}
	found := false
	for _, diff := range change.Diffs {
		if diff.Path == googleads.AttrStatus {
			found = true
			if diff.After != "PAUSED" {
				t.Fatalf("status after = %v, want PAUSED", diff.After)
			}
		}
	}
	if !found {
		t.Fatalf("missing status diff: %+v", change.Diffs)
	}
}

func TestPlanAdGroupOmittedCpcBidIsNoOp(t *testing.T) {
	t.Parallel()

	fake := newAdGroupFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false, "period": "DAILY", "type": "STANDARD"})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	group := sampleSearchAdGroup("31", "Brand", "21")
	group["cpcBidMicros"] = "2500000"
	fake.seedAdGroup(group)
	p, _ := testAdGroupProvider(t, fake)

	got := mustPlanAdGroup(t, p, adGroupStack(t, defaultAdGroupAttrs(t))...)
	if got.HasChanges() {
		t.Fatalf("omitted cpcBid produced changes: %+v", got.Changes)
	}
}

func TestPlanAdGroupIgnoresComputedFields(t *testing.T) {
	t.Parallel()

	fake := newAdGroupFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false, "period": "DAILY", "type": "STANDARD"})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	group := sampleSearchAdGroup("31", "Brand", "21")
	group["primaryStatus"] = "PENDING"
	group["effectiveCpcBidMicros"] = "1000000"
	fake.seedAdGroup(group)
	p, _ := testAdGroupProvider(t, fake)

	got := mustPlanAdGroup(t, p, adGroupStack(t, defaultAdGroupAttrs(t))...)
	if got.HasChanges() {
		t.Fatalf("computed fields produced changes: %+v", got.Changes)
	}
}

func TestPlanAdGroupUpdateStatusAndCpcBid(t *testing.T) {
	t.Parallel()

	fake := newAdGroupFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false, "period": "DAILY", "type": "STANDARD"})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	p, _ := testAdGroupProvider(t, fake)

	attrs := defaultAdGroupAttrs(t)
	attrs[googleads.AttrStatus] = "ENABLED"
	attrs[googleads.AttrCpcBid] = 2
	got := mustPlanAdGroup(t, p, adGroupStack(t, attrs)...)
	change := adGroupChange(t, got)
	if change.Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want ad group update", got.Changes)
	}
}

func mustPlanAdGroup(t *testing.T, p *googleads.Provider, resources ...resource.Resource) *plan.Plan {
	t.Helper()
	got, err := plan.Build(context.Background(), resources, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	return got
}

func adGroupChange(t *testing.T, got *plan.Plan) *plan.Change {
	t.Helper()
	addr := mustAdGroupAddress(t, "brand")
	for i := range got.Changes {
		if got.Changes[i].Address == addr {
			return &got.Changes[i]
		}
	}
	t.Fatalf("missing ad group change: %+v", got.Changes)
	return nil
}

func defaultAdGroupAttrs(t *testing.T) resource.Attributes {
	t.Helper()
	return resource.Attributes{
		googleads.AttrName:     "Brand",
		googleads.AttrCampaign: campaignRef(t, "brand"),
	}
}

func resolvedAdGroupAttrs(t *testing.T, campaignID string) resource.Attributes {
	t.Helper()
	return resource.Attributes{
		googleads.AttrName: "Brand",
		googleads.AttrCampaign: resource.Resolved{
			Address:  mustCampaignAddress(t, "brand"),
			Identity: resource.Identity{ID: campaignID},
			Outputs:  resource.Attributes{"resourceName": "customers/" + testCustomerID + "/campaigns/" + campaignID},
		},
	}
}

func adGroupResource(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{Address: mustAdGroupAddress(t, name), Attributes: attrs}
}

func mustAdGroupAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("googleads.ad_group." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func adGroupStack(t *testing.T, groupAttrs resource.Attributes) []resource.Resource {
	t.Helper()
	return []resource.Resource{
		campaignBudgetResource(t, "brand", resource.Attributes{
			googleads.AttrName:             "Brand daily budget",
			googleads.AttrAmount:           50,
			googleads.AttrExplicitlyShared: false,
		}),
		campaignResource(t, "brand", defaultCampaignAttrs(t)),
		adGroupResource(t, "brand", groupAttrs),
	}
}

func testAdGroupProvider(t *testing.T, fake *adGroupFake) (*googleads.Provider, *httptest.Server) {
	t.Helper()
	if fake == nil {
		fake = newAdGroupFake()
	}
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	t.Cleanup(srv.Close)
	cfg := validConfig(srv.URL)
	cfg.TokenURL = srv.URL + "/oauth/token"
	p := googleads.NewWithHTTPClient(cfg, srv.Client())
	return p, srv
}

func sampleSearchAdGroup(id, name, campaignID string) map[string]any {
	return map[string]any{
		"id":            id,
		"name":          name,
		"status":        "PAUSED",
		"type":          "SEARCH_STANDARD",
		"campaign":      "customers/" + testCustomerID + "/campaigns/" + campaignID,
		"primaryStatus": "PENDING",
	}
}

type adGroupFake struct {
	mu sync.Mutex

	nextID    int64
	adGroups  map[string]map[string]any
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

func newAdGroupFake() *adGroupFake {
	return &adGroupFake{
		adGroups:  map[string]map[string]any{},
		campaigns: map[string]map[string]any{},
		budgets:   map[string]map[string]any{},
	}
}

func (f *adGroupFake) seedAdGroup(group map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storeAdGroupLocked(cloneMap(group))
}

func (f *adGroupFake) seedCampaign(campaign map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cloned := cloneMap(campaign)
	id := stringify(cloned["id"])
	if stringify(cloned["resourceName"]) == "" {
		cloned["resourceName"] = "customers/" + testCustomerID + "/campaigns/" + id
	}
	if stringify(cloned["advertisingChannelType"]) == "" {
		cloned["advertisingChannelType"] = "SEARCH"
	}
	if stringify(cloned["status"]) == "" {
		cloned["status"] = "PAUSED"
	}
	f.campaigns[id] = cloned
}

func (f *adGroupFake) seedBudget(budget map[string]any) {
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

func (f *adGroupFake) storeAdGroupLocked(group map[string]any) {
	id := stringify(group["id"])
	if stringify(group["resourceName"]) == "" {
		group["resourceName"] = "customers/" + testCustomerID + "/adGroups/" + id
	}
	if stringify(group["type"]) == "" {
		group["type"] = "SEARCH_STANDARD"
	}
	if stringify(group["status"]) == "" {
		group["status"] = "PAUSED"
	}
	f.adGroups[id] = group
}

func (f *adGroupFake) handler(w http.ResponseWriter, r *http.Request) {
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
		switch {
		case strings.Contains(query, "from ad_group"):
			_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchAdGroupsLocked(req.Query)})
		case strings.Contains(query, "from campaign_budget"):
			_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchBudgetsLocked(req.Query)})
		case strings.Contains(query, "from campaign"):
			_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchCampaignsLocked(req.Query)})
		case strings.Contains(query, "from customer"):
			_, _ = io.WriteString(w, `{"results":[{"customer":{"id":"`+testCustomerID+`"}}]}`)
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
		}
		return
	}
	if strings.Contains(r.URL.Path, ":mutate") {
		f.lastMutate = string(body)
		collection := mutateCollection(r.URL.Path)
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

func (f *adGroupFake) searchAdGroupsLocked(query string) []any {
	var out []any
	for _, group := range f.adGroups {
		if matchesAdGroupQuery(query, group) {
			out = append(out, map[string]any{"adGroup": cloneMap(group)})
		}
	}
	return out
}

func (f *adGroupFake) searchCampaignsLocked(query string) []any {
	var out []any
	for _, campaign := range f.campaigns {
		if matchesCampaignQuery(query, campaign) {
			out = append(out, map[string]any{"campaign": cloneMap(campaign)})
		}
	}
	return out
}

func (f *adGroupFake) searchBudgetsLocked(query string) []any {
	var out []any
	for _, budget := range f.budgets {
		if matchesCampaignBudgetQuery(query, budget) {
			out = append(out, map[string]any{"campaignBudget": cloneMap(budget)})
		}
	}
	return out
}

func (f *adGroupFake) mutateLocked(collection string, body []byte) (string, error) {
	var req struct {
		Operations []map[string]any `json:"operations"`
	}
	if err := json.Unmarshal(body, &req); err != nil || len(req.Operations) == 0 {
		return "", errors.New("malformed mutate")
	}
	op := req.Operations[0]
	if collection != "adGroups" {
		return "", errors.New("unexpected mutate " + collection)
	}
	if raw, ok := op["create"]; ok {
		group, _ := raw.(map[string]any)
		created := cloneMap(group)
		f.nextID++
		id := strconv.FormatInt(f.nextID, 10)
		created["id"] = id
		if stringify(created["type"]) == "" {
			created["type"] = "SEARCH_STANDARD"
		}
		if stringify(created["status"]) == "" {
			created["status"] = "PAUSED"
		}
		f.storeAdGroupLocked(created)
		return stringify(created["resourceName"]), nil
	}
	if raw, ok := op["update"]; ok {
		group, _ := raw.(map[string]any)
		resourceName := stringify(group["resourceName"])
		id := strings.TrimPrefix(resourceName, "customers/"+testCustomerID+"/adGroups/")
		current, ok := f.adGroups[id]
		if !ok {
			return "", errors.New("missing ad group")
		}
		merged := cloneMap(current)
		for k, v := range group {
			if k == "resourceName" {
				continue
			}
			merged[k] = v
		}
		f.storeAdGroupLocked(merged)
		return resourceName, nil
	}
	return "", errors.New("unsupported mutate")
}

func matchesAdGroupQuery(query string, group map[string]any) bool {
	id := stringify(group["id"])
	name := stringify(group["name"])
	campaign := stringify(group["campaign"])
	if strings.Contains(query, "ad_group.id = ") {
		want := strings.TrimSpace(query[strings.Index(query, "ad_group.id = ")+len("ad_group.id = "):])
		if i := strings.IndexAny(want, " \n"); i >= 0 {
			want = want[:i]
		}
		return want == id
	}
	if strings.Contains(query, "ad_group.campaign = ") {
		start := strings.Index(query, "ad_group.campaign = ") + len("ad_group.campaign = ")
		rest := strings.TrimSpace(query[start:])
		end := strings.Index(rest, " AND ")
		if end >= 0 {
			rest = rest[:end]
		}
		rest = strings.Trim(strings.TrimSpace(rest), "'")
		if rest != campaign {
			return false
		}
	}
	if strings.Contains(query, "ad_group.name = ") {
		start := strings.Index(query, "ad_group.name = ") + len("ad_group.name = ")
		rest := strings.TrimSpace(query[start:])
		if i := strings.Index(rest, " AND "); i >= 0 {
			rest = rest[:i]
		}
		if i := strings.Index(rest, " ORDER "); i >= 0 {
			rest = rest[:i]
		}
		rest = strings.Trim(strings.TrimSpace(rest), "'")
		return rest == name
	}
	return true
}
