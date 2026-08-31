package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/internal/provider"
)

func TestImportMatomoGoogleAdsConversionTagReconstructsOutputRefs(t *testing.T) {
	t.Parallel()

	ads, adsSrv := googleAdsImportProvider(t)
	adsSrv.seedAction(googleAdsConversionActionSeed("12", "Trial Started"))

	matomoP, tmSrv := matomoTagManagerServerProvider(t)
	tmSrv.seedTrigger(cliTMTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	tmSrv.seedTag(cliTMTag{
		ID:            1,
		Name:          "Google Ads trial started",
		Type:          "GoogleAdsConversion",
		FireTriggerID: 4,
		Parameters: map[string]any{
			"googleAdsConversionId":    "AW-9988776655",
			"googleAdsConversionLabel": "AbC-D_efG-h12",
		},
	})

	reg := provider.NewRegistry()
	if err := reg.Register(ads); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(matomoP); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")

	actionYAML := importCLI(t, reg, manifestPath, "googleads.conversion_action.trial_started", "12")
	triggerYAML := importCLI(t, reg, manifestPath, "matomo.trigger.trial_started", "4")

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "matomo.tag.google_ads_trial_started", "1"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("tag import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	tagOut := stdout.String()
	assertGoogleAdsImportOutputClean(t, tagOut)
	if strings.Contains(tagOut, "cli-test-token") || strings.Contains(tagOut, "9988776655") || strings.Contains(tagOut, "AbC-D_efG-h12") {
		t.Fatalf("tag import leaked conversion values or secrets:\n%s", tagOut)
	}
	if !strings.Contains(tagOut, "$ref: googleads.conversion_action.trial_started") {
		t.Fatalf("tag import missing conversion action $ref:\n%s", tagOut)
	}
	if !strings.Contains(tagOut, "output: conversionId") || !strings.Contains(tagOut, "output: conversionLabel") {
		t.Fatalf("tag import missing { $ref, output } YAML:\n%s", tagOut)
	}
	if !strings.Contains(tagOut, "$ref: matomo.trigger.trial_started") {
		t.Fatalf("tag import missing trigger $ref:\n%s", tagOut)
	}

	combined := combineManifestResources(t, actionYAML, triggerYAML, extractYAML(tagOut))
	if err := os.WriteFile(manifestPath, []byte(combined), 0o600); err != nil {
		t.Fatal(err)
	}
	assertPersistedRemoteID(t, manifestPath, "googleads.conversion_action.trial_started", "12")
	assertPersistedRemoteID(t, manifestPath, "matomo.trigger.trial_started", "4")
	assertPersistedRemoteID(t, manifestPath, "matomo.tag.google_ads_trial_started", "1")

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after cross-provider import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after cross-provider import = %q", stdout.String())
	}
	if adsSrv.mutateCount() != 0 {
		t.Fatalf("google ads mutated: %d", adsSrv.mutateCount())
	}
	if tmSrv.mutationCount() != 0 {
		t.Fatalf("matomo mutated: %d", tmSrv.mutationCount())
	}
}

func TestImportMatomoGoogleAdsConversionTagLiteralWhenNoMatch(t *testing.T) {
	t.Parallel()

	matomoP, tmSrv := matomoTagManagerServerProvider(t)
	tmSrv.seedTrigger(cliTMTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	tmSrv.seedTag(cliTMTag{
		ID:            1,
		Name:          "Google Ads trial started",
		Type:          "GoogleAdsConversion",
		FireTriggerID: 4,
		Parameters: map[string]any{
			"googleAdsConversionId":    "9988776655",
			"googleAdsConversionLabel": "AbC-D_efG-h12",
		},
	})

	reg := provider.NewRegistry()
	if err := reg.Register(matomoP); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	triggerYAML := importCLI(t, reg, manifestPath, "matomo.trigger.trial_started", "4")

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "matomo.tag.google_ads_trial_started", "1"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("tag import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	tagOut := stdout.String()
	if strings.Contains(tagOut, "$ref: googleads.conversion_action") {
		t.Fatalf("absent match must not emit a guessed conversion $ref:\n%s", tagOut)
	}
	if !strings.Contains(tagOut, "9988776655") {
		t.Fatalf("absent match should emit conversionId literal:\n%s", tagOut)
	}

	combined := combineManifestResources(t, triggerYAML, extractYAML(tagOut))
	if err := os.WriteFile(manifestPath, []byte(combined), 0o600); err != nil {
		t.Fatal(err)
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after literal import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after literal import = %q", stdout.String())
	}
	if tmSrv.mutationCount() != 0 {
		t.Fatalf("matomo mutated: %d", tmSrv.mutationCount())
	}
}

func TestImportMatomoGoogleAdsConversionTagLiteralWhenAmbiguous(t *testing.T) {
	t.Parallel()

	ads, adsSrv := googleAdsImportProvider(t)
	adsSrv.seedAction(googleAdsConversionActionSeed("12", "Trial Started"))
	adsSrv.seedAction(googleAdsConversionActionSeed("13", "Trial Started Copy"))

	matomoP, tmSrv := matomoTagManagerServerProvider(t)
	tmSrv.seedTrigger(cliTMTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	tmSrv.seedTag(cliTMTag{
		ID:            1,
		Name:          "Google Ads trial started",
		Type:          "GoogleAdsConversion",
		FireTriggerID: 4,
		Parameters: map[string]any{
			"googleAdsConversionId":    "9988776655",
			"googleAdsConversionLabel": "AbC-D_efG-h12",
		},
	})

	reg := provider.NewRegistry()
	if err := reg.Register(ads); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(matomoP); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	importCLI(t, reg, manifestPath, "googleads.conversion_action.trial_started", "12")
	importCLI(t, reg, manifestPath, "googleads.conversion_action.trial_started_copy", "13")
	importCLI(t, reg, manifestPath, "matomo.trigger.trial_started", "4")

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "matomo.tag.google_ads_trial_started", "1"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("tag import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	tagOut := stdout.String()
	if strings.Contains(tagOut, "$ref: googleads.conversion_action") {
		t.Fatalf("ambiguous match must not guess a conversion $ref:\n%s", tagOut)
	}
	if adsSrv.mutateCount() != 0 {
		t.Fatalf("google ads mutated: %d", adsSrv.mutateCount())
	}
	if tmSrv.mutationCount() != 0 {
		t.Fatalf("matomo mutated: %d", tmSrv.mutationCount())
	}
}

func importCLI(t *testing.T, reg *provider.Registry, manifestPath, address, remoteID string) string {
	t.Helper()
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, address, remoteID}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import %s exit = %d; stderr=%q stdout=%q", address, code, stderr.String(), stdout.String())
	}
	return extractYAML(stdout.String())
}

func googleAdsConversionActionSeed(id, name string) map[string]any {
	return map[string]any{
		"id":             id,
		"name":           name,
		"category":       "SIGNUP",
		"status":         "ENABLED",
		"type":           "WEBPAGE",
		"origin":         "WEBSITE",
		"countingType":   "ONE_PER_CLICK",
		"primaryForGoal": true,
		"valueSettings": map[string]any{
			"defaultValue":          0.0,
			"defaultCurrencyCode":   "USD",
			"alwaysUseDefaultValue": true,
		},
		"tagSnippets": []any{
			map[string]any{
				"eventSnippet": "gtag('event', 'conversion', {'send_to': 'AW-9988776655/AbC-D_efG-h12'});",
			},
		},
	}
}
