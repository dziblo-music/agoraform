package googleads_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestValidateCampaignNegativeKeywordValid(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, nil)
	for _, matchType := range []string{"EXACT", "PHRASE", "BROAD", "phrase"} {
		res := campaignNegativeKeywordResource(t, "jobs", defaultCampaignNegativeKeywordAttrs(t))
		res.Attributes[googleads.AttrMatchType] = matchType
		if err := p.Validate(context.Background(), res); err != nil {
			t.Fatalf("Validate matchType %s: %v", matchType, err)
		}
	}
}

func TestValidateCampaignNegativeKeywordErrors(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, nil)
	addr := mustCampaignNegativeKeywordAddress(t, "jobs")
	campaign := campaignRef(t, "brand")

	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name:  "missing text",
			attrs: resource.Attributes{googleads.AttrCampaign: campaign, googleads.AttrMatchType: "PHRASE"},
			want:  "missing required attribute \"text\"",
		},
		{
			name:  "missing campaign",
			attrs: resource.Attributes{googleads.AttrText: "jobs", googleads.AttrMatchType: "PHRASE"},
			want:  "missing required attribute \"campaign\"",
		},
		{
			name:  "missing match type",
			attrs: resource.Attributes{googleads.AttrCampaign: campaign, googleads.AttrText: "jobs"},
			want:  "missing required attribute \"matchType\"",
		},
		{
			name: "campaign not a ref",
			attrs: resource.Attributes{
				googleads.AttrCampaign:  "customers/" + testCustomerID + "/campaigns/21",
				googleads.AttrText:      "jobs",
				googleads.AttrMatchType: "PHRASE",
			},
			want: "$ref",
		},
		{
			name: "campaign wrong type",
			attrs: resource.Attributes{
				googleads.AttrCampaign:  resource.Ref{Address: mustAdGroupAddress(t, "brand")},
				googleads.AttrText:      "jobs",
				googleads.AttrMatchType: "PHRASE",
			},
			want: "googleads.campaign",
		},
		{
			name: "unsupported match type",
			attrs: resource.Attributes{
				googleads.AttrCampaign:  campaign,
				googleads.AttrText:      "jobs",
				googleads.AttrMatchType: "BROAD_MATCH_MODIFIED",
			},
			want: "matchType",
		},
		{
			name: "match type punctuation rejected",
			attrs: resource.Attributes{
				googleads.AttrCampaign:  campaign,
				googleads.AttrText:      "[jobs]",
				googleads.AttrMatchType: "EXACT",
			},
			want: "match-type punctuation",
		},
		{
			name: "phrase punctuation rejected",
			attrs: resource.Attributes{
				googleads.AttrCampaign:  campaign,
				googleads.AttrText:      `"jobs"`,
				googleads.AttrMatchType: "PHRASE",
			},
			want: "match-type punctuation",
		},
		{
			name: "empty text",
			attrs: resource.Attributes{
				googleads.AttrCampaign:  campaign,
				googleads.AttrText:      "   ",
				googleads.AttrMatchType: "PHRASE",
			},
			want: "non-empty",
		},
		{
			name: "negative omitted is implied",
			attrs: resource.Attributes{
				googleads.AttrCampaign:  campaign,
				googleads.AttrText:      "jobs",
				googleads.AttrMatchType: "PHRASE",
			},
			want: "",
		},
		{
			name: "negative attribute rejected",
			attrs: resource.Attributes{
				googleads.AttrCampaign:  campaign,
				googleads.AttrText:      "jobs",
				googleads.AttrMatchType: "PHRASE",
				googleads.AttrNegative:  true,
			},
			want: "always negative",
		},
		{
			name: "positive negative rejected",
			attrs: resource.Attributes{
				googleads.AttrCampaign:  campaign,
				googleads.AttrText:      "jobs",
				googleads.AttrMatchType: "PHRASE",
				googleads.AttrNegative:  false,
			},
			want: "always negative",
		},
		{
			name: "ad group rejected",
			attrs: resource.Attributes{
				googleads.AttrCampaign:  campaign,
				googleads.AttrAdGroup:   adGroupRef(t, "brand"),
				googleads.AttrText:      "jobs",
				googleads.AttrMatchType: "PHRASE",
			},
			want: "not an ad group",
		},
		{
			name: "cpc bid rejected",
			attrs: resource.Attributes{
				googleads.AttrCampaign:  campaign,
				googleads.AttrText:      "jobs",
				googleads.AttrMatchType: "PHRASE",
				googleads.AttrCpcBid:    1.5,
			},
			want: "cpcBid",
		},
		{
			name: "status rejected",
			attrs: resource.Attributes{
				googleads.AttrCampaign:  campaign,
				googleads.AttrText:      "jobs",
				googleads.AttrMatchType: "PHRASE",
				googleads.AttrStatus:    "PAUSED",
			},
			want: "computed",
		},
		{
			name: "computed id",
			attrs: resource.Attributes{
				googleads.AttrCampaign:  campaign,
				googleads.AttrText:      "jobs",
				googleads.AttrMatchType: "PHRASE",
				"id":                    "21~61",
			},
			want: "computed",
		},
		{
			name: "unsupported audience",
			attrs: resource.Attributes{
				googleads.AttrCampaign:  campaign,
				googleads.AttrText:      "jobs",
				googleads.AttrMatchType: "PHRASE",
				"audience":              "in-market",
			},
			want: "unsupported attribute",
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

func TestReadCampaignNegativeKeywordSuccess(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleCampaignNegativeKeyword("21", "61", "Jobs", "PHRASE"))
	p, _ := testTargetingProvider(t, fake)
	bindCampaignIdentity(t, p, "21")

	res := campaignNegativeKeywordResource(t, "jobs", defaultCampaignNegativeKeywordAttrs(t))
	res.Identity = resource.Identity{ID: "21~61"}
	live, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "21~61" {
		t.Fatalf("identity = %q, want 21~61", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrText] != "jobs" {
		t.Fatalf("text = %v, want jobs", live.Attributes[googleads.AttrText])
	}
	if live.Attributes[googleads.AttrMatchType] != "PHRASE" {
		t.Fatalf("matchType = %v, want PHRASE", live.Attributes[googleads.AttrMatchType])
	}
	if _, ok := live.Attributes[googleads.AttrNegative]; ok {
		t.Fatal("negative must not appear in comparable attributes")
	}
	ref, ok := resource.AsRef(live.Attributes[googleads.AttrCampaign])
	if !ok || ref.Address != mustCampaignAddress(t, "brand") {
		t.Fatalf("campaign = %#v, want logical $ref", live.Attributes[googleads.AttrCampaign])
	}
	if live.Computed[googleads.AttrNegative] != true {
		t.Fatalf("computed negative = %v, want true", live.Computed[googleads.AttrNegative])
	}
}

func TestReadCampaignNegativeKeywordByName(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleCampaignNegativeKeyword("21", "61", "Jobs", "PHRASE"))
	p, _ := testTargetingProvider(t, fake)

	live, err := p.Read(context.Background(), campaignNegativeKeywordResource(t, "jobs", resolvedCampaignNegativeKeywordAttrs(t, "21")))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "21~61" {
		t.Fatalf("identity = %q", live.Identity.ID)
	}
}

func TestReadCampaignNegativeKeywordMatchTypes(t *testing.T) {
	t.Parallel()

	for _, matchType := range []string{"EXACT", "PHRASE", "BROAD"} {
		matchType := matchType
		t.Run(matchType, func(t *testing.T) {
			t.Parallel()
			fake := newTargetingFake()
			fake.seedCriterion(sampleCampaignNegativeKeyword("21", "61", "jobs", matchType))
			p, _ := testTargetingProvider(t, fake)
			attrs := resolvedCampaignNegativeKeywordAttrs(t, "21")
			attrs[googleads.AttrMatchType] = matchType
			live, err := p.Read(context.Background(), campaignNegativeKeywordResource(t, "jobs", attrs))
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if live.Attributes[googleads.AttrMatchType] != matchType {
				t.Fatalf("matchType = %v, want %s", live.Attributes[googleads.AttrMatchType], matchType)
			}
		})
	}
}

func TestReadCampaignNegativeKeywordNotFound(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, newTargetingFake())
	_, err := p.Read(context.Background(), campaignNegativeKeywordResource(t, "jobs", resolvedCampaignNegativeKeywordAttrs(t, "21")))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
}

func TestReadCampaignNegativeKeywordRejectsLocation(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleLocationCriterion("21", "61", "geoTargetConstants/2840", false))
	p, _ := testTargetingProvider(t, fake)
	bindCampaignIdentity(t, p, "21")

	res := campaignNegativeKeywordResource(t, "jobs", defaultCampaignNegativeKeywordAttrs(t))
	res.Identity = resource.Identity{ID: "21~61"}
	_, err := p.Read(context.Background(), res)
	if err == nil {
		t.Fatal("expected type error")
	}
	if !strings.Contains(err.Error(), "LOCATION") {
		t.Fatalf("error = %q, want LOCATION guidance", err)
	}
}

func TestReadCampaignNegativeKeywordRejectsPositiveKeyword(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	item := sampleCampaignNegativeKeyword("21", "61", "jobs", "PHRASE")
	item["negative"] = false
	fake.seedCriterion(item)
	p, _ := testTargetingProvider(t, fake)
	bindCampaignIdentity(t, p, "21")

	res := campaignNegativeKeywordResource(t, "jobs", defaultCampaignNegativeKeywordAttrs(t))
	res.Identity = resource.Identity{ID: "21~61"}
	_, err := p.Read(context.Background(), res)
	if err == nil {
		t.Fatal("expected positive keyword error")
	}
	if !strings.Contains(err.Error(), "positive") {
		t.Fatalf("error = %q, want positive keyword guidance", err)
	}
}

func TestReadCampaignNegativeKeywordAPIError(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.searchStatus = http.StatusForbidden
	p, _ := testTargetingProvider(t, fake)
	res := campaignNegativeKeywordResource(t, "jobs", defaultCampaignNegativeKeywordAttrs(t))
	res.Identity = resource.Identity{ID: "21~61"}
	_, err := p.Read(context.Background(), res)
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestCreateCampaignNegativeKeyword(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	p, _ := testTargetingProvider(t, fake)
	live, err := p.Create(context.Background(), campaignNegativeKeywordResource(t, "jobs", resolvedCampaignNegativeKeywordAttrs(t, "21")))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID == "" {
		t.Fatal("create returned empty identity")
	}
	if !strings.Contains(fake.lastMutate, `"negative":true`) && !strings.Contains(fake.lastMutate, `"negative": true`) {
		t.Fatalf("create mutate missing negative: %s", fake.lastMutate)
	}
	if !strings.Contains(fake.lastMutate, "campaigns/21") {
		t.Fatalf("create mutate missing campaign: %s", fake.lastMutate)
	}
	if !strings.Contains(fake.lastMutate, `"text":"jobs"`) && !strings.Contains(fake.lastMutate, `"text": "jobs"`) {
		t.Fatalf("create mutate missing text: %s", fake.lastMutate)
	}
	if !strings.Contains(fake.lastMutate, "PHRASE") {
		t.Fatalf("create mutate missing match type: %s", fake.lastMutate)
	}
}

func TestCreateCampaignNegativeKeywordMatchTypes(t *testing.T) {
	t.Parallel()

	for _, matchType := range []string{"EXACT", "PHRASE", "BROAD"} {
		matchType := matchType
		t.Run(matchType, func(t *testing.T) {
			t.Parallel()
			fake := newTargetingFake()
			p, _ := testTargetingProvider(t, fake)
			attrs := resolvedCampaignNegativeKeywordAttrs(t, "21")
			attrs[googleads.AttrMatchType] = matchType
			if _, err := p.Create(context.Background(), campaignNegativeKeywordResource(t, "jobs", attrs)); err != nil {
				t.Fatalf("Create: %v", err)
			}
			if !strings.Contains(fake.lastMutate, matchType) {
				t.Fatalf("create mutate missing match type %s: %s", matchType, fake.lastMutate)
			}
		})
	}
}

func TestCreateCampaignNegativeKeywordMissingCampaignIdentity(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, newTargetingFake())
	_, err := p.Create(context.Background(), campaignNegativeKeywordResource(t, "jobs", defaultCampaignNegativeKeywordAttrs(t)))
	if err == nil {
		t.Fatal("expected missing campaign identity")
	}
	if !strings.Contains(err.Error(), "campaign") {
		t.Fatalf("error = %q, want campaign identity guidance", err)
	}
}

func TestCreateCampaignNegativeKeywordAPIError(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.mutateStatus = http.StatusBadRequest
	p, _ := testTargetingProvider(t, fake)
	_, err := p.Create(context.Background(), campaignNegativeKeywordResource(t, "jobs", resolvedCampaignNegativeKeywordAttrs(t, "21")))
	if err == nil {
		t.Fatal("expected API error")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestUpdateCampaignNegativeKeywordNoOp(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleCampaignNegativeKeyword("21", "61", "jobs", "PHRASE"))
	p, _ := testTargetingProvider(t, fake)
	desired := campaignNegativeKeywordResource(t, "jobs", resolvedCampaignNegativeKeywordAttrs(t, "21"))
	if _, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "21~61"},
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if fake.lastMutate != "" {
		t.Fatalf("equivalent update mutated remote: %s", fake.lastMutate)
	}
}

func TestUpdateCampaignNegativeKeywordRejectsImmutableText(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleCampaignNegativeKeyword("21", "61", "jobs", "PHRASE"))
	p, _ := testTargetingProvider(t, fake)
	attrs := resolvedCampaignNegativeKeywordAttrs(t, "21")
	attrs[googleads.AttrText] = "careers"
	desired := campaignNegativeKeywordResource(t, "jobs", attrs)
	_, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "21~61"},
	})
	if err == nil {
		t.Fatal("expected immutable text error")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("error = %q, want immutable guidance", err)
	}
}

func TestUpdateCampaignNegativeKeywordRejectsImmutableMatchType(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleCampaignNegativeKeyword("21", "61", "jobs", "PHRASE"))
	p, _ := testTargetingProvider(t, fake)
	attrs := resolvedCampaignNegativeKeywordAttrs(t, "21")
	attrs[googleads.AttrMatchType] = "EXACT"
	desired := campaignNegativeKeywordResource(t, "jobs", attrs)
	_, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "21~61"},
	})
	if err == nil {
		t.Fatal("expected immutable match type error")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("error = %q, want immutable guidance", err)
	}
}

func TestImportCampaignNegativeKeywordRequiresBoundCampaign(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleCampaignNegativeKeyword("21", "61", "jobs", "PHRASE"))
	p, _ := testTargetingProvider(t, fake)
	_, err := p.Import(context.Background(), mustCampaignNegativeKeywordAddress(t, "jobs"), "21~61")
	if err == nil {
		t.Fatal("expected missing campaign binding error")
	}
	if !strings.Contains(err.Error(), "campaign") {
		t.Fatalf("error = %q, want campaign import guidance", err)
	}
}

func TestImportCampaignNegativeKeywordThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleCampaignNegativeKeyword("21", "61", "Jobs", "PHRASE"))
	p, _ := testTargetingProvider(t, fake)

	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignBudgetAddress(t, "brand"), resource.Identity{ID: "11"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	live, err := p.Import(context.Background(), mustCampaignNegativeKeywordAddress(t, "jobs"), "21~61")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if _, ok := live.Attributes[googleads.AttrNegative]; ok {
		t.Fatal("import must omit implied negative from configurable fields")
	}
	if err := st.Bind(mustCampaignNegativeKeywordAddress(t, "jobs"), live.Identity); err != nil {
		t.Fatal(err)
	}
	got, err := plan.BuildWithState(context.Background(), campaignNegativeKeywordStack(t, live.Attributes), func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("imported campaign negative keyword produced changes: %+v", got.Changes)
	}
	if fake.lastMutate != "" {
		t.Fatalf("import mutated remote: %s", fake.lastMutate)
	}
}

func TestNormalizeCampaignNegativeKeywordImportID(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, nil)
	addr := mustCampaignNegativeKeywordAddress(t, "jobs")
	got, err := p.NormalizeImportID(addr, "customers/"+testCustomerID+"/campaignCriteria/21~61")
	if err != nil {
		t.Fatalf("NormalizeImportID: %v", err)
	}
	if got != "21~61" {
		t.Fatalf("id = %q, want 21~61", got)
	}
}

func TestImportCampaignNegativeKeywordRejectsLocationCriterion(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleLocationCriterion("21", "41", "geoTargetConstants/2840", false))
	p, _ := testTargetingProvider(t, fake)
	_, err := p.Import(context.Background(), mustCampaignNegativeKeywordAddress(t, "united_states"), "21~41")
	if err == nil {
		t.Fatal("expected unsupported type error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported type must not look like not found")
	}
	if !strings.Contains(err.Error(), "KEYWORD") {
		t.Fatalf("error = %q, want KEYWORD guidance", err)
	}
}

func TestPlanCampaignNegativeKeywordCreateWhenMissing(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, newTargetingFake())
	got := mustPlanTargeting(t, p, campaignNegativeKeywordStack(t, defaultCampaignNegativeKeywordAttrs(t))...)
	byAddr := map[string]plan.Action{}
	for _, change := range got.Changes {
		byAddr[change.Address.String()] = change.Action
	}
	if byAddr["googleads.campaign_negative_keyword.jobs"] != plan.ActionCreate {
		t.Fatalf("campaign negative keyword action = %v, want create", byAddr["googleads.campaign_negative_keyword.jobs"])
	}
}

func TestPlanCampaignNegativeKeywordUnchangedEquivalentRemote(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleCampaignNegativeKeyword("21", "61", "Jobs", "phrase"))
	p, _ := testTargetingProvider(t, fake)

	attrs := defaultCampaignNegativeKeywordAttrs(t)
	attrs[googleads.AttrText] = "JOBS"
	got := mustPlanTargeting(t, p, campaignNegativeKeywordStack(t, attrs)...)
	if got.HasChanges() {
		t.Fatalf("equivalent campaign negative keyword produced changes: %+v", got.Changes)
	}
}

func TestPlanCampaignNegativeKeywordImmutableIsVisible(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleCampaignNegativeKeyword("21", "61", "jobs", "PHRASE"))
	p, _ := testTargetingProvider(t, fake)
	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignBudgetAddress(t, "brand"), resource.Identity{ID: "11"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustCampaignNegativeKeywordAddress(t, "jobs"), resource.Identity{ID: "21~61"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	attrs := defaultCampaignNegativeKeywordAttrs(t)
	attrs[googleads.AttrText] = "careers"
	_, err := plan.BuildWithState(context.Background(), campaignNegativeKeywordStack(t, attrs), func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err == nil {
		t.Fatal("expected immutable text plan error")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("error = %q, want immutable guidance", err)
	}
}

func campaignNegativeKeywordResource(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{Address: mustCampaignNegativeKeywordAddress(t, name), Attributes: attrs}
}

func mustCampaignNegativeKeywordAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("googleads.campaign_negative_keyword." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func defaultCampaignNegativeKeywordAttrs(t *testing.T) resource.Attributes {
	t.Helper()
	return resource.Attributes{
		googleads.AttrCampaign:  campaignRef(t, "brand"),
		googleads.AttrText:      "jobs",
		googleads.AttrMatchType: "PHRASE",
	}
}

func resolvedCampaignNegativeKeywordAttrs(t *testing.T, campaignID string) resource.Attributes {
	t.Helper()
	return resource.Attributes{
		googleads.AttrCampaign: resource.Resolved{
			Address:  mustCampaignAddress(t, "brand"),
			Identity: resource.Identity{ID: campaignID},
			Outputs:  resource.Attributes{"resourceName": "customers/" + testCustomerID + "/campaigns/" + campaignID},
		},
		googleads.AttrText:      "jobs",
		googleads.AttrMatchType: "PHRASE",
	}
}

func campaignNegativeKeywordStack(t *testing.T, attrs resource.Attributes) []resource.Resource {
	t.Helper()
	return []resource.Resource{
		campaignBudgetResource(t, "brand", resource.Attributes{
			googleads.AttrName:             "Brand daily budget",
			googleads.AttrAmount:           50,
			googleads.AttrExplicitlyShared: false,
		}),
		campaignResource(t, "brand", defaultCampaignAttrs(t)),
		campaignNegativeKeywordResource(t, "jobs", attrs),
	}
}

func sampleCampaignNegativeKeyword(campaignID, criterionID, text, matchType string) map[string]any {
	return map[string]any{
		"criterionId": criterionID,
		"campaign":    "customers/" + testCustomerID + "/campaigns/" + campaignID,
		"type":        "KEYWORD",
		"status":      "ENABLED",
		"negative":    true,
		"keyword":     map[string]any{"text": text, "matchType": matchType},
	}
}
