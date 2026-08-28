package examples_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads"
	"github.com/dziblo-music/agoraform/providers/matomo"
)

func TestManifestsRemainValid(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob(filepath.Join("*", "agoraform.yaml"))
	if err != nil {
		t.Fatalf("find example manifests: %v", err)
	}
	paths = append(paths, "agoraform.yaml")

	required := map[string]bool{
		"googleads-conversion/agoraform.yaml": false,
		"googleads-search/agoraform.yaml":     false,
	}
	for _, path := range paths {
		slash := filepath.ToSlash(path)
		if _, ok := required[slash]; ok {
			required[slash] = true
		}
	}
	for path, found := range required {
		if !found {
			t.Fatalf("missing %s", path)
		}
	}

	for _, path := range paths {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read example: %v", err)
			}
			m, err := manifest.Parse(data, path)
			if err != nil {
				t.Fatalf("parse example: %v", err)
			}

			providers := exampleProviders()
			for name, cfg := range m.Providers {
				p, ok := providers[name]
				if !ok {
					t.Fatalf("unsupported example provider %q", name)
				}
				if configurator, ok := p.(provider.Configurator); ok {
					if err := configurator.Configure(cfg); err != nil {
						t.Fatalf("validate provider configuration: %v", err)
					}
				}
			}

			seen := make(map[string]provider.Provider)
			for _, res := range m.Resources {
				p, ok := providers[res.Address.Provider]
				if !ok {
					t.Fatalf("unsupported example provider %q", res.Address.Provider)
				}
				if err := p.Validate(context.Background(), res); err != nil {
					t.Fatalf("validate %s: %v", res.Address, err)
				}
				seen[p.Name()] = p
			}
			for _, p := range seen {
				if validator, ok := p.(provider.ResourceSetValidator); ok {
					if err := validator.ValidateResourceSet(context.Background(), m.Resources); err != nil {
						t.Fatalf("validate resource set for %s: %v", p.Name(), err)
					}
				}
			}
		})
	}
}

func exampleProviders() map[string]provider.Provider {
	return map[string]provider.Provider{
		matomo.Name: matomo.New(matomo.Config{
			BaseURL:     "https://matomo.example.com",
			TokenAuth:   "test-token",
			SiteID:      "1",
			ContainerID: "example-container",
		}),
		googleads.Name: googleads.New(googleads.Config{
			CustomerID: "1234567890",
		}),
	}
}

func TestGoogleAdsSearchExampleCoversCampaignWorkflow(t *testing.T) {
	t.Parallel()

	const path = "googleads-search/agoraform.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	m, err := manifest.Parse(data, path)
	if err != nil {
		t.Fatalf("parse example: %v", err)
	}

	byType := map[string][]resource.Resource{}
	for _, res := range m.Resources {
		byType[res.Address.Type] = append(byType[res.Address.Type], res)
	}

	requiredTypes := []string{
		googleads.TypeConversionAction,
		googleads.TypeCustomerConversionGoal,
		googleads.TypeCampaignBudget,
		googleads.TypeCampaign,
		googleads.TypeCampaignConversionGoal,
		googleads.TypeCampaignLocation,
		googleads.TypeCampaignLanguage,
		googleads.TypeAdGroup,
		googleads.TypeKeyword,
		googleads.TypeResponsiveSearchAd,
	}
	for _, typ := range requiredTypes {
		if len(byType[typ]) == 0 {
			t.Errorf("missing googleads.%s resource", typ)
		}
	}

	campaigns := byType[googleads.TypeCampaign]
	if len(campaigns) != 1 {
		t.Fatalf("campaigns = %d, want 1", len(campaigns))
	}
	campaign := campaigns[0]
	requireStatus(t, campaign, "PAUSED")
	budgetRef := requireRef(t, campaign, googleads.AttrBudget)
	if budgetRef != "googleads.campaign_budget.search_acquisition" {
		t.Errorf("campaign budget $ref = %s", budgetRef)
	}

	adGroups := byType[googleads.TypeAdGroup]
	if len(adGroups) != 1 {
		t.Fatalf("ad groups = %d, want 1", len(adGroups))
	}
	adGroup := adGroups[0]
	requireStatus(t, adGroup, "PAUSED")
	if got := requireRef(t, adGroup, googleads.AttrCampaign); got != campaign.Address.String() {
		t.Errorf("ad group campaign $ref = %s, want %s", got, campaign.Address)
	}

	goal := onlyResource(t, byType, googleads.TypeCampaignConversionGoal)
	if got := requireRef(t, goal, googleads.AttrCampaign); got != campaign.Address.String() {
		t.Errorf("campaign conversion goal campaign $ref = %s, want %s", got, campaign.Address)
	}
	if got := requireRef(t, goal, googleads.AttrConversionAction); got != "googleads.conversion_action.trial_started" {
		t.Errorf("campaign conversion goal conversionAction $ref = %s", got)
	}

	location := onlyResource(t, byType, googleads.TypeCampaignLocation)
	if got := requireRef(t, location, googleads.AttrCampaign); got != campaign.Address.String() {
		t.Errorf("location campaign $ref = %s, want %s", got, campaign.Address)
	}
	language := onlyResource(t, byType, googleads.TypeCampaignLanguage)
	if got := requireRef(t, language, googleads.AttrCampaign); got != campaign.Address.String() {
		t.Errorf("language campaign $ref = %s, want %s", got, campaign.Address)
	}

	rsa := onlyResource(t, byType, googleads.TypeResponsiveSearchAd)
	requireStatus(t, rsa, "PAUSED")
	if got := requireRef(t, rsa, googleads.AttrAdGroup); got != adGroup.Address.String() {
		t.Errorf("rsa adGroup $ref = %s, want %s", got, adGroup.Address)
	}

	matchTypes := map[string]bool{}
	negatives := 0
	for _, kw := range byType[googleads.TypeKeyword] {
		if got := requireRef(t, kw, googleads.AttrAdGroup); got != adGroup.Address.String() {
			t.Errorf("%s adGroup $ref = %s, want %s", kw.Address, got, adGroup.Address)
		}
		negative, _ := kw.Attributes[googleads.AttrNegative].(bool)
		if negative {
			negatives++
			continue
		}
		requireStatus(t, kw, "PAUSED")
		matchType, _ := kw.Attributes[googleads.AttrMatchType].(string)
		matchTypes[matchType] = true
	}
	if negatives == 0 {
		t.Error("expected at least one negative keyword")
	}
	for _, matchType := range []string{"EXACT", "PHRASE", "BROAD"} {
		if !matchTypes[matchType] {
			t.Errorf("missing positive keyword with matchType %s", matchType)
		}
	}
}

func onlyResource(t *testing.T, byType map[string][]resource.Resource, typ string) resource.Resource {
	t.Helper()
	resources := byType[typ]
	if len(resources) != 1 {
		t.Fatalf("googleads.%s resources = %d, want 1", typ, len(resources))
	}
	return resources[0]
}

func requireStatus(t *testing.T, res resource.Resource, want string) {
	t.Helper()
	got, _ := res.Attributes[googleads.AttrStatus].(string)
	if got != want {
		t.Errorf("%s status = %q, want %q", res.Address, got, want)
	}
}

func requireRef(t *testing.T, res resource.Resource, attr string) string {
	t.Helper()
	ref, ok := resource.AsRef(res.Attributes[attr])
	if !ok {
		t.Fatalf("%s attribute %q is not a $ref", res.Address, attr)
	}
	return ref.Address.String()
}
