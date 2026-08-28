package matomo_test

import (
	"bytes"
	"context"
	"encoding/json"
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

	"github.com/dziblo-music/agoraform/internal/destroy"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestDestroyGoal(t *testing.T) {
	t.Parallel()

	srv := newGoalServer(t)
	srv.seed(apiGoal{ID: 12, Name: "Trial Started", MatchAttribute: "event_action", Pattern: "trialStarted", PatternType: "contains"})
	p := testGoalProvider(t, srv)
	res := goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "trialStarted",
	})
	res.Identity = resource.Identity{ID: "12"}

	result, err := p.Destroy(context.Background(), res)
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if result.Status != provider.DestroyStatusDestroyed {
		t.Fatalf("status = %q", result.Status)
	}
	again, err := p.Destroy(context.Background(), res)
	if err != nil {
		t.Fatalf("Destroy already absent: %v", err)
	}
	if again.Status != provider.DestroyStatusAlreadyAbsent {
		t.Fatalf("status = %q, want already-absent", again.Status)
	}
}

func TestDestroyGoalDeletedFlagIsAlreadyAbsent(t *testing.T) {
	t.Parallel()

	srv := newGoalServer(t)
	srv.seed(apiGoal{ID: 12, Name: "Trial Started", MatchAttribute: "event_action", Pattern: "trialStarted", PatternType: "contains", Deleted: "1"})
	p := testGoalProvider(t, srv)
	res := goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "trialStarted",
	})
	res.Identity = resource.Identity{ID: "12"}

	result, err := p.Destroy(context.Background(), res)
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if result.Status != provider.DestroyStatusAlreadyAbsent {
		t.Fatalf("status = %q, want already-absent", result.Status)
	}
	if srv.goalDeleteCount() != 0 {
		t.Fatalf("deleteCount = %d, want 0 for already-deleted goal", srv.goalDeleteCount())
	}
}

func TestDestroyGoalAuthErrorIsNotAlreadyAbsent(t *testing.T) {
	t.Parallel()

	srv := newGoalServer(t)
	srv.seed(apiGoal{ID: 12, Name: "Trial Started", MatchAttribute: "event_action", Pattern: "trialStarted", PatternType: "contains"})
	srv.fail("Goals.getGoals", `{"result":"error","message":"Unable to authenticate with `+providerToken+`"}`)
	p := testGoalProvider(t, srv)
	res := goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "event_action",
		matomo.AttrPattern:        "trialStarted",
	})
	res.Identity = resource.Identity{ID: "12"}

	_, err := p.Destroy(context.Background(), res)
	if err == nil {
		t.Fatal("Destroy succeeded, want auth error")
	}
	if strings.Contains(err.Error(), providerToken) {
		t.Fatalf("secret leaked: %v", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "already") {
		t.Fatalf("auth error treated as already-absent: %v", err)
	}
}

func TestDestroyVariableAndConfiguration(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{ID: 2, Name: "userId", Type: "DataLayer", Key: "userId"})
	srv.seed(apiVariable{ID: 3, Name: "Matomo Configuration", Type: "MatomoConfiguration"})
	p := testVariableProvider(t, srv)

	dataLayer := variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "DataLayer",
		matomo.AttrName: "userId",
		matomo.AttrKey:  "userId",
	})
	dataLayer.Identity = resource.Identity{ID: "2"}
	if result, err := p.Destroy(context.Background(), dataLayer); err != nil || result.Status != provider.DestroyStatusDestroyed {
		t.Fatalf("data layer Destroy = (%v, %v)", result, err)
	}

	config := variableResource(t, "config", resource.Attributes{
		matomo.AttrType:      "matomoConfiguration",
		matomo.AttrName:      "Matomo Configuration",
		matomo.AttrMatomoURL: "https://matomo.example.com",
		matomo.AttrSiteID:    1,
	})
	config.Identity = resource.Identity{ID: "3"}
	if result, err := p.Destroy(context.Background(), config); err != nil || result.Status != provider.DestroyStatusDestroyed {
		t.Fatalf("config Destroy = (%v, %v)", result, err)
	}
}

func TestDestroyTriggerAndTag(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 5, Name: "trialStarted", Type: "CustomEvent", Event: "trialStarted"})
	srv.seedTag(apiTag{ID: 7, Name: "trialStarted", Type: "Matomo", FireTriggerID: 5, Category: "signup", Action: "trialStarted"})
	p := testTagProvider(t, srv)

	tag := tagResource(t, "trial_started", validTagAttrs(t))
	tag.Identity = resource.Identity{ID: "7"}
	if result, err := p.Destroy(context.Background(), tag); err != nil || result.Status != provider.DestroyStatusDestroyed {
		t.Fatalf("tag Destroy = (%v, %v)", result, err)
	}

	tr := trialStartedTrigger(t)
	tr.Identity = resource.Identity{ID: "5"}
	if result, err := p.Destroy(context.Background(), tr); err != nil || result.Status != provider.DestroyStatusDestroyed {
		t.Fatalf("trigger Destroy = (%v, %v)", result, err)
	}
}

func TestDestroyManagedContainer(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	srv.seed(apiContainer{ID: testManagedContainerID, Name: "Main Website", Context: "web"})
	p := testContainerProvider(t, srv)
	res := containerResource(t, "main", defaultContainerAttrs())
	if err := p.ValidateResourceSet(context.Background(), []resource.Resource{res}); err != nil {
		t.Fatalf("ValidateResourceSet: %v", err)
	}
	res.Identity = resource.Identity{ID: testManagedContainerID}

	result, err := p.Destroy(context.Background(), res)
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if result.Status != provider.DestroyStatusDestroyed {
		t.Fatalf("status = %q", result.Status)
	}
	again, err := p.Destroy(context.Background(), res)
	if err != nil {
		t.Fatalf("Destroy already absent: %v", err)
	}
	if again.Status != provider.DestroyStatusAlreadyAbsent {
		t.Fatalf("status = %q, want already-absent", again.Status)
	}
}

func TestDestroyRefusesExternallySelectedContainer(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	srv.seed(apiContainer{ID: "Bb000002", Name: "External", Context: "web"})
	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:     srv.server.URL,
		TokenAuth:   providerToken,
		SiteID:      "3",
		ContainerID: "Bb000002",
		HTTPClient:  srv.server.Client(),
	}, srv.server.Client())
	res := containerResource(t, "main", defaultContainerAttrs())
	res.Identity = resource.Identity{ID: "Bb000002"}

	_, err := p.Destroy(context.Background(), res)
	if err == nil || !strings.Contains(err.Error(), "externally managed") {
		t.Fatalf("Destroy = %v, want external refusal", err)
	}
	if srv.deleteCount() != 0 {
		t.Fatalf("deleteCount = %d, want 0", srv.deleteCount())
	}
}

func TestDestroyRunReverseOrderAndManagedContainerSkipsPublish(t *testing.T) {
	t.Parallel()

	s := newLifecycleServer(t)
	s.seedManagedGraph()
	p := newLifecycleProvider(t, s, "")
	if err := p.Configure(resource.Attributes{"publish": true, "environment": "live"}); err != nil {
		t.Fatal(err)
	}
	desired := managedGraph(t)
	if err := p.ValidateResourceSet(context.Background(), desired); err != nil {
		t.Fatalf("ValidateResourceSet: %v", err)
	}

	st := mustDestroyStore(t)
	bindGraph(t, st, map[string]string{
		"matomo.container.main":        testManagedContainerID,
		"matomo.variable.user_id":      "2",
		"matomo.trigger.trial_started": "5",
		"matomo.tag.trial_started":     "7",
	})

	var out bytes.Buffer
	result, err := destroy.Run(context.Background(), desired, lookupMatomo(p), st, &out, nil)
	if err != nil {
		t.Fatalf("Run: %v stderr-like out=\n%s", err, out.String())
	}
	if result.Destroyed != 4 || result.Finalized != 0 {
		t.Fatalf("result = %+v, want 4 destroyed and no publication", result)
	}
	if got := s.calledMethods(); !hasPrefix(got, []string{
		"TagManager.deleteContainerTag",
		"TagManager.deleteContainerTrigger",
		"TagManager.deleteContainerVariable",
		"TagManager.deleteContainer",
	}) && !hasOrder(got, "TagManager.deleteContainerTag", "TagManager.deleteContainer") {
		t.Fatalf("methods = %v, want tag before container", got)
	}
	if s.createCount() != 0 || s.publishCount() != 0 {
		t.Fatalf("publication mutations create=%d publish=%d, want 0/0", s.createCount(), s.publishCount())
	}
	if strings.Contains(out.String(), "publish") {
		t.Fatalf("output unexpectedly published:\n%s", out.String())
	}
}

func TestDestroyRunExternalContainerIsProtected(t *testing.T) {
	t.Parallel()

	s := newLifecycleServer(t)
	s.seedExternalGraph()
	p := newLifecycleProvider(t, s, testManagedContainerID)
	desired := externalGraph(t)
	st := mustDestroyStore(t)
	bindGraph(t, st, map[string]string{
		"matomo.variable.user_id":      "2",
		"matomo.trigger.trial_started": "5",
		"matomo.tag.trial_started":     "7",
	})

	result, err := destroy.Run(context.Background(), desired, lookupMatomo(p), st, nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Destroyed != 3 {
		t.Fatalf("result = %+v, want 3 child destructions", result)
	}
	if containsMethod(s.calledMethods(), "TagManager.deleteContainer") {
		t.Fatalf("externally managed container was deleted: %v", s.calledMethods())
	}
}

func TestDestroyRunPublishFalseLeavesDraft(t *testing.T) {
	t.Parallel()

	s := newLifecycleServer(t)
	s.seedExternalGraph()
	p := newLifecycleProvider(t, s, testManagedContainerID)
	if err := p.Configure(resource.Attributes{"publish": false}); err != nil {
		t.Fatal(err)
	}
	desired := externalGraph(t)
	st := mustDestroyStore(t)
	bindGraph(t, st, map[string]string{
		"matomo.variable.user_id":      "2",
		"matomo.trigger.trial_started": "5",
		"matomo.tag.trial_started":     "7",
	})

	var out bytes.Buffer
	result, err := destroy.Run(context.Background(), desired, lookupMatomo(p), st, &out, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Destroyed != 3 || result.Finalized != 0 {
		t.Fatalf("result = %+v", result)
	}
	if s.createCount() != 0 {
		t.Fatalf("created version with publish:false")
	}
	if strings.Contains(out.String(), "publish") {
		t.Fatalf("unexpected publication:\n%s", out.String())
	}
}

func TestDestroyRunPublishTruePublishesOnce(t *testing.T) {
	t.Parallel()

	s := newLifecycleServer(t)
	s.seedExternalGraph()
	p := newLifecycleProvider(t, s, testManagedContainerID)
	if err := p.Configure(resource.Attributes{"publish": true, "environment": "live"}); err != nil {
		t.Fatal(err)
	}
	desired := externalGraph(t)
	st := mustDestroyStore(t)
	bindGraph(t, st, map[string]string{
		"matomo.variable.user_id":      "2",
		"matomo.trigger.trial_started": "5",
		"matomo.tag.trial_started":     "7",
	})

	var out bytes.Buffer
	result, err := destroy.Run(context.Background(), desired, lookupMatomo(p), st, &out, nil)
	if err != nil {
		t.Fatalf("Run: %v\n%s", err, out.String())
	}
	if result.Destroyed != 3 || result.Finalized != 1 {
		t.Fatalf("result = %+v, want destroy + publish", result)
	}
	if s.createCount() != 1 || s.publishCount() != 1 {
		t.Fatalf("create=%d publish=%d, want 1/1", s.createCount(), s.publishCount())
	}
	if !strings.Contains(out.String(), "matomo.container.external: publish -> live") {
		t.Fatalf("plan missing container-scoped publication:\n%s", out.String())
	}

	st2 := mustDestroyStore(t)
	bindGraph(t, st2, map[string]string{
		"matomo.variable.user_id":      "2",
		"matomo.trigger.trial_started": "5",
		"matomo.tag.trial_started":     "7",
	})
	result, err = destroy.Run(context.Background(), desired, lookupMatomo(p), st2, nil, nil)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if result.AlreadyAbsent != 3 {
		t.Fatalf("retry result = %+v, want already-absent children", result)
	}
	if s.createCount() != 1 || s.publishCount() != 1 {
		t.Fatalf("duplicate publication create=%d publish=%d", s.createCount(), s.publishCount())
	}
}

func TestDestroyRunFailedMutationPreventsPublication(t *testing.T) {
	t.Parallel()

	s := newLifecycleServer(t)
	s.seedExternalGraph()
	s.fail("TagManager.deleteContainerTag", `{"result":"error","message":"permission denied"}`)
	p := newLifecycleProvider(t, s, testManagedContainerID)
	if err := p.Configure(resource.Attributes{"publish": true, "environment": "live"}); err != nil {
		t.Fatal(err)
	}
	desired := externalGraph(t)
	st := mustDestroyStore(t)
	bindGraph(t, st, map[string]string{
		"matomo.variable.user_id":      "2",
		"matomo.trigger.trial_started": "5",
		"matomo.tag.trial_started":     "7",
	})

	_, err := destroy.Run(context.Background(), desired, lookupMatomo(p), st, nil, nil)
	if err == nil {
		t.Fatal("Run succeeded, want tag delete failure")
	}
	if s.createCount() != 0 || s.publishCount() != 0 {
		t.Fatalf("published after failed mutation create=%d publish=%d", s.createCount(), s.publishCount())
	}
	if _, ok, _ := st.Identity(mustAddressString(t, "matomo.tag.trial_started")); !ok {
		t.Fatal("failed tag was removed from state")
	}
}

func TestDestroyMalformedResponseIsNotAlreadyAbsent(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{ID: 2, Name: "userId", Type: "DataLayer", Key: "userId"})
	srv.malformed("TagManager.getContainerVariables", `{`)
	p := testVariableProvider(t, srv)
	res := variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "DataLayer",
		matomo.AttrName: "userId",
		matomo.AttrKey:  "userId",
	})
	res.Identity = resource.Identity{ID: "2"}
	_, err := p.Destroy(context.Background(), res)
	if err == nil {
		t.Fatal("Destroy succeeded, want malformed error")
	}
	if strings.Contains(strings.ToLower(err.Error()), "already") {
		t.Fatalf("malformed treated as already-absent: %v", err)
	}
}

func TestPlanFinalizationSkipsWhenManagedContainerDestroyed(t *testing.T) {
	t.Parallel()

	s := newFinalizeServer(t)
	p := newFinalizeProvider(t, s)
	if err := p.Configure(resource.Attributes{"publish": true, "environment": "live"}); err != nil {
		t.Fatal(err)
	}
	planned, err := p.PlanFinalization(context.Background(), []provider.PendingChange{
		{Address: resource.Address{Provider: matomo.Name, Type: matomo.TypeTag, Name: "trial_started"}, Action: "destroy"},
		{Address: resource.Address{Provider: matomo.Name, Type: matomo.TypeContainer, Name: "main"}, Action: "destroy"},
	})
	if err != nil {
		t.Fatalf("PlanFinalization: %v", err)
	}
	if planned != nil {
		t.Fatalf("planned = %+v, want no publication when the container is being deleted", planned)
	}
}

type lifecycleServer struct {
	mu        sync.Mutex
	server    *httptest.Server
	container apiContainer
	variables map[int]apiVariable
	triggers  map[int]apiTagTrigger
	tags      map[int]apiTag
	liveVars  map[int]apiVariable
	liveTrigs map[int]apiTagTrigger
	liveTags  map[int]apiTag
	liveVer   string
	draftVer  string
	fails     map[string]string
	methods   []string
	creates   int
	publishes int
}

func newLifecycleServer(t *testing.T) *lifecycleServer {
	t.Helper()
	s := &lifecycleServer{
		variables: make(map[int]apiVariable),
		triggers:  make(map[int]apiTagTrigger),
		tags:      make(map[int]apiTag),
		fails:     make(map[string]string),
		draftVer:  "9",
		liveVer:   "8",
	}
	s.server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.server.Close)
	return s
}

func (s *lifecycleServer) seedManagedGraph() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.container = apiContainer{ID: testManagedContainerID, Name: "Main Website", Context: "web", Status: "active", Version: "9"}
	s.seedGraphLocked()
}

func (s *lifecycleServer) seedExternalGraph() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.container = apiContainer{ID: testManagedContainerID, Name: "Main Website", Context: "web", Status: "active", Version: "9"}
	s.seedGraphLocked()
}

func (s *lifecycleServer) seedGraphLocked() {
	s.variables[2] = apiVariable{ID: 2, Name: "userId", Type: "DataLayer", Key: "userId", Status: "active", Version: "9"}
	s.triggers[5] = apiTagTrigger{ID: 5, Name: "trialStarted", Type: "CustomEvent", Event: "trialStarted", Status: "active"}
	s.tags[7] = apiTag{ID: 7, Name: "trialStarted", Type: "Matomo", FireTriggerID: 5, Category: "signup", Action: "trialStarted", Status: "active"}
	s.liveVars = map[int]apiVariable{2: s.variables[2]}
	s.liveTrigs = map[int]apiTagTrigger{5: s.triggers[5]}
	s.liveTags = map[int]apiTag{7: s.tags[7]}
}

func (s *lifecycleServer) fail(method, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fails[method] = body
}

func (s *lifecycleServer) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	vals, _ := url.ParseQuery(string(body))
	method := vals.Get("method")
	s.mu.Lock()
	s.methods = append(s.methods, method)
	failBody, fail := s.fails[method]
	s.mu.Unlock()
	if fail {
		_, _ = io.WriteString(w, failBody)
		return
	}
	switch method {
	case "API.getMatomoVersion":
		_, _ = io.WriteString(w, `"5.2.0"`)
	case "TagManager.getAvailableEnvironmentsWithPublishCapability":
		_, _ = io.WriteString(w, `[{"id":"live","name":"Live"}]`)
	case "TagManager.getAvailableEnvironments":
		_, _ = io.WriteString(w, `[{"id":"live","name":"Live"}]`)
	case "TagManager.getContainer":
		s.writeContainer(w)
	case "TagManager.getContainerVariables":
		s.writeVars(w, vals.Get("idContainerVersion"))
	case "TagManager.getContainerTriggers":
		s.writeTriggers(w, vals.Get("idContainerVersion"))
	case "TagManager.getContainerTags":
		s.writeTags(w, vals.Get("idContainerVersion"))
	case "TagManager.deleteContainerVariable":
		s.deleteVar(w, vals.Get("idVariable"))
	case "TagManager.deleteContainerTrigger":
		s.deleteTrig(w, vals.Get("idTrigger"))
	case "TagManager.deleteContainerTag":
		s.deleteTag(w, vals.Get("idTag"))
	case "TagManager.deleteContainer":
		s.deleteContainer(w, vals.Get("idContainer"))
	case "TagManager.createContainerVersion":
		s.createVersion(w)
	case "TagManager.publishContainerVersion":
		s.publishVersion(w, vals.Get("idContainerVersion"))
	default:
		_, _ = io.WriteString(w, `{"result":"error","message":"unknown method"}`)
	}
}

func (s *lifecycleServer) writeContainer(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.EqualFold(s.container.Status, "deleted") {
		_, _ = io.WriteString(w, `false`)
		return
	}
	out := map[string]any{
		"idcontainer": s.container.ID,
		"idsite":      3,
		"name":        s.container.Name,
		"context":     s.container.Context,
		"status":      "active",
		"draft":       map[string]any{"idcontainerversion": s.draftVer},
		"releases":    []any{map[string]any{"idcontainerversion": s.liveVer, "environment": "live"}},
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *lifecycleServer) writeVars(w http.ResponseWriter, version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	src := s.variables
	if version != "" && version != s.draftVer {
		src = s.liveVars
	}
	out := make([]map[string]any, 0, len(src))
	for id, v := range src {
		out = append(out, map[string]any{
			"idvariable":         strconv.Itoa(id),
			"idcontainerversion": version,
			"type":               v.Type,
			"name":               v.Name,
			"status":             "active",
			"parameters":         map[string]any{"dataLayerName": v.Key},
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *lifecycleServer) writeTriggers(w http.ResponseWriter, version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	src := s.triggers
	if version != "" && version != s.draftVer {
		src = s.liveTrigs
	}
	out := make([]map[string]any, 0, len(src))
	for id, tr := range src {
		out = append(out, map[string]any{
			"idtrigger":          strconv.Itoa(id),
			"idcontainerversion": version,
			"type":               tr.Type,
			"name":               tr.Name,
			"status":             "active",
			"parameters":         map[string]any{"eventName": tr.Event},
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *lifecycleServer) writeTags(w http.ResponseWriter, version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	src := s.tags
	if version != "" && version != s.draftVer {
		src = s.liveTags
	}
	out := make([]map[string]any, 0, len(src))
	for id, tag := range src {
		out = append(out, map[string]any{
			"idtag":              strconv.Itoa(id),
			"idcontainerversion": version,
			"type":               tag.Type,
			"name":               tag.Name,
			"status":             "active",
			"fireTriggerIds":     []int{tag.FireTriggerID},
			"parameters":         map[string]any{"eventCategory": tag.Category, "eventAction": tag.Action},
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *lifecycleServer) deleteVar(w http.ResponseWriter, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, _ := strconv.Atoi(id)
	delete(s.variables, n)
	_, _ = io.WriteString(w, `true`)
}

func (s *lifecycleServer) deleteTrig(w http.ResponseWriter, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, _ := strconv.Atoi(id)
	delete(s.triggers, n)
	_, _ = io.WriteString(w, `true`)
}

func (s *lifecycleServer) deleteTag(w http.ResponseWriter, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, _ := strconv.Atoi(id)
	delete(s.tags, n)
	_, _ = io.WriteString(w, `true`)
}

func (s *lifecycleServer) deleteContainer(w http.ResponseWriter, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.container.ID != id {
		_, _ = io.WriteString(w, `{"result":"error","message":"The requested container does not exist"}`)
		return
	}
	s.container.Status = "deleted"
	_, _ = io.WriteString(w, `true`)
}

func (s *lifecycleServer) createVersion(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creates++
	_, _ = io.WriteString(w, `10`)
}

func (s *lifecycleServer) publishVersion(w http.ResponseWriter, version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publishes++
	s.liveVer = version
	s.liveVars = cloneVars(s.variables)
	s.liveTrigs = cloneTrigs(s.triggers)
	s.liveTags = cloneTags(s.tags)
	_, _ = io.WriteString(w, `1`)
}

func (s *lifecycleServer) calledMethods() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.methods))
	copy(out, s.methods)
	return out
}

func (s *lifecycleServer) createCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creates
}

func (s *lifecycleServer) publishCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.publishes
}

func cloneVars(in map[int]apiVariable) map[int]apiVariable {
	out := make(map[int]apiVariable, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneTrigs(in map[int]apiTagTrigger) map[int]apiTagTrigger {
	out := make(map[int]apiTagTrigger, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneTags(in map[int]apiTag) map[int]apiTag {
	out := make(map[int]apiTag, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func newLifecycleProvider(t *testing.T, s *lifecycleServer, containerID string) *matomo.Provider {
	t.Helper()
	return matomo.NewWithHTTPClient(client.Config{
		BaseURL:     s.server.URL,
		TokenAuth:   providerToken,
		SiteID:      "3",
		ContainerID: containerID,
		HTTPClient:  s.server.Client(),
	}, s.server.Client())
}

func managedGraph(t *testing.T) []resource.Resource {
	t.Helper()
	container := containerResource(t, "main", defaultContainerAttrs())
	variable := variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType:      "dataLayer",
		matomo.AttrName:      "userId",
		matomo.AttrKey:       "userId",
		matomo.AttrContainer: resource.Ref{Address: container.Address},
	})
	trigger := triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:      "customEvent",
		matomo.AttrEvent:     "trialStarted",
		matomo.AttrContainer: resource.Ref{Address: container.Address},
	})
	tag := tagResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:          "matomoAnalytics",
		matomo.AttrTrigger:       resource.Ref{Address: trigger.Address},
		matomo.AttrEventCategory: "signup",
		matomo.AttrEventAction:   "trialStarted",
		matomo.AttrContainer:     resource.Ref{Address: container.Address},
	})
	return []resource.Resource{container, variable, trigger, tag}
}

func externalGraph(t *testing.T) []resource.Resource {
	t.Helper()
	variable := variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "DataLayer",
		matomo.AttrName: "userId",
		matomo.AttrKey:  "userId",
	})
	trigger := triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
	})
	tag := tagResource(t, "trial_started", validTagAttrs(t))
	return []resource.Resource{variable, trigger, tag}
}

func bindGraph(t *testing.T, st *state.Store, ids map[string]string) {
	t.Helper()
	for addr, id := range ids {
		parsed, err := resource.ParseAddress(addr)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.Bind(parsed, resource.Identity{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
}

func mustDestroyStore(t *testing.T) *state.Store {
	t.Helper()
	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func lookupMatomo(p *matomo.Provider) destroy.Lookup {
	return func(resource.Address) (provider.Provider, error) {
		return p, nil
	}
}

func mustAddressString(t *testing.T, s string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress(s)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func containsMethod(methods []string, want string) bool {
	for _, m := range methods {
		if m == want {
			return true
		}
	}
	return false
}

func hasOrder(methods []string, first, second string) bool {
	fi, si := -1, -1
	for i, m := range methods {
		if m == first && fi < 0 {
			fi = i
		}
		if m == second && si < 0 {
			si = i
		}
	}
	return fi >= 0 && si >= 0 && fi < si
}

func hasPrefix(got, want []string) bool {
	if len(got) < len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func (s *containerServer) deleteCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deletes
}

func (s *goalServer) goalDeleteCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deletes
}

func TestDestroyManagedChildAlreadyAbsentWhenContainerGone(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	p := testVariableProvider(t, srv)
	res := variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "DataLayer",
		matomo.AttrName: "userId",
		matomo.AttrKey:  "userId",
		matomo.AttrContainer: resource.Resolved{
			Address:  mustAddressString(t, "matomo.container.main"),
			Identity: resource.Identity{ID: "Aa000001"},
		},
	})
	res.Identity = resource.Identity{ID: "2"}
	srv.fail("TagManager.getContainer", `{"result":"error","message":"The requested container does not exist"}`)

	result, err := p.Destroy(context.Background(), res)
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if result.Status != provider.DestroyStatusAlreadyAbsent {
		t.Fatalf("status = %q, want already-absent when the managed container is gone", result.Status)
	}
}

func TestDestroyWrongContainerIsNotAlreadyAbsent(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	p := testVariableProvider(t, srv)
	res := variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "DataLayer",
		matomo.AttrName: "userId",
		matomo.AttrKey:  "userId",
	})
	res.Identity = resource.Identity{ID: "2"}
	srv.fail("TagManager.getContainer", `{"result":"error","message":"The requested container does not exist"}`)
	_, err := p.Destroy(context.Background(), res)
	if err == nil {
		t.Fatal("Destroy succeeded, want wrong-container error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("wrong-container masked as not found: %v", err)
	}
}
