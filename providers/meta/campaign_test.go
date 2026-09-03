package meta_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dziblo-music/agoraform/internal/importer"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/meta"
)

func TestValidateCampaignAndSafeDefaults(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	res := campaignResource(t, "acquisition", standardCampaignAttrs())
	if err := p.Validate(context.Background(), res); err != nil {
		t.Fatal(err)
	}
	want, _, err := p.NormalizeComparable(res, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want[meta.AttrStatus] != "PAUSED" || want[meta.AttrBuyingType] != "AUCTION" || want[meta.AttrAdSetBudgetSharing] != false {
		t.Fatalf("defaults = %#v", want)
	}

	tests := []struct {
		name     string
		mutate   func(resource.Attributes)
		contains string
	}{
		{"legacy objective", func(a resource.Attributes) { a[meta.AttrObjective] = "CONVERSIONS" }, "objective"},
		{"terminal status", func(a resource.Attributes) { a[meta.AttrStatus] = "DELETED" }, "status"},
		{"reserved", func(a resource.Attributes) { a[meta.AttrBuyingType] = "RESERVED" }, "out of scope"},
		{"bad category", func(a resource.Attributes) { a[meta.AttrSpecialAdCategories] = []any{"ALCOHOL"} }, "specialAdCategories"},
		{"duplicate category", func(a resource.Attributes) { a[meta.AttrSpecialAdCategories] = []any{"CREDIT", "credit"} }, "duplicate"},
		{"missing categories", func(a resource.Attributes) { delete(a, meta.AttrSpecialAdCategories) }, "empty list"},
		{"double budget", func(a resource.Attributes) { a[meta.AttrDailyBudget] = 1000; a[meta.AttrLifetimeBudget] = 5000 }, "mutually exclusive"},
		{"fractional budget", func(a resource.Attributes) { a[meta.AttrDailyBudget] = 10.5 }, "smallest unit"},
		{"bid without budget", func(a resource.Attributes) { a[meta.AttrBidStrategy] = "COST_CAP" }, "requires"},
		{"budget sharing with campaign budget", func(a resource.Attributes) { a[meta.AttrDailyBudget] = 1000; a[meta.AttrAdSetBudgetSharing] = true }, "cannot be true"},
		{"non-boolean budget sharing", func(a resource.Attributes) { a[meta.AttrAdSetBudgetSharing] = "false" }, "must be a boolean"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			attrs := standardCampaignAttrs()
			tc.mutate(attrs)
			err := p.Validate(context.Background(), campaignResource(t, "bad", attrs))
			if err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("error = %v, want %q", err, tc.contains)
			}
		})
	}
}

func TestCreateReadUpdateCampaign(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	attrs := standardCampaignAttrs()
	attrs[meta.AttrDailyBudget] = 5000
	attrs[meta.AttrBidStrategy] = "LOWEST_COST_WITHOUT_CAP"
	created, err := p.Create(context.Background(), campaignResource(t, "acquisition", attrs))
	if err != nil {
		t.Fatal(err)
	}
	if created.Identity.ID != testCampaignID || created.Attributes[meta.AttrStatus] != "PAUSED" {
		t.Fatalf("created = %#v", created)
	}
	bound := campaignResource(t, "acquisition", attrs)
	bound.Identity = created.Identity
	live, err := p.Read(context.Background(), bound)
	if err != nil {
		t.Fatal(err)
	}
	updatedAttrs := attrs.Clone()
	updatedAttrs[meta.AttrName] = "Acquisition 2026"
	updatedAttrs[meta.AttrStatus] = "ACTIVE"
	updatedAttrs[meta.AttrDailyBudget] = 6000
	desired := campaignResource(t, "acquisition", updatedAttrs)
	desired.Identity = created.Identity
	updated, err := p.Update(context.Background(), desired, live)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Attributes[meta.AttrName] != "Acquisition 2026" || updated.Attributes[meta.AttrStatus] != "ACTIVE" || updated.Attributes[meta.AttrDailyBudget] != int64(6000) {
		t.Fatalf("updated=%#v", updated.Attributes)
	}
	posts, _ := srv.mutationCounts()
	if posts != 2 {
		t.Fatalf("posts=%d, want create+update", posts)
	}
	if _, err := p.Update(context.Background(), desired, updated); err != nil {
		t.Fatal(err)
	}
	posts, _ = srv.mutationCounts()
	if posts != 2 {
		t.Fatalf("no-op mutated: posts=%d", posts)
	}
}

func TestCreateAndUpdateAdSetBudgetSharing(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	attrs := standardCampaignAttrs()
	attrs[meta.AttrAdSetBudgetSharing] = true
	created, err := p.Create(context.Background(), campaignResource(t, "shared", attrs))
	if err != nil {
		t.Fatal(err)
	}
	if created.Attributes[meta.AttrAdSetBudgetSharing] != true {
		t.Fatalf("created attributes = %#v", created.Attributes)
	}

	desiredAttrs := attrs.Clone()
	desiredAttrs[meta.AttrAdSetBudgetSharing] = false
	desired := campaignResource(t, "shared", desiredAttrs)
	desired.Identity = created.Identity
	updated, err := p.Update(context.Background(), desired, created)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Attributes[meta.AttrAdSetBudgetSharing] != false {
		t.Fatalf("updated attributes = %#v", updated.Attributes)
	}
}

func TestCampaignImmutableChangesFailPlanning(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedCampaign(testCampaignID, graphObject{"name": "Acquisition", "objective": "OUTCOME_SALES"})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(campaignAddress(t, "acquisition"), resource.Identity{ID: testCampaignID}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		mutate   func(resource.Attributes)
		contains string
	}{
		{"objective", func(a resource.Attributes) { a[meta.AttrObjective] = "OUTCOME_TRAFFIC" }, "objective is immutable"},
		{"budget ownership", func(a resource.Attributes) { a[meta.AttrDailyBudget] = 1000 }, "budget ownership/type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attrs := standardCampaignAttrs()
			tc.mutate(attrs)
			_, err := plan.BuildWithState(context.Background(), []resource.Resource{campaignResource(t, "acquisition", attrs)}, func(resource.Address) (provider.Reader, error) { return p, nil }, st)
			if err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("plan error=%v", err)
			}
		})
	}
	posts, deletes := srv.mutationCounts()
	if posts != 0 || deletes != 0 {
		t.Fatalf("plan mutated state posts=%d deletes=%d", posts, deletes)
	}
}

func TestCampaignPlanShowsSafeCreateAndServingTransition(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	createdPlan, err := plan.Build(context.Background(), []resource.Resource{
		campaignResource(t, "new", standardCampaignAttrs()),
	}, func(resource.Address) (provider.Reader, error) { return p, nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(createdPlan.Changes) != 1 || createdPlan.Changes[0].Action != plan.ActionCreate || createdPlan.Changes[0].After[meta.AttrStatus] != "PAUSED" {
		t.Fatalf("create plan = %#v", createdPlan.Changes)
	}

	srv.seedCampaign(testCampaignID, graphObject{"name": "Acquisition", "objective": "OUTCOME_SALES"})
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(campaignAddress(t, "acquisition"), resource.Identity{ID: testCampaignID}); err != nil {
		t.Fatal(err)
	}
	attrs := standardCampaignAttrs()
	attrs[meta.AttrStatus] = "ACTIVE"
	activePlan, err := plan.BuildWithState(context.Background(), []resource.Resource{
		campaignResource(t, "acquisition", attrs),
	}, func(resource.Address) (provider.Reader, error) { return p, nil }, st)
	if err != nil {
		t.Fatal(err)
	}
	change := activePlan.Changes[0]
	if change.Action != plan.ActionUpdate || change.Before[meta.AttrStatus] != "PAUSED" || change.After[meta.AttrStatus] != "ACTIVE" {
		t.Fatalf("serving plan = %#v", change)
	}
}

func TestImportCampaignPreservesActiveStatusAndPlansCleanly(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedCampaign(testCampaignID, graphObject{"name": "Existing", "objective": "OUTCOME_TRAFFIC", "status": "ACTIVE", "configured_status": "ACTIVE", "effective_status": "ACTIVE", "daily_budget": "2500", "bid_strategy": "LOWEST_COST_WITHOUT_CAP", "special_ad_categories": []string{"HOUSING"}})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	live, err := p.Import(context.Background(), campaignAddress(t, "existing"), testCampaignID)
	if err != nil {
		t.Fatal(err)
	}
	if live.Attributes[meta.AttrStatus] != "ACTIVE" {
		t.Fatalf("status=%v", live.Attributes[meta.AttrStatus])
	}
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(live.Address, live.Identity); err != nil {
		t.Fatal(err)
	}
	got, err := plan.BuildWithState(context.Background(), []resource.Resource{{Address: live.Address, Attributes: live.Attributes.Clone()}}, func(resource.Address) (provider.Reader, error) { return p, nil }, st)
	if err != nil {
		t.Fatal(err)
	}
	if got.HasChanges() {
		t.Fatalf("imported plan changed:\n%s", plan.Format(got))
	}
	posts, deletes := srv.mutationCounts()
	if posts != 0 || deletes != 0 {
		t.Fatalf("import mutated posts=%d deletes=%d", posts, deletes)
	}
}

func TestImportCampaignEmitsCanonicalYAML(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedCampaign(testCampaignID, graphObject{
		"name": "Existing", "objective": "OUTCOME_TRAFFIC", "status": "ACTIVE",
		"configured_status": "ACTIVE", "daily_budget": "2500",
		"special_ad_categories": []string{"HOUSING"},
	})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Run(context.Background(), campaignAddress(t, "existing"), testCampaignID, func(resource.Address) (provider.Provider, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"objective: OUTCOME_TRAFFIC", "status: ACTIVE", "dailyBudget: 2500", "specialAdCategories:", "- HOUSING"} {
		if !strings.Contains(result.YAML, want) {
			t.Fatalf("import YAML missing %q:\n%s", want, result.YAML)
		}
	}
	if strings.Contains(result.YAML, "account_id") || strings.Contains(result.YAML, testAccountID) {
		t.Fatalf("account identity leaked into import YAML:\n%s", result.YAML)
	}
}

func TestDestroyCampaignIsIdempotent(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedCampaign(testCampaignID, graphObject{"name": "Acquisition", "objective": "OUTCOME_SALES"})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	res := campaignResource(t, "acquisition", standardCampaignAttrs())
	res.Identity = resource.Identity{ID: testCampaignID}
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
		t.Fatalf("read after delete=%v", err)
	}
}

func TestCampaignAPIErrorDoesNotLeakToken(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	res := campaignResource(t, "missing", standardCampaignAttrs())
	res.Identity = resource.Identity{ID: "123"}
	_, err := p.Read(context.Background(), res)
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("read=%v", err)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("token leaked: %v", err)
	}
}

func TestCampaignCreateReportsAPIFailureWithoutToken(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
			t.Errorf("Authorization = %q", got)
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":{"message":"temporary campaign failure","code":1,"is_transient":true}}`)
	}))
	defer server.Close()
	p := meta.NewWithHTTPClient(meta.Config{
		AccessToken: testToken,
		AdAccountID: testAccountID,
		BaseURL:     server.URL,
		Timeout:     time.Second,
	}, server.Client())
	_, err := p.Create(context.Background(), campaignResource(t, "failure", standardCampaignAttrs()))
	if err == nil || !strings.Contains(err.Error(), "temporary campaign failure") {
		t.Fatalf("create error = %v", err)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("token leaked: %v", err)
	}
}
