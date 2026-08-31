package matomo_test

import (
	"context"
	"errors"
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

func TestValidateGoogleAdsConversionTag(t *testing.T) {
	t.Parallel()

	p := testTagProvider(t, newTagServer(t))
	action := mustParseAddr(t, "googleads.conversion_action.trial_started")
	cases := []struct {
		name  string
		attrs resource.Attributes
	}{
		{name: "literals", attrs: validGoogleAdsTagAttrs(t)},
		{
			name: "output references",
			attrs: resource.Attributes{
				matomo.AttrType:    "googleAdsConversion",
				matomo.AttrTrigger: triggerRef(t, "trial_started"),
				matomo.AttrConversionID: resource.Ref{
					Address: action,
					Output:  "conversionId",
				},
				matomo.AttrConversionLabel: resource.Ref{
					Address: action,
					Output:  "conversionLabel",
				},
			},
		},
		{
			name: "optional fields",
			attrs: withTagAttr(withTagAttr(withTagAttr(validGoogleAdsTagAttrs(t),
				matomo.AttrConversionValue, "10"),
				matomo.AttrConversionCurrency, "USD"),
				matomo.AttrConversionTransactionID, "order-1"),
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := p.Validate(context.Background(), tagResource(t, "google_ads_trial_started", tc.attrs)); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestValidateGoogleAdsConversionTagErrors(t *testing.T) {
	t.Parallel()

	p := testTagProvider(t, newTagServer(t))
	addr := mustTagAddress(t, "google_ads_trial_started")
	action := mustParseAddr(t, "googleads.conversion_action.trial_started")
	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name:  "missing conversionId",
			attrs: resource.Attributes{matomo.AttrType: "googleAdsConversion", matomo.AttrTrigger: triggerRef(t, "trial_started"), matomo.AttrConversionLabel: "AbC-D"},
			want:  "missing required attribute \"conversionId\"",
		},
		{
			name:  "empty conversionLabel",
			attrs: resource.Attributes{matomo.AttrType: "googleAdsConversion", matomo.AttrTrigger: triggerRef(t, "trial_started"), matomo.AttrConversionID: "123", matomo.AttrConversionLabel: ""},
			want:  "non-empty",
		},
		{
			name: "address-only conversionId",
			attrs: resource.Attributes{
				matomo.AttrType:            "googleAdsConversion",
				matomo.AttrTrigger:         triggerRef(t, "trial_started"),
				matomo.AttrConversionID:    resource.Ref{Address: action},
				matomo.AttrConversionLabel: "AbC-D",
			},
			want: "matomo.variable",
		},
		{
			name:  "eventCategory not allowed",
			attrs: withTagAttr(validGoogleAdsTagAttrs(t), matomo.AttrEventCategory, "signup"),
			want:  "not supported for type",
		},
		{
			name:  "matomoConfiguration not allowed",
			attrs: withTagAttr(validGoogleAdsTagAttrs(t), matomo.AttrMatomoConfiguration, variableRef(t, "config")),
			want:  "not supported for type",
		},
		{
			name:  "conversion fields on analytics tag",
			attrs: withTagAttr(validTagAttrs(t), matomo.AttrConversionID, "123"),
			want:  "not supported for type",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := p.Validate(context.Background(), resource.Resource{Address: addr, Attributes: tc.attrs})
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

func TestCreateGoogleAdsConversionTag(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	p := testTagProvider(t, srv)
	res := tagResource(t, "google_ads_trial_started", resource.Attributes{
		matomo.AttrType:            "googleAdsConversion",
		matomo.AttrName:            "Google Ads trial started",
		matomo.AttrTrigger:         resolvedTrigger(t, "trial_started", "4"),
		matomo.AttrConversionID:    "9988776655",
		matomo.AttrConversionLabel: "AbC-D_efG-h12",
		matomo.AttrConversionValue: "0",
	})

	live, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID == "" {
		t.Fatal("create returned empty identity")
	}
	vals := srv.lastCreateValues()
	if vals.Get("type") != "GoogleAdsConversion" {
		t.Fatalf("create type = %q, want GoogleAdsConversion", vals.Get("type"))
	}
	if vals.Get("fireTriggerIds[0]") != "4" {
		t.Fatalf("create fireTriggerIds = %v", vals)
	}
	if vals.Get("parameters[googleAdsConversionId]") != "9988776655" {
		t.Fatalf("conversion id = %v", vals)
	}
	if vals.Get("parameters[googleAdsConversionLabel]") != "AbC-D_efG-h12" {
		t.Fatalf("conversion label = %v", vals)
	}
	if vals.Get("parameters[googleAdsConversionValue]") != "0" {
		t.Fatalf("conversion value = %v", vals)
	}
	if vals.Get("parameters[matomoConfig]") != "" {
		t.Fatal("google ads conversion tag must not send matomoConfig")
	}
	if vals.Get("parameters[trackingType]") != "" {
		t.Fatal("google ads conversion tag must not send trackingType")
	}
}

func TestCreateGoogleAdsConversionTagResolvesOutputs(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	p := testTagProvider(t, srv)
	res := tagResource(t, "google_ads_trial_started", resource.Attributes{
		matomo.AttrType:    "googleAdsConversion",
		matomo.AttrTrigger: resolvedTrigger(t, "trial_started", "4"),
		matomo.AttrConversionID: resource.Ref{
			Address: mustParseAddr(t, "googleads.conversion_action.trial_started"),
			Output:  "conversionId",
		},
		matomo.AttrConversionLabel: resource.Ref{
			Address: mustParseAddr(t, "googleads.conversion_action.trial_started"),
			Output:  "conversionLabel",
		},
	})
	if err := p.Validate(context.Background(), res); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	resolved := tagResource(t, "google_ads_trial_started", resource.Attributes{
		matomo.AttrType:            "googleAdsConversion",
		matomo.AttrTrigger:         resolvedTrigger(t, "trial_started", "4"),
		matomo.AttrConversionID:    "9988776655",
		matomo.AttrConversionLabel: "AbC-D_efG-h12",
	})
	if _, err := p.Create(context.Background(), resolved); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if srv.lastCreateValues().Get("parameters[googleAdsConversionId]") != "9988776655" {
		t.Fatalf("resolved conversion id = %v", srv.lastCreateValues())
	}
}

func TestCreateGoogleAdsConversionTagRejectsEmptyResolvedOutputs(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	p := testTagProvider(t, srv)
	_, err := p.Create(context.Background(), tagResource(t, "google_ads_trial_started", resource.Attributes{
		matomo.AttrType:            "googleAdsConversion",
		matomo.AttrTrigger:         resolvedTrigger(t, "trial_started", "4"),
		matomo.AttrConversionID:    "",
		matomo.AttrConversionLabel: "AbC-D",
	}))
	if err == nil {
		t.Fatal("expected empty conversion id error")
	}
	if srv.createCount() != 0 {
		t.Fatalf("creates = %d, want 0", srv.createCount())
	}
}

func TestUpdateGoogleAdsConversionTagPreservesUnmanagedFields(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{
		ID:            8,
		Name:          "Google Ads trial started",
		Type:          "GoogleAdsConversion",
		FireTriggerID: 4,
		Description:   "keep me",
		BlockIDs:      []int{2},
		Parameters: map[string]any{
			"googleAdsConversionId":            "9988776655",
			"googleAdsConversionLabel":         "old-label",
			"googleAdsConversionValue":         "25",
			"googleAdsConversionCurrency":      "USD",
			"googleAdsConversionTransactionId": "order-9",
			"unowned":                          "keep",
		},
	})
	p := testTagProvider(t, srv)

	desired := tagResource(t, "google_ads_trial_started", resource.Attributes{
		matomo.AttrType:            "googleAdsConversion",
		matomo.AttrName:            "Google Ads trial started",
		matomo.AttrTrigger:         resolvedTrigger(t, "trial_started", "4"),
		matomo.AttrConversionID:    "9988776655",
		matomo.AttrConversionLabel: "new-label",
	})
	if _, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "8"},
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	vals := srv.lastUpdateValues()
	if vals.Get("description") != "keep me" {
		t.Fatalf("description = %q, want preserved", vals.Get("description"))
	}
	if vals.Get("blockTriggerIds[0]") != "2" {
		t.Fatalf("blockTriggerIds = %v", vals)
	}
	if vals.Get("parameters[googleAdsConversionLabel]") != "new-label" {
		t.Fatalf("label = %v", vals)
	}
	if vals.Get("parameters[googleAdsConversionValue]") != "25" {
		t.Fatalf("omitted value was not preserved: %v", vals)
	}
	if vals.Get("parameters[unowned]") != "keep" {
		t.Fatalf("unowned parameter dropped: %v", vals)
	}
}

func TestPlanGoogleAdsConversionTagUnchangedEquivalentRemote(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{
		ID:            7,
		Name:          "google_ads_trial_started",
		Type:          "GoogleAdsConversion",
		FireTriggerID: 4,
		Description:   "ignored",
		Parameters: map[string]any{
			"googleAdsConversionId":    "AW-9988776655",
			"googleAdsConversionLabel": "AbC-D_efG-h12",
		},
	})
	p := testTagProvider(t, srv)

	got := mustPlanTag(t, p, trialStartedTrigger(t), tagResource(t, "google_ads_trial_started", validGoogleAdsTagAttrs(t)))
	if got.HasChanges() {
		t.Fatalf("equivalent remote produced changes: %+v", got.Changes)
	}
}

func TestPlanGoogleAdsConversionTagUnknownUntilApplyKeepsLogicalOutput(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	p := testTagProvider(t, srv)
	action := conversionActionResource(t)
	stub := &conversionActionStub{}
	tag := googleAdsOutputTag(t)

	got, err := plan.Build(context.Background(), []resource.Resource{action, trialStartedTrigger(t), tag}, func(addr resource.Address) (provider.Reader, error) {
		if addr.Provider == "googleads" {
			return stub, nil
		}
		return p, nil
	})
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	change := changeByAddr(t, got, "matomo.tag.google_ads_trial_started")
	if change.Action != plan.ActionCreate {
		t.Fatalf("change = %+v, want create", change)
	}
	out := plan.Format(got)
	if !strings.Contains(out, `output: "conversionId"`) || !strings.Contains(out, `output: "conversionLabel"`) {
		t.Fatalf("plan missing logical output selectors:\n%s", out)
	}
	if strings.Contains(out, "9988776655") || strings.Contains(out, "AbC-D") {
		t.Fatalf("plan fabricated conversion outputs:\n%s", out)
	}
}

func TestPlanGoogleAdsConversionTagNoOpWithMatchingOutput(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{
		ID:            7,
		Name:          "google_ads_trial_started",
		Type:          "GoogleAdsConversion",
		FireTriggerID: 4,
		Parameters: map[string]any{
			"googleAdsConversionId":    "9988776655",
			"googleAdsConversionLabel": "AbC-D_efG-h12",
		},
	})
	p := testTagProvider(t, srv)
	action := conversionActionResource(t)
	stub := &conversionActionStub{live: resource.RemoteResource{
		Address:    action.Address,
		Identity:   resource.Identity{ID: "12"},
		Attributes: resource.Attributes{"name": "Trial Started", "category": "SIGNUP"},
		Computed: resource.Attributes{
			"conversionId":    "9988776655",
			"conversionLabel": "AbC-D_efG-h12",
		},
	}}
	tag := googleAdsOutputTag(t)
	tag.Identity = resource.Identity{ID: "7"}

	st, err := state.New(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustTriggerAddress(t, "trial_started"), resource.Identity{ID: "4"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(tag.Address, resource.Identity{ID: "7"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(action.Address, resource.Identity{ID: "12"}); err != nil {
		t.Fatal(err)
	}

	got, err := plan.BuildWithState(context.Background(), []resource.Resource{action, trialStartedTrigger(t), tag}, func(addr resource.Address) (provider.Reader, error) {
		if addr.Provider == "googleads" {
			return stub, nil
		}
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("BuildWithState: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("matching conversion outputs produced changes:\n%s", plan.Format(got))
	}
}

func TestImportGoogleAdsConversionTagReconstructsUniqueOutputs(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{
		ID:            1,
		Name:          "Google Ads trial started",
		Type:          "GoogleAdsConversion",
		FireTriggerID: 4,
		Parameters: map[string]any{
			"googleAdsConversionId":    "AW-9988776655",
			"googleAdsConversionLabel": "AbC-D_efG-h12",
		},
	})
	p := testTagProvider(t, srv)
	p.SetIdentityCatalog(boundIdentityCatalog(t, "matomo.trigger.trial_started", "4"))
	action := mustParseAddr(t, "googleads.conversion_action.trial_started")
	p.SetOutputMatcher(staticOutputMatcher{
		provider.OutputMatchQuery{Provider: "googleads", ResourceType: "conversion_action", Output: "conversionId", Value: "9988776655"}:       action,
		provider.OutputMatchQuery{Provider: "googleads", ResourceType: "conversion_action", Output: "conversionLabel", Value: "AbC-D_efG-h12"}: action,
	})

	live, err := p.Import(context.Background(), mustTagAddress(t, "google_ads_trial_started"), "1")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	idRef, ok := resource.AsRef(live.Attributes[matomo.AttrConversionID])
	if !ok || !idRef.HasOutput() || idRef.Address != action || idRef.Output != "conversionId" {
		t.Fatalf("conversionId = %+v, want unique output ref", live.Attributes[matomo.AttrConversionID])
	}
	labelRef, ok := resource.AsRef(live.Attributes[matomo.AttrConversionLabel])
	if !ok || labelRef.Output != "conversionLabel" || labelRef.Address != action {
		t.Fatalf("conversionLabel = %+v, want unique output ref", live.Attributes[matomo.AttrConversionLabel])
	}
	if err := p.Validate(context.Background(), resource.Resource{Address: live.Address, Attributes: live.Attributes.Clone()}); err != nil {
		t.Fatalf("imported attributes must validate: %v", err)
	}
}

func TestImportGoogleAdsConversionTagLiteralWhenNoUniqueMatch(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{
		ID:            1,
		Name:          "Google Ads trial started",
		Type:          "GoogleAdsConversion",
		FireTriggerID: 4,
		Parameters: map[string]any{
			"googleAdsConversionId":    "9988776655",
			"googleAdsConversionLabel": "AbC-D_efG-h12",
		},
	})
	p := testTagProvider(t, srv)
	p.SetIdentityCatalog(boundIdentityCatalog(t, "matomo.trigger.trial_started", "4"))

	live, err := p.Import(context.Background(), mustTagAddress(t, "google_ads_trial_started"), "1")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if live.Attributes[matomo.AttrConversionID] != "9988776655" {
		t.Fatalf("conversionId = %v, want literal", live.Attributes[matomo.AttrConversionID])
	}
	if _, ok := resource.AsRef(live.Attributes[matomo.AttrConversionID]); ok {
		t.Fatal("absent match must not emit a guessed $ref")
	}
}

func TestImportGoogleAdsConversionTagLiteralWhenAmbiguous(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{
		ID:            1,
		Name:          "Google Ads trial started",
		Type:          "GoogleAdsConversion",
		FireTriggerID: 4,
		Parameters: map[string]any{
			"googleAdsConversionId":    "9988776655",
			"googleAdsConversionLabel": "AbC-D_efG-h12",
		},
	})
	p := testTagProvider(t, srv)
	p.SetIdentityCatalog(boundIdentityCatalog(t, "matomo.trigger.trial_started", "4"))
	p.SetOutputMatcher(ambiguousOutputMatcher{})

	live, err := p.Import(context.Background(), mustTagAddress(t, "google_ads_trial_started"), "1")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if live.Attributes[matomo.AttrConversionID] != "9988776655" {
		t.Fatalf("ambiguous conversionId = %v, want literal", live.Attributes[matomo.AttrConversionID])
	}
}

func TestImportGoogleAdsConversionTagMalformedParameters(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{
		ID:            1,
		Name:          "Google Ads trial started",
		Type:          "GoogleAdsConversion",
		FireTriggerID: 4,
		Parameters: map[string]any{
			"googleAdsConversionId":    []any{"not", "a", "string"},
			"googleAdsConversionLabel": "AbC-D",
		},
	})
	p := testTagProvider(t, srv)
	p.SetIdentityCatalog(boundIdentityCatalog(t, "matomo.trigger.trial_started", "4"))

	_, err := p.Import(context.Background(), mustTagAddress(t, "google_ads_trial_started"), "1")
	if err == nil {
		t.Fatal("expected malformed parameter error")
	}
	if !strings.Contains(err.Error(), "malformed") && !strings.Contains(err.Error(), "googleAdsConversionId") {
		t.Fatalf("error = %q, want malformed conversion id", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestImportGoogleAdsConversionThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{
		ID:            1,
		Name:          "Google Ads trial started",
		Type:          "GoogleAdsConversion",
		FireTriggerID: 4,
		Parameters: map[string]any{
			"googleAdsConversionId":    "9988776655",
			"googleAdsConversionLabel": "AbC-D_efG-h12",
		},
	})
	p := testTagProvider(t, srv)
	p.SetIdentityCatalog(boundIdentityCatalog(t, "matomo.trigger.trial_started", "4"))

	live, err := p.Import(context.Background(), mustTagAddress(t, "google_ads_trial_started"), "1")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	desired := resource.Resource{Address: live.Address, Attributes: live.Attributes.Clone(), Identity: live.Identity}
	got := mustPlanTag(t, p, trialStartedTrigger(t), desired)
	tagChange := changeByAddr(t, got, live.Address.String())
	if tagChange.Action != plan.ActionUnchanged {
		t.Fatalf("imported tag planned %s: %+v", tagChange.Action, tagChange.Diffs)
	}
}

func TestApplyGoogleAdsConversionTagAfterTrigger(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	p := testTagProvider(t, srv)
	trigger := trialStartedTrigger(t)
	tag := tagResource(t, "google_ads_trial_started", validGoogleAdsTagAttrs(t))
	st, err := state.New(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	result, err := apply.Run(context.Background(), []resource.Resource{tag, trigger}, func(resource.Address) (provider.Provider, error) {
		return p, nil
	}, st, &out)
	if err != nil {
		t.Fatalf("apply.Run: %v\n%s", err, out.String())
	}
	if result.Created != 2 {
		t.Fatalf("result = %+v, want 2 created", result)
	}
	progress := out.String()
	trig := strings.Index(progress, "matomo.trigger.trial_started: created")
	tg := strings.Index(progress, "matomo.tag.google_ads_trial_started: created")
	if trig < 0 || tg < 0 || trig > tg {
		t.Fatalf("apply order:\n%s", progress)
	}
	if srv.lastCreateValues().Get("fireTriggerIds[0]") == "" {
		t.Fatal("apply did not resolve trigger before tag create")
	}
}

func TestApplyGoogleAdsConversionTagAfterConversionAction(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	p := testTagProvider(t, srv)
	stub := &conversionActionStub{}
	action := conversionActionResource(t)
	trigger := trialStartedTrigger(t)
	tag := googleAdsOutputTag(t)
	st, err := state.New(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	result, err := apply.Run(context.Background(), []resource.Resource{tag, trigger, action}, func(addr resource.Address) (provider.Provider, error) {
		if addr.Provider == "googleads" {
			return stub, nil
		}
		return p, nil
	}, st, &out)
	if err != nil {
		t.Fatalf("apply.Run: %v\n%s", err, out.String())
	}
	if result.Created != 3 {
		t.Fatalf("result = %+v, want 3 created", result)
	}
	progress := out.String()
	actionIdx := strings.Index(progress, "googleads.conversion_action.trial_started: created")
	trigIdx := strings.Index(progress, "matomo.trigger.trial_started: created")
	tagIdx := strings.Index(progress, "matomo.tag.google_ads_trial_started: created")
	if actionIdx < 0 || trigIdx < 0 || tagIdx < 0 || actionIdx > tagIdx || trigIdx > tagIdx {
		t.Fatalf("apply order:\n%s", progress)
	}
	vals := srv.lastCreateValues()
	if vals.Get("parameters[googleAdsConversionId]") != "9988776655" {
		t.Fatalf("resolved conversion id = %v", vals)
	}
	if vals.Get("parameters[googleAdsConversionLabel]") != "AbC-D_efG-h12" {
		t.Fatalf("resolved conversion label = %v", vals)
	}
}

func TestReadGoogleAdsConversionTagMalformedResponse(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.malformed("TagManager.getContainerTags", `"oops `+providerToken+`"`)
	p := testTagProvider(t, srv)
	_, err := p.Read(context.Background(), tagResource(t, "google_ads_trial_started", validGoogleAdsTagAttrs(t)))
	if err == nil {
		t.Fatal("expected malformed error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("malformed response must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func validGoogleAdsTagAttrs(t *testing.T) resource.Attributes {
	t.Helper()
	return resource.Attributes{
		matomo.AttrType:            "googleAdsConversion",
		matomo.AttrTrigger:         triggerRef(t, "trial_started"),
		matomo.AttrConversionID:    "9988776655",
		matomo.AttrConversionLabel: "AbC-D_efG-h12",
	}
}

func googleAdsOutputTag(t *testing.T) resource.Resource {
	t.Helper()
	action := mustParseAddr(t, "googleads.conversion_action.trial_started")
	return tagResource(t, "google_ads_trial_started", resource.Attributes{
		matomo.AttrType:    "googleAdsConversion",
		matomo.AttrTrigger: triggerRef(t, "trial_started"),
		matomo.AttrConversionID: resource.Ref{
			Address: action,
			Output:  "conversionId",
		},
		matomo.AttrConversionLabel: resource.Ref{
			Address: action,
			Output:  "conversionLabel",
		},
	})
}

func conversionActionResource(t *testing.T) resource.Resource {
	t.Helper()
	return resource.Resource{
		Address: mustParseAddr(t, "googleads.conversion_action.trial_started"),
		Attributes: resource.Attributes{
			"name":     "Trial Started",
			"category": "SIGNUP",
		},
	}
}

func mustParseAddr(t *testing.T, s string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress(s)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

type conversionActionStub struct {
	live resource.RemoteResource
}

func (conversionActionStub) Name() string { return "googleads" }

func (conversionActionStub) ResourceTypes() []string { return []string{"conversion_action"} }

func (conversionActionStub) Outputs(resourceType string) []provider.OutputSpec {
	if resourceType != "conversion_action" {
		return nil
	}
	return []provider.OutputSpec{
		{Name: "conversionId", Kind: provider.OutputKindString},
		{Name: "conversionLabel", Kind: provider.OutputKindString},
	}
}

func (conversionActionStub) Validate(context.Context, resource.Resource) error { return nil }

func (s *conversionActionStub) Read(_ context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if s.live.Identity.IsZero() {
		return resource.RemoteResource{}, provider.ErrNotFound
	}
	live := s.live
	live.Address = res.Address
	return live, nil
}

func (s *conversionActionStub) Create(_ context.Context, res resource.Resource) (resource.RemoteResource, error) {
	s.live = resource.RemoteResource{
		Address:    res.Address,
		Identity:   resource.Identity{ID: "12"},
		Attributes: res.Attributes.Clone(),
		Computed: resource.Attributes{
			"conversionId":    "9988776655",
			"conversionLabel": "AbC-D_efG-h12",
		},
	}
	return s.live, nil
}

func (conversionActionStub) Update(context.Context, resource.Resource, resource.RemoteResource) (resource.RemoteResource, error) {
	return resource.RemoteResource{}, errors.New("unexpected conversion action update")
}

func (s *conversionActionStub) Import(_ context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	if s.live.Identity.ID != id {
		return resource.RemoteResource{}, provider.ErrNotFound
	}
	live := s.live
	live.Address = addr
	return live, nil
}

type staticOutputMatcher map[provider.OutputMatchQuery]resource.Address

func (m staticOutputMatcher) Match(_ context.Context, query provider.OutputMatchQuery) (resource.Address, provider.OutputMatch, error) {
	addr, ok := m[query]
	if !ok {
		return resource.Address{}, provider.OutputMatchNone, nil
	}
	return addr, provider.OutputMatchUnique, nil
}

type ambiguousOutputMatcher struct{}

func (ambiguousOutputMatcher) Match(context.Context, provider.OutputMatchQuery) (resource.Address, provider.OutputMatch, error) {
	return resource.Address{}, provider.OutputMatchAmbiguous, nil
}
