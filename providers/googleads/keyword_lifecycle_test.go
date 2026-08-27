package googleads_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestCreateNegativeKeywordDefaultsEnabled(t *testing.T) {
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
	if live.Attributes[googleads.AttrStatus] != "ENABLED" {
		t.Fatalf("status = %v, want ENABLED", live.Attributes[googleads.AttrStatus])
	}
	if !strings.Contains(fake.lastMutate, `"status":"ENABLED"`) && !strings.Contains(fake.lastMutate, `"status": "ENABLED"`) {
		t.Fatalf("negative keyword create missing ENABLED status: %s", fake.lastMutate)
	}
}

func TestCreateNegativeKeywordRejectsPaused(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	p, _ := testKeywordProvider(t, fake)
	attrs := resolvedKeywordAttrs(t, "31")
	attrs[googleads.AttrText] = "competitor"
	attrs[googleads.AttrMatchType] = "PHRASE"
	attrs[googleads.AttrNegative] = true
	attrs[googleads.AttrStatus] = "PAUSED"

	_, err := p.Create(context.Background(), keywordResource(t, "competitor", attrs))
	if err == nil {
		t.Fatal("expected negative keyword create status error")
	}
	if !strings.Contains(err.Error(), "ENABLED") || !strings.Contains(err.Error(), "negative") {
		t.Fatalf("error = %q, want negative keyword ENABLED guidance", err)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("rejected negative keyword create mutated remote: %v", fake.mutates)
	}
}

func TestNormalizeNegativeKeywordDefaultsEnabled(t *testing.T) {
	t.Parallel()

	p, _ := testKeywordProvider(t, nil)
	attrs := resolvedKeywordAttrs(t, "31")
	attrs[googleads.AttrText] = "competitor"
	attrs[googleads.AttrMatchType] = "PHRASE"
	attrs[googleads.AttrNegative] = true

	want, got, err := p.NormalizeComparable(keywordResource(t, "competitor", attrs), nil)
	if err != nil {
		t.Fatalf("NormalizeComparable: %v", err)
	}
	if got != nil {
		t.Fatalf("got = %#v, want nil for create normalization", got)
	}
	if want[googleads.AttrStatus] != "ENABLED" {
		t.Fatalf("status = %v, want ENABLED", want[googleads.AttrStatus])
	}
}

func TestUpdateNegativeKeywordRejectsStatusChangeWithoutMutation(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	item := sampleSearchKeyword("31", "41", "competitor", "PHRASE", true)
	item["status"] = "PAUSED"
	fake.seedKeyword(item)
	p, _ := testKeywordProvider(t, fake)
	bindAdGroupIdentity(t, p, "31")

	attrs := resolvedKeywordAttrs(t, "31")
	attrs[googleads.AttrText] = "competitor"
	attrs[googleads.AttrMatchType] = "PHRASE"
	attrs[googleads.AttrNegative] = true
	attrs[googleads.AttrStatus] = "ENABLED"
	desired := keywordResource(t, "competitor", attrs)

	_, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "31~41"},
	})
	if err == nil {
		t.Fatal("expected immutable negative keyword status error")
	}
	if !strings.Contains(err.Error(), "status is immutable") || !strings.Contains(err.Error(), "negative") {
		t.Fatalf("error = %q, want immutable negative keyword status guidance", err)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("rejected negative keyword update mutated remote: %v", fake.mutates)
	}
}

func TestPlanNegativeKeywordStatusChangeFailsWithoutMutation(t *testing.T) {
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
	item := sampleSearchKeyword("31", "41", "competitor", "PHRASE", true)
	item["status"] = "PAUSED"
	fake.seedKeyword(item)
	p, _ := testKeywordProvider(t, fake)

	st := mustGoogleAdsImportStore(t)
	for addr, id := range map[resource.Address]string{
		mustCampaignBudgetAddress(t, "brand"): "11",
		mustCampaignAddress(t, "brand"):       "21",
		mustAdGroupAddress(t, "brand"):        "31",
		mustKeywordAddress(t, "brand_exact"):  "31~41",
	} {
		if err := st.Bind(addr, resource.Identity{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	p.SetIdentityCatalog(st)

	attrs := defaultKeywordAttrs(t)
	attrs[googleads.AttrText] = "competitor"
	attrs[googleads.AttrMatchType] = "PHRASE"
	attrs[googleads.AttrNegative] = true
	attrs[googleads.AttrStatus] = "ENABLED"
	_, err := plan.BuildWithState(context.Background(), keywordStack(t, attrs), func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err == nil {
		t.Fatal("expected immutable negative keyword status plan error")
	}
	if !strings.Contains(err.Error(), "status is immutable") || !strings.Contains(err.Error(), "negative") {
		t.Fatalf("error = %q, want immutable negative keyword status guidance", err)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("plan mutated remote: %v", fake.mutates)
	}
}
