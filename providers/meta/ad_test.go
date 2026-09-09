package meta_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/graph"
	"github.com/dziblo-music/agoraform/internal/importer"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/meta"
)

func TestValidateAdDefaultsPausedAndRequiresTypedReferences(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	res := adResource(t, "instagram", standardAdAttrs(t))
	if err := p.Validate(context.Background(), res); err != nil {
		t.Fatal(err)
	}
	want, _, err := p.NormalizeComparable(res, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want[meta.AttrStatus] != "PAUSED" {
		t.Fatalf("status=%v", want[meta.AttrStatus])
	}
	for _, tc := range []struct {
		key      string
		value    any
		contains string
	}{
		{meta.AttrAdSet, "222333444555666", "resource reference"},
		{meta.AttrCreative, resource.Ref{Address: adSetAddress(t, "wrong")}, "meta.ad_creative"},
	} {
		attrs := standardAdAttrs(t)
		attrs[tc.key] = tc.value
		err := p.Validate(context.Background(), adResource(t, "bad", attrs))
		if err == nil || !strings.Contains(err.Error(), tc.contains) {
			t.Fatalf("%s error=%v", tc.key, err)
		}
	}
}

func TestCreateReadUpdateCreativeAndNoOpAd(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	seedAdDependencies(srv)
	const nextCreativeID = "555666777888999"
	srv.seedCreative(nextCreativeID, graphObject{"name": "Creative v2"})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	catalog := adCatalog(t)
	catalog["meta/ad_creative/"+nextCreativeID] = creativeAddress(t, "instagram_v2")
	p.SetIdentityCatalog(catalog)
	rememberAdDependencies(t, p, testCreativeID)
	created, err := p.Create(context.Background(), adResource(t, "instagram", standardAdAttrs(t)))
	if err != nil {
		t.Fatal(err)
	}
	if created.Identity.ID != testAdID || created.Computed[meta.OutputAdID] != testAdID || created.Attributes[meta.AttrStatus] != "PAUSED" {
		t.Fatalf("created=%#v", created)
	}
	if _, err := p.Import(context.Background(), creativeAddress(t, "instagram_v2"), nextCreativeID); err != nil {
		t.Fatal(err)
	}
	desired := adResource(t, "instagram", standardAdAttrs(t))
	desired.Identity = created.Identity
	desired.Attributes[meta.AttrName] = "Instagram Trial Ad v2"
	desired.Attributes[meta.AttrStatus] = "ACTIVE"
	desired.Attributes[meta.AttrCreative] = resource.Ref{Address: creativeAddress(t, "instagram_v2")}
	updated, err := p.Update(context.Background(), desired, created)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Attributes[meta.AttrName] != "Instagram Trial Ad v2" || updated.Attributes[meta.AttrStatus] != "ACTIVE" || updated.Attributes[meta.AttrCreative].(resource.Ref).Address != creativeAddress(t, "instagram_v2") {
		t.Fatalf("updated=%#v", updated.Attributes)
	}
	posts, _ := srv.mutationCounts()
	if posts != 2 {
		t.Fatalf("posts=%d, want create plus update", posts)
	}
	if _, err := p.Update(context.Background(), desired, updated); err != nil {
		t.Fatal(err)
	}
	posts, _ = srv.mutationCounts()
	if posts != 2 {
		t.Fatalf("no-op posts=%d", posts)
	}
}

func TestAdPlanShowsSafeCreateAndExplicitActivation(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	seedAdDependencies(srv)
	srv.seedAd(testAdID, nil)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adCatalog(t))
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for addr, id := range map[resource.Address]string{adSetAddress(t, "instagram"): testAdSetID, creativeAddress(t, "instagram"): testCreativeID} {
		if err := st.Bind(addr, resource.Identity{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	resources := standardAdResources(t)
	createdPlan, err := plan.BuildWithState(context.Background(), resources, func(resource.Address) (provider.Reader, error) { return p, nil }, st)
	if err != nil {
		t.Fatal(err)
	}
	if change := findChange(t, createdPlan, meta.TypeAd); change.Action != plan.ActionCreate || change.After[meta.AttrStatus] != "PAUSED" {
		t.Fatalf("create=%#v", change)
	}
	if err := st.Bind(adAddress(t, "instagram"), resource.Identity{ID: testAdID}); err != nil {
		t.Fatal(err)
	}
	cleanPlan, err := plan.BuildWithState(context.Background(), resources, func(resource.Address) (provider.Reader, error) { return p, nil }, st)
	if err != nil {
		t.Fatal(err)
	}
	if change := findChange(t, cleanPlan, meta.TypeAd); change.Action != plan.ActionUnchanged {
		t.Fatalf("equivalent ad plan=%#v", change)
	}
	resources[len(resources)-1].Attributes[meta.AttrStatus] = "ACTIVE"
	activePlan, err := plan.BuildWithState(context.Background(), resources, func(resource.Address) (provider.Reader, error) { return p, nil }, st)
	if err != nil {
		t.Fatal(err)
	}
	change := findChange(t, activePlan, meta.TypeAd)
	if change.Action != plan.ActionUpdate || change.Before[meta.AttrStatus] != "PAUSED" || change.After[meta.AttrStatus] != "ACTIVE" {
		t.Fatalf("active=%#v", change)
	}
}

func TestAdDependencyOrder(t *testing.T) {
	t.Parallel()
	resources := standardAdResources(t)
	g, err := graph.Build(resources)
	if err != nil {
		t.Fatal(err)
	}
	created := addressPositions(g.Order())
	destroyed := addressPositions(g.ReverseOrder())
	ad := adAddress(t, "instagram").String()
	adSet := adSetAddress(t, "instagram").String()
	creative := creativeAddress(t, "instagram").String()
	if created[ad] <= created[adSet] || created[ad] <= created[creative] {
		t.Fatalf("create order=%v", g.Order())
	}
	if destroyed[ad] >= destroyed[adSet] || destroyed[ad] >= destroyed[creative] {
		t.Fatalf("destroy order=%v", g.ReverseOrder())
	}
}

func TestAdImmutableParentFailsPlanningWithoutMutation(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedAd(testAdID, nil)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adCatalog(t))
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(adAddress(t, "instagram"), resource.Identity{ID: testAdID}); err != nil {
		t.Fatal(err)
	}
	attrs := standardAdAttrs(t)
	attrs[meta.AttrAdSet] = resource.Ref{Address: adSetAddress(t, "other")}
	p.SetIdentityCatalog(staticCatalog{
		"meta/ad_set/" + testAdSetID:         adSetAddress(t, "instagram"),
		"meta/ad_creative/" + testCreativeID: creativeAddress(t, "instagram"),
	})
	resources := standardAdSetResources(t)
	resources = append(resources,
		adSetResource(t, "other", standardAdSetAttrs(t)),
		creativeResource(t, "instagram", standardImageCreativeAttrs()),
		adResource(t, "instagram", attrs),
	)
	_, err = plan.BuildWithState(context.Background(), resources, func(resource.Address) (provider.Reader, error) { return p, nil }, st)
	if err == nil || !strings.Contains(err.Error(), "adSet is immutable") {
		t.Fatalf("error=%v", err)
	}
	posts, deletes := srv.mutationCounts()
	if posts != 0 || deletes != 0 {
		t.Fatalf("plan mutated posts=%d deletes=%d", posts, deletes)
	}
}

func TestImportAdReconstructsRelationshipsAndPreservesStatus(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedAd(testAdID, graphObject{"status": "ACTIVE", "configured_status": "ACTIVE", "effective_status": "ACTIVE"})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adCatalog(t))
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Run(context.Background(), adAddress(t, "instagram"), testAdID, func(resource.Address) (provider.Provider, error) { return p, nil }, st)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"status: ACTIVE", "$ref: meta.ad_set.instagram", "$ref: meta.ad_creative.instagram"} {
		if !strings.Contains(result.YAML, want) {
			t.Fatalf("YAML missing %q:\n%s", want, result.YAML)
		}
	}
}

func TestImportAdRejectsUnboundRelationships(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedAd(testAdID, nil)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	_, err := p.Import(context.Background(), adAddress(t, "instagram"), testAdID)
	if err == nil || !strings.Contains(err.Error(), "ad-set relationship") {
		t.Fatalf("error=%v", err)
	}
}

func TestAdCreateReturnsConfirmedIdentityWhenRefreshFails(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	seedAdDependencies(srv)
	srv.adRefreshFailures = 1
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adCatalog(t))
	rememberAdDependencies(t, p, testCreativeID)
	created, err := p.Create(context.Background(), adResource(t, "instagram", standardAdAttrs(t)))
	if err != nil {
		t.Fatal(err)
	}
	if created.Identity.ID != testAdID || created.Computed[meta.OutputAdID] != testAdID {
		t.Fatalf("created=%#v", created)
	}
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for addr, id := range map[resource.Address]string{
		campaignAddress(t, "acquisition"):     testCampaignID,
		pixelAddress(t, "website"):            testPixelID,
		conversionAddress(t, "trial_started"): testConvID,
		adSetAddress(t, "instagram"):          testAdSetID,
		creativeAddress(t, "instagram"):       testCreativeID,
	} {
		if err := st.Bind(addr, resource.Identity{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.RecordCreate(adAddress(t, "instagram"), created); err != nil {
		t.Fatal(err)
	}
	retryPlan, err := plan.BuildWithState(context.Background(), standardAdResources(t), func(resource.Address) (provider.Reader, error) { return p, nil }, st)
	if err != nil {
		t.Fatal(err)
	}
	if change := findChange(t, retryPlan, meta.TypeAd); change.Action != plan.ActionUnchanged {
		t.Fatalf("retry plan=%#v", change)
	}
	posts, _ := srv.mutationCounts()
	if posts != 1 {
		t.Fatalf("posts=%d, want one create", posts)
	}
}

func TestDestroyAdIsIdempotent(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedAd(testAdID, nil)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adCatalog(t))
	res := adResource(t, "instagram", standardAdAttrs(t))
	res.Identity = resource.Identity{ID: testAdID}
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

func TestAdAPIFailureDoesNotLeakToken(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	seedAdDependencies(srv)
	srv.adCreateFailure = true
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adCatalog(t))
	rememberAdDependencies(t, p, testCreativeID)
	_, err := p.Create(context.Background(), adResource(t, "failure", standardAdAttrs(t)))
	if err == nil || !strings.Contains(err.Error(), "temporary ad failure") {
		t.Fatalf("error=%v", err)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("token leaked: %v", err)
	}
}

func standardAdAttrs(t *testing.T) resource.Attributes {
	t.Helper()
	return resource.Attributes{
		meta.AttrName:     "Instagram Trial Ad",
		meta.AttrAdSet:    resource.Ref{Address: adSetAddress(t, "instagram")},
		meta.AttrCreative: resource.Ref{Address: creativeAddress(t, "instagram")},
	}
}

func standardAdResources(t *testing.T) []resource.Resource {
	resources := standardAdSetResources(t)
	return append(resources,
		creativeResource(t, "instagram", standardImageCreativeAttrs()),
		adResource(t, "instagram", standardAdAttrs(t)),
	)
}

func adCatalog(t *testing.T) staticCatalog {
	catalog := adSetCatalog(t)
	catalog["meta/ad_set/"+testAdSetID] = adSetAddress(t, "instagram")
	catalog["meta/ad_creative/"+testCreativeID] = creativeAddress(t, "instagram")
	return catalog
}

func rememberAdDependencies(t *testing.T, p *meta.Provider, creativeID string) {
	t.Helper()
	for addr, id := range map[resource.Address]string{
		campaignAddress(t, "acquisition"):     testCampaignID,
		pixelAddress(t, "website"):            testPixelID,
		conversionAddress(t, "trial_started"): testConvID,
	} {
		if _, err := p.Import(context.Background(), addr, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.Import(context.Background(), adSetAddress(t, "instagram"), testAdSetID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Import(context.Background(), creativeAddress(t, "instagram"), creativeID); err != nil {
		t.Fatal(err)
	}
}

func seedAdDependencies(srv *graphServer) {
	srv.seedCampaign(testCampaignID, graphObject{"name": "Acquisition", "objective": "OUTCOME_SALES"})
	srv.seedPixel(testPixelID, "Website")
	srv.seedConversion(testConvID, graphObject{
		"name": "Trial Started", "custom_event_type": "START_TRIAL",
		"rule":  `{"and":[{"event":{"eq":"StartTrial"}}]}`,
		"pixel": graphObject{"id": testPixelID}, "event_source_type": "pixel",
	})
	srv.seedAdSet(testAdSetID, graphObject{
		"lifetime_budget": "50000", "start_time": "2026-09-01T05:00:00Z",
		"end_time": "2026-10-01T05:00:00Z", "targeting": instagramTargetingAPI(),
	})
	srv.seedCreative(testCreativeID, nil)
}

func findChange(t *testing.T, p *plan.Plan, typ string) plan.Change {
	t.Helper()
	for _, change := range p.Changes {
		if change.Address.Type == typ {
			return change
		}
	}
	t.Fatalf("missing %s change", typ)
	return plan.Change{}
}

func addressPositions(addresses []resource.Address) map[string]int {
	out := make(map[string]int, len(addresses))
	for i, addr := range addresses {
		out[addr.String()] = i
	}
	return out
}
