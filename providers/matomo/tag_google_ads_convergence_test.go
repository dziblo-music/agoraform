package matomo_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/matomo"
)

func TestPlanGoogleAdsConversionTagIgnoresOmittedOptionalRemoteFields(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{
		ID:            7,
		Name:          "google_ads_trial_started",
		Type:          "GoogleAdsConversion",
		FireTriggerID: 4,
		Parameters: map[string]any{
			"googleAdsConversionId":            "9988776655",
			"googleAdsConversionLabel":         "AbC-D_efG-h12",
			"googleAdsConversionValue":         "25",
			"googleAdsConversionCurrency":      "USD",
			"googleAdsConversionTransactionId": "order-9",
		},
	})
	p := testTagProvider(t, srv)

	got := mustPlanTag(t, p, trialStartedTrigger(t), tagResource(t, "google_ads_trial_started", validGoogleAdsTagAttrs(t)))
	if got.HasChanges() {
		t.Fatalf("omitted optional remote fields produced drift:\n%s", plan.Format(got))
	}
}

func TestApplyGoogleAdsConversionTagPreservedOptionalFieldsConverges(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{
		ID:            8,
		Name:          "google_ads_trial_started",
		Type:          "GoogleAdsConversion",
		FireTriggerID: 4,
		Parameters: map[string]any{
			"googleAdsConversionId":            "9988776655",
			"googleAdsConversionLabel":         "old-label",
			"googleAdsConversionValue":         "25",
			"googleAdsConversionCurrency":      "USD",
			"googleAdsConversionTransactionId": "order-9",
		},
	})
	p := testTagProvider(t, srv)
	trigger := trialStartedTrigger(t)
	attrs := validGoogleAdsTagAttrs(t)
	attrs[matomo.AttrConversionLabel] = "new-label"
	tag := tagResource(t, "google_ads_trial_started", attrs)

	st, err := state.New(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(trigger.Address, resource.Identity{ID: "4"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(tag.Address, resource.Identity{ID: "8"}); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	result, err := apply.Run(context.Background(), []resource.Resource{tag, trigger}, func(resource.Address) (provider.Provider, error) {
		return p, nil
	}, st, &out)
	if err != nil {
		t.Fatalf("apply.Run: %v\n%s", err, out.String())
	}
	if result.Created != 0 || result.Updated != 1 {
		t.Fatalf("result = %+v, want exactly one update", result)
	}
	vals := srv.lastUpdateValues()
	if vals.Get("parameters[googleAdsConversionValue]") != "25" {
		t.Fatalf("omitted conversion value was not preserved: %v", vals)
	}
	if vals.Get("parameters[googleAdsConversionCurrency]") != "USD" {
		t.Fatalf("omitted conversion currency was not preserved: %v", vals)
	}
	if vals.Get("parameters[googleAdsConversionTransactionId]") != "order-9" {
		t.Fatalf("omitted transaction id was not preserved: %v", vals)
	}

	got, err := plan.BuildWithState(context.Background(), []resource.Resource{tag, trigger}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("BuildWithState after apply: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("post-apply plan did not converge:\n%s", plan.Format(got))
	}
}

func TestImportGoogleAdsConversionTagRetainsOptionalRemoteFields(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{
		ID:            1,
		Name:          "Google Ads trial started",
		Type:          "GoogleAdsConversion",
		FireTriggerID: 4,
		Parameters: map[string]any{
			"googleAdsConversionId":            "9988776655",
			"googleAdsConversionLabel":         "AbC-D_efG-h12",
			"googleAdsConversionValue":         "25",
			"googleAdsConversionCurrency":      "USD",
			"googleAdsConversionTransactionId": "order-9",
		},
	})
	p := testTagProvider(t, srv)
	p.SetIdentityCatalog(boundIdentityCatalog(t, "matomo.trigger.trial_started", "4"))

	live, err := p.Import(context.Background(), mustTagAddress(t, "google_ads_trial_started"), "1")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if live.Attributes[matomo.AttrConversionValue] != "25" {
		t.Fatalf("conversionValue = %v, want 25", live.Attributes[matomo.AttrConversionValue])
	}
	if live.Attributes[matomo.AttrConversionCurrency] != "USD" {
		t.Fatalf("conversionCurrency = %v, want USD", live.Attributes[matomo.AttrConversionCurrency])
	}
	if live.Attributes[matomo.AttrConversionTransactionID] != "order-9" {
		t.Fatalf("conversionTransactionId = %v, want order-9", live.Attributes[matomo.AttrConversionTransactionID])
	}
}
