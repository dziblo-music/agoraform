package cli_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

const (
	cliGoogleAdsCustomerID     = "1234567890"
	cliGoogleAdsDeveloperToken = "cli-test-developer-token"
	cliGoogleAdsClientSecret   = "cli-test-client-secret"
	cliGoogleAdsRefreshToken   = "cli-test-refresh-token"
	cliGoogleAdsAccessToken    = "cli-test-access-token"
)

func TestImportGoogleAdsConversionActionThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.seedAction(map[string]any{
		"id":             "12",
		"name":           "Trial Started",
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
			map[string]any{"eventSnippet": "gtag('event', 'conversion', {'send_to': 'AW-1/" + cliGoogleAdsAccessToken + "'});"},
		},
	})

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.conversion_action.trial_started", "12"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	out := stdout.String()
	assertGoogleAdsImportOutputClean(t, out)
	if !strings.Contains(out, "Imported googleads.conversion_action.trial_started (remote identity 12).") {
		t.Fatalf("stdout missing import confirmation:\n%s", out)
	}

	yamlText := extractYAML(out)
	parsed, err := manifest.Parse([]byte(yamlText), "generated")
	if err != nil {
		t.Fatalf("generated YAML: %v\n%s", err, yamlText)
	}
	if parsed.Resources[0].Attributes["name"] != "Trial Started" {
		t.Fatalf("imported name = %v", parsed.Resources[0].Attributes["name"])
	}
	for _, key := range []string{"id", "resourceName", "type", "tagSnippets", "conversionId", "conversionLabel"} {
		if _, ok := parsed.Resources[0].Attributes[key]; ok {
			t.Fatalf("computed %s present in generated attributes:\n%s", key, yamlText)
		}
	}
	if err := os.WriteFile(manifestPath, []byte(yamlText), 0o600); err != nil {
		t.Fatal(err)
	}
	assertPersistedRemoteID(t, manifestPath, "googleads.conversion_action.trial_started", "12")

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after import = %q, want no changes", stdout.String())
	}
	if srv.mutateCount() != 0 {
		t.Fatalf("conversion action import mutated remote: %d", srv.mutateCount())
	}
}

func TestImportGoogleAdsConversionActionByResourceName(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.seedAction(map[string]any{
		"id":       "12",
		"name":     "Trial Started",
		"category": "SIGNUP",
		"type":     "WEBPAGE",
	})
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{
		"import", "-f", manifestPath, "googleads.conversion_action.trial_started",
		"customers/" + cliGoogleAdsCustomerID + "/conversionActions/12",
	}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "(remote identity 12)") {
		t.Fatalf("stdout should report canonical numeric identity:\n%s", stdout.String())
	}
	assertPersistedRemoteID(t, manifestPath, "googleads.conversion_action.trial_started", "12")
	if srv.mutateCount() != 0 {
		t.Fatalf("resource-name import mutated remote: %d", srv.mutateCount())
	}
}

func TestImportGoogleAdsCustomerConversionGoalThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.seedGoal(map[string]any{"category": "SIGNUP", "origin": "WEBSITE", "biddable": true})

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.customer_conversion_goal.signup", "signup~website"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	out := stdout.String()
	assertGoogleAdsImportOutputClean(t, out)
	if !strings.Contains(out, "Imported googleads.customer_conversion_goal.signup (remote identity SIGNUP~WEBSITE).") {
		t.Fatalf("stdout missing canonical identity confirmation:\n%s", out)
	}
	if strings.Contains(out, "conversionAction") || strings.Contains(out, "$ref") {
		t.Fatalf("goal import reconstructed conversionAction:\n%s", out)
	}

	yamlText := extractYAML(out)
	if err := os.WriteFile(manifestPath, []byte(yamlText), 0o600); err != nil {
		t.Fatal(err)
	}
	assertPersistedRemoteID(t, manifestPath, "googleads.customer_conversion_goal.signup", "SIGNUP~WEBSITE")

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after import = %q, want no changes", stdout.String())
	}
	if srv.mutateCount() != 0 {
		t.Fatalf("conversion goal import mutated remote: %d", srv.mutateCount())
	}
}

func TestImportGoogleAdsConversionActionUnsupportedType(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.seedAction(map[string]any{
		"id":       "3",
		"name":     "App Install",
		"category": "DOWNLOAD",
		"type":     "GOOGLE_PLAY_DOWNLOAD",
	})
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", emptyResourcesManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, "googleads.conversion_action.app_install", "3"}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "WEBPAGE") {
		t.Fatalf("stderr = %q, want WEBPAGE guidance", stderr.String())
	}
	if srv.mutateCount() != 0 {
		t.Fatalf("unsupported import mutated remote: %d", srv.mutateCount())
	}
}

func TestImportGoogleAdsConversionActionInvalidID(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsImportProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", emptyResourcesManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, "googleads.conversion_action.trial_started", "abc"}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "not a valid Google Ads conversion action id") {
		t.Fatalf("stderr = %q, want invalid id diagnostic", stderr.String())
	}
}

func TestImportGoogleAdsConversionActionNotFound(t *testing.T) {
	t.Parallel()

	p, _ := googleAdsImportProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", emptyResourcesManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, "googleads.conversion_action.trial_started", "12"}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "was not found") {
		t.Fatalf("stderr = %q, want not found", stderr.String())
	}
}

func TestImportGoogleAdsConversionActionAPIError(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.searchStatus = http.StatusForbidden
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", emptyResourcesManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, "googleads.conversion_action.trial_started", "12"}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if strings.Contains(stderr.String(), cliGoogleAdsAccessToken) || strings.Contains(stderr.String(), cliGoogleAdsDeveloperToken) {
		t.Fatalf("API error leaked secret:\n%s", stderr.String())
	}
}

func TestImportGoogleAdsCampaignBudgetThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.seedBudget(map[string]any{
		"id":               "12",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
		"status":           "ENABLED",
		"referenceCount":   "1",
	})

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.campaign_budget.brand", "12"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	out := stdout.String()
	assertGoogleAdsImportOutputClean(t, out)
	if !strings.Contains(out, "Imported googleads.campaign_budget.brand (remote identity 12).") {
		t.Fatalf("stdout missing import confirmation:\n%s", out)
	}

	yamlText := extractYAML(out)
	parsed, err := manifest.Parse([]byte(yamlText), "generated")
	if err != nil {
		t.Fatalf("generated YAML: %v\n%s", err, yamlText)
	}
	if parsed.Resources[0].Attributes["name"] != "Brand daily budget" {
		t.Fatalf("imported name = %v", parsed.Resources[0].Attributes["name"])
	}
	for _, key := range []string{"id", "resourceName", "amountMicros", "period", "type", "status", "referenceCount"} {
		if _, ok := parsed.Resources[0].Attributes[key]; ok {
			t.Fatalf("computed %s present in generated attributes:\n%s", key, yamlText)
		}
	}
	if err := os.WriteFile(manifestPath, []byte(yamlText), 0o600); err != nil {
		t.Fatal(err)
	}
	assertPersistedRemoteID(t, manifestPath, "googleads.campaign_budget.brand", "12")

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after import = %q, want no changes", stdout.String())
	}
	if srv.mutateCount() != 0 {
		t.Fatalf("campaign budget import mutated remote: %d", srv.mutateCount())
	}
}

func TestImportGoogleAdsCampaignBudgetByResourceName(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.seedBudget(map[string]any{
		"id":               "12",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
	})
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{
		"import", "-f", manifestPath, "googleads.campaign_budget.brand",
		"customers/" + cliGoogleAdsCustomerID + "/campaignBudgets/12",
	}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "(remote identity 12)") {
		t.Fatalf("stdout should report canonical numeric identity:\n%s", stdout.String())
	}
	assertPersistedRemoteID(t, manifestPath, "googleads.campaign_budget.brand", "12")
	if srv.mutateCount() != 0 {
		t.Fatalf("resource-name import mutated remote: %d", srv.mutateCount())
	}
}

func TestImportGoogleAdsSearchCampaignThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.seedBudget(map[string]any{
		"id":               "11",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
	})
	srv.seedCampaign(map[string]any{
		"id":                     "21",
		"name":                   "Brand",
		"status":                 "PAUSED",
		"advertisingChannelType": "SEARCH",
		"campaignBudget":         "customers/" + cliGoogleAdsCustomerID + "/campaignBudgets/11",
		"biddingStrategyType":    "MANUAL_CPC",
		"manualCpc":              map[string]any{"enhancedCpcEnabled": false},
		"networkSettings": map[string]any{
			"targetGoogleSearch":         true,
			"targetSearchNetwork":        true,
			"targetContentNetwork":       false,
			"targetPartnerSearchNetwork": false,
		},
	})

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.campaign_budget.brand", "11"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("budget import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	budgetYAML := extractYAML(stdout.String())

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.campaign.brand", "21"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("campaign import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	out := stdout.String()
	assertGoogleAdsImportOutputClean(t, out)
	if !strings.Contains(out, "Imported googleads.campaign.brand (remote identity 21).") {
		t.Fatalf("stdout missing import confirmation:\n%s", out)
	}
	campaignYAML := extractYAML(out)
	if !strings.Contains(campaignYAML, "$ref: googleads.campaign_budget.brand") {
		t.Fatalf("campaign YAML missing budget $ref:\n%s", campaignYAML)
	}

	itemStart := strings.Index(campaignYAML, "  - address:")
	if itemStart < 0 {
		t.Fatalf("campaign YAML missing resource item:\n%s", campaignYAML)
	}
	combined := strings.TrimRight(budgetYAML, "\n") + "\n" + campaignYAML[itemStart:]
	if err := os.WriteFile(manifestPath, []byte(combined), 0o600); err != nil {
		t.Fatal(err)
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after import = %q, want no changes", stdout.String())
	}
	if srv.mutateCount() != 0 {
		t.Fatalf("search campaign import mutated remote: %d", srv.mutateCount())
	}
}

func TestImportGoogleAdsCampaignConversionGoalThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.seedBudget(map[string]any{
		"id":               "11",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
	})
	srv.seedCampaign(map[string]any{
		"id":                     "21",
		"name":                   "Brand",
		"status":                 "PAUSED",
		"advertisingChannelType": "SEARCH",
		"campaignBudget":         "customers/" + cliGoogleAdsCustomerID + "/campaignBudgets/11",
		"biddingStrategyType":    "MANUAL_CPC",
		"manualCpc":              map[string]any{"enhancedCpcEnabled": false},
		"networkSettings": map[string]any{
			"targetGoogleSearch":         true,
			"targetSearchNetwork":        true,
			"targetContentNetwork":       false,
			"targetPartnerSearchNetwork": false,
		},
	})
	srv.seedCampaignGoal(map[string]any{
		"campaignId": "21",
		"category":   "SIGNUP",
		"origin":     "WEBSITE",
		"biddable":   true,
	})

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.campaign_budget.brand", "11"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("budget import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	budgetYAML := extractYAML(stdout.String())

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.campaign.brand", "21"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("campaign import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	campaignYAML := extractYAML(stdout.String())

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.campaign_conversion_goal.trial_signup", "21~signup~website"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("goal import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	out := stdout.String()
	assertGoogleAdsImportOutputClean(t, out)
	if !strings.Contains(out, "Imported googleads.campaign_conversion_goal.trial_signup (remote identity 21~SIGNUP~WEBSITE).") {
		t.Fatalf("stdout missing import confirmation:\n%s", out)
	}
	goalYAML := extractYAML(out)
	if !strings.Contains(goalYAML, "$ref: googleads.campaign.brand") {
		t.Fatalf("goal YAML missing campaign $ref:\n%s", goalYAML)
	}
	if strings.Contains(goalYAML, "conversionAction") || strings.Contains(goalYAML, "resourceName") {
		t.Fatalf("goal YAML leaked computed fields:\n%s", goalYAML)
	}

	itemStart := strings.Index(campaignYAML, "  - address:")
	if itemStart < 0 {
		t.Fatalf("campaign YAML missing resource item:\n%s", campaignYAML)
	}
	goalStart := strings.Index(goalYAML, "  - address:")
	if goalStart < 0 {
		t.Fatalf("goal YAML missing resource item:\n%s", goalYAML)
	}
	combined := strings.TrimRight(budgetYAML, "\n") + "\n" + strings.TrimRight(campaignYAML[itemStart:], "\n") + "\n" + goalYAML[goalStart:]
	if err := os.WriteFile(manifestPath, []byte(combined), 0o600); err != nil {
		t.Fatal(err)
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after import = %q, want no changes", stdout.String())
	}
	if srv.mutateCount() != 0 {
		t.Fatalf("campaign conversion goal import mutated remote: %d", srv.mutateCount())
	}
}

func TestImportGoogleAdsAdGroupThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.seedBudget(map[string]any{
		"id":               "11",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
	})
	srv.seedCampaign(map[string]any{
		"id":                     "21",
		"name":                   "Brand",
		"status":                 "PAUSED",
		"advertisingChannelType": "SEARCH",
		"campaignBudget":         "customers/" + cliGoogleAdsCustomerID + "/campaignBudgets/11",
		"biddingStrategyType":    "MANUAL_CPC",
		"manualCpc":              map[string]any{"enhancedCpcEnabled": false},
		"networkSettings": map[string]any{
			"targetGoogleSearch":         true,
			"targetSearchNetwork":        true,
			"targetContentNetwork":       false,
			"targetPartnerSearchNetwork": false,
		},
	})
	srv.seedAdGroup(map[string]any{
		"id":           "31",
		"name":         "Brand",
		"status":       "PAUSED",
		"type":         "SEARCH_STANDARD",
		"campaign":     "customers/" + cliGoogleAdsCustomerID + "/campaigns/21",
		"cpcBidMicros": "1500000",
	})

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.campaign_budget.brand", "11"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("budget import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	budgetYAML := extractYAML(stdout.String())

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.campaign.brand", "21"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("campaign import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	campaignYAML := extractYAML(stdout.String())

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.ad_group.brand", "31"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("ad group import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	out := stdout.String()
	assertGoogleAdsImportOutputClean(t, out)
	if !strings.Contains(out, "Imported googleads.ad_group.brand (remote identity 31).") {
		t.Fatalf("stdout missing import confirmation:\n%s", out)
	}
	groupYAML := extractYAML(out)
	if !strings.Contains(groupYAML, "$ref: googleads.campaign.brand") {
		t.Fatalf("ad group YAML missing campaign $ref:\n%s", groupYAML)
	}
	if strings.Contains(groupYAML, "resourceName") || strings.Contains(groupYAML, "cpcBidMicros") {
		t.Fatalf("ad group YAML leaked computed fields:\n%s", groupYAML)
	}

	itemStart := strings.Index(campaignYAML, "  - address:")
	if itemStart < 0 {
		t.Fatalf("campaign YAML missing resource item:\n%s", campaignYAML)
	}
	groupStart := strings.Index(groupYAML, "  - address:")
	if groupStart < 0 {
		t.Fatalf("ad group YAML missing resource item:\n%s", groupYAML)
	}
	combined := strings.TrimRight(budgetYAML, "\n") + "\n" + strings.TrimRight(campaignYAML[itemStart:], "\n") + "\n" + groupYAML[groupStart:]
	if err := os.WriteFile(manifestPath, []byte(combined), 0o600); err != nil {
		t.Fatal(err)
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after import = %q, want no changes", stdout.String())
	}
	if srv.mutateCount() != 0 {
		t.Fatalf("ad group import mutated remote: %d", srv.mutateCount())
	}
}

func TestImportGoogleAdsKeywordThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.seedBudget(map[string]any{
		"id":               "11",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
	})
	srv.seedCampaign(map[string]any{
		"id":                     "21",
		"name":                   "Brand",
		"status":                 "PAUSED",
		"advertisingChannelType": "SEARCH",
		"campaignBudget":         "customers/" + cliGoogleAdsCustomerID + "/campaignBudgets/11",
		"biddingStrategyType":    "MANUAL_CPC",
		"manualCpc":              map[string]any{"enhancedCpcEnabled": false},
		"networkSettings": map[string]any{
			"targetGoogleSearch":         true,
			"targetSearchNetwork":        true,
			"targetContentNetwork":       false,
			"targetPartnerSearchNetwork": false,
		},
	})
	srv.seedAdGroup(map[string]any{
		"id":           "31",
		"name":         "Brand",
		"status":       "PAUSED",
		"type":         "SEARCH_STANDARD",
		"campaign":     "customers/" + cliGoogleAdsCustomerID + "/campaigns/21",
		"cpcBidMicros": "1500000",
	})
	srv.seedKeyword(map[string]any{
		"criterionId":  "41",
		"adGroup":      "customers/" + cliGoogleAdsCustomerID + "/adGroups/31",
		"status":       "PAUSED",
		"type":         "KEYWORD",
		"negative":     false,
		"cpcBidMicros": "1500000",
		"keyword": map[string]any{
			"text":      "brand",
			"matchType": "EXACT",
		},
	})

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.campaign_budget.brand", "11"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("budget import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	budgetYAML := extractYAML(stdout.String())

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.campaign.brand", "21"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("campaign import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	campaignYAML := extractYAML(stdout.String())

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.ad_group.brand", "31"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("ad group import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	groupYAML := extractYAML(stdout.String())

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.keyword.brand_exact", "31~41"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("keyword import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	out := stdout.String()
	assertGoogleAdsImportOutputClean(t, out)
	if !strings.Contains(out, "Imported googleads.keyword.brand_exact (remote identity 31~41).") {
		t.Fatalf("stdout missing import confirmation:\n%s", out)
	}
	keywordYAML := extractYAML(out)
	if !strings.Contains(keywordYAML, "$ref: googleads.ad_group.brand") {
		t.Fatalf("keyword YAML missing ad group $ref:\n%s", keywordYAML)
	}
	if strings.Contains(keywordYAML, "resourceName") || strings.Contains(keywordYAML, "cpcBidMicros") {
		t.Fatalf("keyword YAML leaked computed fields:\n%s", keywordYAML)
	}

	itemStart := strings.Index(campaignYAML, "  - address:")
	if itemStart < 0 {
		t.Fatalf("campaign YAML missing resource item:\n%s", campaignYAML)
	}
	groupStart := strings.Index(groupYAML, "  - address:")
	if groupStart < 0 {
		t.Fatalf("ad group YAML missing resource item:\n%s", groupYAML)
	}
	keywordStart := strings.Index(keywordYAML, "  - address:")
	if keywordStart < 0 {
		t.Fatalf("keyword YAML missing resource item:\n%s", keywordYAML)
	}
	combined := strings.TrimRight(budgetYAML, "\n") + "\n" + strings.TrimRight(campaignYAML[itemStart:], "\n") + "\n" + strings.TrimRight(groupYAML[groupStart:], "\n") + "\n" + keywordYAML[keywordStart:]
	if err := os.WriteFile(manifestPath, []byte(combined), 0o600); err != nil {
		t.Fatal(err)
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after import = %q, want no changes", stdout.String())
	}
	if srv.mutateCount() != 0 {
		t.Fatalf("keyword import mutated remote: %d", srv.mutateCount())
	}
}

func TestImportGoogleAdsResponsiveSearchAdThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.seedBudget(map[string]any{
		"id":               "11",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
	})
	srv.seedCampaign(map[string]any{
		"id":                     "21",
		"name":                   "Brand",
		"status":                 "PAUSED",
		"advertisingChannelType": "SEARCH",
		"campaignBudget":         "customers/" + cliGoogleAdsCustomerID + "/campaignBudgets/11",
		"biddingStrategyType":    "MANUAL_CPC",
		"manualCpc":              map[string]any{"enhancedCpcEnabled": false},
	})
	srv.seedAdGroup(map[string]any{
		"id":           "31",
		"name":         "Brand",
		"status":       "PAUSED",
		"type":         "SEARCH_STANDARD",
		"campaign":     "customers/" + cliGoogleAdsCustomerID + "/campaigns/21",
		"cpcBidMicros": "1500000",
	})
	srv.seedAd(map[string]any{
		"adGroup": "customers/" + cliGoogleAdsCustomerID + "/adGroups/31",
		"status":  "PAUSED",
		"ad": map[string]any{
			"id":        "71",
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
				"path1": "shoes",
			},
		},
	})

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.campaign_budget.brand", "11"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("budget import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	budgetYAML := extractYAML(stdout.String())

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.campaign.brand", "21"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("campaign import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	campaignYAML := extractYAML(stdout.String())

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.ad_group.brand", "31"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("ad group import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	groupYAML := extractYAML(stdout.String())

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.responsive_search_ad.brand", "31~71"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("rsa import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	out := stdout.String()
	assertGoogleAdsImportOutputClean(t, out)
	if !strings.Contains(out, "Imported googleads.responsive_search_ad.brand (remote identity 31~71).") {
		t.Fatalf("stdout missing import confirmation:\n%s", out)
	}
	rsaYAML := extractYAML(out)
	if !strings.Contains(rsaYAML, "$ref: googleads.ad_group.brand") {
		t.Fatalf("rsa YAML missing ad group $ref:\n%s", rsaYAML)
	}
	if strings.Contains(rsaYAML, "resourceName") || strings.Contains(rsaYAML, "assetPerformanceLabel") || strings.Contains(rsaYAML, "adId") {
		t.Fatalf("rsa YAML leaked computed fields:\n%s", rsaYAML)
	}

	itemStart := strings.Index(campaignYAML, "  - address:")
	if itemStart < 0 {
		t.Fatalf("campaign YAML missing resource item:\n%s", campaignYAML)
	}
	groupStart := strings.Index(groupYAML, "  - address:")
	if groupStart < 0 {
		t.Fatalf("ad group YAML missing resource item:\n%s", groupYAML)
	}
	rsaStart := strings.Index(rsaYAML, "  - address:")
	if rsaStart < 0 {
		t.Fatalf("rsa YAML missing resource item:\n%s", rsaYAML)
	}
	combined := strings.TrimRight(budgetYAML, "\n") + "\n" + strings.TrimRight(campaignYAML[itemStart:], "\n") + "\n" + strings.TrimRight(groupYAML[groupStart:], "\n") + "\n" + rsaYAML[rsaStart:]
	if err := os.WriteFile(manifestPath, []byte(combined), 0o600); err != nil {
		t.Fatal(err)
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after import = %q, want no changes", stdout.String())
	}
	if srv.mutateCount() != 0 {
		t.Fatalf("rsa import mutated remote: %d", srv.mutateCount())
	}
}

func TestImportGoogleAdsCampaignLocationAndLanguage(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.seedBudget(map[string]any{
		"id":               "11",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
	})
	srv.seedCampaign(map[string]any{
		"id":                     "21",
		"name":                   "Brand",
		"status":                 "PAUSED",
		"advertisingChannelType": "SEARCH",
		"campaignBudget":         "customers/" + cliGoogleAdsCustomerID + "/campaignBudgets/11",
		"biddingStrategyType":    "MANUAL_CPC",
		"manualCpc":              map[string]any{"enhancedCpcEnabled": false},
		"geoTargetTypeSetting": map[string]any{
			"positiveGeoTargetType": "PRESENCE",
			"negativeGeoTargetType": "PRESENCE",
		},
	})
	srv.seedGeo(map[string]any{
		"id":            "2840",
		"name":          "United States",
		"canonicalName": "United States",
		"countryCode":   "US",
		"targetType":    "Country",
		"status":        "ENABLED",
	})
	srv.seedLanguage(map[string]any{
		"id":         "1000",
		"code":       "en",
		"name":       "English",
		"targetable": true,
	})
	srv.seedCriterion(map[string]any{
		"criterionId": "41",
		"campaign":    "customers/" + cliGoogleAdsCustomerID + "/campaigns/21",
		"type":        "LOCATION",
		"status":      "ENABLED",
		"negative":    false,
		"location":    map[string]any{"geoTargetConstant": "geoTargetConstants/2840"},
	})
	srv.seedCriterion(map[string]any{
		"criterionId": "51",
		"campaign":    "customers/" + cliGoogleAdsCustomerID + "/campaigns/21",
		"type":        "LANGUAGE",
		"status":      "ENABLED",
		"language":    map[string]any{"languageConstant": "languageConstants/1000"},
	})

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.campaign_budget.brand", "11"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("budget import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.campaign.brand", "21"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("campaign import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	campaignYAML := extractYAML(stdout.String())
	if !strings.Contains(campaignYAML, "locationTargeting") || !strings.Contains(campaignYAML, "PRESENCE") {
		t.Fatalf("campaign YAML missing location targeting:\n%s", campaignYAML)
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.campaign_location.united_states", "21~41"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("location import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	out := stdout.String()
	assertGoogleAdsImportOutputClean(t, out)
	if !strings.Contains(out, "Imported googleads.campaign_location.united_states (remote identity 21~41).") {
		t.Fatalf("stdout missing location import confirmation:\n%s", out)
	}
	locationYAML := extractYAML(out)
	if !strings.Contains(locationYAML, "$ref: googleads.campaign.brand") {
		t.Fatalf("location YAML missing campaign $ref:\n%s", locationYAML)
	}
	if !strings.Contains(locationYAML, "United States") {
		t.Fatalf("location YAML missing canonical name:\n%s", locationYAML)
	}
	if strings.Contains(locationYAML, "geoTargetConstants") {
		t.Fatalf("location YAML leaked geo target constant:\n%s", locationYAML)
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "googleads.campaign_language.english", "21~51"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("language import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	out = stdout.String()
	assertGoogleAdsImportOutputClean(t, out)
	languageYAML := extractYAML(out)
	if !strings.Contains(languageYAML, "language: en") {
		t.Fatalf("language YAML missing ISO code:\n%s", languageYAML)
	}
	if strings.Contains(languageYAML, "languageConstants") {
		t.Fatalf("language YAML leaked language constant:\n%s", languageYAML)
	}
	if srv.mutateCount() != 0 {
		t.Fatalf("targeting import mutated remote: %d", srv.mutateCount())
	}
}

func TestImportGoogleAdsYAMLIsDeterministic(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.seedAction(map[string]any{
		"id":       "12",
		"name":     "Trial Started",
		"category": "SIGNUP",
		"type":     "WEBPAGE",
		"status":   "ENABLED",
	})
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	first := importGoogleAdsYAML(t, p, "googleads.conversion_action.trial_started", "12")
	second := importGoogleAdsYAML(t, p, "googleads.conversion_action.trial_started", "12")
	if first != second {
		t.Fatalf("YAML differed:\n%s\n---\n%s", first, second)
	}
}

func importGoogleAdsYAML(t *testing.T, p *googleads.Provider, address, remoteID string) string {
	t.Helper()
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, address, remoteID}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	return extractYAML(stdout.String())
}

func assertGoogleAdsImportOutputClean(t *testing.T, out string) {
	t.Helper()
	for _, secret := range []string{cliGoogleAdsDeveloperToken, cliGoogleAdsClientSecret, cliGoogleAdsRefreshToken, cliGoogleAdsAccessToken} {
		if strings.Contains(out, secret) {
			t.Fatalf("import output leaked secret %q:\n%s", secret, out)
		}
	}
}

func googleAdsImportProvider(t *testing.T) (*googleads.Provider, *cliGoogleAdsFake) {
	t.Helper()
	srv := newCLIGoogleAdsServer(t)
	p := googleads.NewWithHTTPClient(googleads.Config{
		DeveloperToken: cliGoogleAdsDeveloperToken,
		ClientID:       "cli-test-client-id",
		ClientSecret:   cliGoogleAdsClientSecret,
		RefreshToken:   cliGoogleAdsRefreshToken,
		CustomerID:     cliGoogleAdsCustomerID,
		BaseURL:        srv.URL,
		TokenURL:       srv.URL + "/oauth/token",
		HTTPClient:     srv.Client(),
	}, srv.Client())
	return p, srv.fake
}

type cliGoogleAdsServer struct {
	*httptest.Server
	fake *cliGoogleAdsFake
}

func newCLIGoogleAdsServer(t *testing.T) *cliGoogleAdsServer {
	t.Helper()
	fake := &cliGoogleAdsFake{
		actions:       map[string]map[string]any{},
		goals:         map[string]map[string]any{},
		campaignGoals: map[string]map[string]any{},
		budgets:       map[string]map[string]any{},
		campaigns:     map[string]map[string]any{},
		adGroups:      map[string]map[string]any{},
		keywords:      map[string]map[string]any{},
		ads:           map[string]map[string]any{},
		criteria:      map[string]map[string]any{},
		geos:          map[string]map[string]any{},
		languages:     map[string]map[string]any{},
	}
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	t.Cleanup(srv.Close)
	return &cliGoogleAdsServer{Server: srv, fake: fake}
}

type cliGoogleAdsFake struct {
	mu            sync.Mutex
	actions       map[string]map[string]any
	goals         map[string]map[string]any
	campaignGoals map[string]map[string]any
	budgets       map[string]map[string]any
	campaigns     map[string]map[string]any
	adGroups      map[string]map[string]any
	keywords      map[string]map[string]any
	ads           map[string]map[string]any
	criteria      map[string]map[string]any
	geos          map[string]map[string]any
	languages     map[string]map[string]any
	searchStatus  int
	mutates       int
}

func (f *cliGoogleAdsFake) seedAction(action map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cloned := cloneAnyMap(action)
	id := stringifyAny(cloned["id"])
	if stringifyAny(cloned["resourceName"]) == "" {
		cloned["resourceName"] = "customers/" + cliGoogleAdsCustomerID + "/conversionActions/" + id
	}
	f.actions[id] = cloned
}

func (f *cliGoogleAdsFake) seedGoal(goal map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cloned := cloneAnyMap(goal)
	id := strings.ToUpper(stringifyAny(cloned["category"])) + "~" + strings.ToUpper(stringifyAny(cloned["origin"]))
	if stringifyAny(cloned["resourceName"]) == "" {
		cloned["resourceName"] = "customers/" + cliGoogleAdsCustomerID + "/customerConversionGoals/" + id
	}
	f.goals[id] = cloned
}

func (f *cliGoogleAdsFake) seedBudget(budget map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cloned := cloneAnyMap(budget)
	id := stringifyAny(cloned["id"])
	if stringifyAny(cloned["resourceName"]) == "" {
		cloned["resourceName"] = "customers/" + cliGoogleAdsCustomerID + "/campaignBudgets/" + id
	}
	if stringifyAny(cloned["period"]) == "" {
		cloned["period"] = "DAILY"
	}
	if stringifyAny(cloned["type"]) == "" {
		cloned["type"] = "STANDARD"
	}
	f.budgets[id] = cloned
}

func (f *cliGoogleAdsFake) seedCampaignGoal(goal map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cloned := cloneAnyMap(goal)
	campaignID := stringifyAny(cloned["campaignId"])
	id := campaignID + "~" + strings.ToUpper(stringifyAny(cloned["category"])) + "~" + strings.ToUpper(stringifyAny(cloned["origin"]))
	if stringifyAny(cloned["resourceName"]) == "" {
		cloned["resourceName"] = "customers/" + cliGoogleAdsCustomerID + "/campaignConversionGoals/" + id
	}
	if stringifyAny(cloned["campaign"]) == "" {
		cloned["campaign"] = "customers/" + cliGoogleAdsCustomerID + "/campaigns/" + campaignID
	}
	f.campaignGoals[id] = cloned
}

func (f *cliGoogleAdsFake) seedCampaign(campaign map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cloned := cloneAnyMap(campaign)
	id := stringifyAny(cloned["id"])
	if stringifyAny(cloned["resourceName"]) == "" {
		cloned["resourceName"] = "customers/" + cliGoogleAdsCustomerID + "/campaigns/" + id
	}
	if stringifyAny(cloned["advertisingChannelType"]) == "" {
		cloned["advertisingChannelType"] = "SEARCH"
	}
	f.campaigns[id] = cloned
}

func (f *cliGoogleAdsFake) seedAdGroup(group map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cloned := cloneAnyMap(group)
	id := stringifyAny(cloned["id"])
	if stringifyAny(cloned["resourceName"]) == "" {
		cloned["resourceName"] = "customers/" + cliGoogleAdsCustomerID + "/adGroups/" + id
	}
	if stringifyAny(cloned["type"]) == "" {
		cloned["type"] = "SEARCH_STANDARD"
	}
	f.adGroups[id] = cloned
}

func (f *cliGoogleAdsFake) seedKeyword(item map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cloned := cloneAnyMap(item)
	adGroup := stringifyAny(cloned["adGroup"])
	adGroupID := strings.TrimPrefix(adGroup, "customers/"+cliGoogleAdsCustomerID+"/adGroups/")
	criterionID := stringifyAny(cloned["criterionId"])
	id := adGroupID + "~" + criterionID
	if stringifyAny(cloned["resourceName"]) == "" {
		cloned["resourceName"] = "customers/" + cliGoogleAdsCustomerID + "/adGroupCriteria/" + id
	}
	if stringifyAny(cloned["type"]) == "" {
		cloned["type"] = "KEYWORD"
	}
	f.keywords[id] = cloned
}

func (f *cliGoogleAdsFake) seedAd(item map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cloned := cloneAnyMap(item)
	adGroup := stringifyAny(cloned["adGroup"])
	adGroupID := strings.TrimPrefix(adGroup, "customers/"+cliGoogleAdsCustomerID+"/adGroups/")
	ad, _ := cloned["ad"].(map[string]any)
	adID := stringifyAny(ad["id"])
	id := adGroupID + "~" + adID
	if stringifyAny(cloned["resourceName"]) == "" {
		cloned["resourceName"] = "customers/" + cliGoogleAdsCustomerID + "/adGroupAds/" + id
	}
	if ad != nil && stringifyAny(ad["resourceName"]) == "" {
		ad["resourceName"] = "customers/" + cliGoogleAdsCustomerID + "/ads/" + adID
	}
	if ad != nil && stringifyAny(ad["type"]) == "" {
		ad["type"] = "RESPONSIVE_SEARCH_AD"
	}
	f.ads[id] = cloned
}

func (f *cliGoogleAdsFake) seedCriterion(item map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cloned := cloneAnyMap(item)
	campaign := stringifyAny(cloned["campaign"])
	campaignID := strings.TrimPrefix(campaign, "customers/"+cliGoogleAdsCustomerID+"/campaigns/")
	criterionID := stringifyAny(cloned["criterionId"])
	id := campaignID + "~" + criterionID
	if stringifyAny(cloned["resourceName"]) == "" {
		cloned["resourceName"] = "customers/" + cliGoogleAdsCustomerID + "/campaignCriteria/" + id
	}
	f.criteria[id] = cloned
}

func (f *cliGoogleAdsFake) seedGeo(item map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cloned := cloneAnyMap(item)
	id := stringifyAny(cloned["id"])
	if stringifyAny(cloned["resourceName"]) == "" {
		cloned["resourceName"] = "geoTargetConstants/" + id
	}
	f.geos[id] = cloned
}

func (f *cliGoogleAdsFake) seedLanguage(item map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cloned := cloneAnyMap(item)
	id := stringifyAny(cloned["id"])
	if stringifyAny(cloned["resourceName"]) == "" {
		cloned["resourceName"] = "languageConstants/" + id
	}
	f.languages[id] = cloned
}

func (f *cliGoogleAdsFake) mutateCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mutates
}

func (f *cliGoogleAdsFake) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, _ := io.ReadAll(r.Body)

	if strings.HasSuffix(r.URL.Path, "/oauth/token") {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"`+cliGoogleAdsAccessToken+`","expires_in":3600,"token_type":"Bearer"}`)
		return
	}
	if strings.Contains(r.URL.Path, ":mutate") {
		f.mutates++
		http.Error(w, `{"error":{"message":"unexpected mutate `+cliGoogleAdsDeveloperToken+`"}}`, http.StatusBadRequest)
		return
	}
	if strings.Contains(r.URL.Path, "geoTargetConstants:suggest") {
		http.NotFound(w, r)
		return
	}
	if !strings.Contains(r.URL.Path, "/googleAds:search") {
		http.NotFound(w, r)
		return
	}

	var req struct {
		Query string `json:"query"`
	}
	_ = json.Unmarshal(body, &req)
	query := strings.ToLower(req.Query)
	w.Header().Set("Content-Type", "application/json")
	if f.searchStatus >= 400 && !strings.Contains(query, "from customer") {
		w.WriteHeader(f.searchStatus)
		_, _ = io.WriteString(w, `{"error":{"code":403,"message":"query failed `+cliGoogleAdsAccessToken+`","status":"PERMISSION_DENIED"}}`)
		return
	}
	if strings.Contains(query, "from customer_conversion_goal") {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchGoalsLocked(req.Query)})
		return
	}
	if strings.Contains(query, "from campaign_criterion") {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchCriteriaLocked(req.Query)})
		return
	}
	if strings.Contains(query, "from geo_target_constant") {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchGeosLocked(req.Query)})
		return
	}
	if strings.Contains(query, "from language_constant") {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchLanguagesLocked(req.Query)})
		return
	}
	if strings.Contains(query, "from ad_group_criterion") {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchKeywordsLocked(req.Query)})
		return
	}
	if strings.Contains(query, "from ad_group_ad") {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchAdsLocked(req.Query)})
		return
	}
	if strings.Contains(query, "from ad_group") {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchAdGroupsLocked(req.Query)})
		return
	}
	if strings.Contains(query, "from campaign_conversion_goal") {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchCampaignGoalsLocked(req.Query)})
		return
	}
	if strings.Contains(query, "from campaign_budget") {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchBudgetsLocked(req.Query)})
		return
	}
	if strings.Contains(query, "from campaign") {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchCampaignsLocked(req.Query)})
		return
	}
	if strings.Contains(query, "from customer") {
		_, _ = io.WriteString(w, `{"results":[{"customer":{"id":"`+cliGoogleAdsCustomerID+`"}}]}`)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchActionsLocked(req.Query)})
}

func (f *cliGoogleAdsFake) searchActionsLocked(query string) []any {
	var out []any
	for id, action := range f.actions {
		if strings.Contains(query, "conversion_action.id = ") {
			want := strings.TrimSpace(query[strings.Index(query, "conversion_action.id = ")+len("conversion_action.id = "):])
			if i := strings.IndexAny(want, " \n"); i >= 0 {
				want = want[:i]
			}
			if want != id {
				continue
			}
		}
		out = append(out, map[string]any{"conversionAction": cloneAnyMap(action)})
	}
	return out
}

func (f *cliGoogleAdsFake) searchBudgetsLocked(query string) []any {
	var out []any
	for id, budget := range f.budgets {
		if strings.Contains(query, "campaign_budget.id = ") {
			want := strings.TrimSpace(query[strings.Index(query, "campaign_budget.id = ")+len("campaign_budget.id = "):])
			if i := strings.IndexAny(want, " \n"); i >= 0 {
				want = want[:i]
			}
			if want != id {
				continue
			}
		}
		out = append(out, map[string]any{"campaignBudget": cloneAnyMap(budget)})
	}
	return out
}

func (f *cliGoogleAdsFake) searchCampaignsLocked(query string) []any {
	var out []any
	for id, campaign := range f.campaigns {
		if strings.Contains(query, "campaign.id = ") {
			want := strings.TrimSpace(query[strings.Index(query, "campaign.id = ")+len("campaign.id = "):])
			if i := strings.IndexAny(want, " \n"); i >= 0 {
				want = want[:i]
			}
			if want != id {
				continue
			}
		}
		out = append(out, map[string]any{"campaign": cloneAnyMap(campaign)})
	}
	return out
}

func (f *cliGoogleAdsFake) searchAdGroupsLocked(query string) []any {
	var out []any
	for id, group := range f.adGroups {
		if strings.Contains(query, "ad_group.id = ") {
			want := strings.TrimSpace(query[strings.Index(query, "ad_group.id = ")+len("ad_group.id = "):])
			if i := strings.IndexAny(want, " \n"); i >= 0 {
				want = want[:i]
			}
			if want != id {
				continue
			}
		}
		out = append(out, map[string]any{"adGroup": cloneAnyMap(group)})
	}
	return out
}

func (f *cliGoogleAdsFake) searchKeywordsLocked(query string) []any {
	var out []any
	for id, item := range f.keywords {
		if strings.Contains(query, "ad_group_criterion.criterion_id = ") {
			want := strings.TrimSpace(query[strings.Index(query, "ad_group_criterion.criterion_id = ")+len("ad_group_criterion.criterion_id = "):])
			if i := strings.IndexAny(want, " \n"); i >= 0 {
				want = want[:i]
			}
			criterionID := stringifyAny(item["criterionId"])
			if want != criterionID {
				continue
			}
		}
		if strings.Contains(query, "ad_group.id = ") {
			want := strings.TrimSpace(query[strings.Index(query, "ad_group.id = ")+len("ad_group.id = "):])
			if i := strings.IndexAny(want, " \n"); i >= 0 {
				want = want[:i]
			}
			parts := strings.Split(id, "~")
			if len(parts) != 2 || parts[0] != want {
				continue
			}
		}
		out = append(out, map[string]any{"adGroupCriterion": cloneAnyMap(item)})
	}
	return out
}

func (f *cliGoogleAdsFake) searchAdsLocked(query string) []any {
	var out []any
	for id, item := range f.ads {
		if strings.Contains(query, "ad_group_ad.ad.id = ") {
			want := strings.TrimSpace(query[strings.Index(query, "ad_group_ad.ad.id = ")+len("ad_group_ad.ad.id = "):])
			if i := strings.IndexAny(want, " \n"); i >= 0 {
				want = want[:i]
			}
			ad, _ := item["ad"].(map[string]any)
			if stringifyAny(ad["id"]) != want {
				continue
			}
		}
		if strings.Contains(query, "ad_group.id = ") {
			want := strings.TrimSpace(query[strings.Index(query, "ad_group.id = ")+len("ad_group.id = "):])
			if i := strings.IndexAny(want, " \n"); i >= 0 {
				want = want[:i]
			}
			parts := strings.Split(id, "~")
			if len(parts) != 2 || parts[0] != want {
				continue
			}
		}
		out = append(out, map[string]any{"adGroupAd": cloneAnyMap(item)})
	}
	return out
}

func (f *cliGoogleAdsFake) searchGoalsLocked(query string) []any {
	var out []any
	for id, goal := range f.goals {
		if strings.Contains(query, "customer_conversion_goal.category = ") || strings.Contains(query, "customer_conversion_goal.origin = ") {
			if !strings.Contains(strings.ToUpper(query), strings.Split(id, "~")[0]) {
				continue
			}
		}
		out = append(out, map[string]any{"customerConversionGoal": cloneAnyMap(goal)})
	}
	return out
}

func (f *cliGoogleAdsFake) searchCampaignGoalsLocked(query string) []any {
	var out []any
	for id, goal := range f.campaignGoals {
		if strings.Contains(query, "campaign_conversion_goal.campaign = ") {
			wantCampaign := "customers/" + cliGoogleAdsCustomerID + "/campaigns/" + strings.Split(id, "~")[0]
			if !strings.Contains(query, wantCampaign) {
				continue
			}
		}
		out = append(out, map[string]any{"campaignConversionGoal": cloneAnyMap(goal)})
	}
	return out
}

func (f *cliGoogleAdsFake) searchCriteriaLocked(query string) []any {
	var out []any
	for id, item := range f.criteria {
		if strings.Contains(query, "campaign_criterion.criterion_id = ") {
			want := strings.TrimSpace(query[strings.Index(query, "campaign_criterion.criterion_id = ")+len("campaign_criterion.criterion_id = "):])
			if i := strings.IndexAny(want, " \n"); i >= 0 {
				want = want[:i]
			}
			parts := strings.Split(id, "~")
			if len(parts) != 2 || parts[1] != want {
				continue
			}
		}
		if strings.Contains(query, "campaign.id = ") {
			want := strings.TrimSpace(query[strings.Index(query, "campaign.id = ")+len("campaign.id = "):])
			if i := strings.IndexAny(want, " \n"); i >= 0 {
				want = want[:i]
			}
			parts := strings.Split(id, "~")
			if len(parts) != 2 || parts[0] != want {
				continue
			}
		}
		out = append(out, map[string]any{"campaignCriterion": cloneAnyMap(item)})
	}
	return out
}

func (f *cliGoogleAdsFake) searchGeosLocked(query string) []any {
	var out []any
	for _, geo := range f.geos {
		resourceName := stringifyAny(geo["resourceName"])
		if strings.Contains(query, "geo_target_constant.resource_name = ") {
			if !strings.Contains(query, resourceName) {
				continue
			}
		}
		out = append(out, map[string]any{"geoTargetConstant": cloneAnyMap(geo)})
	}
	return out
}

func (f *cliGoogleAdsFake) searchLanguagesLocked(query string) []any {
	var out []any
	for _, lang := range f.languages {
		resourceName := stringifyAny(lang["resourceName"])
		if strings.Contains(query, "language_constant.resource_name = ") {
			if !strings.Contains(query, resourceName) {
				continue
			}
		}
		out = append(out, map[string]any{"languageConstant": cloneAnyMap(lang)})
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func stringifyAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		return ""
	}
}
