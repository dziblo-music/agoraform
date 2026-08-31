package matomo_test

import (
	"context"
	"testing"

	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo"
)

func TestImportGoogleAdsConversionTagDoesNotCrossMatchOutputPair(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{
		ID:            1,
		Name:          "Google Ads trial started",
		Type:          "GoogleAdsConversion",
		FireTriggerID: 4,
		Parameters: map[string]any{
			"googleAdsConversionId":    "AW-111",
			"googleAdsConversionLabel": "BBB",
		},
	})

	p := testTagProvider(t, srv)
	p.SetIdentityCatalog(boundIdentityCatalog(t, "matomo.trigger.trial_started", "4"))

	actionA := mustParseAddr(t, "googleads.conversion_action.action_a")
	actionB := mustParseAddr(t, "googleads.conversion_action.action_b")
	p.SetOutputMatcher(staticOutputMatcher{
		{Output: matomo.AttrConversionID, Value: "111", Address: actionA},
		{Output: matomo.AttrConversionLabel, Value: "BBB", Address: actionB},
	})

	live, err := p.Import(context.Background(), mustTagAddress(t, "google_ads_trial_started"), "1")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if ref, ok := resource.AsRef(live.Attributes[matomo.AttrConversionID]); ok {
		t.Fatalf("conversionId cross-matched to %+v; want literal", ref)
	}
	if ref, ok := resource.AsRef(live.Attributes[matomo.AttrConversionLabel]); ok {
		t.Fatalf("conversionLabel cross-matched to %+v; want literal", ref)
	}

	id, ok := live.Attributes[matomo.AttrConversionID].(string)
	if !ok || id != "AW-111" {
		t.Fatalf("conversionId = %#v, want literal AW-111", live.Attributes[matomo.AttrConversionID])
	}
	label, ok := live.Attributes[matomo.AttrConversionLabel].(string)
	if !ok || label != "BBB" {
		t.Fatalf("conversionLabel = %#v, want literal BBB", live.Attributes[matomo.AttrConversionLabel])
	}

	if srv.createCount() != 0 || srv.updateCount() != 0 {
		t.Fatalf("import mutated Matomo: creates=%d updates=%d", srv.createCount(), srv.updateCount())
	}
}
