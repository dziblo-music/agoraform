package cli_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestPublishHelp(t *testing.T) {
	t.Parallel()

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"publish", "--help"})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"publish", "agoraform.yaml", "never publishes", "apply"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing %q:\n%s", want, out)
		}
	}
}

func TestPublishConflictingFileArgs(t *testing.T) {
	t.Parallel()

	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"publish", "-f", "a.yaml", "b.yaml"})
	if code != cli.ExitUsage {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitUsage, stderr.String())
	}
}

func TestPublishFakeProviderNoop(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", validManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"publish", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No publication required.") {
		t.Fatalf("stdout = %q, want no publication required", stdout.String())
	}
}

func TestPublishApplyThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	p, srv := cliPublishProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoVariableManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"apply", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("apply exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if srv.called("TagManager.createContainerVersion") || srv.called("TagManager.publishContainerVersion") {
		t.Fatalf("apply published a container: %v", srv.methods())
	}

	st, err := state.Load(state.PathForManifest(path))
	if err != nil {
		t.Fatal(err)
	}
	id, ok, err := st.Identity(mustCLIAddress(t, "matomo.variable.user_id"))
	if err != nil || !ok || id.ID == "" {
		t.Fatalf("persisted identity = (%v,%v,%v)", id, ok, err)
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"publish", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("publish exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if strings.Contains(out, "cli-test-token") {
		t.Fatalf("publish leaked token:\n%s", out)
	}
	if !strings.Contains(out, "matomo.container.main: creating version...") || !strings.Contains(out, "matomo.container.main: published") {
		t.Fatalf("publish stdout missing progress:\n%s", out)
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after publish exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after apply+publish = %q, want no changes", stdout.String())
	}

	srv.resetCalls()
	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"publish", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("second publish exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "no publication required") {
		t.Fatalf("second publish stdout = %q", stdout.String())
	}
	if srv.called("TagManager.createContainerVersion") || srv.called("TagManager.publishContainerVersion") {
		t.Fatalf("repeated publish created a duplicate: %v", srv.methods())
	}
}

func TestPublishMissingContainerConfig(t *testing.T) {
	t.Parallel()

	p, _ := matomoGoalApplyProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoGoalManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"publish", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), matomo.EnvContainerID) {
		t.Fatalf("stderr = %q, want %s", stderr.String(), matomo.EnvContainerID)
	}
	if strings.Contains(stderr.String(), "cli-test-token") {
		t.Fatalf("stderr leaked token: %s", stderr.String())
	}
}

func TestPublishInvalidManifest(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "bad.yaml", invalidManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"publish", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
}

func TestPublishEmptyManifestNoProviders(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", emptyResourcesManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"publish", "-f", path}, provider.NewRegistry())
	if code != cli.ExitOK {
		t.Fatalf("exit = %d; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No publication required.") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestPublishVersionCreationFailure(t *testing.T) {
	t.Parallel()

	p, srv := cliPublishProvider(t)
	srv.failCreate = `{"result":"error","message":"cannot create version cli-test-token"}`
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoVariableManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"publish", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "create container version") {
		t.Fatalf("stderr = %q, want version creation error", stderr.String())
	}
	if strings.Contains(stderr.String(), "cli-test-token") {
		t.Fatalf("stderr leaked token: %s", stderr.String())
	}
}

func TestPublishPublicationFailure(t *testing.T) {
	t.Parallel()

	p, srv := cliPublishProvider(t)
	srv.failPublish = `{"result":"error","message":"cannot publish cli-test-token"}`
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoVariableManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"publish", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "publish container version") {
		t.Fatalf("stderr = %q, want publication error", stderr.String())
	}
	if strings.Contains(stderr.String(), "cli-test-token") {
		t.Fatalf("stderr leaked token: %s", stderr.String())
	}
}

func cliPublishProvider(t *testing.T) (*matomo.Provider, *cliPublishServer) {
	t.Helper()
	srv := newCLIPublishServer(t)
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)
	srv.http = httpSrv
	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:     httpSrv.URL,
		TokenAuth:   "cli-test-token",
		SiteID:      "3",
		ContainerID: "6OMh6taM",
		HTTPClient:  httpSrv.Client(),
	}, httpSrv.Client())
	return p, srv
}

type cliPublishVariable struct {
	ID   string
	Name string
	Key  string
}

type cliPublishServer struct {
	mu           sync.Mutex
	draftVersion string
	nextVersion  int
	nextVar      int
	liveVersion  string
	variables    map[string][]cliPublishVariable
	calls        []string
	failCreate   string
	failPublish  string
	http         *httptest.Server
}

func newCLIPublishServer(t *testing.T) *cliPublishServer {
	t.Helper()
	return &cliPublishServer{
		draftVersion: "9",
		nextVersion:  10,
		nextVar:      2,
		variables:    map[string][]cliPublishVariable{"9": {}},
	}
}

func (s *cliPublishServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	vals, _ := url.ParseQuery(string(body))
	method := vals.Get("method")
	s.mu.Lock()
	s.calls = append(s.calls, method)
	s.mu.Unlock()

	switch method {
	case "API.getMatomoVersion":
		_, _ = io.WriteString(w, `"5.2.0"`)
	case "Goals.getGoals":
		_, _ = io.WriteString(w, `[]`)
	case "TagManager.getAvailableEnvironments":
		_, _ = io.WriteString(w, `[{"id":"live","name":"Live"}]`)
	case "TagManager.getContainer":
		s.writeContainer(w)
	case "TagManager.getContainerVariables":
		s.writeVariables(w, vals.Get("idContainerVersion"))
	case "TagManager.getContainerTriggers", "TagManager.getContainerTags":
		_, _ = io.WriteString(w, `[]`)
	case "TagManager.addContainerVariable":
		s.addVariable(w, vals)
	case "TagManager.createContainerVersion":
		s.createVersion(w)
	case "TagManager.publishContainerVersion":
		s.publishVersion(w, vals)
	default:
		_, _ = io.WriteString(w, `{"result":"error","message":"unknown method"}`)
	}
}

func (s *cliPublishServer) writeContainer(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]any{
		"idcontainer": "6OMh6taM",
		"idsite":      3,
		"draft":       map[string]any{"idcontainerversion": s.draftVersion},
		"releases":    []any{},
	}
	if s.liveVersion != "" {
		out["releases"] = []any{
			map[string]any{"idcontainerversion": s.liveVersion, "environment": "live"},
		}
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *cliPublishServer) writeVariables(w http.ResponseWriter, version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vars := s.variables[version]
	out := make([]map[string]any, 0, len(vars))
	for _, v := range vars {
		out = append(out, map[string]any{
			"idvariable":         v.ID,
			"idcontainerversion": version,
			"idsite":             "3",
			"type":               "DataLayer",
			"name":               v.Name,
			"status":             "active",
			"parameters":         map[string]any{"dataLayerName": v.Key},
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *cliPublishServer) addVariable(w http.ResponseWriter, vals url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := strconv.Itoa(s.nextVar)
	s.nextVar++
	s.variables[s.draftVersion] = append(s.variables[s.draftVersion], cliPublishVariable{
		ID:   id,
		Name: vals.Get("name"),
		Key:  vals.Get("parameters[dataLayerName]"),
	})
	_, _ = io.WriteString(w, id)
}

func (s *cliPublishServer) createVersion(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failCreate != "" {
		_, _ = io.WriteString(w, s.failCreate)
		return
	}
	id := strconv.Itoa(s.nextVersion)
	s.nextVersion++
	copied := make([]cliPublishVariable, 0, len(s.variables[s.draftVersion]))
	for i, v := range s.variables[s.draftVersion] {
		v.ID = strconv.Itoa(100 + i)
		copied = append(copied, v)
	}
	s.variables[id] = copied
	_, _ = io.WriteString(w, id)
}

func (s *cliPublishServer) publishVersion(w http.ResponseWriter, vals url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failPublish != "" {
		_, _ = io.WriteString(w, s.failPublish)
		return
	}
	s.liveVersion = vals.Get("idContainerVersion")
	_, _ = io.WriteString(w, `1`)
}

func (s *cliPublishServer) called(method string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.calls {
		if m == method {
			return true
		}
	}
	return false
}

func (s *cliPublishServer) methods() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.calls))
	copy(out, s.calls)
	return out
}

func (s *cliPublishServer) resetCalls() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = nil
}
