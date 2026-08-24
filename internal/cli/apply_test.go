package cli_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestApplyCreateAndIdempotentPlan(t *testing.T) {
	t.Parallel()

	p := fake.New()
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", validManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"apply", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("apply exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "fake.widget.homepage: creating...") || !strings.Contains(out, "fake.widget.homepage: created") {
		t.Fatalf("apply stdout missing create progress:\n%s", out)
	}
	if !strings.Contains(out, "Apply complete! 1 created, 0 updated.") {
		t.Fatalf("apply stdout missing summary:\n%s", out)
	}

	st, err := state.Load(state.PathForManifest(path))
	if err != nil {
		t.Fatal(err)
	}
	id, ok, err := st.Identity(mustCLIAddress(t, "fake.widget.homepage"))
	if err != nil || !ok || id.ID == "" {
		t.Fatalf("persisted identity = (%v,%v,%v)", id, ok, err)
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after apply exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after apply = %q, want no changes", stdout.String())
	}
}

func TestApplyUpdate(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustCLIAddress(t, "fake.widget.homepage")
	if err := p.Seed(resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage banner"},
		Computed:   resource.Attributes{fake.AttrSerial: 4},
	}); err != nil {
		t.Fatal(err)
	}

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	const updated = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: Homepage banner
      color: blue
`
	path := writeManifest(t, "agoraform.yaml", updated)
	st, err := state.New(state.PathForManifest(path))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(addr, resource.Identity{ID: "widget-1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"apply", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("apply exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "Apply complete! 0 created, 1 updated.") {
		t.Fatalf("stdout = %q, want update summary", stdout.String())
	}

	loaded, err := state.Load(state.PathForManifest(path))
	if err != nil {
		t.Fatal(err)
	}
	id, ok, err := loaded.Identity(addr)
	if err != nil || !ok || id.ID != "widget-1" {
		t.Fatalf("identity after update = (%v,%v,%v), want widget-1", id, ok, err)
	}
}

func TestApplyZeroChange(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustCLIAddress(t, "fake.widget.homepage")
	if err := p.Seed(resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage banner"},
		Computed:   resource.Attributes{fake.AttrSerial: 4},
	}); err != nil {
		t.Fatal(err)
	}
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", validManifest)
	st, err := state.New(state.PathForManifest(path))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(addr, resource.Identity{ID: "widget-1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"apply", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("apply exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "Apply complete! 0 created, 0 updated.") {
		t.Fatalf("stdout = %q, want zero-change summary", stdout.String())
	}

	_, creates, updates, imports := p.Calls()
	if creates != 0 || updates != 0 || imports != 0 {
		t.Fatalf("zero-change apply mutated provider: creates=%d updates=%d imports=%d", creates, updates, imports)
	}
}

func TestApplyValidationFailureDoesNotMutate(t *testing.T) {
	t.Parallel()

	p := fake.New()
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	const missingTitle = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      color: blue
`
	path := writeManifest(t, "bad.yaml", missingTitle)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"apply", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "title") {
		t.Fatalf("stderr = %q, want title validation error", stderr.String())
	}

	_, creates, updates, imports := p.Calls()
	if creates != 0 || updates != 0 || imports != 0 {
		t.Fatalf("validation failure mutated provider: creates=%d updates=%d imports=%d", creates, updates, imports)
	}
}

func TestApplyInvalidManifest(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "bad.yaml", invalidManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"apply", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
}

func TestApplyMalformedStateFile(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", validManifest)
	if err := os.WriteFile(state.PathForManifest(path), []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}

	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"apply", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "malformed") && !strings.Contains(stderr.String(), "state") {
		t.Fatalf("stderr = %q, want malformed state diagnostic", stderr.String())
	}
}

func TestApplyConflictingFileArgs(t *testing.T) {
	t.Parallel()

	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"apply", "-f", "a.yaml", "b.yaml"})
	if code != cli.ExitUsage {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitUsage, stderr.String())
	}
}

func TestApplyNoRegisteredProviders(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", validManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"apply", "-f", path}, provider.NewRegistry())
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "registered provider") {
		t.Fatalf("stderr = %q, want registered provider error", stderr.String())
	}
}

func TestApplyEmptyManifestNoProviders(t *testing.T) {
	t.Parallel()

	const emptyManifest = "apiVersion: agoraform.io/v1alpha1\nresources: []\n"
	path := writeManifest(t, "agoraform.yaml", emptyManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"apply", "-f", path})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Apply complete! 0 created, 0 updated.") {
		t.Fatalf("stdout = %q, want zero-change apply", stdout.String())
	}
}

func TestApplyHelpDocumentsBehavior(t *testing.T) {
	t.Parallel()

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"apply", "--help"})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"apply", "agoraform.yaml", "agoraform.state.json", "deletes remote resources"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing %q:\n%s", want, out)
		}
	}
}

func TestApplyMatomoGoalCreateThenPlan(t *testing.T) {
	t.Parallel()

	p, _ := matomoGoalApplyProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoGoalManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"apply", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("apply exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "matomo.goal.trial_started: created") {
		t.Fatalf("stdout missing create:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "cli-test-token") {
		t.Fatalf("apply output leaked token:\n%s", stdout.String())
	}

	st, err := state.Load(state.PathForManifest(path))
	if err != nil {
		t.Fatal(err)
	}
	id, ok, err := st.Identity(mustCLIAddress(t, "matomo.goal.trial_started"))
	if err != nil || !ok || id.ID == "" {
		t.Fatalf("persisted matomo identity = (%v,%v,%v)", id, ok, err)
	}
	raw, err := os.ReadFile(state.PathForManifest(path))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "cli-test-token") {
		t.Fatal("state file leaked token")
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after apply exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after matomo apply = %q, want no changes", stdout.String())
	}
}

func matomoGoalApplyProvider(t *testing.T) (*matomo.Provider, *httptest.Server) {
	t.Helper()
	srv := newCLIGoalServer(t)
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)
	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:    httpSrv.URL,
		TokenAuth:  "cli-test-token",
		SiteID:     "3",
		HTTPClient: httpSrv.Client(),
	}, httpSrv.Client())
	return p, httpSrv
}

type cliGoalServer struct {
	mu     sync.Mutex
	nextID int
	goals  map[string]map[string]any
}

func newCLIGoalServer(t *testing.T) *cliGoalServer {
	t.Helper()
	return &cliGoalServer{
		nextID: 1,
		goals:  make(map[string]map[string]any),
	}
}

func (s *cliGoalServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	vals, _ := url.ParseQuery(string(body))
	switch vals.Get("method") {
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

func (s *cliGoalServer) writeGoals(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.goals) == 0 {
		_, _ = io.WriteString(w, `[]`)
		return
	}
	_ = json.NewEncoder(w).Encode(s.goals)
}

func (s *cliGoalServer) addGoal(w http.ResponseWriter, vals url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := strconv.Itoa(s.nextID)
	s.nextID++
	patternType := vals.Get("patternType")
	if patternType == "" {
		patternType = "contains"
	}
	s.goals[id] = map[string]any{
		"idgoal":          id,
		"idsite":          "3",
		"name":            vals.Get("name"),
		"match_attribute": vals.Get("matchAttribute"),
		"pattern":         vals.Get("pattern"),
		"pattern_type":    patternType,
		"revenue":         "0",
	}
	_, _ = io.WriteString(w, id)
}

func (s *cliGoalServer) updateGoal(w http.ResponseWriter, vals url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := vals.Get("idGoal")
	g, ok := s.goals[id]
	if !ok {
		_, _ = io.WriteString(w, `{"result":"error","message":"goal not found"}`)
		return
	}
	g["name"] = vals.Get("name")
	g["match_attribute"] = vals.Get("matchAttribute")
	g["pattern"] = vals.Get("pattern")
	g["pattern_type"] = vals.Get("patternType")
	s.goals[id] = g
	_, _ = io.WriteString(w, `null`)
}
