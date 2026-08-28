package cli_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestImportGoogleAdsSearchStackThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	seedCLIBrandSearch(srv)

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")

	budgetYAML := mustCLIImport(t, reg, manifestPath, "googleads.campaign_budget.brand", "11")
	campaignYAML := mustCLIImport(t, reg, manifestPath, "googleads.campaign.brand", "customers/"+cliGoogleAdsCustomerID+"/campaigns/21")
	if !strings.Contains(campaignYAML, "$ref: googleads.campaign_budget.brand") {
		t.Fatalf("campaign YAML missing budget $ref:\n%s", campaignYAML)
	}
	goalYAML := mustCLIImport(t, reg, manifestPath, "googleads.campaign_conversion_goal.trial_signup", "21~signup~website")
	if !strings.Contains(goalYAML, "$ref: googleads.campaign.brand") {
		t.Fatalf("goal YAML missing campaign $ref:\n%s", goalYAML)
	}
	groupYAML := mustCLIImport(t, reg, manifestPath, "googleads.ad_group.brand", "31")
	if !strings.Contains(groupYAML, "$ref: googleads.campaign.brand") {
		t.Fatalf("ad group YAML missing campaign $ref:\n%s", groupYAML)
	}
	keywordYAML := mustCLIImport(t, reg, manifestPath, "googleads.keyword.brand_exact", "31~41")
	if !strings.Contains(keywordYAML, "$ref: googleads.ad_group.brand") {
		t.Fatalf("keyword YAML missing ad group $ref:\n%s", keywordYAML)
	}
	negativeYAML := mustCLIImport(t, reg, manifestPath, "googleads.keyword.competitor_neg", "31~42")
	if !strings.Contains(negativeYAML, "negative: true") {
		t.Fatalf("negative keyword YAML missing negative:\n%s", negativeYAML)
	}
	rsaYAML := mustCLIImport(t, reg, manifestPath, "googleads.responsive_search_ad.brand", "customers/"+cliGoogleAdsCustomerID+"/adGroupAds/31~71")
	if !strings.Contains(rsaYAML, "$ref: googleads.ad_group.brand") {
		t.Fatalf("rsa YAML missing ad group $ref:\n%s", rsaYAML)
	}
	locationYAML := mustCLIImport(t, reg, manifestPath, "googleads.campaign_location.united_states", "21~41")
	if !strings.Contains(locationYAML, "United States") {
		t.Fatalf("location YAML missing canonical name:\n%s", locationYAML)
	}
	languageYAML := mustCLIImport(t, reg, manifestPath, "googleads.campaign_language.english", "21~51")
	if !strings.Contains(languageYAML, "language: en") {
		t.Fatalf("language YAML missing ISO code:\n%s", languageYAML)
	}

	for _, leaked := range []string{"resourceName", "amountMicros", "cpcBidMicros", "geoTargetConstants", "languageConstants", "assetPerformanceLabel", cliGoogleAdsAccessToken} {
		for _, yamlText := range []string{budgetYAML, campaignYAML, goalYAML, groupYAML, keywordYAML, negativeYAML, rsaYAML, locationYAML, languageYAML} {
			if strings.Contains(yamlText, leaked) {
				t.Fatalf("generated YAML leaked %q:\n%s", leaked, yamlText)
			}
		}
	}

	combined := combineManifestResources(t, budgetYAML, campaignYAML, goalYAML, groupYAML, keywordYAML, negativeYAML, rsaYAML, locationYAML, languageYAML)
	if err := os.WriteFile(manifestPath, []byte(combined), 0o600); err != nil {
		t.Fatal(err)
	}
	assertPersistedRemoteID(t, manifestPath, "googleads.campaign.brand", "21")
	assertPersistedRemoteID(t, manifestPath, "googleads.keyword.competitor_neg", "31~42")
	assertPersistedRemoteID(t, manifestPath, "googleads.responsive_search_ad.brand", "31~71")

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after import = %q, want no changes", stdout.String())
	}
	if srv.mutateCount() != 0 {
		t.Fatalf("search stack import mutated remote: %d", srv.mutateCount())
	}
}

func TestImportGoogleAdsSearchUnsupportedRemote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		seed     func(*cliGoogleAdsFake)
		address  string
		remoteID string
		want     string
	}{
		{
			name: "display campaign",
			seed: func(srv *cliGoogleAdsFake) {
				srv.seedCampaign(map[string]any{
					"id":                     "21",
					"name":                   "Display",
					"status":                 "PAUSED",
					"advertisingChannelType": "DISPLAY",
					"campaignBudget":         "customers/" + cliGoogleAdsCustomerID + "/campaignBudgets/11",
					"biddingStrategyType":    "MANUAL_CPC",
				})
			},
			address:  "googleads.campaign.display",
			remoteID: "21",
			want:     "SEARCH",
		},
		{
			name: "dynamic search ads campaign",
			seed: func(srv *cliGoogleAdsFake) {
				srv.seedCampaign(map[string]any{
					"id":                     "21",
					"name":                   "DSA",
					"status":                 "PAUSED",
					"advertisingChannelType": "SEARCH",
					"campaignBudget":         "customers/" + cliGoogleAdsCustomerID + "/campaignBudgets/11",
					"biddingStrategyType":    "MANUAL_CPC",
					"dynamicSearchAdsSetting": map[string]any{
						"domainName":   "example.com",
						"languageCode": "en",
					},
				})
			},
			address:  "googleads.campaign.dsa",
			remoteID: "21",
			want:     "Dynamic Search Ads",
		},
		{
			name: "keyword with final urls",
			seed: func(srv *cliGoogleAdsFake) {
				srv.seedKeyword(map[string]any{
					"criterionId": "9",
					"adGroup":     "customers/" + cliGoogleAdsCustomerID + "/adGroups/31",
					"status":      "PAUSED",
					"type":        "KEYWORD",
					"keyword": map[string]any{
						"text":      "brand",
						"matchType": "EXACT",
					},
					"finalUrls": []any{"https://example.com/landing"},
				})
			},
			address:  "googleads.keyword.brand_url",
			remoteID: "31~9",
			want:     "finalUrls",
		},
		{
			name: "shopping ad group",
			seed: func(srv *cliGoogleAdsFake) {
				srv.seedAdGroup(map[string]any{
					"id":       "31",
					"name":     "Shopping",
					"status":   "PAUSED",
					"type":     "SHOPPING_PRODUCT_ADS",
					"campaign": "customers/" + cliGoogleAdsCustomerID + "/campaigns/21",
				})
			},
			address:  "googleads.ad_group.shopping",
			remoteID: "31",
			want:     "SEARCH_STANDARD",
		},
		{
			name: "non-keyword criterion",
			seed: func(srv *cliGoogleAdsFake) {
				srv.seedKeyword(map[string]any{
					"criterionId": "9",
					"adGroup":     "customers/" + cliGoogleAdsCustomerID + "/adGroups/31",
					"status":      "PAUSED",
					"type":        "LISTING_GROUP",
				})
			},
			address:  "googleads.keyword.listing",
			remoteID: "31~9",
			want:     "KEYWORD",
		},
		{
			name: "expanded text ad",
			seed: func(srv *cliGoogleAdsFake) {
				srv.seedAd(map[string]any{
					"adGroup": "customers/" + cliGoogleAdsCustomerID + "/adGroups/31",
					"status":  "PAUSED",
					"ad": map[string]any{
						"id":   "9",
						"type": "EXPANDED_TEXT_AD",
					},
				})
			},
			address:  "googleads.responsive_search_ad.expanded",
			remoteID: "31~9",
			want:     "RESPONSIVE_SEARCH_AD",
		},
		{
			name: "language criterion as location",
			seed: func(srv *cliGoogleAdsFake) {
				srv.seedCriterion(map[string]any{
					"criterionId": "51",
					"campaign":    "customers/" + cliGoogleAdsCustomerID + "/campaigns/21",
					"type":        "LANGUAGE",
					"status":      "ENABLED",
					"language":    map[string]any{"languageConstant": "languageConstants/1000"},
				})
			},
			address:  "googleads.campaign_location.english",
			remoteID: "21~51",
			want:     "LOCATION",
		},
		{
			name: "lifetime budget",
			seed: func(srv *cliGoogleAdsFake) {
				srv.seedBudget(map[string]any{
					"id":               "3",
					"name":             "Lifetime budget",
					"amountMicros":     "50000000",
					"explicitlyShared": false,
					"period":           "CUSTOM_PERIOD",
					"type":             "STANDARD",
				})
			},
			address:  "googleads.campaign_budget.lifetime",
			remoteID: "3",
			want:     "DAILY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, srv := googleAdsImportProvider(t)
			tt.seed(srv)
			reg := provider.NewRegistry()
			if err := reg.Register(p); err != nil {
				t.Fatal(err)
			}
			path := writeManifest(t, "agoraform.yaml", emptyResourcesManifest)
			streams, _, stderr := testStreams()
			code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, tt.address, tt.remoteID}, reg)
			if code != cli.ExitError {
				t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tt.want)
			}
			if srv.mutateCount() != 0 {
				t.Fatalf("unsupported import mutated remote: %d", srv.mutateCount())
			}
		})
	}
}

func TestImportGoogleAdsSearchMissingParent(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	seedCLIBrandSearch(srv)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", emptyResourcesManifest)

	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, "googleads.campaign.brand", "21"}, reg)
	if code != cli.ExitError {
		t.Fatalf("campaign without budget exit = %d; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "campaign_budget") {
		t.Fatalf("stderr = %q, want budget import guidance", stderr.String())
	}

	streams, _, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, "googleads.ad_group.brand", "31"}, reg)
	if code != cli.ExitError {
		t.Fatalf("ad group without campaign exit = %d; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "campaign") {
		t.Fatalf("stderr = %q, want campaign import guidance", stderr.String())
	}

	streams, _, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, "googleads.keyword.brand_exact", "31~41"}, reg)
	if code != cli.ExitError {
		t.Fatalf("keyword without ad group exit = %d; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "ad group") {
		t.Fatalf("stderr = %q, want ad group import guidance", stderr.String())
	}

	streams, _, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, "googleads.responsive_search_ad.brand", "31~71"}, reg)
	if code != cli.ExitError {
		t.Fatalf("rsa without ad group exit = %d; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "ad group") {
		t.Fatalf("stderr = %q, want ad group import guidance", stderr.String())
	}

	if srv.mutateCount() != 0 {
		t.Fatalf("missing-parent import mutated remote: %d", srv.mutateCount())
	}
}

func TestImportGoogleAdsSearchInvalidIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		address  string
		remoteID string
		want     string
	}{
		{"googleads.campaign_budget.brand", "abc", "not a valid Google Ads campaign budget id"},
		{"googleads.campaign.brand", "abc", "not a valid Google Ads campaign id"},
		{"googleads.ad_group.brand", "abc", "not a valid Google Ads ad group id"},
		{"googleads.keyword.brand_exact", "abc", "not a valid Google Ads keyword id"},
		{"googleads.responsive_search_ad.brand", "abc", "not a valid Google Ads responsive search ad id"},
		{"googleads.campaign_location.united_states", "abc", "not a valid Google Ads campaign criterion id"},
		{"googleads.campaign_language.english", "abc", "not a valid Google Ads campaign criterion id"},
		{"googleads.campaign_conversion_goal.trial_signup", "not-an-id", "not a valid Google Ads campaign conversion goal id"},
	}

	p, _ := googleAdsImportProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", emptyResourcesManifest)

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			streams, _, stderr := testStreams()
			code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, tt.address, tt.remoteID}, reg)
			if code != cli.ExitError {
				t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestImportGoogleAdsSearchNotFound(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", emptyResourcesManifest)

	tests := []struct {
		address  string
		remoteID string
	}{
		{"googleads.campaign_budget.brand", "11"},
		{"googleads.campaign.brand", "21"},
		{"googleads.ad_group.brand", "31"},
		{"googleads.keyword.brand_exact", "31~41"},
		{"googleads.responsive_search_ad.brand", "31~71"},
		{"googleads.campaign_location.united_states", "21~41"},
		{"googleads.campaign_language.english", "21~51"},
		{"googleads.campaign_conversion_goal.trial_signup", "21~SIGNUP~WEBSITE"},
	}
	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			streams, _, stderr := testStreams()
			code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, tt.address, tt.remoteID}, reg)
			if code != cli.ExitError {
				t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
			}
			if !strings.Contains(stderr.String(), "was not found") {
				t.Fatalf("stderr = %q, want not found", stderr.String())
			}
		})
	}
	if srv.mutateCount() != 0 {
		t.Fatalf("not-found import mutated remote: %d", srv.mutateCount())
	}
}

func TestImportGoogleAdsSearchAPIError(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	srv.searchStatus = http.StatusForbidden
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", emptyResourcesManifest)

	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, "googleads.campaign.brand", "21"}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if strings.Contains(stderr.String(), cliGoogleAdsAccessToken) || strings.Contains(stderr.String(), cliGoogleAdsDeveloperToken) {
		t.Fatalf("API error leaked secret:\n%s", stderr.String())
	}
	if srv.mutateCount() != 0 {
		t.Fatalf("API error import mutated remote: %d", srv.mutateCount())
	}
}

func TestImportGoogleAdsSearchYAMLIsDeterministic(t *testing.T) {
	t.Parallel()

	p, srv := googleAdsImportProvider(t)
	seedCLIBrandSearch(srv)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	firstBudget := importGoogleAdsYAML(t, p, "googleads.campaign_budget.brand", "11")
	secondBudget := importGoogleAdsYAML(t, p, "googleads.campaign_budget.brand", "11")
	if firstBudget != secondBudget {
		t.Fatalf("budget YAML differed:\n%s\n---\n%s", firstBudget, secondBudget)
	}

	first := importCLISearchWithParents(t, p, "googleads.responsive_search_ad.brand", "31~71")
	second := importCLISearchWithParents(t, p, "googleads.responsive_search_ad.brand", "31~71")
	if first != second {
		t.Fatalf("rsa YAML differed:\n%s\n---\n%s", first, second)
	}
}

func seedCLIBrandSearch(srv *cliGoogleAdsFake) {
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
		"geoTargetTypeSetting": map[string]any{
			"positiveGeoTargetType": "PRESENCE",
			"negativeGeoTargetType": "PRESENCE",
		},
	})
	srv.seedCampaignGoal(map[string]any{
		"campaignId": "21",
		"category":   "SIGNUP",
		"origin":     "WEBSITE",
		"biddable":   true,
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
	srv.seedKeyword(map[string]any{
		"criterionId": "42",
		"adGroup":     "customers/" + cliGoogleAdsCustomerID + "/adGroups/31",
		"status":      "ENABLED",
		"type":        "KEYWORD",
		"negative":    true,
		"keyword": map[string]any{
			"text":      "competitor",
			"matchType": "PHRASE",
		},
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
}

func mustCLIImport(t *testing.T, reg *provider.Registry, manifestPath, address, remoteID string) string {
	t.Helper()
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, address, remoteID}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import %s %s exit = %d; stderr=%q stdout=%q", address, remoteID, code, stderr.String(), stdout.String())
	}
	out := stdout.String()
	assertGoogleAdsImportOutputClean(t, out)
	return extractYAML(out)
}

func importCLISearchWithParents(t *testing.T, p *googleads.Provider, address, remoteID string) string {
	t.Helper()
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	_ = mustCLIImport(t, reg, manifestPath, "googleads.campaign_budget.brand", "11")
	_ = mustCLIImport(t, reg, manifestPath, "googleads.campaign.brand", "21")
	_ = mustCLIImport(t, reg, manifestPath, "googleads.ad_group.brand", "31")
	return mustCLIImport(t, reg, manifestPath, address, remoteID)
}
