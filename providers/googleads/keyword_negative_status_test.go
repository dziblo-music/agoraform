package googleads_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestValidateNegativeKeywordRejectsPausedStatus(t *testing.T) {
	t.Parallel()

	p, _ := testKeywordProvider(t, nil)
	attrs := defaultKeywordAttrs(t)
	attrs[googleads.AttrText] = "competitor"
	attrs[googleads.AttrMatchType] = "PHRASE"
	attrs[googleads.AttrNegative] = true
	attrs[googleads.AttrStatus] = "PAUSED"

	err := p.Validate(context.Background(), keywordResource(t, "competitor_neg", attrs))
	if err == nil {
		t.Fatal("expected negative keyword status validation error")
	}
	if !strings.Contains(err.Error(), "negative keywords") || !strings.Contains(err.Error(), "ENABLED") {
		t.Fatalf("error = %q, want negative keyword ENABLED guidance", err)
	}
}

func TestCreateNegativeKeywordDefaultsEnabled(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	p, _ := testKeywordProvider(t, fake)
	attrs := resolvedKeywordAttrs(t, "31")
	attrs[googleads.AttrText] = "competitor"
	attrs[googleads.AttrMatchType] = "PHRASE"
	attrs[googleads.AttrNegative] = true

	live, err := p.Create(context.Background(), keywordResource(t, "competitor_neg", attrs))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Attributes[googleads.AttrStatus] != "ENABLED" {
		t.Fatalf("status = %v, want ENABLED", live.Attributes[googleads.AttrStatus])
	}
	if !strings.Contains(fake.lastMutate, `"status":"ENABLED"`) && !strings.Contains(fake.lastMutate, `"status": "ENABLED"`) {
		t.Fatalf("create mutate missing ENABLED: %s", fake.lastMutate)
	}
}

func TestPlanNegativeKeywordOmittedStatusDefaultsEnabled(t *testing.T) {
	t.Parallel()

	fake := newKeywordFake()
	fake.seedBudget(map[string]any{
		"id":               "11",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
	})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	fake.seedAdGroup(sampleSearchAdGroup("31", "Brand", "21"))
	live := sampleSearchKeyword("31", "42", "competitor", "PHRASE", true)
	live["status"] = "ENABLED"
	fake.seedKeyword(live)
	p, _ := testKeywordProvider(t, fake)

	attrs := resource.Attributes{
		googleads.AttrAdGroup:   adGroupRef(t, "brand"),
		googleads.AttrText:      "competitor",
		googleads.AttrMatchType: "PHRASE",
		googleads.AttrNegative:  true,
	}
	got := mustPlanKeyword(t, p, keywordStack(t, attrs)...)
	if got.HasChanges() {
		t.Fatalf("omitted negative keyword status produced changes: %+v", got.Changes)
	}
}
