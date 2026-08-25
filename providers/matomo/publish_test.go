package matomo_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestPublishContainerCreatesAndPublishesVersion(t *testing.T) {
	t.Parallel()

	srv := newPublishServer(t)
	p := publishProvider(t, srv, "")

	result, err := p.PublishContainer(context.Background())
	if err != nil {
		t.Fatalf("PublishContainer: %v", err)
	}
	if !result.Created || result.Address != matomo.ContainerAddress {
		t.Fatalf("result = %+v, want created %s", result, matomo.ContainerAddress)
	}
	if !srv.called("TagManager.createContainerVersion") || !srv.called("TagManager.publishContainerVersion") {
		t.Fatalf("methods = %v, want create and publish", srv.methods())
	}
	if srv.lastPublishEnvironment() != client.DefaultEnvironment {
		t.Fatalf("environment = %q", srv.lastPublishEnvironment())
	}
}

func TestPublishContainerNoopWhenAlreadyPublished(t *testing.T) {
	t.Parallel()

	srv := newPublishServer(t)
	p := publishProvider(t, srv, "")
	if _, err := p.PublishContainer(context.Background()); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	srv.resetMutations()

	result, err := p.PublishContainer(context.Background())
	if err != nil {
		t.Fatalf("second publish: %v", err)
	}
	if result.Created {
		t.Fatal("expected no-op publish")
	}
	if srv.called("TagManager.createContainerVersion") || srv.called("TagManager.publishContainerVersion") {
		t.Fatalf("no-op created a duplicate version: %v", srv.methods())
	}
}

func TestPublishContainerMissingConfiguration(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  client.Config
		want string
	}{
		{name: "url", cfg: client.Config{TokenAuth: "tok", SiteID: "3", ContainerID: "6OMh6taM"}, want: matomo.EnvURL},
		{name: "token", cfg: client.Config{BaseURL: "https://matomo.example.com", SiteID: "3", ContainerID: "6OMh6taM"}, want: matomo.EnvTokenAuth},
		{name: "site", cfg: client.Config{BaseURL: "https://matomo.example.com", TokenAuth: "tok", ContainerID: "6OMh6taM"}, want: matomo.EnvSiteID},
		{name: "container", cfg: client.Config{BaseURL: "https://matomo.example.com", TokenAuth: "tok", SiteID: "3"}, want: matomo.EnvContainerID},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := matomo.New(tc.cfg)
			_, err := p.PublishContainer(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("PublishContainer = %v, want %s", err, tc.want)
			}
			assertNoProviderSecret(t, err.Error())
			if strings.Contains(err.Error(), "tok") {
				t.Fatalf("secret leaked in %q", err.Error())
			}
		})
	}
}

func TestPublishContainerInvalidEnvironment(t *testing.T) {
	t.Parallel()

	srv := newPublishServer(t)
	p := publishProvider(t, srv, "nope")
	_, err := p.PublishContainer(context.Background())
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("PublishContainer = %v, want invalid environment", err)
	}
	if srv.called("TagManager.createContainerVersion") || srv.called("TagManager.publishContainerVersion") {
		t.Fatalf("invalid environment attempted publication: %v", srv.methods())
	}
}

func TestPublishContainerVersionCreationFailure(t *testing.T) {
	t.Parallel()

	srv := newPublishServer(t)
	srv.failCreate = `{"result":"error","message":"cannot create version ` + providerToken + `"}`
	p := publishProvider(t, srv, "")
	_, err := p.PublishContainer(context.Background())
	if err == nil || !strings.Contains(err.Error(), "create container version") {
		t.Fatalf("PublishContainer = %v, want version creation error", err)
	}
	assertNoProviderSecret(t, err.Error())
	if srv.called("TagManager.publishContainerVersion") {
		t.Fatal("publish called after version creation failed")
	}
}

func TestPublishContainerPublicationFailure(t *testing.T) {
	t.Parallel()

	srv := newPublishServer(t)
	srv.failPublish = `{"result":"error","message":"cannot publish ` + providerToken + `"}`
	p := publishProvider(t, srv, "")
	_, err := p.PublishContainer(context.Background())
	if err == nil || !strings.Contains(err.Error(), "publish container version") {
		t.Fatalf("PublishContainer = %v, want publication error", err)
	}
	assertNoProviderSecret(t, err.Error())
	if !srv.called("TagManager.createContainerVersion") {
		t.Fatal("expected version creation before publication failure")
	}
}

func TestPublishContainerMalformedEnvironments(t *testing.T) {
	t.Parallel()

	srv := newPublishServer(t)
	srv.failEnvironments = `"oops ` + providerToken + `"`
	p := publishProvider(t, srv, "")
	_, err := p.PublishContainer(context.Background())
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("PublishContainer = %v, want malformed environments", err)
	}
	assertNoProviderSecret(t, err.Error())
	if srv.called("TagManager.createContainerVersion") {
		t.Fatal("malformed environments attempted publication")
	}
}

func TestPublishContainerUsesConfiguredEnvironment(t *testing.T) {
	t.Parallel()

	srv := newPublishServer(t)
	p := publishProvider(t, srv, "dev")
	if _, err := p.PublishContainer(context.Background()); err != nil {
		t.Fatalf("PublishContainer: %v", err)
	}
	if srv.lastPublishEnvironment() != "dev" {
		t.Fatalf("environment = %q, want dev", srv.lastPublishEnvironment())
	}
}

func publishProvider(t *testing.T, srv *publishServer, environment string) *matomo.Provider {
	t.Helper()
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)
	srv.server = httpSrv
	return matomo.NewWithHTTPClient(client.Config{
		BaseURL:     httpSrv.URL,
		TokenAuth:   providerToken,
		SiteID:      "3",
		ContainerID: "6OMh6taM",
		Environment: environment,
		HTTPClient:  httpSrv.Client(),
	}, httpSrv.Client())
}

type publishEntity struct {
	ID   string
	Name string
	Type string
	Key  string
}

type publishServer struct {
	mu               sync.Mutex
	draftVersion     string
	nextVersion      int
	liveVersion      string
	entities         map[string][]publishEntity // version -> variables
	calls            []string
	createCount      int
	publishCount     int
	publishEnv       string
	failCreate       string
	failPublish      string
	failEnvironments string
	server           *httptest.Server
}

func newPublishServer(t *testing.T) *publishServer {
	t.Helper()
	s := &publishServer{
		draftVersion: "9",
		nextVersion:  10,
		entities:     map[string][]publishEntity{},
	}
	s.entities["9"] = []publishEntity{{ID: "2", Name: "userId", Type: "DataLayer", Key: "userId"}}
	return s
}

func (s *publishServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	vals, _ := url.ParseQuery(string(body))
	method := vals.Get("method")
	s.mu.Lock()
	s.calls = append(s.calls, method)
	s.mu.Unlock()

	switch method {
	case "API.getMatomoVersion":
		_, _ = io.WriteString(w, `"5.2.0"`)
	case "TagManager.getAvailableEnvironments":
		if s.failEnvironments != "" {
			_, _ = io.WriteString(w, s.failEnvironments)
			return
		}
		_, _ = io.WriteString(w, `[{"id":"live","name":"Live"},{"id":"dev","name":"Dev"}]`)
	case "TagManager.getContainer":
		s.writeContainer(w)
	case "TagManager.getContainerVariables":
		s.writeVariables(w, vals.Get("idContainerVersion"))
	case "TagManager.getContainerTriggers", "TagManager.getContainerTags":
		_, _ = io.WriteString(w, `[]`)
	case "TagManager.createContainerVersion":
		s.createVersion(w)
	case "TagManager.publishContainerVersion":
		s.publishVersion(w, vals)
	default:
		_, _ = io.WriteString(w, `{"result":"error","message":"unknown method"}`)
	}
}

func (s *publishServer) writeContainer(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]any{
		"idcontainer": "6OMh6taM",
		"idsite":      3,
		"name":        "Website",
		"draft":       map[string]any{"idcontainerversion": s.draftVersion},
		"releases":    []any{},
	}
	if s.liveVersion != "" {
		out["releases"] = []any{
			map[string]any{"idcontainerversion": s.liveVersion, "environment": "live"},
			map[string]any{"idcontainerversion": s.liveVersion, "environment": "dev"},
		}
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *publishServer) writeVariables(w http.ResponseWriter, version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entities := s.entities[version]
	out := make([]map[string]any, 0, len(entities))
	for _, e := range entities {
		out = append(out, map[string]any{
			"idvariable":         e.ID,
			"idcontainerversion": version,
			"type":               e.Type,
			"name":               e.Name,
			"status":             "active",
			"parameters":         map[string]any{"dataLayerName": e.Key},
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *publishServer) createVersion(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.createCount++
	if s.failCreate != "" {
		_, _ = io.WriteString(w, s.failCreate)
		return
	}
	id := strconv.Itoa(s.nextVersion)
	s.nextVersion++
	copied := make([]publishEntity, 0, len(s.entities[s.draftVersion]))
	for i, e := range s.entities[s.draftVersion] {
		e.ID = strconv.Itoa(100 + i)
		copied = append(copied, e)
	}
	s.entities[id] = copied
	_, _ = io.WriteString(w, id)
}

func (s *publishServer) publishVersion(w http.ResponseWriter, vals url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publishCount++
	s.publishEnv = vals.Get("environment")
	if s.failPublish != "" {
		_, _ = io.WriteString(w, s.failPublish)
		return
	}
	s.liveVersion = vals.Get("idContainerVersion")
	_, _ = io.WriteString(w, `1`)
}

func (s *publishServer) called(method string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.calls {
		if m == method {
			return true
		}
	}
	return false
}

func (s *publishServer) methods() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.calls))
	copy(out, s.calls)
	return out
}

func (s *publishServer) lastPublishEnvironment() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.publishEnv
}

func (s *publishServer) resetMutations() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = nil
	s.createCount = 0
	s.publishCount = 0
}
