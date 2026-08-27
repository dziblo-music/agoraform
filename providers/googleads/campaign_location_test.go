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

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestValidateCampaignLocationValid(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, nil)
	res := campaignLocationResource(t, "united_states", defaultCampaignLocationAttrs(t))
	if err := p.Validate(context.Background(), res); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateCampaignLocationErrors(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, nil)
	addr := mustCampaignLocationAddress(t, "united_states")
	campaign := campaignRef(t, "brand")

	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name:  "missing location",
			attrs: resource.Attributes{googleads.AttrCampaign: campaign},
			want:  "missing required attribute \"location\"",
		},
		{
			name:  "missing campaign",
			attrs: resource.Attributes{googleads.AttrLocation: "United States"},
			want:  "missing required attribute \"campaign\"",
		},
		{
			name: "campaign not a ref",
			attrs: resource.Attributes{
				googleads.AttrCampaign: "customers/" + testCustomerID + "/campaigns/21",
				googleads.AttrLocation: "United States",
			},
			want: "$ref",
		},
		{
			name: "campaign wrong type",
			attrs: resource.Attributes{
				googleads.AttrCampaign: resource.Ref{Address: mustAdGroupAddress(t, "brand")},
				googleads.AttrLocation: "United States",
			},
			want: "googleads.campaign",
		},
		{
			name: "empty location",
			attrs: resource.Attributes{
				googleads.AttrCampaign: campaign,
				googleads.AttrLocation: "   ",
			},
			want: "non-empty",
		},
		{
			name: "negative accepted",
			attrs: resource.Attributes{
				googleads.AttrCampaign: campaign,
				googleads.AttrLocation: "Canada",
				googleads.AttrNegative: true,
			},
			want: "",
		},
		{
			name: "computed id",
			attrs: resource.Attributes{
				googleads.AttrCampaign: campaign,
				googleads.AttrLocation: "United States",
				"id":                   "21~41",
			},
			want: "computed",
		},
		{
			name: "unsupported radius",
			attrs: resource.Attributes{
				googleads.AttrCampaign: campaign,
				googleads.AttrLocation: "United States",
				"radius":               "10mi",
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

func TestReadCampaignLocationSuccess(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleLocationCriterion("21", "41", "geoTargetConstants/2840", false))
	p, _ := testTargetingProvider(t, fake)
	bindCampaignIdentity(t, p, "21")

	res := campaignLocationResource(t, "united_states", defaultCampaignLocationAttrs(t))
	res.Identity = resource.Identity{ID: "21~41"}
	live, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "21~41" {
		t.Fatalf("identity = %q, want 21~41", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrLocation] != "United States" && live.Attributes[googleads.AttrLocation] != "US" {
		t.Fatalf("location = %v, want United States", live.Attributes[googleads.AttrLocation])
	}
	if live.Attributes[googleads.AttrNegative] != false {
		t.Fatalf("negative = %v, want false", live.Attributes[googleads.AttrNegative])
	}
	ref, ok := resource.AsRef(live.Attributes[googleads.AttrCampaign])
	if !ok || ref.Address != mustCampaignAddress(t, "brand") {
		t.Fatalf("campaign = %#v, want logical $ref", live.Attributes[googleads.AttrCampaign])
	}
	if live.Computed["geoTargetConstant"] != "geoTargetConstants/2840" {
		t.Fatalf("computed geo target = %v", live.Computed["geoTargetConstant"])
	}
	if _, ok := live.Attributes["geoTargetConstant"]; ok {
		t.Fatal("geoTargetConstant must not appear in comparable attributes")
	}
}

func TestReadCampaignLocationByName(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleLocationCriterion("21", "41", "geoTargetConstants/2840", false))
	p, _ := testTargetingProvider(t, fake)

	live, err := p.Read(context.Background(), campaignLocationResource(t, "united_states", resolvedCampaignLocationAttrs(t, "21")))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "21~41" {
		t.Fatalf("identity = %q", live.Identity.ID)
	}
}

func TestReadCampaignLocationCountryCodeAlias(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleLocationCriterion("21", "41", "geoTargetConstants/2840", false))
	p, _ := testTargetingProvider(t, fake)

	attrs := resolvedCampaignLocationAttrs(t, "21")
	attrs[googleads.AttrLocation] = "US"
	live, err := p.Read(context.Background(), campaignLocationResource(t, "united_states", attrs))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "21~41" {
		t.Fatalf("identity = %q", live.Identity.ID)
	}
}

func TestReadCampaignLocationNotFound(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, newTargetingFake())
	_, err := p.Read(context.Background(), campaignLocationResource(t, "united_states", resolvedCampaignLocationAttrs(t, "21")))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
}

func TestReadCampaignLocationAmbiguous(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, newTargetingFake())
	attrs := resolvedCampaignLocationAttrs(t, "21")
	attrs[googleads.AttrLocation] = "Springfield"
	_, err := p.Read(context.Background(), campaignLocationResource(t, "springfield", attrs))
	if err == nil {
		t.Fatal("expected ambiguous location error")
	}
	if !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "Springfield") {
		t.Fatalf("error = %q, want ambiguous Springfield guidance", err)
	}
}

func TestReadCampaignLocationUnknown(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, newTargetingFake())
	attrs := resolvedCampaignLocationAttrs(t, "21")
	attrs[googleads.AttrLocation] = "Atlantis"
	_, err := p.Read(context.Background(), campaignLocationResource(t, "atlantis", attrs))
	if err == nil {
		t.Fatal("expected missing location error")
	}
	if !strings.Contains(err.Error(), "Atlantis") {
		t.Fatalf("error = %q, want unknown location guidance", err)
	}
}

func TestReadCampaignLocationRejectsNonLocation(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	item := sampleLocationCriterion("21", "41", "geoTargetConstants/2840", false)
	item["type"] = "LANGUAGE"
	delete(item, "location")
	item["language"] = map[string]any{"languageConstant": "languageConstants/1000"}
	fake.seedCriterion(item)
	p, _ := testTargetingProvider(t, fake)
	bindCampaignIdentity(t, p, "21")

	res := campaignLocationResource(t, "united_states", defaultCampaignLocationAttrs(t))
	res.Identity = resource.Identity{ID: "21~41"}
	_, err := p.Read(context.Background(), res)
	if err == nil {
		t.Fatal("expected type error")
	}
	if !strings.Contains(err.Error(), "LANGUAGE") {
		t.Fatalf("error = %q, want LANGUAGE guidance", err)
	}
}

func TestReadCampaignLocationAPIError(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.searchStatus = http.StatusForbidden
	p, _ := testTargetingProvider(t, fake)
	res := campaignLocationResource(t, "united_states", defaultCampaignLocationAttrs(t))
	res.Identity = resource.Identity{ID: "21~41"}
	_, err := p.Read(context.Background(), res)
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestCreateCampaignLocation(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	p, _ := testTargetingProvider(t, fake)
	live, err := p.Create(context.Background(), campaignLocationResource(t, "united_states", resolvedCampaignLocationAttrs(t, "21")))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID == "" {
		t.Fatal("create returned empty identity")
	}
	if !strings.Contains(fake.lastMutate, "geoTargetConstants/2840") {
		t.Fatalf("create mutate missing geo target: %s", fake.lastMutate)
	}
	if !strings.Contains(fake.lastMutate, "campaigns/21") {
		t.Fatalf("create mutate missing campaign: %s", fake.lastMutate)
	}
}

func TestCreateCampaignLocationNegative(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	p, _ := testTargetingProvider(t, fake)
	attrs := resolvedCampaignLocationAttrs(t, "21")
	attrs[googleads.AttrLocation] = "Canada"
	attrs[googleads.AttrNegative] = true
	if _, err := p.Create(context.Background(), campaignLocationResource(t, "exclude_canada", attrs)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.Contains(fake.lastMutate, `"negative":true`) && !strings.Contains(fake.lastMutate, `"negative": true`) {
		t.Fatalf("create mutate missing negative: %s", fake.lastMutate)
	}
	if !strings.Contains(fake.lastMutate, "geoTargetConstants/2124") {
		t.Fatalf("create mutate missing Canada geo target: %s", fake.lastMutate)
	}
}

func TestCreateCampaignLocationMissingCampaignIdentity(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, newTargetingFake())
	_, err := p.Create(context.Background(), campaignLocationResource(t, "united_states", defaultCampaignLocationAttrs(t)))
	if err == nil {
		t.Fatal("expected missing campaign identity")
	}
	if !strings.Contains(err.Error(), "campaign") {
		t.Fatalf("error = %q, want campaign identity guidance", err)
	}
}

func TestCreateCampaignLocationAPIError(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.mutateStatus = http.StatusBadRequest
	p, _ := testTargetingProvider(t, fake)
	_, err := p.Create(context.Background(), campaignLocationResource(t, "united_states", resolvedCampaignLocationAttrs(t, "21")))
	if err == nil {
		t.Fatal("expected API error")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestUpdateCampaignLocationNoOp(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleLocationCriterion("21", "41", "geoTargetConstants/2840", false))
	p, _ := testTargetingProvider(t, fake)
	desired := campaignLocationResource(t, "united_states", resolvedCampaignLocationAttrs(t, "21"))
	if _, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "21~41"},
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if fake.lastMutate != "" {
		t.Fatalf("equivalent update mutated remote: %s", fake.lastMutate)
	}
}

func TestUpdateCampaignLocationRejectsImmutableLocation(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleLocationCriterion("21", "41", "geoTargetConstants/2840", false))
	p, _ := testTargetingProvider(t, fake)
	attrs := resolvedCampaignLocationAttrs(t, "21")
	attrs[googleads.AttrLocation] = "Canada"
	desired := campaignLocationResource(t, "united_states", attrs)
	_, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "21~41"},
	})
	if err == nil {
		t.Fatal("expected immutable location error")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("error = %q, want immutable guidance", err)
	}
}

func TestImportCampaignLocationRequiresBoundCampaign(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleLocationCriterion("21", "41", "geoTargetConstants/2840", false))
	p, _ := testTargetingProvider(t, fake)
	_, err := p.Import(context.Background(), mustCampaignLocationAddress(t, "united_states"), "21~41")
	if err == nil {
		t.Fatal("expected missing campaign binding error")
	}
	if !strings.Contains(err.Error(), "campaign") {
		t.Fatalf("error = %q, want campaign import guidance", err)
	}
}

func TestImportCampaignLocationThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleLocationCriterion("21", "41", "geoTargetConstants/2840", false))
	p, _ := testTargetingProvider(t, fake)

	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignBudgetAddress(t, "brand"), resource.Identity{ID: "11"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	live, err := p.Import(context.Background(), mustCampaignLocationAddress(t, "united_states"), "21~41")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if err := st.Bind(mustCampaignLocationAddress(t, "united_states"), live.Identity); err != nil {
		t.Fatal(err)
	}
	got, err := plan.BuildWithState(context.Background(), campaignLocationStack(t, live.Attributes), func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("imported location produced changes: %+v", got.Changes)
	}
}

func TestNormalizeCampaignLocationImportID(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, nil)
	addr := mustCampaignLocationAddress(t, "united_states")
	got, err := p.NormalizeImportID(addr, "customers/"+testCustomerID+"/campaignCriteria/21~41")
	if err != nil {
		t.Fatalf("NormalizeImportID: %v", err)
	}
	if got != "21~41" {
		t.Fatalf("id = %q, want 21~41", got)
	}
}

func TestPlanCampaignLocationCreateWhenMissing(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, newTargetingFake())
	got := mustPlanTargeting(t, p, campaignLocationStack(t, defaultCampaignLocationAttrs(t))...)
	byAddr := map[string]plan.Action{}
	for _, change := range got.Changes {
		byAddr[change.Address.String()] = change.Action
	}
	if byAddr["googleads.campaign_location.united_states"] != plan.ActionCreate {
		t.Fatalf("location action = %v, want create", byAddr["googleads.campaign_location.united_states"])
	}
}

func TestPlanCampaignLocationUnchangedEquivalentRemote(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleLocationCriterion("21", "41", "geoTargetConstants/2840", false))
	p, _ := testTargetingProvider(t, fake)

	attrs := defaultCampaignLocationAttrs(t)
	attrs[googleads.AttrLocation] = "US"
	got := mustPlanTargeting(t, p, campaignLocationStack(t, attrs)...)
	if got.HasChanges() {
		t.Fatalf("equivalent location produced changes: %+v", got.Changes)
	}
}

func TestPlanCampaignLocationImmutableIsVisible(t *testing.T) {
	t.Parallel()

	fake := newTargetingFake()
	fake.seedCriterion(sampleLocationCriterion("21", "41", "geoTargetConstants/2840", false))
	p, _ := testTargetingProvider(t, fake)
	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignBudgetAddress(t, "brand"), resource.Identity{ID: "11"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustCampaignLocationAddress(t, "united_states"), resource.Identity{ID: "21~41"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	attrs := defaultCampaignLocationAttrs(t)
	attrs[googleads.AttrLocation] = "Canada"
	_, err := plan.BuildWithState(context.Background(), campaignLocationStack(t, attrs), func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err == nil {
		t.Fatal("expected immutable location plan error")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("error = %q, want immutable guidance", err)
	}
}

func campaignLocationResource(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{Address: mustCampaignLocationAddress(t, name), Attributes: attrs}
}

func mustCampaignLocationAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("googleads.campaign_location." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func defaultCampaignLocationAttrs(t *testing.T) resource.Attributes {
	t.Helper()
	return resource.Attributes{
		googleads.AttrCampaign: campaignRef(t, "brand"),
		googleads.AttrLocation: "United States",
	}
}

func resolvedCampaignLocationAttrs(t *testing.T, campaignID string) resource.Attributes {
	t.Helper()
	return resource.Attributes{
		googleads.AttrCampaign: resource.Resolved{
			Address:  mustCampaignAddress(t, "brand"),
			Identity: resource.Identity{ID: campaignID},
			Outputs:  resource.Attributes{"resourceName": "customers/" + testCustomerID + "/campaigns/" + campaignID},
		},
		googleads.AttrLocation: "United States",
	}
}

func campaignLocationStack(t *testing.T, locationAttrs resource.Attributes) []resource.Resource {
	t.Helper()
	return []resource.Resource{
		campaignBudgetResource(t, "brand", resource.Attributes{
			googleads.AttrName:             "Brand daily budget",
			googleads.AttrAmount:           50,
			googleads.AttrExplicitlyShared: false,
		}),
		campaignResource(t, "brand", defaultCampaignAttrs(t)),
		campaignLocationResource(t, "united_states", locationAttrs),
	}
}

func mustPlanTargeting(t *testing.T, p *googleads.Provider, resources ...resource.Resource) *plan.Plan {
	t.Helper()
	got, err := plan.Build(context.Background(), resources, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	return got
}

func sampleLocationCriterion(campaignID, criterionID, geoTarget string, negative bool) map[string]any {
	return map[string]any{
		"criterionId": criterionID,
		"campaign":    "customers/" + testCustomerID + "/campaigns/" + campaignID,
		"type":        "LOCATION",
		"status":      "ENABLED",
		"negative":    negative,
		"location":    map[string]any{"geoTargetConstant": geoTarget},
	}
}

func sampleLanguageCriterion(campaignID, criterionID, languageConstant string) map[string]any {
	return map[string]any{
		"criterionId": criterionID,
		"campaign":    "customers/" + testCustomerID + "/campaigns/" + campaignID,
		"type":        "LANGUAGE",
		"status":      "ENABLED",
		"language":    map[string]any{"languageConstant": languageConstant},
	}
}

func testTargetingProvider(t *testing.T, fake *targetingFake) (*googleads.Provider, *httptest.Server) {
	t.Helper()
	if fake == nil {
		fake = newTargetingFake()
	}
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	t.Cleanup(srv.Close)
	cfg := validConfig(srv.URL)
	cfg.TokenURL = srv.URL + "/oauth/token"
	p := googleads.NewWithHTTPClient(cfg, srv.Client())
	return p, srv
}

type targetingFake struct {
	mu sync.Mutex

	nextID       int64
	criteria     map[string]map[string]any
	campaigns    map[string]map[string]any
	budgets      map[string]map[string]any
	geos         []map[string]any
	languages    []map[string]any
	searchStatus int
	mutateStatus int
	lastQuery    string
	lastMutate   string
}

func newTargetingFake() *targetingFake {
	f := &targetingFake{
		criteria:  map[string]map[string]any{},
		campaigns: map[string]map[string]any{},
		budgets:   map[string]map[string]any{},
		geos: []map[string]any{
			{"id": "2840", "name": "United States", "canonicalName": "United States", "countryCode": "US", "targetType": "Country", "status": "ENABLED"},
			{"id": "2124", "name": "Canada", "canonicalName": "Canada", "countryCode": "CA", "targetType": "Country", "status": "ENABLED"},
			{"id": "21137", "name": "California", "canonicalName": "California, United States", "countryCode": "US", "targetType": "State", "status": "ENABLED"},
			{"id": "1021001", "name": "Springfield", "canonicalName": "Springfield, Illinois, United States", "countryCode": "US", "targetType": "City", "status": "ENABLED"},
			{"id": "1021002", "name": "Springfield", "canonicalName": "Springfield, Massachusetts, United States", "countryCode": "US", "targetType": "City", "status": "ENABLED"},
		},
		languages: []map[string]any{
			{"id": "1000", "code": "en", "name": "English", "targetable": true},
			{"id": "1003", "code": "es", "name": "Spanish", "targetable": true},
		},
	}
	for i := range f.geos {
		id := stringify(f.geos[i]["id"])
		f.geos[i]["resourceName"] = "geoTargetConstants/" + id
	}
	for i := range f.languages {
		id := stringify(f.languages[i]["id"])
		f.languages[i]["resourceName"] = "languageConstants/" + id
	}
	f.seedCampaignLocked(sampleSearchCampaign("21", "Brand", "11"))
	f.seedBudgetLocked(map[string]any{
		"id":               "11",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
	})
	return f
}

func (f *targetingFake) seedCriterion(item map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storeCriterionLocked(cloneMap(item))
}

func (f *targetingFake) seedCampaignLocked(campaign map[string]any) {
	cloned := cloneMap(campaign)
	id := stringify(cloned["id"])
	if stringify(cloned["resourceName"]) == "" {
		cloned["resourceName"] = "customers/" + testCustomerID + "/campaigns/" + id
	}
	f.campaigns[id] = cloned
}

func (f *targetingFake) seedBudgetLocked(budget map[string]any) {
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

func (f *targetingFake) storeCriterionLocked(item map[string]any) {
	campaign := stringify(item["campaign"])
	campaignID := strings.TrimPrefix(campaign, "customers/"+testCustomerID+"/campaigns/")
	criterionID := stringify(item["criterionId"])
	id := campaignID + "~" + criterionID
	if stringify(item["resourceName"]) == "" {
		item["resourceName"] = "customers/" + testCustomerID + "/campaignCriteria/" + id
	}
	f.criteria[id] = item
}

func (f *targetingFake) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, _ := io.ReadAll(r.Body)

	if strings.HasSuffix(r.URL.Path, "/oauth/token") {
		writeToken(w)
		return
	}
	if strings.Contains(r.URL.Path, "geoTargetConstants:suggest") {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"geoTargetConstantSuggestions": f.suggestLocked(body)})
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
		query := strings.ToLower(req.Query)
		switch {
		case strings.Contains(query, "from campaign_criterion"):
			_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchCriteriaLocked(req.Query)})
		case strings.Contains(query, "from geo_target_constant"):
			_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchGeosLocked(req.Query)})
		case strings.Contains(query, "from language_constant"):
			_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchLanguagesLocked(req.Query)})
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
		if f.mutateStatus >= 400 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(f.mutateStatus)
			_, _ = io.WriteString(w, `{"error":{"code":400,"message":"mutate failed `+testDeveloperToken+`","status":"INVALID_ARGUMENT"}}`)
			return
		}
		resourceName, err := f.mutateLocked(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"resourceName": resourceName}}})
		return
	}
	http.NotFound(w, r)
}

func (f *targetingFake) suggestLocked(body []byte) []any {
	var req struct {
		LocationNames struct {
			Names []string `json:"names"`
		} `json:"locationNames"`
	}
	_ = json.Unmarshal(body, &req)
	var out []any
	for _, name := range req.LocationNames.Names {
		for _, geo := range f.geos {
			if strings.EqualFold(stringify(geo["name"]), name) || strings.EqualFold(stringify(geo["canonicalName"]), name) {
				out = append(out, map[string]any{"geoTargetConstant": cloneMap(geo), "searchTerm": name})
			}
		}
	}
	return out
}

func (f *targetingFake) searchGeosLocked(query string) []any {
	var out []any
	for _, geo := range f.geos {
		if matchesGeoQuery(query, geo) {
			out = append(out, map[string]any{"geoTargetConstant": cloneMap(geo)})
		}
	}
	return out
}

func (f *targetingFake) searchLanguagesLocked(query string) []any {
	var out []any
	for _, lang := range f.languages {
		if matchesLanguageQuery(query, lang) {
			out = append(out, map[string]any{"languageConstant": cloneMap(lang)})
		}
	}
	return out
}

func (f *targetingFake) searchCriteriaLocked(query string) []any {
	var out []any
	for _, item := range f.criteria {
		if matchesCampaignCriterionQuery(query, item) {
			out = append(out, map[string]any{"campaignCriterion": cloneMap(item)})
		}
	}
	return out
}

func (f *targetingFake) searchCampaignsLocked(query string) []any {
	var out []any
	for _, campaign := range f.campaigns {
		if matchesCampaignQuery(query, campaign) {
			out = append(out, map[string]any{"campaign": cloneMap(campaign)})
		}
	}
	return out
}

func (f *targetingFake) searchBudgetsLocked(query string) []any {
	var out []any
	for _, budget := range f.budgets {
		if matchesCampaignBudgetQuery(query, budget) {
			out = append(out, map[string]any{"campaignBudget": cloneMap(budget)})
		}
	}
	return out
}

func (f *targetingFake) mutateLocked(body []byte) (string, error) {
	var req struct {
		Operations []map[string]any `json:"operations"`
	}
	if err := json.Unmarshal(body, &req); err != nil || len(req.Operations) == 0 {
		return "", errors.New("malformed mutate")
	}
	op := req.Operations[0]
	raw, ok := op["create"]
	if !ok {
		return "", errors.New("unsupported mutate")
	}
	item, _ := raw.(map[string]any)
	created := cloneMap(item)
	f.nextID++
	criterionID := strconv.FormatInt(f.nextID, 10)
	created["criterionId"] = criterionID
	if stringify(created["status"]) == "" {
		created["status"] = "ENABLED"
	}
	if stringify(created["type"]) == "" {
		if _, ok := created["location"]; ok {
			created["type"] = "LOCATION"
		}
		if _, ok := created["language"]; ok {
			created["type"] = "LANGUAGE"
		}
	}
	f.storeCriterionLocked(created)
	return stringify(created["resourceName"]), nil
}

func matchesGeoQuery(query string, geo map[string]any) bool {
	resourceName := stringify(geo["resourceName"])
	country := stringify(geo["countryCode"])
	targetType := stringify(geo["targetType"])
	if strings.Contains(query, "geo_target_constant.resource_name = ") {
		want := gaqlQuoted(query, "geo_target_constant.resource_name = ")
		return strings.EqualFold(want, resourceName)
	}
	if strings.Contains(query, "geo_target_constant.country_code = ") {
		want := gaqlQuoted(query, "geo_target_constant.country_code = ")
		if !strings.EqualFold(want, country) {
			return false
		}
	}
	if strings.Contains(query, "geo_target_constant.target_type = ") {
		want := gaqlQuoted(query, "geo_target_constant.target_type = ")
		return strings.EqualFold(want, targetType)
	}
	return true
}

func matchesLanguageQuery(query string, lang map[string]any) bool {
	resourceName := stringify(lang["resourceName"])
	code := stringify(lang["code"])
	if strings.Contains(query, "language_constant.resource_name = ") {
		want := gaqlQuoted(query, "language_constant.resource_name = ")
		return strings.EqualFold(want, resourceName)
	}
	if strings.Contains(query, "language_constant.code = ") {
		want := gaqlQuoted(query, "language_constant.code = ")
		return strings.EqualFold(want, code)
	}
	return true
}

func matchesCampaignCriterionQuery(query string, item map[string]any) bool {
	campaign := stringify(item["campaign"])
	campaignID := strings.TrimPrefix(campaign, "customers/"+testCustomerID+"/campaigns/")
	criterionID := stringify(item["criterionId"])
	if stringify(item["status"]) == "REMOVED" {
		return false
	}
	if strings.Contains(query, "campaign_criterion.criterion_id = ") {
		want := strings.TrimSpace(query[strings.Index(query, "campaign_criterion.criterion_id = ")+len("campaign_criterion.criterion_id = "):])
		if i := strings.IndexAny(want, " \n"); i >= 0 {
			want = want[:i]
		}
		if want != criterionID {
			return false
		}
	}
	if strings.Contains(query, "campaign.id = ") {
		want := strings.TrimSpace(query[strings.Index(query, "campaign.id = ")+len("campaign.id = "):])
		if i := strings.IndexAny(want, " \n"); i >= 0 {
			want = want[:i]
		}
		if want != campaignID {
			return false
		}
	}
	if strings.Contains(query, "campaign_criterion.type = ") {
		want := gaqlQuoted(query, "campaign_criterion.type = ")
		if !strings.EqualFold(want, stringify(item["type"])) {
			return false
		}
	}
	if strings.Contains(query, "campaign_criterion.location.geo_target_constant = ") {
		want := gaqlQuoted(query, "campaign_criterion.location.geo_target_constant = ")
		info, _ := item["location"].(map[string]any)
		if !strings.EqualFold(want, stringify(info["geoTargetConstant"])) {
			return false
		}
	}
	if strings.Contains(query, "campaign_criterion.language.language_constant = ") {
		want := gaqlQuoted(query, "campaign_criterion.language.language_constant = ")
		info, _ := item["language"].(map[string]any)
		if !strings.EqualFold(want, stringify(info["languageConstant"])) {
			return false
		}
	}
	if strings.Contains(query, "campaign_criterion.negative = ") {
		rest := strings.TrimSpace(query[strings.Index(query, "campaign_criterion.negative = ")+len("campaign_criterion.negative = "):])
		if i := strings.IndexAny(rest, " \n"); i >= 0 {
			rest = rest[:i]
		}
		wantNeg := strings.EqualFold(rest, "TRUE")
		gotNeg, _ := item["negative"].(bool)
		if wantNeg != gotNeg {
			return false
		}
	}
	return true
}

func gaqlQuoted(query, prefix string) string {
	start := strings.Index(query, prefix)
	if start < 0 {
		return ""
	}
	rest := strings.TrimSpace(query[start+len(prefix):])
	if i := strings.Index(rest, " AND "); i >= 0 {
		rest = rest[:i]
	}
	if i := strings.IndexAny(rest, "\n"); i >= 0 {
		rest = rest[:i]
	}
	return strings.Trim(strings.TrimSpace(rest), "'")
}
