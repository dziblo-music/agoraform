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

func TestValidateCampaignLanguageValid(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, nil)
	if err := p.Validate(context.Background(), campaignLanguageResource(t, "english", defaultCampaignLanguageAttrs(t))); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateCampaignLanguageErrors(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, nil)
	addr := mustCampaignLanguageAddress(t, "english")
	campaign := campaignRef(t, "brand")

	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name:  "missing language",
			attrs: resource.Attributes{googleads.AttrCampaign: campaign},
			want:  "missing required attribute \"language\"",
		},
		{
			name:  "missing campaign",
			attrs: resource.Attributes{googleads.AttrLanguage: "en"},
			want:  "missing required attribute \"campaign\"",
		},
		{
			name: "negative rejected",
			attrs: resource.Attributes{
				googleads.AttrCampaign: campaign,
				googleads.AttrLanguage: "en",
				googleads.AttrNegative: true,
			},
			want: "cannot be negative",
		},
		{
			name: "empty language",
			attrs: resource.Attributes{
				googleads.AttrCampaign: campaign,
				googleads.AttrLanguage: "  ",
			},
			want: "non-empty",
		},
		{
			name: "computed id",
			attrs: resource.Attributes{
				googleads.AttrCampaign: campaign,
				googleads.AttrLanguage: "en",
				"id":                   "21~51",
			},
			want: "computed",
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
		})
	}
}

func TestReadCampaignLanguageSuccess(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleLanguageCriterion("21", "51", "languageConstants/1000"))
	p, _ := testTargetingProvider(t, fake)
	bindCampaignIdentity(t, p, "21")

	res := campaignLanguageResource(t, "english", defaultCampaignLanguageAttrs(t))
	res.Identity = resource.Identity{ID: "21~51"}
	live, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "21~51" {
		t.Fatalf("identity = %q", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrLanguage] != "en" {
		t.Fatalf("language = %v, want en", live.Attributes[googleads.AttrLanguage])
	}
	if live.Computed["languageConstant"] != "languageConstants/1000" {
		t.Fatalf("computed language = %v", live.Computed["languageConstant"])
	}
}

func TestReadCampaignLanguageByName(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleLanguageCriterion("21", "51", "languageConstants/1000"))
	p, _ := testTargetingProvider(t, fake)

	attrs := resolvedCampaignLanguageAttrs(t, "21")
	attrs[googleads.AttrLanguage] = "English"
	live, err := p.Read(context.Background(), campaignLanguageResource(t, "english", attrs))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "21~51" {
		t.Fatalf("identity = %q", live.Identity.ID)
	}
}

func TestReadCampaignLanguageUnknown(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, newTargetingFake())
	attrs := resolvedCampaignLanguageAttrs(t, "21")
	attrs[googleads.AttrLanguage] = "Klingon"
	_, err := p.Read(context.Background(), campaignLanguageResource(t, "klingon", attrs))
	if err == nil {
		t.Fatal("expected unknown language error")
	}
	if !strings.Contains(err.Error(), "Klingon") {
		t.Fatalf("error = %q, want unknown language guidance", err)
	}
}

func TestReadCampaignLanguageRejectsNonLanguage(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleLocationCriterion("21", "51", "geoTargetConstants/2840", false))
	p, _ := testTargetingProvider(t, fake)
	bindCampaignIdentity(t, p, "21")
	res := campaignLanguageResource(t, "english", defaultCampaignLanguageAttrs(t))
	res.Identity = resource.Identity{ID: "21~51"}
	_, err := p.Read(context.Background(), res)
	if err == nil {
		t.Fatal("expected type error")
	}
	if !strings.Contains(err.Error(), "LOCATION") {
		t.Fatalf("error = %q, want LOCATION guidance", err)
	}
}

func TestReadCampaignLanguageAPIError(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.searchStatus = http.StatusForbidden
	p, _ := testTargetingProvider(t, fake)
	res := campaignLanguageResource(t, "english", defaultCampaignLanguageAttrs(t))
	res.Identity = resource.Identity{ID: "21~51"}
	_, err := p.Read(context.Background(), res)
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestCreateCampaignLanguage(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	p, _ := testTargetingProvider(t, fake)
	live, err := p.Create(context.Background(), campaignLanguageResource(t, "english", resolvedCampaignLanguageAttrs(t, "21")))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID == "" {
		t.Fatal("create returned empty identity")
	}
	if !strings.Contains(fake.lastMutate, "languageConstants/1000") {
		t.Fatalf("create mutate missing language constant: %s", fake.lastMutate)
	}
}

func TestCreateCampaignLanguageAPIError(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.mutateStatus = http.StatusBadRequest
	p, _ := testTargetingProvider(t, fake)
	_, err := p.Create(context.Background(), campaignLanguageResource(t, "english", resolvedCampaignLanguageAttrs(t, "21")))
	if err == nil {
		t.Fatal("expected API error")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestUpdateCampaignLanguageNoOp(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleLanguageCriterion("21", "51", "languageConstants/1000"))
	p, _ := testTargetingProvider(t, fake)
	desired := campaignLanguageResource(t, "english", resolvedCampaignLanguageAttrs(t, "21"))
	if _, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "21~51"},
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if fake.lastMutate != "" {
		t.Fatalf("equivalent update mutated remote: %s", fake.lastMutate)
	}
}

func TestUpdateCampaignLanguageRejectsImmutableLanguage(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleLanguageCriterion("21", "51", "languageConstants/1000"))
	p, _ := testTargetingProvider(t, fake)
	attrs := resolvedCampaignLanguageAttrs(t, "21")
	attrs[googleads.AttrLanguage] = "es"
	desired := campaignLanguageResource(t, "english", attrs)
	_, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "21~51"},
	})
	if err == nil {
		t.Fatal("expected immutable language error")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("error = %q, want immutable guidance", err)
	}
}

func TestImportCampaignLanguageRequiresBoundCampaign(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleLanguageCriterion("21", "51", "languageConstants/1000"))
	p, _ := testTargetingProvider(t, fake)
	_, err := p.Import(context.Background(), mustCampaignLanguageAddress(t, "english"), "21~51")
	if err == nil {
		t.Fatal("expected missing campaign binding error")
	}
	if !strings.Contains(err.Error(), "campaign") {
		t.Fatalf("error = %q, want campaign import guidance", err)
	}
}

func TestImportCampaignLanguageThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleLanguageCriterion("21", "51", "languageConstants/1000"))
	p, _ := testTargetingProvider(t, fake)
	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignBudgetAddress(t, "brand"), resource.Identity{ID: "11"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	live, err := p.Import(context.Background(), mustCampaignLanguageAddress(t, "english"), "21~51")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if err := st.Bind(mustCampaignLanguageAddress(t, "english"), live.Identity); err != nil {
		t.Fatal(err)
	}
	got, err := plan.BuildWithState(context.Background(), campaignLanguageStack(t, live.Attributes), func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("imported language produced changes: %+v", got.Changes)
	}
}

func TestPlanCampaignLanguageCreateWhenMissing(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, newTargetingFake())
	got := mustPlanTargeting(t, p, campaignLanguageStack(t, defaultCampaignLanguageAttrs(t))...)
	byAddr := map[string]plan.Action{}
	for _, change := range got.Changes {
		byAddr[change.Address.String()] = change.Action
	}
	if byAddr["googleads.campaign_language.english"] != plan.ActionCreate {
		t.Fatalf("language action = %v, want create", byAddr["googleads.campaign_language.english"])
	}
}

func TestPlanCampaignLanguageUnchangedEquivalentRemote(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleLanguageCriterion("21", "51", "languageConstants/1000"))
	p, _ := testTargetingProvider(t, fake)

	attrs := defaultCampaignLanguageAttrs(t)
	attrs[googleads.AttrLanguage] = "English"
	got := mustPlanTargeting(t, p, campaignLanguageStack(t, attrs)...)
	if got.HasChanges() {
		t.Fatalf("equivalent language produced changes: %+v", got.Changes)
	}
}

func TestPlanCampaignLanguageImmutableIsVisible(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleLanguageCriterion("21", "51", "languageConstants/1000"))
	p, _ := testTargetingProvider(t, fake)
	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignBudgetAddress(t, "brand"), resource.Identity{ID: "11"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustCampaignLanguageAddress(t, "english"), resource.Identity{ID: "21~51"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	attrs := defaultCampaignLanguageAttrs(t)
	attrs[googleads.AttrLanguage] = "es"
	_, err := plan.BuildWithState(context.Background(), campaignLanguageStack(t, attrs), func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err == nil {
		t.Fatal("expected immutable language plan error")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("error = %q, want immutable guidance", err)
	}
}

func campaignLanguageResource(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{Address: mustCampaignLanguageAddress(t, name), Attributes: attrs}
}

func mustCampaignLanguageAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("googleads.campaign_language." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func defaultCampaignLanguageAttrs(t *testing.T) resource.Attributes {
	t.Helper()
	return resource.Attributes{
		googleads.AttrCampaign: campaignRef(t, "brand"),
		googleads.AttrLanguage: "en",
	}
}

func resolvedCampaignLanguageAttrs(t *testing.T, campaignID string) resource.Attributes {
	t.Helper()
	return resource.Attributes{
		googleads.AttrCampaign: resource.Resolved{
			Address:  mustCampaignAddress(t, "brand"),
			Identity: resource.Identity{ID: campaignID},
			Outputs:  resource.Attributes{"resourceName": "customers/" + testCustomerID + "/campaigns/" + campaignID},
		},
		googleads.AttrLanguage: "en",
	}
}

func campaignLanguageStack(t *testing.T, languageAttrs resource.Attributes) []resource.Resource {
	t.Helper()
	return []resource.Resource{
		campaignBudgetResource(t, "brand", resource.Attributes{
			googleads.AttrName:             "Brand daily budget",
			googleads.AttrAmount:           50,
			googleads.AttrExplicitlyShared: false,
		}),
		campaignResource(t, "brand", defaultCampaignAttrs(t)),
		campaignLanguageResource(t, "english", languageAttrs),
	}
}
