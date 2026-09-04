package meta_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/importer"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/meta"
)

func TestValidateAdSetCanonicalizesSafeInstagramWebsiteConversion(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	res := adSetResource(t, "instagram", standardAdSetAttrs(t))
	if err := p.Validate(context.Background(), res); err != nil {
		t.Fatal(err)
	}
	want, _, err := p.NormalizeComparable(res, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want[meta.AttrStatus] != "PAUSED" || want[meta.AttrBillingEvent] != "IMPRESSIONS" || want[meta.AttrBidStrategy] != "LOWEST_COST_WITHOUT_CAP" {
		t.Fatalf("defaults = %#v", want)
	}
	targeting := want[meta.AttrTargeting].(map[string]any)
	if targeting["ageMin"] != int64(18) || targeting["ageMax"] != int64(65) {
		t.Fatalf("targeting defaults = %#v", targeting)
	}
	positions := targeting["instagramPositions"].([]any)
	if strings.Join([]string{positions[0].(string), positions[1].(string), positions[2].(string)}, ",") != "FEED,REELS,STORIES" {
		t.Fatalf("positions = %#v", positions)
	}
}

func TestValidateAdSetRejectsInvalidCombinations(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	tests := []struct {
		name, contains string
		mutate         func(resource.Attributes)
	}{
		{"double budget", "mutually exclusive", func(a resource.Attributes) { a[meta.AttrDailyBudget] = 1000 }},
		{"lifetime without end", "requires both", func(a resource.Attributes) { delete(a, meta.AttrEndTime) }},
		{"bad schedule", "must be after", func(a resource.Attributes) { a[meta.AttrEndTime] = "2026-09-01T00:00:00Z" }},
		{"bid cap without amount", "requires bidAmount", func(a resource.Attributes) { a[meta.AttrBidStrategy] = "LOWEST_COST_WITH_BID_CAP" }},
		{"amount without cap", "valid only", func(a resource.Attributes) { a[meta.AttrBidAmount] = 100 }},
		{"placement without platform", "requires publisherPlatforms", func(a resource.Attributes) { a[meta.AttrTargeting].(map[string]any)["publisherPlatforms"] = []any{} }},
		{"raw targeting", "unsupported targeting field", func(a resource.Attributes) { a[meta.AttrTargeting].(map[string]any)["interests"] = []any{"music"} }},
		{"clicks with conversion", "valid only", func(a resource.Attributes) { a[meta.AttrOptimizationGoal] = "LINK_CLICKS" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			attrs := standardAdSetAttrs(t)
			tc.mutate(attrs)
			err := p.Validate(context.Background(), adSetResource(t, "bad", attrs))
			if err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("error=%v, want %q", err, tc.contains)
			}
		})
	}
}

func TestValidateAdSetResourceSetBudgetAndConversionRelationships(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	campaign := campaignResource(t, "acquisition", standardCampaignAttrs())
	pixel := pixelResource(t, "website")
	conversion := conversionResource(t, "trial_started", websiteConversionAttrs(t))
	ad := adSetResource(t, "instagram", standardAdSetAttrs(t))
	if err := p.ValidateResourceSet(context.Background(), []resource.Resource{campaign, pixel, conversion, ad}); err != nil {
		t.Fatal(err)
	}

	campaignBudget := campaign
	campaignBudget.Attributes = campaignBudget.Attributes.Clone()
	campaignBudget.Attributes[meta.AttrDailyBudget] = 5000
	if err := p.ValidateResourceSet(context.Background(), []resource.Resource{campaignBudget, pixel, conversion, ad}); err == nil || !strings.Contains(err.Error(), "ownership conflicts") {
		t.Fatalf("conflict error=%v", err)
	}
	noBudget := ad
	noBudget.Attributes = noBudget.Attributes.Clone()
	delete(noBudget.Attributes, meta.AttrLifetimeBudget)
	if err := p.ValidateResourceSet(context.Background(), []resource.Resource{campaign, pixel, conversion, noBudget}); err == nil || !strings.Contains(err.Error(), "must declare exactly one") {
		t.Fatalf("missing budget error=%v", err)
	}
	otherPixel := pixelAddress(t, "other")
	mismatch := ad
	mismatch.Attributes = mismatch.Attributes.Clone()
	mismatch.Attributes[meta.AttrPixel] = resource.Ref{Address: otherPixel}
	if err := p.ValidateResourceSet(context.Background(), []resource.Resource{campaign, pixel, conversion, mismatch}); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("pixel mismatch error=%v", err)
	}
}

func TestValidateCampaignBudgetAndLinkClickAdSet(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	campaignAttrs := standardCampaignAttrs()
	campaignAttrs[meta.AttrDailyBudget] = 5000
	campaign := campaignResource(t, "traffic", campaignAttrs)
	campaign.Attributes[meta.AttrObjective] = "OUTCOME_TRAFFIC"
	attrs := resource.Attributes{
		meta.AttrName:             "Instagram Traffic",
		meta.AttrCampaign:         resource.Ref{Address: campaign.Address},
		meta.AttrOptimizationGoal: "LINK_CLICKS",
		meta.AttrDestinationType:  "WEBSITE",
		meta.AttrTargeting:        map[string]any{"countries": []any{"US"}},
	}
	ad := adSetResource(t, "traffic", attrs)
	if err := p.Validate(context.Background(), ad); err != nil {
		t.Fatal(err)
	}
	if err := p.ValidateResourceSet(context.Background(), []resource.Resource{campaign, ad}); err != nil {
		t.Fatal(err)
	}
}

func TestCreateReadUpdateAndNoOpAdSet(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedCampaign(testCampaignID, graphObject{"name": "Acquisition", "objective": "OUTCOME_SALES"})
	srv.seedPixel(testPixelID, "Website")
	srv.seedConversion(testConvID, graphObject{"name": "Trial Started", "custom_event_type": "START_TRIAL", "rule": `{"and":[{"event":{"eq":"StartTrial"}}]}`, "pixel": graphObject{"id": testPixelID}, "event_source_type": "pixel"})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adSetCatalog(t))
	rememberAdSetDependencies(t, p)
	attrs := standardAdSetAttrs(t)
	created, err := p.Create(context.Background(), adSetResource(t, "instagram", attrs))
	if err != nil {
		t.Fatal(err)
	}
	if created.Identity.ID != testAdSetID || created.Attributes[meta.AttrStatus] != "PAUSED" {
		t.Fatalf("created=%#v", created)
	}
	targeting := created.Attributes[meta.AttrTargeting].(map[string]any)
	if got := targeting["publisherPlatforms"].([]any)[0]; got != "INSTAGRAM" {
		t.Fatalf("platform=%v", got)
	}
	desired := adSetResource(t, "instagram", attrs.Clone())
	desired.Identity = created.Identity
	desired.Attributes[meta.AttrName] = "Instagram US"
	desired.Attributes[meta.AttrStatus] = "ACTIVE"
	desired.Attributes[meta.AttrLifetimeBudget] = 60000
	desired.Attributes[meta.AttrEndTime] = "2026-10-02T05:00:00Z"
	updated, err := p.Update(context.Background(), desired, created)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Attributes[meta.AttrName] != "Instagram US" || updated.Attributes[meta.AttrStatus] != "ACTIVE" || updated.Attributes[meta.AttrLifetimeBudget] != int64(60000) {
		t.Fatalf("updated=%#v", updated.Attributes)
	}
	posts, _ := srv.mutationCounts()
	if posts != 2 {
		t.Fatalf("posts=%d", posts)
	}
	if _, err := p.Update(context.Background(), desired, updated); err != nil {
		t.Fatal(err)
	}
	posts, _ = srv.mutationCounts()
	if posts != 2 {
		t.Fatalf("no-op posts=%d", posts)
	}
}

func TestAdSetPlanShowsPausedCreateAndActiveTransition(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedCampaign(testCampaignID, graphObject{"name": "Acquisition", "objective": "OUTCOME_SALES"})
	srv.seedPixel(testPixelID, "Website")
	srv.seedConversion(testConvID, graphObject{"name": "Trial Started", "custom_event_type": "START_TRIAL", "rule": `{"and":[{"event":{"eq":"StartTrial"}}]}`, "pixel": graphObject{"id": testPixelID}, "event_source_type": "pixel"})
	srv.seedAdSet(testAdSetID, graphObject{"name": "Instagram US", "lifetime_budget": "50000", "start_time": "2026-09-01T05:00:00+0000", "end_time": "2026-10-01T05:00:00+0000", "targeting": instagramTargetingAPI()})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adSetCatalog(t))
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for addr, id := range map[resource.Address]string{campaignAddress(t, "acquisition"): testCampaignID, pixelAddress(t, "website"): testPixelID, conversionAddress(t, "trial_started"): testConvID} {
		if err := st.Bind(addr, resource.Identity{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	resources := standardAdSetResources(t)
	createdPlan, err := plan.BuildWithState(context.Background(), resources, func(resource.Address) (provider.Reader, error) { return p, nil }, st)
	if err != nil {
		t.Fatal(err)
	}
	var create plan.Change
	for _, change := range createdPlan.Changes {
		if change.Address.Type == meta.TypeAdSet {
			create = change
		}
	}
	if create.Action != plan.ActionCreate || create.After[meta.AttrStatus] != "PAUSED" {
		t.Fatalf("create=%#v", create)
	}
	if err := st.Bind(adSetAddress(t, "instagram"), resource.Identity{ID: testAdSetID}); err != nil {
		t.Fatal(err)
	}
	cleanPlan, err := plan.BuildWithState(context.Background(), resources, func(resource.Address) (provider.Reader, error) { return p, nil }, st)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range cleanPlan.Changes {
		if change.Address.Type == meta.TypeAdSet && change.Action != plan.ActionUnchanged {
			t.Fatalf("equivalent ad set plan = %#v", change)
		}
	}
	resources[len(resources)-1].Attributes[meta.AttrStatus] = "ACTIVE"
	activePlan, err := plan.BuildWithState(context.Background(), resources, func(resource.Address) (provider.Reader, error) { return p, nil }, st)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range activePlan.Changes {
		if change.Address.Type == meta.TypeAdSet {
			if change.Action != plan.ActionUpdate || change.Before[meta.AttrStatus] != "PAUSED" || change.After[meta.AttrStatus] != "ACTIVE" {
				t.Fatalf("active=%#v", change)
			}
		}
	}
}

func TestAdSetImmutableChangesFailPlanning(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedCampaign(testCampaignID, graphObject{"name": "Acquisition", "objective": "OUTCOME_SALES"})
	srv.seedPixel(testPixelID, "Website")
	srv.seedConversion(testConvID, graphObject{"name": "Trial Started", "custom_event_type": "START_TRIAL", "rule": `{"and":[{"event":{"eq":"StartTrial"}}]}`, "pixel": graphObject{"id": testPixelID}, "event_source_type": "pixel"})
	srv.seedAdSet(testAdSetID, graphObject{"lifetime_budget": "50000", "start_time": "2026-09-01T05:00:00Z", "end_time": "2026-10-01T05:00:00Z", "targeting": instagramTargetingAPI()})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adSetCatalog(t))
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for addr, id := range map[resource.Address]string{campaignAddress(t, "acquisition"): testCampaignID, pixelAddress(t, "website"): testPixelID, conversionAddress(t, "trial_started"): testConvID, adSetAddress(t, "instagram"): testAdSetID} {
		if err := st.Bind(addr, resource.Identity{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	resources := standardAdSetResources(t)
	resources[len(resources)-1].Attributes[meta.AttrStartTime] = "2026-09-02T05:00:00Z"
	_, err = plan.BuildWithState(context.Background(), resources, func(resource.Address) (provider.Reader, error) { return p, nil }, st)
	if err == nil || !strings.Contains(err.Error(), "startTime is immutable") {
		t.Fatalf("error=%v", err)
	}
	posts, deletes := srv.mutationCounts()
	if posts != 0 || deletes != 0 {
		t.Fatalf("plan mutated posts=%d deletes=%d", posts, deletes)
	}
}

func TestImportAdSetReconstructsRelationshipsAndCanonicalTargeting(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedAdSet(testAdSetID, graphObject{"status": "ACTIVE", "configured_status": "ACTIVE", "effective_status": "ACTIVE", "lifetime_budget": "50000", "start_time": "2026-09-01T00:00:00-0500", "end_time": "2026-10-01T00:00:00-0500", "targeting": instagramTargetingAPI()})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adSetCatalog(t))
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Run(context.Background(), adSetAddress(t, "instagram"), testAdSetID, func(resource.Address) (provider.Provider, error) { return p, nil }, st)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"status: ACTIVE", "$ref: meta.campaign.acquisition", "$ref: meta.pixel.website", "$ref: meta.custom_conversion.trial_started", "publisherPlatforms:", "- INSTAGRAM", "instagramPositions:", "- FEED", "- STORIES", "- REELS", "startTime: \"2026-09-01T05:00:00Z\""} {
		if !strings.Contains(result.YAML, want) {
			t.Fatalf("YAML missing %q:\n%s", want, result.YAML)
		}
	}
}

func TestImportAdSetRejectsUnboundRelationships(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedAdSet(testAdSetID, graphObject{"lifetime_budget": "50000", "start_time": "2026-09-01T00:00:00Z", "end_time": "2026-10-01T00:00:00Z"})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	_, err := p.Import(context.Background(), adSetAddress(t, "instagram"), testAdSetID)
	if err == nil || !strings.Contains(err.Error(), "campaign relationship") {
		t.Fatalf("error=%v", err)
	}
}

func TestAdSetAPIFailureDoesNotLeakToken(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedCampaign(testCampaignID, graphObject{"name": "Acquisition", "objective": "OUTCOME_SALES"})
	srv.seedPixel(testPixelID, "Website")
	srv.seedConversion(testConvID, graphObject{"name": "Trial Started", "custom_event_type": "START_TRIAL", "rule": `{"and":[{"event":{"eq":"StartTrial"}}]}`, "pixel": graphObject{"id": testPixelID}, "event_source_type": "pixel"})
	srv.adSetCreateFailure = true
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adSetCatalog(t))
	rememberAdSetDependencies(t, p)
	_, err := p.Create(context.Background(), adSetResource(t, "failure", standardAdSetAttrs(t)))
	if err == nil || !strings.Contains(err.Error(), "temporary ad set failure") {
		t.Fatalf("error=%v", err)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("token leaked: %v", err)
	}
}

func TestDestroyAdSetIsIdempotent(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedAdSet(testAdSetID, graphObject{"lifetime_budget": "50000", "start_time": "2026-09-01T00:00:00Z", "end_time": "2026-10-01T00:00:00Z"})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adSetCatalog(t))
	res := adSetResource(t, "instagram", standardAdSetAttrs(t))
	res.Identity = resource.Identity{ID: testAdSetID}
	got, err := p.Destroy(context.Background(), res)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != provider.DestroyStatusRemoved {
		t.Fatalf("status=%q", got.Status)
	}
	got, err = p.Destroy(context.Background(), res)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != provider.DestroyStatusAlreadyAbsent {
		t.Fatalf("second status=%q", got.Status)
	}
	if _, err := p.Read(context.Background(), res); !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("read=%v", err)
	}
}

func standardAdSetAttrs(t *testing.T) resource.Attributes {
	t.Helper()
	return resource.Attributes{meta.AttrName: "Instagram US", meta.AttrCampaign: resource.Ref{Address: campaignAddress(t, "acquisition")}, meta.AttrLifetimeBudget: 50000, meta.AttrStartTime: "2026-09-01T00:00:00-05:00", meta.AttrEndTime: "2026-10-01T00:00:00-05:00", meta.AttrOptimizationGoal: "OFFSITE_CONVERSIONS", meta.AttrDestinationType: "WEBSITE", meta.AttrPixel: resource.Ref{Address: pixelAddress(t, "website")}, meta.AttrCustomConversion: resource.Ref{Address: conversionAddress(t, "trial_started")}, meta.AttrTargeting: map[string]any{"countries": []any{"us"}, "publisherPlatforms": []any{"instagram"}, "instagramPositions": []any{"stories", "feed", "reels"}, "devicePlatforms": []any{"mobile"}}}
}
func standardAdSetResources(t *testing.T) []resource.Resource {
	return []resource.Resource{campaignResource(t, "acquisition", standardCampaignAttrs()), pixelResource(t, "website"), conversionResource(t, "trial_started", websiteConversionAttrs(t)), adSetResource(t, "instagram", standardAdSetAttrs(t))}
}
func adSetCatalog(t *testing.T) staticCatalog {
	return staticCatalog{"meta/campaign/" + testCampaignID: campaignAddress(t, "acquisition"), "meta/pixel/" + testPixelID: pixelAddress(t, "website"), "meta/custom_conversion/" + testConvID: conversionAddress(t, "trial_started")}
}
func rememberAdSetDependencies(t *testing.T, p *meta.Provider) {
	t.Helper()
	for addr, id := range map[resource.Address]string{campaignAddress(t, "acquisition"): testCampaignID, pixelAddress(t, "website"): testPixelID, conversionAddress(t, "trial_started"): testConvID} {
		if _, err := p.Import(context.Background(), addr, id); err != nil {
			t.Fatal(err)
		}
	}
}
func instagramTargetingAPI() graphObject {
	return graphObject{"geo_locations": graphObject{"countries": []string{"US"}}, "age_min": 18, "age_max": 65, "publisher_platforms": []string{"instagram"}, "instagram_positions": []string{"stream", "story", "reels"}, "device_platforms": []string{"mobile"}}
}
