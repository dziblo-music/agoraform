package cli_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestImportMatomoContainerPersistsIdentityThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	p, srv := matomoManagedContainerServerProvider(t)
	srv.seedContainer(cliTMContainer{ID: "6OMh6taM", Name: "Main Website", Context: "web", Description: "primary"})

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "matomo.container.main", "6OMh6taM"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if strings.Contains(out, "cli-test-token") || strings.Contains(out, "idcontainer") || strings.Contains(out, "idContainer") {
		t.Fatalf("container import leaked secret or identity:\n%s", out)
	}
	if !strings.Contains(out, "matomo.container.main") || !strings.Contains(out, "Main Website") {
		t.Fatalf("container import YAML missing expected fields:\n%s", out)
	}

	yamlText := extractYAML(out)
	if err := os.WriteFile(manifestPath, []byte(yamlText), 0o600); err != nil {
		t.Fatal(err)
	}
	assertPersistedRemoteID(t, manifestPath, "matomo.container.main", "6OMh6taM")

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after container import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after container import = %q", stdout.String())
	}
	if srv.mutationCount() != 0 {
		t.Fatalf("container import mutated remote: %d", srv.mutationCount())
	}
}

func TestImportMatomoVariablePersistsIdentityThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	p, srv := matomoTagManagerServerProvider(t)
	srv.seedVariable(cliTMVariable{ID: 2, Name: "userId", Type: "DataLayer", Key: "userId"})

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "matomo.variable.user_id", "2"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if strings.Contains(out, "cli-test-token") || strings.Contains(out, "idvariable") || strings.Contains(out, "idVariable") {
		t.Fatalf("variable import leaked secret or identity:\n%s", out)
	}

	yamlText := extractYAML(out)
	if err := os.WriteFile(manifestPath, []byte(yamlText), 0o600); err != nil {
		t.Fatal(err)
	}
	assertPersistedRemoteID(t, manifestPath, "matomo.variable.user_id", "2")

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after variable import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after variable import = %q", stdout.String())
	}
	if srv.mutationCount() != 0 {
		t.Fatalf("variable import mutated remote: %d", srv.mutationCount())
	}
}

func TestImportMatomoTriggerPersistsIdentityThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	p, srv := matomoTagManagerServerProvider(t)
	srv.seedTrigger(cliTMTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "matomo.trigger.trial_started", "4"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if strings.Contains(out, "cli-test-token") || strings.Contains(out, "idtrigger") || strings.Contains(out, "idTrigger") {
		t.Fatalf("trigger import leaked secret or identity:\n%s", out)
	}

	yamlText := extractYAML(out)
	if err := os.WriteFile(manifestPath, []byte(yamlText), 0o600); err != nil {
		t.Fatal(err)
	}
	assertPersistedRemoteID(t, manifestPath, "matomo.trigger.trial_started", "4")

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after trigger import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after trigger import = %q", stdout.String())
	}
	if srv.mutationCount() != 0 {
		t.Fatalf("trigger import mutated remote: %d", srv.mutationCount())
	}
}

func TestImportMatomoTagReconstructsTriggerRefThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	p, srv := matomoTagManagerServerProvider(t)
	srv.seedTrigger(cliTMTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(cliTMTag{ID: 1, Name: "trialStarted", FireTriggerID: 4, Category: "signup", Action: "trialStarted"})

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "matomo.trigger.trial_started", "4"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("trigger import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	triggerYAML := extractYAML(stdout.String())

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "matomo.tag.trial_started", "1"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("tag import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	tagOut := stdout.String()
	if strings.Contains(tagOut, "cli-test-token") || strings.Contains(tagOut, "idtag") || strings.Contains(tagOut, "idTag") {
		t.Fatalf("tag import leaked secret or identity:\n%s", tagOut)
	}
	if !strings.Contains(tagOut, "$ref: matomo.trigger.trial_started") {
		t.Fatalf("tag import missing reconstructed trigger $ref:\n%s", tagOut)
	}
	tagYAML := extractYAML(tagOut)

	combined := combineManifestResources(t, triggerYAML, tagYAML)
	if err := os.WriteFile(manifestPath, []byte(combined), 0o600); err != nil {
		t.Fatal(err)
	}
	assertPersistedRemoteID(t, manifestPath, "matomo.trigger.trial_started", "4")
	assertPersistedRemoteID(t, manifestPath, "matomo.tag.trial_started", "1")

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after tag import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after tag import = %q", stdout.String())
	}
	if srv.mutationCount() != 0 {
		t.Fatalf("tag import mutated remote: %d", srv.mutationCount())
	}
}

func TestImportMatomoTagRequiresBoundTrigger(t *testing.T) {
	t.Parallel()

	p, srv := matomoTagManagerServerProvider(t)
	srv.seedTrigger(cliTMTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(cliTMTag{ID: 1, Name: "trialStarted", FireTriggerID: 4, Category: "signup", Action: "trialStarted"})

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", emptyResourcesManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, "matomo.tag.trial_started", "1"}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), `fire trigger id "4"`) || !strings.Contains(stderr.String(), "not bound") {
		t.Fatalf("stderr = %q, want unbound trigger guidance", stderr.String())
	}
}

func TestImportMatomoTagInvalidID(t *testing.T) {
	t.Parallel()

	p, _ := matomoTagManagerServerProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", emptyResourcesManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, "matomo.tag.trial_started", "abc"}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "valid Matomo tag id") && !strings.Contains(stderr.String(), "identity") {
		t.Fatalf("stderr = %q, want invalid id diagnostic", stderr.String())
	}
}

func assertPersistedRemoteID(t *testing.T, manifestPath, address, wantID string) {
	t.Helper()
	st, err := state.Load(state.PathForManifest(manifestPath))
	if err != nil {
		t.Fatal(err)
	}
	id, ok, err := st.Identity(mustCLIAddress(t, address))
	if err != nil || !ok || id.ID != wantID {
		t.Fatalf("persisted %s = (%v,%v,%v), want %s", address, id, ok, err, wantID)
	}
}

func combineManifestResources(t *testing.T, fragments ...string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("apiVersion: ")
	b.WriteString(manifest.APIVersion)
	b.WriteString("\nresources:\n")
	for _, fragment := range fragments {
		items, err := resourceListYAML(fragment)
		if err != nil {
			t.Fatalf("extract resources: %v\n%s", err, fragment)
		}
		b.WriteString(items)
		if !strings.HasSuffix(items, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func resourceListYAML(fragment string) (string, error) {
	const marker = "resources:\n"
	i := strings.Index(fragment, marker)
	if i < 0 {
		return "", fmt.Errorf("missing resources list")
	}
	return fragment[i+len(marker):], nil
}

func matomoManagedContainerServerProvider(t *testing.T) (*matomo.Provider, *cliTagManagerServer) {
	t.Helper()
	srv := newCLITagManagerServer(t)
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)
	srv.server = httpSrv
	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:    httpSrv.URL,
		TokenAuth:  "cli-test-token",
		SiteID:     "3",
		HTTPClient: httpSrv.Client(),
	}, httpSrv.Client())
	return p, srv
}

func matomoTagManagerServerProvider(t *testing.T) (*matomo.Provider, *cliTagManagerServer) {
	t.Helper()
	srv := newCLITagManagerServer(t)
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)
	srv.server = httpSrv
	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:     httpSrv.URL,
		TokenAuth:   "cli-test-token",
		SiteID:      "3",
		ContainerID: "6OMh6taM",
		HTTPClient:  httpSrv.Client(),
	}, httpSrv.Client())
	return p, srv
}

type cliTMContainer struct {
	ID          string
	Name        string
	Context     string
	Description string
}

type cliTMVariable struct {
	ID   int
	Name string
	Type string
	Key  string
}

type cliTMTrigger struct {
	ID    int
	Name  string
	Event string
	Type  string
}

type cliTMTag struct {
	ID            int
	Name          string
	FireTriggerID int
	Category      string
	Action        string
	EventName     string
}

type cliTagManagerServer struct {
	mu         sync.Mutex
	containers map[string]cliTMContainer
	variables  map[int]cliTMVariable
	triggers   map[int]cliTMTrigger
	tags       map[int]cliTMTag
	creates    int
	updates    int
	server     *httptest.Server
}

func newCLITagManagerServer(t *testing.T) *cliTagManagerServer {
	t.Helper()
	return &cliTagManagerServer{
		containers: map[string]cliTMContainer{
			"6OMh6taM": {ID: "6OMh6taM", Name: "Website", Context: "web"},
		},
		variables: map[int]cliTMVariable{
			1: {ID: 1, Name: "Matomo Configuration", Type: "MatomoConfiguration"},
		},
		triggers: make(map[int]cliTMTrigger),
		tags:     make(map[int]cliTMTag),
	}
}

func (s *cliTagManagerServer) seedContainer(c cliTMContainer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.Context == "" {
		c.Context = "web"
	}
	s.containers[c.ID] = c
}

func (s *cliTagManagerServer) seedVariable(v cliTMVariable) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.variables[v.ID] = v
}

func (s *cliTagManagerServer) seedTrigger(tr cliTMTrigger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tr.Type == "" {
		tr.Type = "CustomEvent"
	}
	s.triggers[tr.ID] = tr
}

func (s *cliTagManagerServer) seedTag(tag cliTMTag) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tags[tag.ID] = tag
}

func (s *cliTagManagerServer) mutationCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creates + s.updates
}

func (s *cliTagManagerServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	vals, _ := url.ParseQuery(string(body))
	switch vals.Get("method") {
	case "API.getMatomoVersion":
		_, _ = io.WriteString(w, `"5.2.0"`)
	case "TagManager.getContainers":
		s.writeContainers(w)
	case "TagManager.getContainer":
		s.writeContainer(w, vals.Get("idContainer"))
	case "TagManager.getContainerVariables":
		s.writeVariables(w)
	case "TagManager.getContainerTriggers":
		s.writeTriggers(w)
	case "TagManager.getContainerTags":
		s.writeTags(w)
	case "TagManager.addContainerVariable", "TagManager.addContainerTrigger", "TagManager.addContainerTag", "TagManager.addContainer":
		s.mu.Lock()
		s.creates++
		s.mu.Unlock()
		_, _ = io.WriteString(w, `{"result":"error","message":"unexpected create"}`)
	case "TagManager.updateContainerVariable", "TagManager.updateContainerTrigger", "TagManager.updateContainerTag", "TagManager.updateContainer":
		s.mu.Lock()
		s.updates++
		s.mu.Unlock()
		_, _ = io.WriteString(w, `{"result":"error","message":"unexpected update"}`)
	case "Goals.getGoals":
		_, _ = io.WriteString(w, `[]`)
	default:
		_, _ = io.WriteString(w, `{"result":"error","message":"unknown method"}`)
	}
}

func (s *cliTagManagerServer) writeContainers(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]map[string]any, 0, len(s.containers))
	for _, c := range s.containers {
		out = append(out, s.containerJSON(c))
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *cliTagManagerServer) writeContainer(w http.ResponseWriter, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.containers[id]
	if !ok {
		_, _ = io.WriteString(w, `false`)
		return
	}
	_ = json.NewEncoder(w).Encode(s.containerJSON(c))
}

func (s *cliTagManagerServer) containerJSON(c cliTMContainer) map[string]any {
	return map[string]any{
		"idcontainer": c.ID,
		"idsite":      3,
		"name":        c.Name,
		"context":     c.Context,
		"description": c.Description,
		"status":      "active",
		"draft":       map[string]any{"idcontainerversion": 9},
	}
}

func (s *cliTagManagerServer) writeVariables(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]map[string]any, 0, len(s.variables))
	for id, v := range s.variables {
		item := map[string]any{
			"idvariable":         strconv.Itoa(id),
			"idcontainerversion": "9",
			"idsite":             "3",
			"type":               v.Type,
			"name":               v.Name,
			"status":             "active",
			"parameters":         map[string]any{},
		}
		if v.Key != "" {
			item["parameters"] = map[string]any{"dataLayerName": v.Key}
		}
		out = append(out, item)
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *cliTagManagerServer) writeTriggers(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]map[string]any, 0, len(s.triggers))
	for id, tr := range s.triggers {
		out = append(out, map[string]any{
			"idtrigger":          strconv.Itoa(id),
			"idcontainerversion": "9",
			"idsite":             "3",
			"type":               tr.Type,
			"name":               tr.Name,
			"status":             "active",
			"parameters":         map[string]any{"eventName": tr.Event},
			"conditions":         []any{},
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *cliTagManagerServer) writeTags(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]map[string]any, 0, len(s.tags))
	for id, tag := range s.tags {
		fire := []any{}
		if tag.FireTriggerID != 0 {
			fire = []any{tag.FireTriggerID}
		}
		params := map[string]any{
			"trackingType":  "event",
			"eventCategory": tag.Category,
			"eventAction":   tag.Action,
			"matomoConfig":  map[string]any{"name": "Matomo Configuration", "type": "MatomoConfiguration"},
		}
		if tag.EventName != "" {
			params["eventName"] = tag.EventName
		}
		out = append(out, map[string]any{
			"idtag":              strconv.Itoa(id),
			"idcontainerversion": "9",
			"idsite":             "3",
			"type":               "Matomo",
			"name":               tag.Name,
			"status":             "active",
			"fireTriggerIds":     fire,
			"blockTriggerIds":    []any{},
			"parameters":         params,
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}
