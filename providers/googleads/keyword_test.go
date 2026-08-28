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

func TestValidateKeywordValid(t *testing.T) {
	t.Parallel()

	p, _ := testKeywordProvider(t, nil)
	res := keywordResource(t, "brand_exact", defaultKeywordAttrs(t))
	if err := p.Validate(context.Background(), res); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateKeywordErrors(t *testing.T) {
	t.Parallel()

	p, _ := testKeywordProvider(t, nil)
	addr := mustKeywordAddress(t, "brand_exact")
	adGroup := adGroupRef(t, "brand")

	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name:  "missing text",
			attrs: resource.Attributes{googleads.AttrAdGroup: adGroup, googleads.AttrMatchType: "EXACT"},
			want:  "missing required attribute \"text\"",
		},
		{
			name:  "missing ad group",
			attrs: resource.Attributes{googleads.AttrText: "brand", googleads.AttrMatchType: "EXACT"},
			want:  "missing required attribute \"adGroup\"",
		},
		{
			name:  "missing match type",
			attrs: resource.Attributes{googleads.AttrAdGroup: adGroup, googleads.AttrText: "brand"},
			want:  "missing required attribute \"matchType\"",
		},
		{
			name: "ad group not a ref",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:   "customers/" + testCustomerID + "/adGroups/31",
				googleads.AttrText:      "brand",
				googleads.AttrMatchType: "EXACT",
			},
			want: "$ref",
		},
		{
			name: "ad group wrong type",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:   resource.Ref{Address: mustCampaignAddress(t, "brand")},
				googleads.AttrText:      "brand",
				googleads.AttrMatchType: "EXACT",
			},
			want: "googleads.ad_group",
		},
		{
			name: "unsupported match type",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:   adGroup,
				googleads.AttrText:      "brand",
				googleads.AttrMatchType: "BROAD_MATCH_MODIFIED",
			},
			want: "matchType",
		},
		{
			name: "exact accepted",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:   adGroup,
				googleads.AttrText:      "Brand",
				googleads.AttrMatchType: "exact",
			},
			want: "",
		},
		{
			name: "phrase accepted",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:   adGroup,
				googleads.AttrText:      "brand shoes",
				googleads.AttrMatchType: "PHRASE",
			},
			want: "",
		},
		{
			name: "broad accepted",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:   adGroup,
				googleads.AttrText:      "brand",
				googleads.AttrMatchType: "BROAD",
			},
			want: "",
		},
		{
			name: "match type punctuation rejected",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:   adGroup,
				googleads.AttrText:      "[brand]",
				googleads.AttrMatchType: "EXACT",
			},
			want: "match-type punctuation",
		},
		{
			name: "phrase punctuation rejected",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:   adGroup,
				googleads.AttrText:      `"brand shoes"`,
				googleads.AttrMatchType: "PHRASE",
			},
			want: "match-type punctuation",
		},
		{
			name: "empty text",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:   adGroup,
				googleads.AttrText:      "   ",
				googleads.AttrMatchType: "EXACT",
			},
			want: "non-empty",
		},
		{
			name: "removed status rejected",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:   adGroup,
				googleads.AttrText:      "brand",
				googleads.AttrMatchType: "EXACT",
				googleads.AttrStatus:    "REMOVED",
			},
			want: "status",
		},
		{
			name: "negative with cpc bid rejected",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:   adGroup,
				googleads.AttrText:      "competitor",
				googleads.AttrMatchType: "EXACT",
				googleads.AttrNegative:  true,
				googleads.AttrCpcBid:    1.5,
			},
			want: "negative",
		},
		{
			name: "negative accepted",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:   adGroup,
				googleads.AttrText:      "competitor",
				googleads.AttrMatchType: "PHRASE",
				googleads.AttrNegative:  true,
			},
			want: "",
		},
		{
			name: "computed id",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:   adGroup,
				googleads.AttrText:      "brand",
				googleads.AttrMatchType: "EXACT",
				"id":                    "31~41",
			},
			want: "computed",
		},
		{
			name: "unsupported audience attribute",
			attrs: resource.Attributes{
				googleads.AttrAdGroup:   adGroup,
				googleads.AttrText:      "brand",
				googleads.AttrMatchType: "EXACT",
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

func TestReadKeywordSuccess(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	fake.seedKeyword(sampleSearchKeyword("31", "41", "brand", "EXACT", false))
	p, _ := testKeywordProvider(t, fake)
	bindAdGroupIdentity(t, p, "31")

	res := keywordResource(t, "brand_exact", defaultKeywordAttrs(t))
	res.Identity = resource.Identity{ID: "31~41"}
	live, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "31~41" {
		t.Fatalf("identity = %q, want 31~41", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrText] != "brand" {
		t.Fatalf("text = %v", live.Attributes[googleads.AttrText])
	}
	if live.Attributes[googleads.AttrMatchType] != "EXACT" {
		t.Fatalf("matchType = %v, want EXACT", live.Attributes[googleads.AttrMatchType])
	}
	if live.Attributes[googleads.AttrNegative] != false {
		t.Fatalf("negative = %v, want false", live.Attributes[googleads.AttrNegative])
	}
	ref, ok := resource.AsRef(live.Attributes[googleads.AttrAdGroup])
	if !ok || ref.Address != mustAdGroupAddress(t, "brand") {
		t.Fatalf("adGroup = %#v, want logical $ref", live.Attributes[googleads.AttrAdGroup])
	}
	if live.Computed["id"] != "31~41" {
		t.Fatalf("computed id = %v", live.Computed["id"])
	}
	if _, ok := live.Attributes["id"]; ok {
		t.Fatal("id must not appear in comparable attributes")
	}
	if _, ok := live.Attributes["cpcBidMicros"]; ok {
		t.Fatal("native cpcBidMicros must not appear in comparable attributes")
	}
}

func TestReadKeywordNotFound(t *testing.T) {
	t.Parallel()

	p, _ := testKeywordProvider(t, newKeywordFake())
	_, err := p.Read(context.Background(), keywordResource(t, "brand_exact", resolvedKeywordAttrs(t, "31")))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadKeywordUnboundWithoutAdGroupIdentityIsNotFound(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	fake.seedKeyword(sampleSearchKeyword("31", "41", "brand", "EXACT", false))
	p, _ := testKeywordProvider(t, fake)

	_, err := p.Read(context.Background(), keywordResource(t, "brand_exact", defaultKeywordAttrs(t)))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound when ad group identity is unknown", err)
	}
}

func TestReadKeywordSameTextDifferentAdGroups(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	fake.seedKeyword(sampleSearchKeyword("99", "50", "brand", "EXACT", false))
	fake.seedKeyword(sampleSearchKeyword("31", "41", "brand", "EXACT", false))
	p, _ := testKeywordProvider(t, fake)

	live, err := p.Read(context.Background(), keywordResource(t, "brand_exact", resolvedKeywordAttrs(t, "31")))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "31~41" {
		t.Fatalf("identity = %q, want 31~41 from the referenced ad group", live.Identity.ID)
	}
}

func TestReadKeywordUnboundIgnoresRemoteTextCase(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	fake.seedKeyword(sampleSearchKeyword("31", "41", "Brand", "EXACT", false))
	p, _ := testKeywordProvider(t, fake)

	live, err := p.Read(context.Background(), keywordResource(t, "brand_exact", resolvedKeywordAttrs(t, "31")))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "31~41" {
		t.Fatalf("identity = %q, want 31~41", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrText] != "brand" {
		t.Fatalf("text = %v, want normalized brand", live.Attributes[googleads.AttrText])
	}
	if strings.Contains(strings.ToLower(fake.lastQuery), "keyword.text = ") {
		t.Fatalf("unbound read must not filter keyword text in GAQL: %s", fake.lastQuery)
	}
}

func TestReadKeywordNegativeAndPositiveAreDistinct(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	fake.seedKeyword(sampleSearchKeyword("31", "41", "brand", "EXACT", false))
	fake.seedKeyword(sampleSearchKeyword("31", "42", "brand", "EXACT", true))
	p, _ := testKeywordProvider(t, fake)

	attrs := resolvedKeywordAttrs(t, "31")
	attrs[googleads.AttrNegative] = true
	live, err := p.Read(context.Background(), keywordResource(t, "brand_neg", attrs))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "31~42" {
		t.Fatalf("identity = %q, want 31~42 negative criterion", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrNegative] != true {
		t.Fatalf("negative = %v, want true", live.Attributes[googleads.AttrNegative])
	}
}

func TestReadKeywordRejectsNonKeywordCriterion(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	item := sampleSearchKeyword("31", "9", "brand", "EXACT", false)
	item["type"] = "LISTING_GROUP"
	fake.seedKeyword(item)
	p, _ := testKeywordProvider(t, fake)

	res := keywordResource(t, "listing", defaultKeywordAttrs(t))
	res.Identity = resource.Identity{ID: "31~9"}
	_, err := p.Read(context.Background(), res)
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

func TestReadKeywordRejectsURLSettings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		set  func(map[string]any)
		want string
	}{
		{
			name: "final urls",
			set:  func(item map[string]any) { item["finalUrls"] = []any{"https://example.com/landing"} },
			want: "finalUrls",
		},
		{
			name: "final mobile urls",
			set:  func(item map[string]any) { item["finalMobileUrls"] = []any{"https://m.example.com/landing"} },
			want: "finalMobileUrls",
		},
		{
			name: "final url suffix",
			set:  func(item map[string]any) { item["finalUrlSuffix"] = "utm_source=google" },
			want: "finalUrlSuffix",
		},
		{
			name: "tracking url template",
			set:  func(item map[string]any) { item["trackingUrlTemplate"] = "https://tracker.example/{lpurl}" },
			want: "trackingUrlTemplate",
		},
		{
			name: "custom url parameters",
			set: func(item map[string]any) {
				item["urlCustomParameters"] = []any{map[string]any{"key": "utm_campaign", "value": "brand"}}
			},
			want: "urlCustomParameters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := newKeywordFake()
			item := sampleSearchKeyword("31", "9", "brand", "EXACT", false)
			tt.set(item)
			fake.seedKeyword(item)
			p, _ := testKeywordProvider(t, fake)

			res := keywordResource(t, "brand_exact", defaultKeywordAttrs(t))
			res.Identity = resource.Identity{ID: "31~9"}
			_, err := p.Read(context.Background(), res)
			if err == nil {
				t.Fatal("expected unsupported URL settings error")
			}
			if errors.Is(err, provider.ErrNotFound) {
				t.Fatal("unsupported URL settings must not look like not found")
			}
			if !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "does not manage") {
				t.Fatalf("error = %q, want %s guidance", err, tt.want)
			}
		})
	}
}

func TestReadKeywordAPIError(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	fake.searchStatus = http.StatusForbidden
	p, _ := testKeywordProvider(t, fake)
	_, err := p.Read(context.Background(), keywordResource(t, "brand_exact", resolvedKeywordAttrs(t, "31")))
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestCreateKeywordDefaultsPausedPositive(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	p, _ := testKeywordProvider(t, fake)

	live, err := p.Create(context.Background(), keywordResource(t, "brand_exact", resolvedKeywordAttrs(t, "31")))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID == "" {
		t.Fatal("create returned empty identity")
	}
	if live.Attributes[googleads.AttrStatus] != "PAUSED" {
		t.Fatalf("status = %v, want PAUSED", live.Attributes[googleads.AttrStatus])
	}
	if live.Attributes[googleads.AttrNegative] != false {
		t.Fatalf("negative = %v, want false", live.Attributes[googleads.AttrNegative])
	}
	if !strings.Contains(fake.lastMutate, `"status":"PAUSED"`) && !strings.Contains(fake.lastMutate, `"status": "PAUSED"`) {
		t.Fatalf("create mutate missing PAUSED: %s", fake.lastMutate)
	}
	if !strings.Contains(fake.lastMutate, `"matchType":"EXACT"`) && !strings.Contains(fake.lastMutate, `"matchType": "EXACT"`) {
		t.Fatalf("create mutate missing EXACT: %s", fake.lastMutate)
	}
	if !strings.Contains(fake.lastMutate, "adGroups/31") {
		t.Fatalf("create mutate missing resolved ad group: %s", fake.lastMutate)
	}
}

func TestCreateKeywordResolvesAdGroupReference(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	p, _ := testKeywordProvider(t, fake)
	attrs := defaultKeywordAttrs(t)
	attrs[googleads.AttrAdGroup] = resource.Resolved{
		Address:  mustAdGroupAddress(t, "brand"),
		Identity: resource.Identity{ID: "31"},
		Outputs:  resource.Attributes{"resourceName": "customers/" + testCustomerID + "/adGroups/31"},
	}
	attrs[googleads.AttrCpcBid] = 1.25
	if _, err := p.Create(context.Background(), keywordResource(t, "brand_exact", attrs)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.Contains(fake.lastMutate, `"cpcBidMicros":"1250000"`) && !strings.Contains(fake.lastMutate, `"cpcBidMicros": "1250000"`) {
		t.Fatalf("create mutate missing cpc bid micros: %s", fake.lastMutate)
	}
	if !strings.Contains(strings.ToLower(fake.lastMutate), `"text":"brand"`) {
		t.Fatalf("create mutate missing normalized text: %s", fake.lastMutate)
	}
}

func TestCreateNegativeKeyword(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	p, _ := testKeywordProvider(t, fake)
	attrs := resolvedKeywordAttrs(t, "31")
	attrs[googleads.AttrText] = "competitor"
	attrs[googleads.AttrMatchType] = "PHRASE"
	attrs[googleads.AttrNegative] = true
	live, err := p.Create(context.Background(), keywordResource(t, "competitor", attrs))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Attributes[googleads.AttrNegative] != true {
		t.Fatalf("negative = %v, want true", live.Attributes[googleads.AttrNegative])
	}
	if !strings.Contains(fake.lastMutate, `"negative":true`) && !strings.Contains(fake.lastMutate, `"negative": true`) {
		t.Fatalf("create mutate missing negative: %s", fake.lastMutate)
	}
}

func TestCreateKeywordMissingAdGroupIdentity(t *testing.T) {
	t.Parallel()

	p, _ := testKeywordProvider(t, newKeywordFake())
	_, err := p.Create(context.Background(), keywordResource(t, "brand_exact", defaultKeywordAttrs(t)))
	if err == nil {
		t.Fatal("expected missing ad group identity error")
	}
	if !strings.Contains(err.Error(), "googleads.ad_group.brand") {
		t.Fatalf("error = %q, want ad group address", err)
	}
}

func TestCreateKeywordAPIError(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	fake.mutateStatus = http.StatusBadRequest
	p, _ := testKeywordProvider(t, fake)
	_, err := p.Create(context.Background(), keywordResource(t, "brand_exact", resolvedKeywordAttrs(t, "31")))
	if err == nil {
		t.Fatal("expected API error")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestUpdateKeyword(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	item := sampleSearchKeyword("31", "41", "brand", "EXACT", false)
	item["cpcBidMicros"] = "1000000"
	fake.seedKeyword(item)
	p, _ := testKeywordProvider(t, fake)
	bindAdGroupIdentity(t, p, "31")

	desired := keywordResource(t, "brand_exact", resource.Attributes{
		googleads.AttrAdGroup: resource.Resolved{
			Address:  mustAdGroupAddress(t, "brand"),
			Identity: resource.Identity{ID: "31"},
			Outputs:  resource.Attributes{"resourceName": "customers/" + testCustomerID + "/adGroups/31"},
		},
		googleads.AttrText:      "brand",
		googleads.AttrMatchType: "EXACT",
		googleads.AttrStatus:    "ENABLED",
		googleads.AttrCpcBid:    2,
	})
	live, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "31~41"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if live.Identity.ID != "31~41" {
		t.Fatalf("identity = %q, want 31~41", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrStatus] != "ENABLED" {
		t.Fatalf("status = %v, want ENABLED", live.Attributes[googleads.AttrStatus])
	}
	if !strings.Contains(fake.lastMutate, "updateMask") {
		t.Fatalf("update missing updateMask: %s", fake.lastMutate)
	}
	if strings.Contains(fake.lastMutate, `"keyword"`) && strings.Contains(fake.lastMutate, "updateMask") {
		t.Fatalf("update must not mutate immutable keyword fields: %s", fake.lastMutate)
	}
}

func TestUpdateKeywordNoOp(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	fake.seedKeyword(sampleSearchKeyword("31", "41", "brand", "EXACT", false))
	p, _ := testKeywordProvider(t, fake)
	bindAdGroupIdentity(t, p, "31")

	desired := keywordResource(t, "brand_exact", resolvedKeywordAttrs(t, "31"))
	if _, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "31~41"},
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("no-op update mutated remote: %v", fake.mutates)
	}
}

func TestUpdateKeywordRejectsImmutableText(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	fake.seedKeyword(sampleSearchKeyword("31", "41", "brand", "EXACT", false))
	p, _ := testKeywordProvider(t, fake)
	bindAdGroupIdentity(t, p, "31")

	desired := keywordResource(t, "brand_exact", resource.Attributes{
		googleads.AttrAdGroup:   resolvedKeywordAttrs(t, "31")[googleads.AttrAdGroup],
		googleads.AttrText:      "shoes",
		googleads.AttrMatchType: "EXACT",
	})
	_, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "31~41"},
	})
	if err == nil {
		t.Fatal("expected immutable text error")
	}
	if !strings.Contains(err.Error(), "immutable") || !strings.Contains(err.Error(), "text") {
		t.Fatalf("error = %q, want immutable text", err)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("rejected text change mutated remote: %v", fake.mutates)
	}
}

func TestUpdateKeywordRejectsImmutableMatchType(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	fake.seedKeyword(sampleSearchKeyword("31", "41", "brand", "EXACT", false))
	p, _ := testKeywordProvider(t, fake)
	bindAdGroupIdentity(t, p, "31")

	desired := keywordResource(t, "brand_exact", resource.Attributes{
		googleads.AttrAdGroup:   resolvedKeywordAttrs(t, "31")[googleads.AttrAdGroup],
		googleads.AttrText:      "brand",
		googleads.AttrMatchType: "PHRASE",
	})
	_, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "31~41"},
	})
	if err == nil {
		t.Fatal("expected immutable matchType error")
	}
	if !strings.Contains(err.Error(), "immutable") || !strings.Contains(err.Error(), "matchType") {
		t.Fatalf("error = %q, want immutable matchType", err)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("rejected matchType change mutated remote: %v", fake.mutates)
	}
}

func TestUpdateKeywordRejectsAdGroupChange(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	fake.seedKeyword(sampleSearchKeyword("31", "41", "brand", "EXACT", false))
	p, _ := testKeywordProvider(t, fake)
	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustAdGroupAddress(t, "brand"), resource.Identity{ID: "31"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustAdGroupAddress(t, "other"), resource.Identity{ID: "88"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	desired := keywordResource(t, "brand_exact", resource.Attributes{
		googleads.AttrAdGroup: resource.Resolved{
			Address:  mustAdGroupAddress(t, "other"),
			Identity: resource.Identity{ID: "88"},
		},
		googleads.AttrText:      "brand",
		googleads.AttrMatchType: "EXACT",
	})
	_, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "31~41"},
	})
	if err == nil {
		t.Fatal("expected immutable ad group error")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("error = %q, want immutable adGroup", err)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("rejected ad group change mutated remote: %v", fake.mutates)
	}
}

func TestUpdateKeywordAPIError(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	fake.seedKeyword(sampleSearchKeyword("31", "41", "brand", "EXACT", false))
	fake.mutateStatus = http.StatusBadRequest
	p, _ := testKeywordProvider(t, fake)
	bindAdGroupIdentity(t, p, "31")

	desired := keywordResource(t, "brand_exact", resource.Attributes{
		googleads.AttrAdGroup:   resolvedKeywordAttrs(t, "31")[googleads.AttrAdGroup],
		googleads.AttrText:      "brand",
		googleads.AttrMatchType: "EXACT",
		googleads.AttrStatus:    "ENABLED",
	})
	_, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "31~41"},
	})
	if err == nil {
		t.Fatal("expected API error")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestImportKeywordRequiresBoundAdGroup(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	fake.seedKeyword(sampleSearchKeyword("31", "41", "brand", "EXACT", false))
	p, _ := testKeywordProvider(t, fake)
	_, err := p.Import(context.Background(), mustKeywordAddress(t, "brand_exact"), "31~41")
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

func TestImportKeywordThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
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
	item := sampleSearchKeyword("31", "41", "brand", "EXACT", false)
	item["cpcBidMicros"] = "1500000"
	fake.seedKeyword(item)
	p, _ := testKeywordProvider(t, fake)

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

	got, err := importer.Run(context.Background(), mustKeywordAddress(t, "brand_exact"), "31~41", lookupGoogleAds(p), st)
	if err != nil {
		t.Fatalf("importer.Run: %v", err)
	}
	if got.Identity.ID != "31~41" {
		t.Fatalf("identity = %q, want 31~41", got.Identity.ID)
	}
	assertNoProviderSecret(t, got.YAML)
	for _, leak := range []string{"resourceName", "cpcBidMicros", "criterionId", testAccessToken} {
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

func TestNormalizeKeywordImportID(t *testing.T) {
	t.Parallel()

	p, _ := testKeywordProvider(t, nil)
	addr := mustKeywordAddress(t, "brand_exact")
	got, err := p.NormalizeImportID(addr, "31~41")
	if err != nil || got != "31~41" {
		t.Fatalf("composite = (%q, %v), want 31~41", got, err)
	}
	got, err = p.NormalizeImportID(addr, "customers/"+testCustomerID+"/adGroupCriteria/31~41")
	if err != nil || got != "31~41" {
		t.Fatalf("resource name = (%q, %v), want 31~41", got, err)
	}
	_, err = p.NormalizeImportID(addr, "abc")
	if err == nil || !strings.Contains(err.Error(), "not a valid Google Ads keyword id") {
		t.Fatalf("invalid id error = %v", err)
	}
}

func TestImportKeywordUnsupportedType(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	item := sampleSearchKeyword("31", "9", "brand", "EXACT", false)
	item["type"] = "LISTING_GROUP"
	fake.seedKeyword(item)
	p, _ := testKeywordProvider(t, fake)
	_, err := p.Import(context.Background(), mustKeywordAddress(t, "listing"), "31~9")
	if err == nil {
		t.Fatal("expected unsupported type error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported type must not look like not found")
	}
	if !strings.Contains(err.Error(), "KEYWORD") {
		t.Fatalf("error = %q, want KEYWORD guidance", err)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("unsupported import mutated remote: %v", fake.mutates)
	}
}

func TestImportKeywordRejectsURLSettings(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	item := sampleSearchKeyword("31", "9", "brand", "EXACT", false)
	item["finalUrls"] = []any{"https://example.com/landing"}
	fake.seedKeyword(item)
	p, _ := testKeywordProvider(t, fake)
	_, err := p.Import(context.Background(), mustKeywordAddress(t, "brand_exact"), "31~9")
	if err == nil {
		t.Fatal("expected unsupported URL settings error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported URL settings must not look like not found")
	}
	if !strings.Contains(err.Error(), "finalUrls") || !strings.Contains(err.Error(), "does not manage") {
		t.Fatalf("error = %q, want finalUrls guidance", err)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("unsupported import mutated remote: %v", fake.mutates)
	}
}

func TestImportKeywordNegativeThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
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
	item := sampleSearchKeyword("31", "42", "competitor", "PHRASE", true)
	item["status"] = "ENABLED"
	fake.seedKeyword(item)
	p, _ := testKeywordProvider(t, fake)

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

	got, err := importer.Run(context.Background(), mustKeywordAddress(t, "competitor_neg"), "31~42", lookupGoogleAds(p), st)
	if err != nil {
		t.Fatalf("importer.Run: %v", err)
	}
	if got.Identity.ID != "31~42" {
		t.Fatalf("identity = %q, want 31~42", got.Identity.ID)
	}
	if !strings.Contains(got.YAML, "negative: true") {
		t.Fatalf("generated YAML missing negative:\n%s", got.YAML)
	}
	if strings.Contains(got.YAML, "cpcBid") {
		t.Fatalf("negative keyword YAML leaked cpcBid:\n%s", got.YAML)
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

func TestImportKeywordNotFound(t *testing.T) {
	t.Parallel()

	p, _ := testKeywordProvider(t, newKeywordFake())
	_, err := p.Import(context.Background(), mustKeywordAddress(t, "brand_exact"), "31~41")
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Import = %v, want ErrNotFound", err)
	}
}

func TestImportKeywordInvalidID(t *testing.T) {
	t.Parallel()

	p, _ := testKeywordProvider(t, newKeywordFake())
	_, err := p.Import(context.Background(), mustKeywordAddress(t, "brand_exact"), "abc")
	if err == nil || !strings.Contains(err.Error(), "not a valid Google Ads keyword id") {
		t.Fatalf("Import = %v, want invalid id", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestPlanKeywordCreateWhenMissing(t *testing.T) {
	t.Parallel()

	p, _ := testKeywordProvider(t, newKeywordFake())
	got := mustPlanKeyword(t, p, keywordStack(t, defaultKeywordAttrs(t))...)
	if len(got.Changes) != 4 {
		t.Fatalf("changes = %+v, want 4", got.Changes)
	}
	byAddr := map[string]plan.Action{}
	for _, change := range got.Changes {
		byAddr[change.Address.String()] = change.Action
	}
	if byAddr["googleads.campaign_budget.brand"] != plan.ActionCreate || byAddr["googleads.campaign.brand"] != plan.ActionCreate || byAddr["googleads.ad_group.brand"] != plan.ActionCreate || byAddr["googleads.keyword.brand_exact"] != plan.ActionCreate {
		t.Fatalf("actions = %+v, want create/create/create/create", byAddr)
	}
}

func TestPlanKeywordUnchangedEquivalentRemote(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
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
	item := sampleSearchKeyword("31", "41", "brand", "EXACT", false)
	item["cpcBidMicros"] = "1500000"
	fake.seedKeyword(item)
	p, _ := testKeywordProvider(t, fake)

	attrs := defaultKeywordAttrs(t)
	attrs[googleads.AttrText] = "Brand"
	attrs[googleads.AttrMatchType] = "exact"
	attrs[googleads.AttrCpcBid] = "1.50"
	got := mustPlanKeyword(t, p, keywordStack(t, attrs)...)
	if got.HasChanges() {
		t.Fatalf("equivalent remote produced changes: %+v", got.Changes)
	}
}

func TestPlanKeywordOmittedStatusDefaultsPaused(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false, "period": "DAILY", "type": "STANDARD"})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	live := sampleSearchKeyword("31", "41", "brand", "EXACT", false)
	live["status"] = "ENABLED"
	fake.seedKeyword(live)
	p, _ := testKeywordProvider(t, fake)

	got := mustPlanKeyword(t, p, keywordStack(t, defaultKeywordAttrs(t))...)
	change := keywordChange(t, got)
	if change.Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want keyword update to PAUSED", got.Changes)
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

func TestPlanKeywordOmittedCpcBidIsNoOp(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false, "period": "DAILY", "type": "STANDARD"})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	item := sampleSearchKeyword("31", "41", "brand", "EXACT", false)
	item["cpcBidMicros"] = "2500000"
	fake.seedKeyword(item)
	p, _ := testKeywordProvider(t, fake)

	got := mustPlanKeyword(t, p, keywordStack(t, defaultKeywordAttrs(t))...)
	if got.HasChanges() {
		t.Fatalf("omitted cpcBid produced changes: %+v", got.Changes)
	}
}

func TestPlanKeywordImmutableTextIsVisible(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false, "period": "DAILY", "type": "STANDARD"})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	fake.seedKeyword(sampleSearchKeyword("31", "41", "brand", "EXACT", false))
	p, _ := testKeywordProvider(t, fake)

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
	if err := st.Bind(mustKeywordAddress(t, "brand_exact"), resource.Identity{ID: "31~41"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	attrs := defaultKeywordAttrs(t)
	attrs[googleads.AttrText] = "shoes"
	_, err := plan.BuildWithState(context.Background(), keywordStack(t, attrs), func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err == nil {
		t.Fatal("expected immutable text plan error")
	}
	if !strings.Contains(err.Error(), "immutable") || !strings.Contains(err.Error(), "text") {
		t.Fatalf("error = %q, want immutable text in plan output", err)
	}
}

func TestPlanKeywordIgnoresComputedFields(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false, "period": "DAILY", "type": "STANDARD"})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	item := sampleSearchKeyword("31", "41", "brand", "EXACT", false)
	item["qualityInfo"] = map[string]any{"qualityScore": 7}
	fake.seedKeyword(item)
	p, _ := testKeywordProvider(t, fake)

	got := mustPlanKeyword(t, p, keywordStack(t, defaultKeywordAttrs(t))...)
	if got.HasChanges() {
		t.Fatalf("computed fields produced changes: %+v", got.Changes)
	}
}

func mustPlanKeyword(t *testing.T, p *googleads.Provider, resources ...resource.Resource) *plan.Plan {
	t.Helper()
	got, err := plan.Build(context.Background(), resources, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	return got
}

func keywordChange(t *testing.T, got *plan.Plan) *plan.Change {
	t.Helper()
	addr := mustKeywordAddress(t, "brand_exact")
	for i := range got.Changes {
		if got.Changes[i].Address == addr {
			return &got.Changes[i]
		}
	}
	t.Fatalf("missing keyword change: %+v", got.Changes)
	return nil
}

func defaultKeywordAttrs(t *testing.T) resource.Attributes {
	t.Helper()
	return resource.Attributes{
		googleads.AttrAdGroup:   adGroupRef(t, "brand"),
		googleads.AttrText:      "brand",
		googleads.AttrMatchType: "EXACT",
	}
}

func resolvedKeywordAttrs(t *testing.T, adGroupID string) resource.Attributes {
	t.Helper()
	return resource.Attributes{
		googleads.AttrAdGroup: resource.Resolved{
			Address:  mustAdGroupAddress(t, "brand"),
			Identity: resource.Identity{ID: adGroupID},
			Outputs:  resource.Attributes{"resourceName": "customers/" + testCustomerID + "/adGroups/" + adGroupID},
		},
		googleads.AttrText:      "brand",
		googleads.AttrMatchType: "EXACT",
	}
}

func keywordResource(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{Address: mustKeywordAddress(t, name), Attributes: attrs}
}

func mustKeywordAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("googleads.keyword." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func adGroupRef(t *testing.T, name string) resource.Ref {
	t.Helper()
	return resource.Ref{Address: mustAdGroupAddress(t, name)}
}

func bindAdGroupIdentity(t *testing.T, p *googleads.Provider, adGroupID string) {
	t.Helper()
	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustAdGroupAddress(t, "brand"), resource.Identity{ID: adGroupID}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)
}

func keywordStack(t *testing.T, keywordAttrs resource.Attributes) []resource.Resource {
	t.Helper()
	return []resource.Resource{
		campaignBudgetResource(t, "brand", resource.Attributes{
			googleads.AttrName:             "Brand daily budget",
			googleads.AttrAmount:           50,
			googleads.AttrExplicitlyShared: false,
		}),
		campaignResource(t, "brand", defaultCampaignAttrs(t)),
		adGroupResource(t, "brand", defaultAdGroupAttrs(t)),
		keywordResource(t, "brand_exact", keywordAttrs),
	}
}

func testKeywordProvider(t *testing.T, fake *keywordFake) (*googleads.Provider, *httptest.Server) {
	t.Helper()
	if fake == nil {
		fake = newKeywordFake()
	}
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	t.Cleanup(srv.Close)
	cfg := validConfig(srv.URL)
	cfg.TokenURL = srv.URL + "/oauth/token"
	p := googleads.NewWithHTTPClient(cfg, srv.Client())
	return p, srv
}

func sampleSearchKeyword(adGroupID, criterionID, text, matchType string, negative bool) map[string]any {
	return map[string]any{
		"criterionId": criterionID,
		"adGroup":     "customers/" + testCustomerID + "/adGroups/" + adGroupID,
		"status":      "PAUSED",
		"type":        "KEYWORD",
		"negative":    negative,
		"keyword": map[string]any{
			"text":      text,
			"matchType": matchType,
		},
	}
}

type keywordFake struct {
	mu sync.Mutex

	nextID    int64
	keywords  map[string]map[string]any
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

func newKeywordFake() *keywordFake {
	return &keywordFake{
		keywords:  map[string]map[string]any{},
		adGroups:  map[string]map[string]any{},
		campaigns: map[string]map[string]any{},
		budgets:   map[string]map[string]any{},
	}
}

func (f *keywordFake) seedKeyword(item map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storeKeywordLocked(cloneMap(item))
}

func (f *keywordFake) seedAdGroup(group map[string]any) {
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

func (f *keywordFake) seedCampaign(campaign map[string]any) {
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

func (f *keywordFake) seedBudget(budget map[string]any) {
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

func (f *keywordFake) storeKeywordLocked(item map[string]any) {
	adGroup := stringify(item["adGroup"])
	adGroupID := strings.TrimPrefix(adGroup, "customers/"+testCustomerID+"/adGroups/")
	criterionID := stringify(item["criterionId"])
	id := adGroupID + "~" + criterionID
	if stringify(item["resourceName"]) == "" {
		item["resourceName"] = "customers/" + testCustomerID + "/adGroupCriteria/" + id
	}
	if stringify(item["type"]) == "" {
		item["type"] = "KEYWORD"
	}
	if stringify(item["status"]) == "" {
		item["status"] = "PAUSED"
	}
	f.keywords[id] = item
}

func (f *keywordFake) handler(w http.ResponseWriter, r *http.Request) {
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
		case strings.Contains(query, "from ad_group_criterion"):
			_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchKeywordsLocked(req.Query)})
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

func (f *keywordFake) searchKeywordsLocked(query string) []any {
	var out []any
	for _, item := range f.keywords {
		if matchesKeywordQuery(query, item) {
			out = append(out, map[string]any{"adGroupCriterion": cloneMap(item)})
		}
	}
	return out
}

func (f *keywordFake) searchAdGroupsLocked(query string) []any {
	var out []any
	for _, group := range f.adGroups {
		if matchesAdGroupQuery(query, group) {
			out = append(out, map[string]any{"adGroup": cloneMap(group)})
		}
	}
	return out
}

func (f *keywordFake) searchCampaignsLocked(query string) []any {
	var out []any
	for _, campaign := range f.campaigns {
		if matchesCampaignQuery(query, campaign) {
			out = append(out, map[string]any{"campaign": cloneMap(campaign)})
		}
	}
	return out
}

func (f *keywordFake) searchBudgetsLocked(query string) []any {
	var out []any
	for _, budget := range f.budgets {
		if matchesCampaignBudgetQuery(query, budget) {
			out = append(out, map[string]any{"campaignBudget": cloneMap(budget)})
		}
	}
	return out
}

func (f *keywordFake) mutateLocked(collection string, body []byte) (string, error) {
	var req struct {
		Operations []map[string]any `json:"operations"`
	}
	if err := json.Unmarshal(body, &req); err != nil || len(req.Operations) == 0 {
		return "", errors.New("malformed mutate")
	}
	op := req.Operations[0]
	if collection != "adGroupCriteria" {
		return "", errors.New("unexpected mutate " + collection)
	}
	if raw, ok := op["create"]; ok {
		item, _ := raw.(map[string]any)
		created := cloneMap(item)
		f.nextID++
		criterionID := strconv.FormatInt(f.nextID, 10)
		created["criterionId"] = criterionID
		if stringify(created["type"]) == "" {
			created["type"] = "KEYWORD"
		}
		if stringify(created["status"]) == "" {
			created["status"] = "PAUSED"
		}
		if _, ok := created["negative"]; !ok {
			created["negative"] = false
		}
		f.storeKeywordLocked(created)
		return stringify(created["resourceName"]), nil
	}
	if raw, ok := op["update"]; ok {
		item, _ := raw.(map[string]any)
		resourceName := stringify(item["resourceName"])
		id := strings.TrimPrefix(resourceName, "customers/"+testCustomerID+"/adGroupCriteria/")
		current, ok := f.keywords[id]
		if !ok {
			return "", errors.New("missing keyword")
		}
		merged := cloneMap(current)
		for k, v := range item {
			if k == "resourceName" {
				continue
			}
			merged[k] = v
		}
		f.storeKeywordLocked(merged)
		return resourceName, nil
	}
	return "", errors.New("unsupported mutate")
}

func matchesKeywordQuery(query string, item map[string]any) bool {
	adGroup := stringify(item["adGroup"])
	adGroupID := strings.TrimPrefix(adGroup, "customers/"+testCustomerID+"/adGroups/")
	criterionID := stringify(item["criterionId"])
	if stringify(item["status"]) == "REMOVED" {
		return false
	}
	if strings.Contains(query, "ad_group_criterion.criterion_id = ") {
		want := strings.TrimSpace(query[strings.Index(query, "ad_group_criterion.criterion_id = ")+len("ad_group_criterion.criterion_id = "):])
		if i := strings.IndexAny(want, " \n"); i >= 0 {
			want = want[:i]
		}
		if want != criterionID {
			return false
		}
		if strings.Contains(query, "ad_group.id = ") {
			wantGroup := strings.TrimSpace(query[strings.Index(query, "ad_group.id = ")+len("ad_group.id = "):])
			if i := strings.IndexAny(wantGroup, " \n"); i >= 0 {
				wantGroup = wantGroup[:i]
			}
			return wantGroup == adGroupID
		}
		return true
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
	info, _ := item["keyword"].(map[string]any)
	if strings.Contains(query, "ad_group_criterion.keyword.text = ") {
		start := strings.Index(query, "ad_group_criterion.keyword.text = ") + len("ad_group_criterion.keyword.text = ")
		rest := strings.TrimSpace(query[start:])
		if i := strings.Index(rest, " AND "); i >= 0 {
			rest = rest[:i]
		}
		rest = strings.Trim(strings.TrimSpace(rest), "'")
		if rest != stringify(info["text"]) {
			return false
		}
	}
	if strings.Contains(query, "ad_group_criterion.keyword.match_type = ") {
		start := strings.Index(query, "ad_group_criterion.keyword.match_type = ") + len("ad_group_criterion.keyword.match_type = ")
		rest := strings.TrimSpace(query[start:])
		if i := strings.Index(rest, " AND "); i >= 0 {
			rest = rest[:i]
		}
		rest = strings.Trim(strings.TrimSpace(rest), "'")
		if !strings.EqualFold(rest, stringify(info["matchType"])) {
			return false
		}
	}
	if strings.Contains(query, "ad_group_criterion.negative = ") {
		start := strings.Index(query, "ad_group_criterion.negative = ") + len("ad_group_criterion.negative = ")
		rest := strings.TrimSpace(query[start:])
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
