package examples_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/dziblo-music/agoraform/internal/graph"
	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads"
	"github.com/dziblo-music/agoraform/providers/matomo"
)

func TestManifestsRemainValid(t *testing.T) {
	t.Parallel()

	paths := exampleManifestPaths(t)
	required := map[string]bool{
		"agoraform.yaml":                           false,
		"googleads-conversion/agoraform.yaml":      false,
		"googleads-search/agoraform.yaml":          false,
		"matomo-conversion/agoraform.yaml":         false,
		"matomo-googleads/agoraform.yaml":          false,
		"matomo-googleads/external/agoraform.yaml": false,
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

			m := loadExample(t, path)
			providers := exampleProviders(m)
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

func exampleManifestPaths(t *testing.T) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Base(path) != "agoraform.yaml" {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		t.Fatalf("find example manifests: %v", err)
	}
	return paths
}

func loadExample(t *testing.T, path string) *manifest.Manifest {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	m, err := manifest.Parse(data, path)
	if err != nil {
		t.Fatalf("parse example: %v", err)
	}
	return m
}

func exampleProviders(m *manifest.Manifest) map[string]provider.Provider {
	cfg := matomo.Config{
		BaseURL:   "https://matomo.example.com",
		TokenAuth: "test-token",
		SiteID:    "1",
	}
	if m == nil || !hasManagedMatomoContainer(m) {
		cfg.ContainerID = "example-container"
	}
	return map[string]provider.Provider{
		matomo.Name: matomo.New(cfg),
		googleads.Name: googleads.New(googleads.Config{
			CustomerID: "1234567890",
		}),
	}
}

func hasManagedMatomoContainer(m *manifest.Manifest) bool {
	for _, res := range m.Resources {
		if res.Address.Provider == matomo.Name && res.Address.Type == matomo.TypeContainer {
			return true
		}
	}
	return false
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

	customerGoal := onlyResource(t, byType, googleads.TypeCustomerConversionGoal)
	if got := requireRef(t, customerGoal, googleads.AttrConversionAction); got != "googleads.conversion_action.trial_started" {
		t.Errorf("customer conversion goal conversionAction $ref = %s", got)
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

func TestMatomoGoogleAdsExampleCoversLifecycleWorkflow(t *testing.T) {
	t.Parallel()

	const path = "matomo-googleads/agoraform.yaml"
	m := loadExample(t, path)
	assertMatomoPublication(t, m)

	byType := resourcesByProviderType(m.Resources)
	container := onlyTypedResource(t, byType, matomo.Name, matomo.TypeContainer)
	if container.Address.String() != "matomo.container.main" {
		t.Errorf("container address = %s, want matomo.container.main", container.Address)
	}
	if got, _ := container.Attributes[matomo.AttrName].(string); got != "Main Website" {
		t.Errorf("container name = %q, want Main Website", got)
	}
	if got, _ := container.Attributes[matomo.AttrContext].(string); got != "web" {
		t.Errorf("container context = %q, want web", got)
	}

	config := resourceByAddress(t, m.Resources, "matomo.variable.config")
	if got, _ := config.Attributes[matomo.AttrType].(string); got != "matomoConfiguration" {
		t.Errorf("config type = %q, want matomoConfiguration", got)
	}
	if got := requireRef(t, config, matomo.AttrContainer); got != container.Address.String() {
		t.Errorf("config container $ref = %s, want %s", got, container.Address)
	}

	trialID := resourceByAddress(t, m.Resources, "matomo.variable.trial_id")
	if got, _ := trialID.Attributes[matomo.AttrType].(string); got != "dataLayer" {
		t.Errorf("trial_id type = %q, want dataLayer", got)
	}
	if got, _ := trialID.Attributes[matomo.AttrKey].(string); got != "trialId" {
		t.Errorf("trial_id key = %q, want trialId", got)
	}
	if got := requireRef(t, trialID, matomo.AttrContainer); got != container.Address.String() {
		t.Errorf("trial_id container $ref = %s, want %s", got, container.Address)
	}

	trigger := resourceByAddress(t, m.Resources, "matomo.trigger.trial_started")
	if got, _ := trigger.Attributes[matomo.AttrType].(string); got != "customEvent" {
		t.Errorf("trigger type = %q, want customEvent", got)
	}
	if got, _ := trigger.Attributes["event"].(string); got != "trialStarted" {
		t.Errorf("trigger event = %q, want trialStarted", got)
	}
	if got := requireRef(t, trigger, matomo.AttrContainer); got != container.Address.String() {
		t.Errorf("trigger container $ref = %s, want %s", got, container.Address)
	}

	action := onlyTypedResource(t, byType, googleads.Name, googleads.TypeConversionAction)
	if action.Address.String() != "googleads.conversion_action.trial_started" {
		t.Errorf("conversion action = %s", action.Address)
	}

	goal := onlyTypedResource(t, byType, googleads.Name, googleads.TypeCustomerConversionGoal)
	if got := requireRef(t, goal, googleads.AttrConversionAction); got != action.Address.String() {
		t.Errorf("customer conversion goal conversionAction $ref = %s", got)
	}

	tag := resourceByAddress(t, m.Resources, "matomo.tag.google_ads_trial_started")
	if got, _ := tag.Attributes[matomo.AttrType].(string); got != "googleAdsConversion" {
		t.Errorf("tag type = %q, want googleAdsConversion", got)
	}
	if got := requireRef(t, tag, matomo.AttrContainer); got != container.Address.String() {
		t.Errorf("tag container $ref = %s, want %s", got, container.Address)
	}
	if got := requireRef(t, tag, matomo.AttrTrigger); got != trigger.Address.String() {
		t.Errorf("tag trigger $ref = %s, want %s", got, trigger.Address)
	}
	requireOutputRef(t, tag, matomo.AttrConversionID, action.Address.String(), googleads.OutputConversionID)
	requireOutputRef(t, tag, matomo.AttrConversionLabel, action.Address.String(), googleads.OutputConversionLabel)
	if got := requireRef(t, tag, matomo.AttrConversionTransactionID); got != trialID.Address.String() {
		t.Errorf("tag conversionTransactionId $ref = %s, want %s", got, trialID.Address)
	}

	for _, res := range m.Resources {
		if res.Address.Provider != matomo.Name {
			continue
		}
		switch res.Address.Type {
		case matomo.TypeVariable, matomo.TypeTrigger, matomo.TypeTag:
			if got := requireRef(t, res, matomo.AttrContainer); got != container.Address.String() {
				t.Errorf("%s container $ref = %s, want %s", res.Address, got, container.Address)
			}
		}
	}
}

func TestMatomoGoogleAdsExternalExampleOmitsManagedContainer(t *testing.T) {
	t.Parallel()

	const path = "matomo-googleads/external/agoraform.yaml"
	m := loadExample(t, path)
	assertMatomoPublication(t, m)
	if hasManagedMatomoContainer(m) {
		t.Fatal("external example must omit matomo.container")
	}

	tag := resourceByAddress(t, m.Resources, "matomo.tag.google_ads_trial_started")
	if _, ok := tag.Attributes[matomo.AttrContainer]; ok {
		t.Fatal("external tag must omit container")
	}
	requireOutputRef(t, tag, matomo.AttrConversionID, "googleads.conversion_action.trial_started", googleads.OutputConversionID)
	requireOutputRef(t, tag, matomo.AttrConversionLabel, "googleads.conversion_action.trial_started", googleads.OutputConversionLabel)

	for _, res := range m.Resources {
		if res.Address.Provider != matomo.Name {
			continue
		}
		if _, ok := res.Attributes[matomo.AttrContainer]; ok {
			t.Errorf("%s must omit container in external-container mode", res.Address)
		}
	}
}

func TestMatomoGoogleAdsExampleApplyAndDestroyOrder(t *testing.T) {
	t.Parallel()

	m := loadExample(t, "matomo-googleads/agoraform.yaml")
	assertAddressOrder(t, m.Resources, []string{
		"googleads.conversion_action.trial_started",
		"googleads.customer_conversion_goal.signup",
		"matomo.container.main",
		"matomo.trigger.trial_started",
		"matomo.variable.config",
		"matomo.variable.trial_id",
		"matomo.tag.google_ads_trial_started",
	})
}

func TestMatomoGoogleAdsExternalExampleApplyAndDestroyOrder(t *testing.T) {
	t.Parallel()

	m := loadExample(t, "matomo-googleads/external/agoraform.yaml")
	assertAddressOrder(t, m.Resources, []string{
		"googleads.conversion_action.trial_started",
		"googleads.customer_conversion_goal.signup",
		"matomo.trigger.trial_started",
		"matomo.variable.config",
		"matomo.variable.trial_id",
		"matomo.tag.google_ads_trial_started",
	})
}

func assertMatomoPublication(t *testing.T, m *manifest.Manifest) {
	t.Helper()
	cfg, ok := m.Providers[matomo.Name]
	if !ok {
		t.Fatal("missing matomo provider configuration")
	}
	if got, ok := cfg["publish"].(bool); !ok || !got {
		t.Fatalf("matomo publish = %#v, want true", cfg["publish"])
	}
	if got, _ := cfg["environment"].(string); got != "live" {
		t.Fatalf("matomo environment = %#v, want live", cfg["environment"])
	}
	if _, ok := m.Providers[googleads.Name]; !ok {
		t.Fatal("missing googleads provider configuration")
	}
}

func assertAddressOrder(t *testing.T, resources []resource.Resource, wantApply []string) {
	t.Helper()
	g, err := graph.Build(resources)
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	gotApply := addressStrings(g.Order())
	if !reflect.DeepEqual(gotApply, wantApply) {
		t.Fatalf("apply order = %v, want %v", gotApply, wantApply)
	}
	wantDestroy := reverseStrings(wantApply)
	gotDestroy := addressStrings(g.ReverseOrder())
	if !reflect.DeepEqual(gotDestroy, wantDestroy) {
		t.Fatalf("destroy order = %v, want %v", gotDestroy, wantDestroy)
	}
}

func resourcesByProviderType(resources []resource.Resource) map[string][]resource.Resource {
	byType := map[string][]resource.Resource{}
	for _, res := range resources {
		key := res.Address.Provider + "." + res.Address.Type
		byType[key] = append(byType[key], res)
	}
	return byType
}

func onlyTypedResource(t *testing.T, byType map[string][]resource.Resource, providerName, typ string) resource.Resource {
	t.Helper()
	key := providerName + "." + typ
	resources := byType[key]
	if len(resources) != 1 {
		t.Fatalf("%s.%s resources = %d, want 1", providerName, typ, len(resources))
	}
	return resources[0]
}

func resourceByAddress(t *testing.T, resources []resource.Resource, want string) resource.Resource {
	t.Helper()
	for _, res := range resources {
		if res.Address.String() == want {
			return res
		}
	}
	t.Fatalf("missing resource %s", want)
	return resource.Resource{}
}

func requireOutputRef(t *testing.T, res resource.Resource, attr, wantAddr, wantOutput string) {
	t.Helper()
	ref, ok := resource.AsRef(res.Attributes[attr])
	if !ok {
		t.Fatalf("%s attribute %q is not a $ref", res.Address, attr)
	}
	if ref.Address.String() != wantAddr {
		t.Errorf("%s attribute %q $ref = %s, want %s", res.Address, attr, ref.Address, wantAddr)
	}
	if ref.Output != wantOutput {
		t.Errorf("%s attribute %q output = %q, want %q", res.Address, attr, ref.Output, wantOutput)
	}
}

func addressStrings(addrs []resource.Address) []string {
	out := make([]string, len(addrs))
	for i, addr := range addrs {
		out[i] = addr.String()
	}
	return out
}

func reverseStrings(in []string) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[len(in)-1-i] = v
	}
	return out
}
