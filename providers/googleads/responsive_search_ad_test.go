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

func TestValidateResponsiveSearchAdValid(t *testing.T) {
	t.Parallel()

	p, _ := testRSAProvider(t, nil)
	if err := p.Validate(context.Background(), rsaResource(t, "brand", defaultRSAAttrs(t))); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateResponsiveSearchAdErrors(t *testing.T) {
	t.Parallel()

	p, _ := testRSAProvider(t, nil)
	addr := mustRSAAddress(t, "brand")
	adGroup := adGroupRef(t, "brand")

	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name: "missing ad group",
			attrs: resource.Attributes{
				googleads.AttrFinalUrls:    []any{"https://example.com/"},
				googleads.AttrHeadlines:    defaultRSAHeadlines(),
				googleads.AttrDescriptions: defaultRSADescriptions(),
			},
			want: "missing required attribute \"adGroup\"",
		},
		{
			name: "ad group not a ref",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:      "customers/" + testCustomerID + "/adGroups/31",
				googleads.AttrFinalUrls:    []any{"https://example.com/"},
				googleads.AttrHeadlines:    defaultRSAHeadlines(),
				googleads.AttrDescriptions: defaultRSADescriptions(),
			},
			want: "$ref",
		},
		{
			name: "too few headlines",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:      adGroup,
				googleads.AttrFinalUrls:    []any{"https://example.com/"},
				googleads.AttrHeadlines:    []any{"Buy shoes", "Free shipping"},
				googleads.AttrDescriptions: defaultRSADescriptions(),
			},
			want: "at least 3",
		},
		{
			name: "headline too long",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:      adGroup,
				googleads.AttrFinalUrls:    []any{"https://example.com/"},
				googleads.AttrHeadlines:    []any{"Buy shoes online", "Free shipping today", "This headline is definitely too long"},
				googleads.AttrDescriptions: defaultRSADescriptions(),
			},
			want: "at most 30",
		},
		{
			name: "too few descriptions",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:      adGroup,
				googleads.AttrFinalUrls:    []any{"https://example.com/"},
				googleads.AttrHeadlines:    defaultRSAHeadlines(),
				googleads.AttrDescriptions: []any{"Find shoes that fit your style."},
			},
			want: "at least 2",
		},
		{
			name: "missing final urls",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:      adGroup,
				googleads.AttrHeadlines:    defaultRSAHeadlines(),
				googleads.AttrDescriptions: defaultRSADescriptions(),
			},
			want: "missing required attribute \"finalUrls\"",
		},
		{
			name: "invalid final url",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:      adGroup,
				googleads.AttrFinalUrls:    []any{"example.com"},
				googleads.AttrHeadlines:    defaultRSAHeadlines(),
				googleads.AttrDescriptions: defaultRSADescriptions(),
			},
			want: "http or https",
		},
		{
			name: "path2 without path1",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:      adGroup,
				googleads.AttrFinalUrls:    []any{"https://example.com/"},
				googleads.AttrHeadlines:    defaultRSAHeadlines(),
				googleads.AttrDescriptions: defaultRSADescriptions(),
				googleads.AttrPath2:        "sale",
			},
			want: "path1",
		},
		{
			name: "path with slash",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:      adGroup,
				googleads.AttrFinalUrls:    []any{"https://example.com/"},
				googleads.AttrHeadlines:    defaultRSAHeadlines(),
				googleads.AttrDescriptions: defaultRSADescriptions(),
				googleads.AttrPath1:        "shoes/sale",
			},
			want: "'/'",
		},
		{
			name: "duplicate headline",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:      adGroup,
				googleads.AttrFinalUrls:    []any{"https://example.com/"},
				googleads.AttrHeadlines:    []any{"Buy shoes online", "Free shipping today", "buy shoes online"},
				googleads.AttrDescriptions: defaultRSADescriptions(),
			},
			want: "duplicate",
		},
		{
			name: "duplicate pin",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:   adGroup,
				googleads.AttrFinalUrls: []any{"https://example.com/"},
				googleads.AttrHeadlines: []any{
					map[string]any{"text": "Buy shoes online", "pin": "HEADLINE_1"},
					map[string]any{"text": "Free shipping today", "pin": "headline_1"},
					"Shop the collection",
				},
				googleads.AttrDescriptions: defaultRSADescriptions(),
			},
			want: "at most once",
		},
		{
			name: "invalid pin",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:   adGroup,
				googleads.AttrFinalUrls: []any{"https://example.com/"},
				googleads.AttrHeadlines: []any{
					map[string]any{"text": "Buy shoes online", "pin": "DESCRIPTION_1"},
					"Free shipping today",
					"Shop the collection",
				},
				googleads.AttrDescriptions: defaultRSADescriptions(),
			},
			want: "pin",
		},
		{
			name: "computed id",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:      adGroup,
				googleads.AttrFinalUrls:    []any{"https://example.com/"},
				googleads.AttrHeadlines:    defaultRSAHeadlines(),
				googleads.AttrDescriptions: defaultRSADescriptions(),
				"id":                       "1",
			},
			want: "computed",
		},
		{
			name: "unsupported images attribute",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:      adGroup,
				googleads.AttrFinalUrls:    []any{"https://example.com/"},
				googleads.AttrHeadlines:    defaultRSAHeadlines(),
				googleads.AttrDescriptions: defaultRSADescriptions(),
				"images":                   []any{"logo.png"},
			},
			want: "unsupported attribute",
		},
		{
			name: "pinned headline accepted",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:   adGroup,
				googleads.AttrFinalUrls: []any{"https://example.com/"},
				googleads.AttrHeadlines: []any{
					map[string]any{"text": "Buy shoes online", "pin": "headline_1"},
					"Free shipping today",
					"Shop the collection",
				},
				googleads.AttrDescriptions: defaultRSADescriptions(),
				googleads.AttrPath1:        "shoes",
				googleads.AttrPath2:        "sale",
				googleads.AttrStatus:       "ENABLED",
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

func TestReadResponsiveSearchAdSuccess(t *testing.T) {
	t.Parallel()

	fake := newRSAFake()
	fake.seedRSA(sampleSearchRSA("31", "71"))
	p, _ := testRSAProvider(t, fake)
	bindAdGroupIdentity(t, p, "31")

	res := rsaResource(t, "brand", defaultRSAAttrs(t))
	res.Identity = resource.Identity{ID: "31~71"}
	live, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "31~71" {
		t.Fatalf("identity = %q, want 31~71", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrStatus] != "PAUSED" {
		t.Fatalf("status = %v, want PAUSED", live.Attributes[googleads.AttrStatus])
	}
	ref, ok := resource.AsRef(live.Attributes[googleads.AttrAdGroup])
	if !ok || ref.Address != mustAdGroupAddress(t, "brand") {
		t.Fatalf("adGroup = %#v, want logical $ref", live.Attributes[googleads.AttrAdGroup])
	}
	if live.Computed["id"] != "31~71" || live.Computed["adId"] != "71" {
		t.Fatalf("computed = %+v", live.Computed)
	}
	if _, ok := live.Attributes["id"]; ok {
		t.Fatal("id must not appear in comparable attributes")
	}
	if _, ok := live.Attributes["adId"]; ok {
		t.Fatal("adId must not appear in comparable attributes")
	}
	headlines, _ := live.Attributes[googleads.AttrHeadlines].([]any)
	if len(headlines) != 3 || headlines[0] != "Buy shoes online" {
		t.Fatalf("headlines = %#v", live.Attributes[googleads.AttrHeadlines])
	}
}

func TestReadResponsiveSearchAdNotFound(t *testing.T) {
	t.Parallel()

	p, _ := testRSAProvider(t, newRSAFake())
	_, err := p.Read(context.Background(), rsaResource(t, "brand", resolvedRSAAttrs(t, "31")))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadResponsiveSearchAdUnboundWithoutAdGroupIdentityIsNotFound(t *testing.T) {
	t.Parallel()

	fake := newRSAFake()
	fake.seedRSA(sampleSearchRSA("31", "71"))
	p, _ := testRSAProvider(t, fake)

	_, err := p.Read(context.Background(), rsaResource(t, "brand", defaultRSAAttrs(t)))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound when ad group identity is unknown", err)
	}
}

func TestReadResponsiveSearchAdUnboundMatchesCreative(t *testing.T) {
	t.Parallel()

	fake := newRSAFake()
	fake.seedRSA(sampleSearchRSA("99", "50"))
	fake.seedRSA(sampleSearchRSA("31", "71"))
	p, _ := testRSAProvider(t, fake)

	live, err := p.Read(context.Background(), rsaResource(t, "brand", resolvedRSAAttrs(t, "31")))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "31~71" {
		t.Fatalf("identity = %q, want 31~71 from the referenced ad group", live.Identity.ID)
	}
}

func TestReadResponsiveSearchAdRejectsNonRSA(t *testing.T) {
	t.Parallel()

	fake := newRSAFake()
	item := sampleSearchRSA("31", "9")
	ad, _ := item["ad"].(map[string]any)
	ad["type"] = "EXPANDED_TEXT_AD"
	fake.seedRSA(item)
	p, _ := testRSAProvider(t, fake)

	res := rsaResource(t, "brand", defaultRSAAttrs(t))
	res.Identity = resource.Identity{ID: "31~9"}
	_, err := p.Read(context.Background(), res)
	if err == nil {
		t.Fatal("expected unsupported type error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported type must not look like not found")
	}
	if !strings.Contains(err.Error(), "RESPONSIVE_SEARCH_AD") {
		t.Fatalf("error = %q, want RESPONSIVE_SEARCH_AD guidance", err)
	}
}

func TestReadResponsiveSearchAdMalformed(t *testing.T) {
	t.Parallel()

	fake := newRSAFake()
	fake.searchBody = `{"results":[{"adGroupAd":{"resourceName":"not-a-resource"}}]}`
	p, _ := testRSAProvider(t, fake)
	res := rsaResource(t, "brand", resolvedRSAAttrs(t, "31"))
	res.Identity = resource.Identity{ID: "31~71"}
	_, err := p.Read(context.Background(), res)
	if err == nil {
		t.Fatal("expected malformed error")
	}
	if !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("error = %q, want malformed", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadResponsiveSearchAdAPIError(t *testing.T) {
	t.Parallel()

	fake := newRSAFake()
	fake.searchStatus = http.StatusForbidden
	p, _ := testRSAProvider(t, fake)
	_, err := p.Read(context.Background(), rsaResource(t, "brand", resolvedRSAAttrs(t, "31")))
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestCreateResponsiveSearchAdDefaultsPaused(t *testing.T) {
	t.Parallel()

	fake := newRSAFake()
	p, _ := testRSAProvider(t, fake)

	live, err := p.Create(context.Background(), rsaResource(t, "brand", resolvedRSAAttrs(t, "31")))
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
	if !strings.Contains(fake.lastMutate, "adGroups/31") {
		t.Fatalf("create mutate missing resolved ad group: %s", fake.lastMutate)
	}
	if !strings.Contains(fake.lastMutate, `"text":"Buy shoes online"`) && !strings.Contains(fake.lastMutate, `"text": "Buy shoes online"`) {
		t.Fatalf("create mutate missing headline: %s", fake.lastMutate)
	}
	if len(fake.mutates) != 1 || fake.mutates[0] != "adGroupAds" {
		t.Fatalf("mutates = %v, want adGroupAds create", fake.mutates)
	}
}

func TestCreateResponsiveSearchAdResolvesAdGroupReference(t *testing.T) {
	t.Parallel()

	fake := newRSAFake()
	p, _ := testRSAProvider(t, fake)
	attrs := defaultRSAAttrs(t)
	attrs[googleads.AttrAdGroup] = resource.Resolved{
		Address:  mustAdGroupAddress(t, "brand"),
		Identity: resource.Identity{ID: "31"},
		Outputs:  resource.Attributes{"resourceName": "customers/" + testCustomerID + "/adGroups/31"},
	}
	attrs[googleads.AttrPath1] = "shoes"
	if _, err := p.Create(context.Background(), rsaResource(t, "brand", attrs)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.Contains(fake.lastMutate, `"path1":"shoes"`) && !strings.Contains(fake.lastMutate, `"path1": "shoes"`) {
		t.Fatalf("create mutate missing path1: %s", fake.lastMutate)
	}
}

func TestCreateResponsiveSearchAdMissingAdGroupIdentity(t *testing.T) {
	t.Parallel()

	p, _ := testRSAProvider(t, newRSAFake())
	_, err := p.Create(context.Background(), rsaResource(t, "brand", defaultRSAAttrs(t)))
	if err == nil {
		t.Fatal("expected missing ad group identity error")
	}
	if !strings.Contains(err.Error(), "googleads.ad_group.brand") {
		t.Fatalf("error = %q, want ad group address", err)
	}
}

func TestCreateResponsiveSearchAdAPIError(t *testing.T) {
	t.Parallel()

	fake := newRSAFake()
	fake.mutateStatus = http.StatusBadRequest
	p, _ := testRSAProvider(t, fake)
	_, err := p.Create(context.Background(), rsaResource(t, "brand", resolvedRSAAttrs(t, "31")))
	if err == nil {
		t.Fatal("expected API error")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestUpdateResponsiveSearchAdStatus(t *testing.T) {
	t.Parallel()

	fake := newRSAFake()
	fake.seedRSA(sampleSearchRSA("31", "71"))
	p, _ := testRSAProvider(t, fake)
	bindAdGroupIdentity(t, p, "31")

	desired := rsaResource(t, "brand", resolvedRSAAttrs(t, "31"))
	desired.Attributes[googleads.AttrStatus] = "ENABLED"
	live, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "31~71"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if live.Identity.ID != "31~71" {
		t.Fatalf("identity = %q, want 31~71", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrStatus] != "ENABLED" {
		t.Fatalf("status = %v, want ENABLED", live.Attributes[googleads.AttrStatus])
	}
	if len(fake.mutates) != 1 || fake.mutates[0] != "adGroupAds" {
		t.Fatalf("mutates = %v, want status-only adGroupAds update", fake.mutates)
	}
	if !strings.Contains(fake.lastMutate, "updateMask") || !strings.Contains(fake.lastMutate, "status") {
		t.Fatalf("update missing status mask: %s", fake.lastMutate)
	}
}

func TestUpdateResponsiveSearchAdReplacesCreative(t *testing.T) {
	t.Parallel()

	fake := newRSAFake()
	fake.seedRSA(sampleSearchRSA("31", "71"))
	p, _ := testRSAProvider(t, fake)
	bindAdGroupIdentity(t, p, "31")

	desired := rsaResource(t, "brand", resolvedRSAAttrs(t, "31"))
	desired.Attributes[googleads.AttrHeadlines] = []any{"Shop shoes today", "Free shipping today", "Shop the collection"}
	live, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "31~71"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if live.Identity.ID != "31~71" {
		t.Fatalf("identity = %q, want unchanged 31~71 after creative replacement", live.Identity.ID)
	}
	if len(fake.mutates) != 1 || fake.mutates[0] != "ads" {
		t.Fatalf("mutates = %v, want ads creative replacement", fake.mutates)
	}
	if !strings.Contains(fake.lastMutate, "responsiveSearchAd.headlines") {
		t.Fatalf("creative replacement missing headline mask: %s", fake.lastMutate)
	}
	if !strings.Contains(fake.lastMutate, "Shop shoes today") {
		t.Fatalf("creative replacement missing new headline: %s", fake.lastMutate)
	}
}

func TestUpdateResponsiveSearchAdNoOp(t *testing.T) {
	t.Parallel()

	fake := newRSAFake()
	fake.seedRSA(sampleSearchRSA("31", "71"))
	p, _ := testRSAProvider(t, fake)
	bindAdGroupIdentity(t, p, "31")

	desired := rsaResource(t, "brand", resolvedRSAAttrs(t, "31"))
	live, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "31~71"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if live.Identity.ID != "31~71" {
		t.Fatalf("identity = %q, want 31~71", live.Identity.ID)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("no-op update mutated remote: %v", fake.mutates)
	}
}

func TestUpdateResponsiveSearchAdImmutableAdGroup(t *testing.T) {
	t.Parallel()

	fake := newRSAFake()
	fake.seedRSA(sampleSearchRSA("31", "71"))
	p, _ := testRSAProvider(t, fake)
	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustAdGroupAddress(t, "brand"), resource.Identity{ID: "31"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustAdGroupAddress(t, "generic"), resource.Identity{ID: "99"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	desired := rsaResource(t, "brand", resolvedRSAAttrs(t, "99"))
	desired.Attributes[googleads.AttrAdGroup] = resource.Resolved{
		Address:  mustAdGroupAddress(t, "generic"),
		Identity: resource.Identity{ID: "99"},
		Outputs:  resource.Attributes{"resourceName": "customers/" + testCustomerID + "/adGroups/99"},
	}
	_, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "31~71"},
	})
	if err == nil {
		t.Fatal("expected immutable ad group error")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("error = %q, want immutable adGroup", err)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("immutable update mutated remote: %v", fake.mutates)
	}
}

func TestImportResponsiveSearchAdRequiresBoundAdGroup(t *testing.T) {
	t.Parallel()

	fake := newRSAFake()
	fake.seedRSA(sampleSearchRSA("31", "71"))
	p, _ := testRSAProvider(t, fake)
	_, err := p.Import(context.Background(), mustRSAAddress(t, "brand"), "31~71")
	if err == nil {
		t.Fatal("expected missing ad group binding error")
	}
	if !strings.Contains(err.Error(), "ad group") {
		t.Fatalf("error = %q, want ad group import guidance", err)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("import mutated remote: %v", fake.mutates)
	}
}

func TestImportResponsiveSearchAdThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	fake := newRSAFake()
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
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	item := sampleSearchRSA("31", "71")
	item["ad"].(map[string]any)["responsiveSearchAd"].(map[string]any)["path1"] = "shoes"
	fake.seedRSA(item)
	p, _ := testRSAProvider(t, fake)

	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignBudgetAddress(t, "brand"), resource.Identity{ID: "11"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustAdGroupAddress(t, "brand"), resource.Identity{ID: "31"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	got, err := importer.Run(context.Background(), mustRSAAddress(t, "brand"), "31~71", lookupGoogleAds(p), st)
	if err != nil {
		t.Fatalf("importer.Run: %v", err)
	}
	if got.Identity.ID != "31~71" {
		t.Fatalf("identity = %q, want 31~71", got.Identity.ID)
	}
	assertNoProviderSecret(t, got.YAML)
	for _, leak := range []string{"resourceName", "adId", "assetPerformanceLabel", testAccessToken} {
		if strings.Contains(got.YAML, leak) {
			t.Fatalf("generated YAML leaked %q:\n%s", leak, got.YAML)
		}
	}
	if !strings.Contains(got.YAML, "$ref: googleads.ad_group.brand") {
		t.Fatalf("generated YAML missing ad group $ref:\n%s", got.YAML)
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
  - address: googleads.ad_group.brand
    attributes:
      name: Brand
      campaign:
        $ref: googleads.campaign.brand
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

func TestNormalizeResponsiveSearchAdImportID(t *testing.T) {
	t.Parallel()

	p, _ := testRSAProvider(t, nil)
	addr := mustRSAAddress(t, "brand")
	got, err := p.NormalizeImportID(addr, "31~71")
	if err != nil || got != "31~71" {
		t.Fatalf("composite = (%q, %v), want 31~71", got, err)
	}
	got, err = p.NormalizeImportID(addr, "customers/"+testCustomerID+"/adGroupAds/31~71")
	if err != nil || got != "31~71" {
		t.Fatalf("resource name = (%q, %v), want 31~71", got, err)
	}
	_, err = p.NormalizeImportID(addr, "abc")
	if err == nil || !strings.Contains(err.Error(), "not a valid Google Ads responsive search ad id") {
		t.Fatalf("invalid id error = %v", err)
	}
}

func TestPlanResponsiveSearchAdCreateWhenMissing(t *testing.T) {
	t.Parallel()

	p, _ := testRSAProvider(t, newRSAFake())
	got := mustPlanRSA(t, p, rsaStack(t, defaultRSAAttrs(t))...)
	if len(got.Changes) != 4 {
		t.Fatalf("changes = %+v, want 4", got.Changes)
	}
	byAddr := map[string]plan.Action{}
	for _, c := range got.Changes {
		byAddr[c.Address.String()] = c.Action
	}
	if byAddr["googleads.campaign_budget.brand"] != plan.ActionCreate || byAddr["googleads.campaign.brand"] != plan.ActionCreate || byAddr["googleads.ad_group.brand"] != plan.ActionCreate || byAddr["googleads.responsive_search_ad.brand"] != plan.ActionCreate {
		t.Fatalf("actions = %+v, want create stack", byAddr)
	}
}

func TestPlanResponsiveSearchAdUnchangedEquivalentRemote(t *testing.T) {
	t.Parallel()

	fake := newRSAFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false, "period": "DAILY", "type": "STANDARD"})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	fake.seedRSA(sampleSearchRSA("31", "71"))
	p, _ := testRSAProvider(t, fake)

	got := mustPlanRSA(t, p, rsaStack(t, defaultRSAAttrs(t))...)
	if got.HasChanges() {
		t.Fatalf("equivalent remote produced changes: %+v", got.Changes)
	}
}

func TestPlanResponsiveSearchAdIgnoresAssetMetadataAndUnpinnedLivePin(t *testing.T) {
	t.Parallel()

	fake := newRSAFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false, "period": "DAILY", "type": "STANDARD"})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	item := sampleSearchRSA("31", "71")
	headlines := item["ad"].(map[string]any)["responsiveSearchAd"].(map[string]any)["headlines"].([]any)
	headlines[0] = map[string]any{
		"text":                  "Buy shoes online",
		"pinnedField":           "HEADLINE_1",
		"assetPerformanceLabel": "PENDING",
		"policySummaryInfo":     map[string]any{"approvalStatus": "APPROVED"},
	}
	fake.seedRSA(item)
	p, _ := testRSAProvider(t, fake)

	got := mustPlanRSA(t, p, rsaStack(t, defaultRSAAttrs(t))...)
	if got.HasChanges() {
		t.Fatalf("unmanaged pin and asset metadata produced changes: %+v", got.Changes)
	}
}

func TestPlanResponsiveSearchAdCreativeReplacementIsVisible(t *testing.T) {
	t.Parallel()

	fake := newRSAFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false, "period": "DAILY", "type": "STANDARD"})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	fake.seedRSA(sampleSearchRSA("31", "71"))
	p, _ := testRSAProvider(t, fake)

	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignBudgetAddress(t, "brand"), resource.Identity{ID: "11"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustAdGroupAddress(t, "brand"), resource.Identity{ID: "31"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustRSAAddress(t, "brand"), resource.Identity{ID: "31~71"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	attrs := defaultRSAAttrs(t)
	attrs[googleads.AttrHeadlines] = []any{"Shop shoes today", "Free shipping today", "Shop the collection"}
	got, err := plan.BuildWithState(context.Background(), rsaStack(t, attrs), func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("plan.BuildWithState: %v", err)
	}
	change := rsaChange(t, got)
	if change.Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want creative replacement update", change)
	}
	found := false
	for _, diff := range change.Diffs {
		if strings.HasPrefix(diff.Path, googleads.AttrHeadlines) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing headline replacement diff: %+v", change.Diffs)
	}
	rendered := plan.Format(got)
	if !strings.Contains(rendered, "Shop shoes today") {
		t.Fatalf("plan output missing replacement headline:\n%s", rendered)
	}
}

func TestPlanResponsiveSearchAdStatusUpdate(t *testing.T) {
	t.Parallel()

	fake := newRSAFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false, "period": "DAILY", "type": "STANDARD"})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	live := sampleSearchRSA("31", "71")
	live["status"] = "ENABLED"
	fake.seedRSA(live)
	p, _ := testRSAProvider(t, fake)

	got := mustPlanRSA(t, p, rsaStack(t, defaultRSAAttrs(t))...)
	change := rsaChange(t, got)
	if change.Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want status update to PAUSED", got.Changes)
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

func TestPlanResponsiveSearchAdImmutableAdGroupIsVisible(t *testing.T) {
	t.Parallel()

	fake := newRSAFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false, "period": "DAILY", "type": "STANDARD"})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	fake.seedAdGroup(sampleSearchAdGroup("99", "Generic", "21"))
	fake.seedRSA(sampleSearchRSA("31", "71"))
	p, _ := testRSAProvider(t, fake)

	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignBudgetAddress(t, "brand"), resource.Identity{ID: "11"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustAdGroupAddress(t, "brand"), resource.Identity{ID: "31"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustAdGroupAddress(t, "generic"), resource.Identity{ID: "99"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustRSAAddress(t, "brand"), resource.Identity{ID: "31~71"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	attrs := defaultRSAAttrs(t)
	attrs[googleads.AttrAdGroup] = adGroupRef(t, "generic")
	resources := []resource.Resource{
		campaignBudgetResource(t, "brand", resource.Attributes{
			googleads.AttrName:             "Brand daily budget",
			googleads.AttrAmount:           50,
			googleads.AttrExplicitlyShared: false,
		}),
		campaignResource(t, "brand", defaultCampaignAttrs(t)),
		adGroupResource(t, "brand", defaultAdGroupAttrs(t)),
		adGroupResource(t, "generic", resource.Attributes{
			googleads.AttrName:     "Generic",
			googleads.AttrCampaign: campaignRef(t, "brand"),
		}),
		rsaResource(t, "brand", attrs),
	}
	_, err := plan.BuildWithState(context.Background(), resources, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err == nil {
		t.Fatal("expected ad group change plan error")
	}
	if !strings.Contains(err.Error(), "does not match referenced ad group") && !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("error = %q, want bound identity/ad group mismatch", err)
	}
}

func mustPlanRSA(t *testing.T, p *googleads.Provider, resources ...resource.Resource) *plan.Plan {
	t.Helper()
	got, err := plan.Build(context.Background(), resources, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	return got
}

func rsaChange(t *testing.T, got *plan.Plan) *plan.Change {
	t.Helper()
	addr := mustRSAAddress(t, "brand")
	for i := range got.Changes {
		if got.Changes[i].Address == addr {
			return &got.Changes[i]
		}
	}
	t.Fatalf("missing RSA change: %+v", got.Changes)
	return nil
}

func defaultRSAHeadlines() []any {
	return []any{"Buy shoes online", "Free shipping today", "Shop the collection"}
}

func defaultRSADescriptions() []any {
	return []any{"Find shoes that fit your style.", "Free returns on every order."}
}

func defaultRSAAttrs(t *testing.T) resource.Attributes {
	t.Helper()
	return resource.Attributes{
		googleads.AttrAdGroup:      adGroupRef(t, "brand"),
		googleads.AttrFinalUrls:    []any{"https://example.com/"},
		googleads.AttrHeadlines:    defaultRSAHeadlines(),
		googleads.AttrDescriptions: defaultRSADescriptions(),
	}
}

func resolvedRSAAttrs(t *testing.T, adGroupID string) resource.Attributes {
	t.Helper()
	attrs := defaultRSAAttrs(t)
	attrs[googleads.AttrAdGroup] = resource.Resolved{
		Address:  mustAdGroupAddress(t, "brand"),
		Identity: resource.Identity{ID: adGroupID},
		Outputs:  resource.Attributes{"resourceName": "customers/" + testCustomerID + "/adGroups/" + adGroupID},
	}
	return attrs
}

func rsaResource(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{Address: mustRSAAddress(t, name), Attributes: attrs}
}

func mustRSAAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("googleads.responsive_search_ad." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func rsaStack(t *testing.T, rsaAttrs resource.Attributes) []resource.Resource {
	t.Helper()
	return []resource.Resource{
		campaignBudgetResource(t, "brand", resource.Attributes{
			googleads.AttrName:             "Brand daily budget",
			googleads.AttrAmount:           50,
			googleads.AttrExplicitlyShared: false,
		}),
		campaignResource(t, "brand", defaultCampaignAttrs(t)),
		adGroupResource(t, "brand", defaultAdGroupAttrs(t)),
		rsaResource(t, "brand", rsaAttrs),
	}
}

func testRSAProvider(t *testing.T, fake *rsaFake) (*googleads.Provider, *httptest.Server) {
	t.Helper()
	if fake == nil {
		fake = newRSAFake()
	}
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	t.Cleanup(srv.Close)
	cfg := validConfig(srv.URL)
	cfg.TokenURL = srv.URL + "/oauth/token"
	p := googleads.NewWithHTTPClient(cfg, srv.Client())
	return p, srv
}

func sampleSearchRSA(adGroupID, adID string) map[string]any {
	return map[string]any{
		"adGroup": "customers/" + testCustomerID + "/adGroups/" + adGroupID,
		"status":  "PAUSED",
		"ad": map[string]any{
			"id":        adID,
			"type":      "RESPONSIVE_SEARCH_AD",
			"finalUrls": []any{"https://example.com/"},
			"responsiveSearchAd": map[string]any{
				"headlines": []any{
					map[string]any{"text": "Buy shoes online", "assetPerformanceLabel": "PENDING"},
					map[string]any{"text": "Free shipping today"},
					map[string]any{"text": "Shop the collection"},
				},
				"descriptions": []any{
					map[string]any{"text": "Find shoes that fit your style."},
					map[string]any{"text": "Free returns on every order."},
				},
			},
		},
	}
}

type rsaFake struct {
	mu sync.Mutex

	nextID    int64
	ads       map[string]map[string]any
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

func newRSAFake() *rsaFake {
	return &rsaFake{
		ads:       map[string]map[string]any{},
		adGroups:  map[string]map[string]any{},
		campaigns: map[string]map[string]any{},
		budgets:   map[string]map[string]any{},
	}
}

func (f *rsaFake) seedRSA(item map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storeRSALocked(cloneMap(item))
}

func (f *rsaFake) seedAdGroup(group map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cloned := cloneMap(group)
	id := stringify(cloned["id"])
	if stringify(cloned["resourceName"]) == "" {
		cloned["resourceName"] = "customers/" + testCustomerID + "/adGroups/" + id
	}
	if stringify(cloned["type"]) == "" {
		cloned["type"] = "SEARCH_STANDARD"
	}
	if stringify(cloned["status"]) == "" {
		cloned["status"] = "PAUSED"
	}
	f.adGroups[id] = cloned
}

func (f *rsaFake) seedCampaign(campaign map[string]any) {
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

func (f *rsaFake) seedBudget(budget map[string]any) {
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

func (f *rsaFake) storeRSALocked(item map[string]any) {
	adGroup := stringify(item["adGroup"])
	adGroupID := strings.TrimPrefix(adGroup, "customers/"+testCustomerID+"/adGroups/")
	ad, _ := item["ad"].(map[string]any)
	if ad == nil {
		ad = map[string]any{}
		item["ad"] = ad
	}
	adID := stringify(ad["id"])
	id := adGroupID + "~" + adID
	if stringify(item["resourceName"]) == "" {
		item["resourceName"] = "customers/" + testCustomerID + "/adGroupAds/" + id
	}
	if stringify(ad["resourceName"]) == "" {
		ad["resourceName"] = "customers/" + testCustomerID + "/ads/" + adID
	}
	if stringify(ad["type"]) == "" {
		ad["type"] = "RESPONSIVE_SEARCH_AD"
	}
	if stringify(item["status"]) == "" {
		item["status"] = "PAUSED"
	}
	f.ads[id] = item
}

func (f *rsaFake) handler(w http.ResponseWriter, r *http.Request) {
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
		case strings.Contains(query, "from ad_group_ad"):
			_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchRSAsLocked(req.Query)})
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

func (f *rsaFake) searchRSAsLocked(query string) []any {
	var out []any
	for _, item := range f.ads {
		if matchesRSAQuery(query, item) {
			out = append(out, map[string]any{"adGroupAd": cloneMap(item)})
		}
	}
	return out
}

func (f *rsaFake) searchAdGroupsLocked(query string) []any {
	var out []any
	for _, group := range f.adGroups {
		if matchesAdGroupQuery(query, group) {
			out = append(out, map[string]any{"adGroup": cloneMap(group)})
		}
	}
	return out
}

func (f *rsaFake) searchCampaignsLocked(query string) []any {
	var out []any
	for _, campaign := range f.campaigns {
		if matchesCampaignQuery(query, campaign) {
			out = append(out, map[string]any{"campaign": cloneMap(campaign)})
		}
	}
	return out
}

func (f *rsaFake) searchBudgetsLocked(query string) []any {
	var out []any
	for _, budget := range f.budgets {
		if matchesCampaignBudgetQuery(query, budget) {
			out = append(out, map[string]any{"campaignBudget": cloneMap(budget)})
		}
	}
	return out
}

func (f *rsaFake) mutateLocked(collection string, body []byte) (string, error) {
	var req struct {
		Operations []map[string]any `json:"operations"`
	}
	if err := json.Unmarshal(body, &req); err != nil || len(req.Operations) == 0 {
		return "", errors.New("malformed mutate")
	}
	op := req.Operations[0]
	switch collection {
	case "adGroupAds":
		if raw, ok := op["create"]; ok {
			item, _ := raw.(map[string]any)
			created := cloneMap(item)
			ad, _ := created["ad"].(map[string]any)
			if ad == nil {
				ad = map[string]any{}
				created["ad"] = ad
			}
			f.nextID++
			ad["id"] = strconv.FormatInt(f.nextID, 10)
			if stringify(ad["type"]) == "" {
				ad["type"] = "RESPONSIVE_SEARCH_AD"
			}
			if stringify(created["status"]) == "" {
				created["status"] = "PAUSED"
			}
			f.storeRSALocked(created)
			return stringify(created["resourceName"]), nil
		}
		if raw, ok := op["update"]; ok {
			item, _ := raw.(map[string]any)
			resourceName := stringify(item["resourceName"])
			id := strings.TrimPrefix(resourceName, "customers/"+testCustomerID+"/adGroupAds/")
			current, ok := f.ads[id]
			if !ok {
				return "", errors.New("missing ad group ad")
			}
			merged := cloneMap(current)
			for k, v := range item {
				if k == "resourceName" || k == "ad" {
					continue
				}
				merged[k] = v
			}
			f.storeRSALocked(merged)
			return resourceName, nil
		}
	case "ads":
		if raw, ok := op["update"]; ok {
			item, _ := raw.(map[string]any)
			resourceName := stringify(item["resourceName"])
			adID := strings.TrimPrefix(resourceName, "customers/"+testCustomerID+"/ads/")
			for _, current := range f.ads {
				ad, _ := current["ad"].(map[string]any)
				if stringify(ad["id"]) != adID {
					continue
				}
				merged := cloneMap(current)
				mergedAd := cloneMap(ad)
				for k, v := range item {
					if k == "resourceName" {
						continue
					}
					if k == "responsiveSearchAd" {
						existing, _ := mergedAd["responsiveSearchAd"].(map[string]any)
						updated := cloneMap(existing)
						incoming, _ := v.(map[string]any)
						for ik, iv := range incoming {
							updated[ik] = iv
						}
						mergedAd["responsiveSearchAd"] = updated
						continue
					}
					mergedAd[k] = v
				}
				merged["ad"] = mergedAd
				f.storeRSALocked(merged)
				return resourceName, nil
			}
			return "", errors.New("missing ad")
		}
	}
	return "", errors.New("unexpected mutate " + collection)
}

func matchesRSAQuery(query string, item map[string]any) bool {
	adGroup := stringify(item["adGroup"])
	adGroupID := strings.TrimPrefix(adGroup, "customers/"+testCustomerID+"/adGroups/")
	ad, _ := item["ad"].(map[string]any)
	adID := stringify(ad["id"])
	if stringify(item["status"]) == "REMOVED" {
		return false
	}
	if strings.Contains(query, "ad_group_ad.ad.id = ") {
		want := strings.TrimSpace(query[strings.Index(query, "ad_group_ad.ad.id = ")+len("ad_group_ad.ad.id = "):])
		if i := strings.IndexAny(want, " \n"); i >= 0 {
			want = want[:i]
		}
		if want != adID {
			return false
		}
	}
	if strings.Contains(query, "ad_group.id = ") {
		wantGroup := strings.TrimSpace(query[strings.Index(query, "ad_group.id = ")+len("ad_group.id = "):])
		if i := strings.IndexAny(wantGroup, " \n"); i >= 0 {
			wantGroup = wantGroup[:i]
		}
		if wantGroup != adGroupID {
			return false
		}
	}
	return true
}
