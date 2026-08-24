package matomo_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestValidateGoalValid(t *testing.T) {
	t.Parallel()

	p := testGoalProvider(t, newGoalServer(t))
	res := goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "trialStarted",
	})
	if err := p.Validate(context.Background(), res); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateGoalErrors(t *testing.T) {
	t.Parallel()

	p := testGoalProvider(t, newGoalServer(t))
	addr := mustGoalAddress(t, "trial_started")

	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name:  "missing name",
			attrs: resource.Attributes{matomo.AttrMatchAttribute: "event_action", matomo.AttrPattern: "x"},
			want:  "missing required attribute \"name\"",
		},
		{
			name:  "missing matchAttribute",
			attrs: resource.Attributes{matomo.AttrName: "Trial Started", matomo.AttrPattern: "x"},
			want:  "missing required attribute \"matchAttribute\"",
		},
		{
			name:  "missing pattern",
			attrs: resource.Attributes{matomo.AttrName: "Trial Started", matomo.AttrMatchAttribute: "event_action"},
			want:  "pattern",
		},
		{
			name:  "unknown matchAttribute",
			attrs: resource.Attributes{matomo.AttrName: "Trial Started", matomo.AttrMatchAttribute: "cookies", matomo.AttrPattern: "x"},
			want:  "matchAttribute",
		},
		{
			name:  "unsupported property",
			attrs: resource.Attributes{matomo.AttrName: "Trial Started", matomo.AttrMatchAttribute: "manually", "revenue": "10"},
			want:  "computed",
		},
		{
			name:  "unknown property",
			attrs: resource.Attributes{matomo.AttrName: "Trial Started", matomo.AttrMatchAttribute: "manually", "event": "trialStarted"},
			want:  "unsupported attribute",
		},
		{
			name:  "computed idgoal",
			attrs: resource.Attributes{matomo.AttrName: "Trial Started", matomo.AttrMatchAttribute: "manually", "idgoal": "1"},
			want:  "computed",
		},
		{
			name: "manual with pattern",
			attrs: resource.Attributes{
				matomo.AttrName:           "Manual",
				matomo.AttrMatchAttribute: "manually",
				matomo.AttrPattern:        "ignored",
			},
			want: "not used",
		},
		{
			name: "numeric pattern type",
			attrs: resource.Attributes{
				matomo.AttrName:           "Long visit",
				matomo.AttrMatchAttribute: "visit_duration",
				matomo.AttrPattern:        "30",
				matomo.AttrPatternType:    "contains",
			},
			want: "greater_than",
		},
		{
			name: "url exact missing scheme",
			attrs: resource.Attributes{
				matomo.AttrName:           "Checkout",
				matomo.AttrMatchAttribute: "url",
				matomo.AttrPattern:        "example.com/thanks",
				matomo.AttrPatternType:    "exact",
			},
			want: "http://",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := p.Validate(context.Background(), resource.Resource{Address: addr, Attributes: tc.attrs})
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err, tc.want)
			}
			if !strings.Contains(err.Error(), addr.String()) {
				t.Fatalf("error = %q, want address", err)
			}
			assertNoProviderSecret(t, err.Error())
		})
	}
}

func TestValidateGoalRequiresSiteID(t *testing.T) {
	t.Parallel()

	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:   "https://matomo.example.com",
		TokenAuth: providerToken,
	}, http.DefaultClient)
	err := p.Validate(context.Background(), goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "manually",
	}))
	if err == nil {
		t.Fatal("expected site id error")
	}
	if !strings.Contains(err.Error(), matomo.EnvSiteID) {
		t.Fatalf("error = %q, want %s", err, matomo.EnvSiteID)
	}
}

func TestReadGoalSuccess(t *testing.T) {
	t.Parallel()

	srv := newGoalServer(t)
	srv.seed(apiGoal{
		ID:             1,
		Name:           "Trial Started",
		MatchAttribute: "event_action",
		Pattern:        "trialStarted",
		PatternType:    "exact",
		CaseSensitive:  "0",
		Revenue:        "0",
		IDSite:         "3",
	})
	p := testGoalProvider(t, srv)

	live, err := p.Read(context.Background(), goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "trialStarted",
		matomo.AttrPatternType:    "exact",
	}))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "1" {
		t.Fatalf("identity = %q, want 1", live.Identity.ID)
	}
	if live.Attributes[matomo.AttrName] != "Trial Started" {
		t.Fatalf("name = %v", live.Attributes[matomo.AttrName])
	}
	if live.Computed["idgoal"] != "1" {
		t.Fatalf("computed idgoal = %v", live.Computed["idgoal"])
	}
	if live.Computed["revenue"] != "0" {
		t.Fatalf("computed revenue = %v, should not be a comparable attribute", live.Computed["revenue"])
	}
	if _, ok := live.Attributes["revenue"]; ok {
		t.Fatal("revenue must not appear in comparable attributes")
	}
}

func TestReadGoalNotFound(t *testing.T) {
	t.Parallel()

	p := testGoalProvider(t, newGoalServer(t))
	_, err := p.Read(context.Background(), goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "trialStarted",
	}))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadGoalDuplicateName(t *testing.T) {
	t.Parallel()

	srv := newGoalServer(t)
	srv.seed(apiGoal{ID: 1, Name: "Trial Started", MatchAttribute: "event_action", Pattern: "a", PatternType: "contains"})
	srv.seed(apiGoal{ID: 4, Name: "Trial Started", MatchAttribute: "event_name", Pattern: "b", PatternType: "contains"})
	p := testGoalProvider(t, srv)

	_, err := p.Read(context.Background(), goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "a",
	}))
	if err == nil {
		t.Fatal("expected duplicate name error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("duplicate names must not look like not found")
	}
	if !strings.Contains(err.Error(), "multiple remote goals") {
		t.Fatalf("error = %q", err)
	}
}

func TestCreateGoal(t *testing.T) {
	t.Parallel()

	srv := newGoalServer(t)
	p := testGoalProvider(t, srv)
	res := goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "trialStarted",
	})

	live, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID == "" {
		t.Fatal("create returned empty identity")
	}
	if live.Attributes[matomo.AttrName] != "Trial Started" {
		t.Fatalf("name = %v", live.Attributes[matomo.AttrName])
	}

	got, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read after create: %v", err)
	}
	if got.Identity.ID != live.Identity.ID {
		t.Fatalf("read id = %q, want %q", got.Identity.ID, live.Identity.ID)
	}
}

func TestUpdateGoal(t *testing.T) {
	t.Parallel()

	srv := newGoalServer(t)
	srv.seed(apiGoal{
		ID:             8,
		Name:           "Trial Started",
		MatchAttribute: "event_action",
		Pattern:        "oldPattern",
		PatternType:    "contains",
	})
	p := testGoalProvider(t, srv)

	desired := goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "trialStarted",
		matomo.AttrPatternType:    "exact",
	})
	live, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "8"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if live.Identity.ID != "8" {
		t.Fatalf("identity = %q, want 8", live.Identity.ID)
	}
	if live.Attributes[matomo.AttrPattern] != "trialStarted" {
		t.Fatalf("pattern = %v", live.Attributes[matomo.AttrPattern])
	}
	if live.Attributes[matomo.AttrPatternType] != "exact" {
		t.Fatalf("patternType = %v", live.Attributes[matomo.AttrPatternType])
	}
}

func TestPlanGoalCreateWhenMissing(t *testing.T) {
	t.Parallel()

	p := testGoalProvider(t, newGoalServer(t))
	res := goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "trialStarted",
	})

	got := mustPlanGoal(t, p, res)
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionCreate {
		t.Fatalf("change = %+v, want create", got.Changes)
	}
}

func TestPlanGoalUnchangedEquivalentRemote(t *testing.T) {
	t.Parallel()

	srv := newGoalServer(t)
	srv.seed(apiGoal{
		ID:             2,
		Name:           "Trial Started",
		MatchAttribute: "event_action",
		Pattern:        "trialStarted",
		PatternType:    "contains",
		CaseSensitive:  "0",
		Revenue:        "0",
		Description:    "",
		IDSite:         "3",
	})
	p := testGoalProvider(t, srv)

	// Desired omits patternType (defaults to contains) and does not
	// declare computed Matomo fields such as revenue or idgoal.
	res := goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "trialStarted",
	})
	got := mustPlanGoal(t, p, res)
	if got.HasChanges() {
		t.Fatalf("equivalent remote produced changes: %+v", got.Changes)
	}
	if got.Changes[0].Identity.ID != "2" {
		t.Fatalf("identity = %q, want 2", got.Changes[0].Identity.ID)
	}
}

func TestPlanGoalUpdateConfigurableField(t *testing.T) {
	t.Parallel()

	srv := newGoalServer(t)
	srv.seed(apiGoal{
		ID:             2,
		Name:           "Trial Started",
		MatchAttribute: "event_action",
		Pattern:        "old",
		PatternType:    "contains",
	})
	p := testGoalProvider(t, srv)

	res := goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "trialStarted",
	})
	got := mustPlanGoal(t, p, res)
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want update", got.Changes)
	}
	var pattern *plan.AttributeDiff
	for i := range got.Changes[0].Diffs {
		if got.Changes[0].Diffs[i].Path == matomo.AttrPattern {
			pattern = &got.Changes[0].Diffs[i]
		}
	}
	if pattern == nil || pattern.Before != "old" || pattern.After != "trialStarted" {
		t.Fatalf("pattern diff = %+v", got.Changes[0].Diffs)
	}
}

func TestPlanGoalIgnoresComputedFields(t *testing.T) {
	t.Parallel()

	srv := newGoalServer(t)
	srv.seed(apiGoal{
		ID:                  5,
		Name:                "Manual",
		MatchAttribute:      "manually",
		Pattern:             "manually",
		PatternType:         "contains",
		CaseSensitive:       "0",
		Revenue:             "25",
		AllowMultiple:       "1",
		EventValueAsRevenue: "0",
		IDSite:              "3",
	})
	p := testGoalProvider(t, srv)

	res := goalResource(t, "manual", resource.Attributes{
		matomo.AttrName:           "Manual",
		matomo.AttrMatchAttribute: "manually",
	})
	got := mustPlanGoal(t, p, res)
	if got.HasChanges() {
		t.Fatalf("computed/manual defaults produced changes: %+v", got.Changes)
	}
}

func TestReadGoalAPIError(t *testing.T) {
	t.Parallel()

	srv := newGoalServer(t)
	srv.fail("Goals.getGoals", `{"result":"error","message":"Unable to authenticate with `+providerToken+`"}`)
	p := testGoalProvider(t, srv)

	_, err := p.Read(context.Background(), goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "trialStarted",
	}))
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	if !strings.Contains(err.Error(), "authenticate") && !strings.Contains(err.Error(), "Goals.getGoals") {
		t.Fatalf("error = %q, want API diagnostic", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadGoalMalformedResponse(t *testing.T) {
	t.Parallel()

	srv := newGoalServer(t)
	srv.malformed("Goals.getGoals", `"oops `+providerToken+`"`)
	p := testGoalProvider(t, srv)

	_, err := p.Read(context.Background(), goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "trialStarted",
	}))
	if err == nil {
		t.Fatal("expected malformed error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("malformed response must not be ErrNotFound")
	}
	if !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("error = %q, want malformed", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestCreateGoalValidatesBeforeMutation(t *testing.T) {
	t.Parallel()

	srv := newGoalServer(t)
	p := testGoalProvider(t, srv)
	_, err := p.Create(context.Background(), goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
	}))
	if err == nil {
		t.Fatal("expected validation error")
	}
	if srv.creates != 0 {
		t.Fatalf("creates = %d, want 0", srv.creates)
	}
}

func TestImportGoal(t *testing.T) {
	t.Parallel()

	srv := newGoalServer(t)
	srv.seed(apiGoal{ID: 1, Name: "Trial Started", MatchAttribute: "event_action", Pattern: "trialStarted", PatternType: "contains"})
	p := testGoalProvider(t, srv)
	addr := mustGoalAddress(t, "trial_started")
	live, err := p.Import(context.Background(), addr, "1")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if live.Identity.ID != "1" {
		t.Fatalf("identity = %q, want 1", live.Identity.ID)
	}
	if live.Attributes[matomo.AttrName] != "Trial Started" {
		t.Fatalf("name = %v", live.Attributes[matomo.AttrName])
	}
}

func TestValidateNumericPatternFromYAMLNumber(t *testing.T) {
	t.Parallel()

	p := testGoalProvider(t, newGoalServer(t))
	err := p.Validate(context.Background(), goalResource(t, "long_visit", resource.Attributes{
		matomo.AttrName:           "Long visit",
		matomo.AttrMatchAttribute: "visit_duration",
		matomo.AttrPattern:        30,
	}))
	if err != nil {
		t.Fatalf("numeric pattern should coerce: %v", err)
	}
}

type apiGoal struct {
	ID                  int
	IDSite              string
	Name                string
	MatchAttribute      string
	Pattern             string
	PatternType         string
	CaseSensitive       string
	AllowMultiple       string
	Revenue             string
	Description         string
	Deleted             string
	EventValueAsRevenue string
}

type goalServer struct {
	mu      sync.Mutex
	nextID  int
	goals   map[int]apiGoal
	fails   map[string]string
	creates int
	updates int
	server  *httptest.Server
}

func newGoalServer(t *testing.T) *goalServer {
	t.Helper()
	s := &goalServer{
		nextID: 1,
		goals:  make(map[int]apiGoal),
		fails:  make(map[string]string),
	}
	s.server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.server.Close)
	return s
}

func (s *goalServer) seed(g apiGoal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if g.ID == 0 {
		g.ID = s.nextID
		s.nextID++
	}
	if g.ID >= s.nextID {
		s.nextID = g.ID + 1
	}
	if g.IDSite == "" {
		g.IDSite = "3"
	}
	s.goals[g.ID] = g
}

func (s *goalServer) fail(method, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fails[method] = body
}

func (s *goalServer) malformed(method, body string) {
	s.fail(method, body)
}

func (s *goalServer) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	method := vals.Get("method")

	s.mu.Lock()
	failBody, fail := s.fails[method]
	s.mu.Unlock()
	if fail {
		_, _ = io.WriteString(w, failBody)
		return
	}

	switch method {
	case "API.getMatomoVersion":
		_, _ = io.WriteString(w, `"5.2.0"`)
	case "Goals.getGoals":
		s.writeGoals(w)
	case "Goals.addGoal":
		s.addGoal(w, vals)
	case "Goals.updateGoal":
		s.updateGoal(w, vals)
	default:
		_, _ = io.WriteString(w, `{"result":"error","message":"unknown method"}`)
	}
}

func (s *goalServer) writeGoals(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.goals) == 0 {
		_, _ = io.WriteString(w, `[]`)
		return
	}
	out := make(map[string]any, len(s.goals))
	for id, g := range s.goals {
		item := map[string]any{
			"idgoal":                 strconv.Itoa(id),
			"idsite":                 g.IDSite,
			"name":                   g.Name,
			"description":            g.Description,
			"match_attribute":        g.MatchAttribute,
			"pattern":                g.Pattern,
			"pattern_type":           g.PatternType,
			"case_sensitive":         g.CaseSensitive,
			"allow_multiple":         g.AllowMultiple,
			"revenue":                g.Revenue,
			"deleted":                g.Deleted,
			"event_value_as_revenue": g.EventValueAsRevenue,
		}
		if g.MatchAttribute == "manually" {
			// Match Matomo formatGoal(): omit unused fields.
			delete(item, "pattern")
			delete(item, "pattern_type")
			delete(item, "case_sensitive")
		}
		out[strconv.Itoa(id)] = item
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *goalServer) addGoal(w http.ResponseWriter, vals url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creates++
	id := s.nextID
	s.nextID++
	s.goals[id] = apiGoal{
		ID:             id,
		IDSite:         "3",
		Name:           vals.Get("name"),
		MatchAttribute: vals.Get("matchAttribute"),
		Pattern:        vals.Get("pattern"),
		PatternType:    vals.Get("patternType"),
	}
	_, _ = io.WriteString(w, strconv.Itoa(id))
}

func (s *goalServer) updateGoal(w http.ResponseWriter, vals url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates++
	id, err := strconv.Atoi(vals.Get("idGoal"))
	if err != nil {
		_, _ = io.WriteString(w, `{"result":"error","message":"invalid idGoal"}`)
		return
	}
	g, ok := s.goals[id]
	if !ok {
		_, _ = io.WriteString(w, `{"result":"error","message":"goal not found"}`)
		return
	}
	g.Name = vals.Get("name")
	g.MatchAttribute = vals.Get("matchAttribute")
	g.Pattern = vals.Get("pattern")
	g.PatternType = vals.Get("patternType")
	s.goals[id] = g
	_, _ = io.WriteString(w, `null`)
}

func testGoalProvider(t *testing.T, srv *goalServer) *matomo.Provider {
	t.Helper()
	return matomo.NewWithHTTPClient(client.Config{
		BaseURL:    srv.server.URL,
		TokenAuth:  providerToken,
		SiteID:     "3",
		HTTPClient: srv.server.Client(),
	}, srv.server.Client())
}

func goalResource(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{
		Address:    mustGoalAddress(t, name),
		Attributes: attrs,
	}
}

func mustGoalAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("matomo.goal." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func mustPlanGoal(t *testing.T, p *matomo.Provider, res resource.Resource) *plan.Plan {
	t.Helper()
	got, err := plan.Build(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	return got
}

func assertNoProviderSecret(t *testing.T, s string) {
	t.Helper()
	if strings.Contains(s, providerToken) {
		t.Fatalf("secret leaked in %q", s)
	}
}

func TestGoalResourceTypesRegistered(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{BaseURL: "https://matomo.example.com", TokenAuth: providerToken, SiteID: "1"})
	if !provider.Supports(p, matomo.TypeGoal) {
		t.Fatal("matomo.goal must be registered")
	}
}
