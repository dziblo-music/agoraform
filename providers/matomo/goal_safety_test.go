package matomo_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestUpdateGoalPreservesUnmanagedMatomoFields(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	pattern := "oldPattern"
	var update url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		vals, _ := url.ParseQuery(string(body))
		switch vals.Get("method") {
		case "Goals.getGoals":
			mu.Lock()
			p := pattern
			mu.Unlock()
			_, _ = io.WriteString(w, `{"7":{"idgoal":"7","idsite":"3","name":"Trial Started","description":"Paid signup","match_attribute":"event_action","pattern":"`+p+`","pattern_type":"contains","case_sensitive":"1","allow_multiple":"1","revenue":"25.50","deleted":"0","event_value_as_revenue":"1"}}`)
		case "Goals.updateGoal":
			mu.Lock()
			update = vals
			pattern = vals.Get("pattern")
			mu.Unlock()
			_, _ = io.WriteString(w, `null`)
		default:
			_, _ = io.WriteString(w, `"5.2.0"`)
		}
	}))
	t.Cleanup(srv.Close)

	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:    srv.URL,
		TokenAuth:  providerToken,
		SiteID:     "3",
		HTTPClient: srv.Client(),
	}, srv.Client())
	desired := goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "trialStarted",
	})
	desired.Identity = resource.Identity{ID: "7"}

	live, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "7"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if live.Identity.ID != "7" || live.Attributes[matomo.AttrPattern] != "trialStarted" {
		t.Fatalf("live = %+v", live)
	}

	mu.Lock()
	got := update
	mu.Unlock()
	if got == nil {
		t.Fatal("Goals.updateGoal was not called")
	}
	checks := map[string]string{
		"idGoal":                           "7",
		"caseSensitive":                    "1",
		"revenue":                          "25.50",
		"allowMultipleConversionsPerVisit": "1",
		"description":                      "Paid signup",
		"useEventValueAsRevenue":           "1",
	}
	for key, want := range checks {
		if got.Get(key) != want {
			t.Fatalf("%s = %q, want %q; request=%v", key, got.Get(key), want, got)
		}
	}
}

func TestReadGoalUsesBoundIDGoalInsteadOfNameDiscovery(t *testing.T) {
	t.Parallel()

	srv := newGoalServer(t)
	srv.seed(apiGoal{ID: 5, Name: "Trial Started", MatchAttribute: "event_action", Pattern: "trialStarted", PatternType: "contains"})
	srv.seed(apiGoal{ID: 6, Name: "Trial Started", MatchAttribute: "event_action", Pattern: "other", PatternType: "contains"})
	p := testGoalProvider(t, srv)

	res := goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "trialStarted",
	})
	res.Identity = resource.Identity{ID: "5"}
	live, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "5" {
		t.Fatalf("identity = %q, want 5", live.Identity.ID)
	}
}

func TestBoundGoalNameIsImmutable(t *testing.T) {
	t.Parallel()

	srv := newGoalServer(t)
	srv.seed(apiGoal{ID: 5, Name: "Trial Started", MatchAttribute: "event_action", Pattern: "trialStarted", PatternType: "contains"})
	p := testGoalProvider(t, srv)

	res := goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Renamed",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "trialStarted",
	})
	res.Identity = resource.Identity{ID: "5"}
	_, err := p.Read(context.Background(), res)
	if err == nil {
		t.Fatal("expected immutable name error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("rename must not become a create candidate: %v", err)
	}
	if !strings.Contains(err.Error(), "immutable") || !strings.Contains(err.Error(), "name") {
		t.Fatalf("error = %q, want immutable name diagnostic", err)
	}
}

func TestBoundGoalMissingRemoteIsNotCreateCandidate(t *testing.T) {
	t.Parallel()

	p := testGoalProvider(t, newGoalServer(t))
	res := goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "trialStarted",
	})
	res.Identity = resource.Identity{ID: "99"}
	_, err := p.Read(context.Background(), res)
	if err == nil {
		t.Fatal("expected stale identity error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("stale identity must not plan a replacement create: %v", err)
	}
	if !strings.Contains(err.Error(), "persisted identity") {
		t.Fatalf("error = %q, want persisted identity diagnostic", err)
	}
}

func TestPlanBoundGoalIdentityDoesNotProduceDiff(t *testing.T) {
	t.Parallel()

	srv := newGoalServer(t)
	srv.seed(apiGoal{ID: 5, Name: "Trial Started", MatchAttribute: "event_action", Pattern: "trialStarted", PatternType: "contains"})
	p := testGoalProvider(t, srv)
	res := goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "trialStarted",
	})
	res.Identity = resource.Identity{ID: "5"}
	got := mustPlanGoal(t, p, res)
	if got.HasChanges() {
		t.Fatalf("identity binding produced diff: %+v", got.Changes)
	}
	if got.Changes[0].Identity.ID != "5" {
		t.Fatalf("identity = %q, want 5", got.Changes[0].Identity.ID)
	}
}

func TestValidateFileExactMatchesMatomoHTTPRequirement(t *testing.T) {
	t.Parallel()

	p := testGoalProvider(t, newGoalServer(t))
	err := p.Validate(context.Background(), goalResource(t, "download", resource.Attributes{
		matomo.AttrName:           "Download",
		matomo.AttrMatchAttribute: "file",
		matomo.AttrPattern:        "download.zip",
		matomo.AttrPatternType:    "exact",
	}))
	if err == nil {
		t.Fatal("expected exact file validation error")
	}
	if !strings.Contains(err.Error(), "http://") {
		t.Fatalf("error = %q, want Matomo HTTP-prefix requirement", err)
	}
}

func TestValidateRejectsManifestIDGoal(t *testing.T) {
	t.Parallel()

	p := testGoalProvider(t, newGoalServer(t))
	for _, value := range []any{"", "abc", "0", -1, "12"} {
		value := value
		t.Run("id_"+strings.ReplaceAll(strings.TrimSpace(toTestString(value)), "-", "neg"), func(t *testing.T) {
			err := p.Validate(context.Background(), goalResource(t, "trial_started", resource.Attributes{
				matomo.AttrIDGoal:         value,
				matomo.AttrName:           "Trial Started",
				matomo.AttrMatchAttribute: "event_action",
				matomo.AttrPattern:        "trialStarted",
			}))
			if err == nil || !strings.Contains(err.Error(), matomo.AttrIDGoal) || !strings.Contains(err.Error(), "local state") {
				t.Fatalf("idGoal %v: error=%v", value, err)
			}
		})
	}
}

func TestMutablePatternChangeDoesNotRebindBoundGoal(t *testing.T) {
	t.Parallel()

	srv := newGoalServer(t)
	srv.seed(apiGoal{ID: 5, Name: "Trial Started", MatchAttribute: "event_action", Pattern: "trialStarted", PatternType: "contains"})
	srv.seed(apiGoal{ID: 6, Name: "Trial Started", MatchAttribute: "event_action", Pattern: "other", PatternType: "contains"})
	p := testGoalProvider(t, srv)

	res := goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "other",
	})
	res.Identity = resource.Identity{ID: "5"}
	got := mustPlanGoal(t, p, res)
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want update of bound goal", got.Changes)
	}
	if got.Changes[0].Identity.ID != "5" {
		t.Fatalf("identity = %q, want 5", got.Changes[0].Identity.ID)
	}
}

func TestPlanUsesPersistedStateIdentity(t *testing.T) {
	t.Parallel()

	srv := newGoalServer(t)
	srv.seed(apiGoal{ID: 12, Name: "Trial Started", MatchAttribute: "event_action", Pattern: "trialStarted", PatternType: "contains"})
	p := testGoalProvider(t, srv)
	res := goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "trialStarted",
	})

	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(res.Address, resource.Identity{ID: "12"}); err != nil {
		t.Fatal(err)
	}

	got, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("BuildWithState: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("state-bound equivalent goal produced changes: %+v", got.Changes)
	}
	if got.Changes[0].Identity.ID != "12" {
		t.Fatalf("identity = %q, want 12", got.Changes[0].Identity.ID)
	}
}

func toTestString(v any) string {
	switch x := v.(type) {
	case string:
		if x == "" {
			return "empty"
		}
		return x
	case int:
		return strconv.Itoa(x)
	default:
		return "value"
	}
}
