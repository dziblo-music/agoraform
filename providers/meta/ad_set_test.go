package meta_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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
		{"double budget", "mutually exclusive", func(a resource.Attributes) { a[meta.AttrDailyBudget] = 10 }},
		{"lifetime without end", "requires both", func(a resource.Attributes) { delete(a, meta.AttrEndTime) }},
		{"bad schedule", "must be after", func(a resource.Attributes) { a[meta.AttrEndTime] = "2026-09-01T00:00:00Z" }},
		{"bid cap without amount", "requires bidAmount", func(a resource.Attributes) { a[meta.AttrBidStrategy] = "LOWEST_COST_WITH_BID_CAP" }},
		{"amount without cap", "valid only", func(a resource.Attributes) { a[meta.AttrBidAmount] = 1 }},
		{"placement without platform", "requires publisherPlatforms", func(a resource.Attributes) { a[meta.AttrTargeting].(map[string]any)["publisherPlatforms"] = []any{} }},
		{"raw targeting", "unsupported targeting field", func(a resource.Attributes) {
			a[meta.AttrTargeting].(map[string]any)["behaviors"] = []any{"frequent_travelers"}
		}},
		{"name-only interest", "name-only matching is not supported", func(a resource.Attributes) {
			a[meta.AttrTargeting].(map[string]any)["interests"] = []any{map[string]any{"name": "Music production"}}
		}},
		{"string interest", "must be a numeric Meta identifier", func(a resource.Attributes) {
			a[meta.AttrTargeting].(map[string]any)["interests"] = []any{"music"}
		}},
		{"include and exclude overlap", "cannot include and exclude", func(a resource.Attributes) {
			a[meta.AttrTargeting].(map[string]any)["customAudiences"] = []any{map[string]any{"id": testAudienceIncludeID}}
			a[meta.AttrTargeting].(map[string]any)["excludedCustomAudiences"] = []any{map[string]any{"id": testAudienceIncludeID}}
		}},
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
	campaignBudget.Attributes[meta.AttrDailyBudget] = 50
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
	campaignAttrs[meta.AttrDailyBudget] = 50
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
	desired.Attributes[meta.AttrLifetimeBudget] = 600
	desired.Attributes[meta.AttrEndTime] = "2026-10-02T05:00:00Z"
	updated, err := p.Update(context.Background(), desired, created)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Attributes[meta.AttrName] != "Instagram US" || updated.Attributes[meta.AttrStatus] != "ACTIVE" || updated.Attributes[meta.AttrLifetimeBudget] != int64(600) {
		t.Fatalf("updated=%#v", updated.Attributes)
	}
	if got := srv.adSetField(testAdSetID, "lifetime_budget"); got != "60000" {
		t.Fatalf("lifetime_budget sent to Meta = %v, want minimum-denomination 60000", got)
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

func TestValidateAdSetNormalizesAudienceTargeting(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	attrs := standardAdSetAttrs(t)
	attrs[meta.AttrTargeting].(map[string]any)["customAudiences"] = []any{map[string]any{"id": testAudienceExcludeID}, map[string]any{"id": testAudienceIncludeID, "name": "Prospects"}}
	attrs[meta.AttrTargeting].(map[string]any)["excludedCustomAudiences"] = []any{map[string]any{"id": "999888777666555"}}
	attrs[meta.AttrTargeting].(map[string]any)["interests"] = []any{map[string]any{"id": testInterestID, "name": "Music production"}, map[string]any{"id": "6003020834693"}}
	if err := p.Validate(context.Background(), adSetResource(t, "instagram", attrs)); err != nil {
		t.Fatal(err)
	}
	want, _, err := p.NormalizeComparable(adSetResource(t, "instagram", attrs), nil)
	if err != nil {
		t.Fatal(err)
	}
	targeting := want[meta.AttrTargeting].(map[string]any)
	includes := targeting["customAudiences"].([]any)
	if len(includes) != 2 {
		t.Fatalf("customAudiences = %#v", includes)
	}
	if includes[0].(map[string]any)["id"] != testAudienceExcludeID || includes[1].(map[string]any)["id"] != testAudienceIncludeID {
		t.Fatalf("customAudiences order = %#v", includes)
	}
	if _, ok := includes[1].(map[string]any)["name"]; ok {
		t.Fatalf("comparable targeting must omit display names: %#v", includes)
	}
}

func TestCreateAdSetAudienceAndInterestTargeting(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedCampaign(testCampaignID, graphObject{"name": "Acquisition", "objective": "OUTCOME_SALES"})
	srv.seedPixel(testPixelID, "Website")
	srv.seedConversion(testConvID, graphObject{"name": "Trial Started", "custom_event_type": "START_TRIAL", "rule": `{"and":[{"event":{"eq":"StartTrial"}}]}`, "pixel": graphObject{"id": testPixelID}, "event_source_type": "pixel"})
	srv.seedAudience(testAudienceIncludeID, graphObject{"name": "Lookalike prospects", "subtype": "LOOKALIKE"})
	srv.seedAudience(testAudienceExcludeID, graphObject{"name": "Existing customers", "subtype": "WEBSITE"})
	srv.seedInterest(testInterestID, "Music production", true)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adSetCatalog(t))
	rememberAdSetDependencies(t, p)
	attrs := audienceAdSetAttrs(t)
	created, err := p.Create(context.Background(), adSetResource(t, "instagram", attrs))
	if err != nil {
		t.Fatal(err)
	}
	got := srv.adSetField(testAdSetID, "targeting").(graphObject)
	custom, _ := json.Marshal(got["custom_audiences"])
	excluded, _ := json.Marshal(got["excluded_custom_audiences"])
	flexible, _ := json.Marshal(got["flexible_spec"])
	if !strings.Contains(string(custom), testAudienceIncludeID) {
		t.Fatalf("custom_audiences = %s", custom)
	}
	if !strings.Contains(string(excluded), testAudienceExcludeID) {
		t.Fatalf("excluded_custom_audiences = %s", excluded)
	}
	if !strings.Contains(string(flexible), testInterestID) {
		t.Fatalf("flexible_spec = %s", flexible)
	}
	targeting := created.Attributes[meta.AttrTargeting].(map[string]any)
	if got := targeting["customAudiences"].([]any)[0].(map[string]any)["id"]; got != testAudienceIncludeID {
		t.Fatalf("custom audience id = %v", got)
	}
	if got := targeting["excludedCustomAudiences"].([]any)[0].(map[string]any)["id"]; got != testAudienceExcludeID {
		t.Fatalf("excluded custom audience id = %v", got)
	}
	if got := targeting["interests"].([]any)[0].(map[string]any)["id"]; got != testInterestID {
		t.Fatalf("interest id = %v", got)
	}
	desired := adSetResource(t, "instagram", attrs.Clone())
	desired.Identity = created.Identity
	if _, err := p.Update(context.Background(), desired, created); err != nil {
		t.Fatal(err)
	}
	posts, _ := srv.mutationCounts()
	if posts != 1 {
		t.Fatalf("name-only no-op posts=%d", posts)
	}
}

func TestAdSetPlanShowsAudienceTargetingUpdate(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedCampaign(testCampaignID, graphObject{"name": "Acquisition", "objective": "OUTCOME_SALES"})
	srv.seedPixel(testPixelID, "Website")
	srv.seedConversion(testConvID, graphObject{"name": "Trial Started", "custom_event_type": "START_TRIAL", "rule": `{"and":[{"event":{"eq":"StartTrial"}}]}`, "pixel": graphObject{"id": testPixelID}, "event_source_type": "pixel"})
	srv.seedAdSet(testAdSetID, graphObject{"name": "Instagram US", "lifetime_budget": "50000", "start_time": "2026-09-01T05:00:00+0000", "end_time": "2026-10-01T05:00:00+0000", "targeting": audienceTargetingAPI()})
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
	resources := append(standardAdSetResources(t)[:3], adSetResource(t, "instagram", audienceAdSetAttrs(t)))
	cleanPlan, err := plan.BuildWithState(context.Background(), resources, func(resource.Address) (provider.Reader, error) { return p, nil }, st)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range cleanPlan.Changes {
		if change.Address.Type == meta.TypeAdSet && change.Action != plan.ActionUnchanged {
			t.Fatalf("equivalent audience targeting plan = %#v", change)
		}
	}
	resources[len(resources)-1].Attributes = audienceAdSetAttrs(t)
	resources[len(resources)-1].Attributes[meta.AttrTargeting].(map[string]any)["excludedCustomAudiences"] = []any{map[string]any{"id": "999888777666555"}}
	updatedPlan, err := plan.BuildWithState(context.Background(), resources, func(resource.Address) (provider.Reader, error) { return p, nil }, st)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range updatedPlan.Changes {
		if change.Address.Type == meta.TypeAdSet {
			if change.Action != plan.ActionUpdate {
				t.Fatalf("targeting update action = %#v", change)
			}
			before := change.Before[meta.AttrTargeting].(map[string]any)["excludedCustomAudiences"].([]any)[0].(map[string]any)["id"]
			after := change.After[meta.AttrTargeting].(map[string]any)["excludedCustomAudiences"].([]any)[0].(map[string]any)["id"]
			if before != testAudienceExcludeID || after != "999888777666555" {
				t.Fatalf("targeting diff before=%v after=%v", before, after)
			}
		}
	}
}

func TestImportAdSetPreservesAudienceIdentifiers(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedAdSet(testAdSetID, graphObject{"lifetime_budget": "50000", "start_time": "2026-09-01T00:00:00-0500", "end_time": "2026-10-01T00:00:00-0500", "targeting": audienceTargetingAPI()})
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
	for _, want := range []string{"customAudiences:", "- id: \"" + testAudienceIncludeID + "\"", "excludedCustomAudiences:", "- id: \"" + testAudienceExcludeID + "\"", "name: Existing customers", "interests:", "- id: \"" + testInterestID + "\"", "name: Music production"} {
		if !strings.Contains(result.YAML, want) {
			t.Fatalf("YAML missing %q:\n%s", want, result.YAML)
		}
	}
}

func TestImportAdSetRejectsUnsupportedFlexibleSpec(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	targeting := instagramTargetingAPI()
	targeting["flexible_spec"] = []any{graphObject{"behaviors": []any{graphObject{"id": "6002714895372"}}}}
	srv.seedAdSet(testAdSetID, graphObject{"lifetime_budget": "50000", "start_time": "2026-09-01T00:00:00Z", "end_time": "2026-10-01T00:00:00Z", "targeting": targeting})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adSetCatalog(t))
	_, err := p.Import(context.Background(), adSetAddress(t, "instagram"), testAdSetID)
	if err == nil || !strings.Contains(err.Error(), "unsupported flexible_spec field") {
		t.Fatalf("error=%v", err)
	}
}

func TestCreateAdSetRejectsMissingAndInaccessibleAudiences(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, contains string
		setup          func(*graphServer)
		mutate         func(resource.Attributes)
	}{
		{
			name:     "missing custom audience",
			contains: "custom audience 555666777888999 was not found",
			setup:    func(*graphServer) {},
			mutate: func(a resource.Attributes) {
				a[meta.AttrTargeting].(map[string]any)["customAudiences"] = []any{map[string]any{"id": testAudienceIncludeID}}
			},
		},
		{
			name:     "inaccessible custom audience",
			contains: "is not accessible with the configured token",
			setup: func(s *graphServer) {
				s.seedAudienceFailure(testAudienceIncludeID, http.StatusForbidden, `{"error":{"message":"permission denied","code":200}}`)
			},
			mutate: func(a resource.Attributes) {
				a[meta.AttrTargeting].(map[string]any)["customAudiences"] = []any{map[string]any{"id": testAudienceIncludeID}}
			},
		},
		{
			name:     "authorization failure",
			contains: "authorization failed",
			setup: func(s *graphServer) {
				s.seedAudienceFailure(testAudienceIncludeID, http.StatusUnauthorized, `{"error":{"message":"invalid token `+testToken+`","type":"OAuthException","code":190}}`)
			},
			mutate: func(a resource.Attributes) {
				a[meta.AttrTargeting].(map[string]any)["customAudiences"] = []any{map[string]any{"id": testAudienceIncludeID}}
			},
		},
		{
			name:     "unsupported audience type",
			contains: "unsupported subtype MEASUREMENT",
			setup: func(s *graphServer) {
				s.seedAudience(testAudienceIncludeID, graphObject{"subtype": "MEASUREMENT"})
			},
			mutate: func(a resource.Attributes) {
				a[meta.AttrTargeting].(map[string]any)["customAudiences"] = []any{map[string]any{"id": testAudienceIncludeID}}
			},
		},
		{
			name:     "not a custom audience",
			contains: "is not a Custom Audience",
			setup: func(s *graphServer) {
				s.seedAudience(testAudienceIncludeID, graphObject{"name": "A Page", "subtype": ""})
				delete(s.audiences[testAudienceIncludeID], "subtype")
			},
			mutate: func(a resource.Attributes) {
				a[meta.AttrTargeting].(map[string]any)["customAudiences"] = []any{map[string]any{"id": testAudienceIncludeID}}
			},
		},
		{
			name:     "missing interest",
			contains: "interest 6003139266461 was not found",
			setup:    func(*graphServer) {},
			mutate: func(a resource.Attributes) {
				a[meta.AttrTargeting].(map[string]any)["interests"] = []any{map[string]any{"id": testInterestID}}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newGraphServer(t)
			srv.seedCampaign(testCampaignID, graphObject{"name": "Acquisition", "objective": "OUTCOME_SALES"})
			srv.seedPixel(testPixelID, "Website")
			srv.seedConversion(testConvID, graphObject{"name": "Trial Started", "custom_event_type": "START_TRIAL", "rule": `{"and":[{"event":{"eq":"StartTrial"}}]}`, "pixel": graphObject{"id": testPixelID}, "event_source_type": "pixel"})
			tc.setup(srv)
			httpSrv := srv.start()
			defer httpSrv.Close()
			p := testProvider(t, httpSrv)
			p.SetIdentityCatalog(adSetCatalog(t))
			rememberAdSetDependencies(t, p)
			attrs := standardAdSetAttrs(t)
			tc.mutate(attrs)
			_, err := p.Create(context.Background(), adSetResource(t, "instagram", attrs))
			if err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("error=%v, want %q", err, tc.contains)
			}
			if strings.Contains(err.Error(), testToken) {
				t.Fatalf("token leaked: %v", err)
			}
			posts, deletes := srv.mutationCounts()
			if posts != 0 || deletes != 0 {
				t.Fatalf("mutated posts=%d deletes=%d", posts, deletes)
			}
		})
	}
}

func TestDestroyAdSetLeavesCustomAudiencesUntouched(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedAudience(testAudienceExcludeID, graphObject{"name": "Existing customers", "subtype": "WEBSITE"})
	srv.seedAdSet(testAdSetID, graphObject{"lifetime_budget": "50000", "start_time": "2026-09-01T00:00:00Z", "end_time": "2026-10-01T00:00:00Z", "targeting": audienceTargetingAPI()})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adSetCatalog(t))
	res := adSetResource(t, "instagram", audienceAdSetAttrs(t))
	res.Identity = resource.Identity{ID: testAdSetID}
	got, err := p.Destroy(context.Background(), res)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != provider.DestroyStatusRemoved {
		t.Fatalf("status=%q", got.Status)
	}
	if srv.audience(testAudienceExcludeID) == nil {
		t.Fatal("excluded custom audience was deleted with the ad set")
	}
	if srv.audience(testAudienceIncludeID) != nil {
		t.Fatal("destroy seeded an included audience")
	}
	for _, req := range srv.recordedRequests() {
		if strings.Contains(req, "DELETE") && (strings.Contains(req, testAudienceExcludeID) || strings.Contains(req, testAudienceIncludeID)) {
			t.Fatalf("destroy issued audience mutation %q", req)
		}
	}
}

func audienceAdSetAttrs(t *testing.T) resource.Attributes {
	t.Helper()
	attrs := standardAdSetAttrs(t)
	targeting := attrs[meta.AttrTargeting].(map[string]any)
	targeting["customAudiences"] = []any{map[string]any{"id": testAudienceIncludeID}}
	targeting["excludedCustomAudiences"] = []any{map[string]any{"id": testAudienceExcludeID, "name": "Existing customers"}}
	targeting["interests"] = []any{map[string]any{"id": testInterestID, "name": "Music production"}}
	return attrs
}

func audienceTargetingAPI() graphObject {
	targeting := instagramTargetingAPI()
	targeting["custom_audiences"] = []any{graphObject{"id": testAudienceIncludeID}}
	targeting["excluded_custom_audiences"] = []any{graphObject{"id": testAudienceExcludeID, "name": "Existing customers"}}
	targeting["flexible_spec"] = []any{graphObject{"interests": []any{graphObject{"id": testInterestID, "name": "Music production"}}}}
	return targeting
}

func standardAdSetAttrs(t *testing.T) resource.Attributes {
	t.Helper()
	return resource.Attributes{meta.AttrName: "Instagram US", meta.AttrCampaign: resource.Ref{Address: campaignAddress(t, "acquisition")}, meta.AttrLifetimeBudget: 500, meta.AttrStartTime: "2026-09-01T00:00:00-05:00", meta.AttrEndTime: "2026-10-01T00:00:00-05:00", meta.AttrOptimizationGoal: "OFFSITE_CONVERSIONS", meta.AttrDestinationType: "WEBSITE", meta.AttrPixel: resource.Ref{Address: pixelAddress(t, "website")}, meta.AttrCustomConversion: resource.Ref{Address: conversionAddress(t, "trial_started")}, meta.AttrTargeting: map[string]any{"countries": []any{"us"}, "publisherPlatforms": []any{"instagram"}, "instagramPositions": []any{"stories", "feed", "reels"}, "devicePlatforms": []any{"mobile"}}}
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
