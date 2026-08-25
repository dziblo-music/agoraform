package matomo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestPublicationIsDeclarativeProviderFinalization(t *testing.T) {
	t.Parallel()

	s := newFinalizeServer(t)
	p := newFinalizeProvider(t, s)
	if err := p.Configure(resource.Attributes{"publish": true, "environment": "live"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	planned, err := p.PlanFinalization(context.Background(), []provider.PendingChange{{
		Address: resource.Address{Provider: matomo.Name, Type: matomo.TypeTag, Name: "trial_started"},
		Action:  "update",
	}})
	if err != nil {
		t.Fatalf("PlanFinalization: %v", err)
	}
	if planned == nil || planned.Action != "publish" || planned.Target != "live" {
		t.Fatalf("planned = %+v, want publish -> live", planned)
	}
	if !planned.Conditional {
		t.Fatalf("planned = %+v, want conditional publication", planned)
	}

	result, err := p.Finalize(context.Background(), *planned)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if !result.Changed {
		t.Fatalf("result = %+v, want changed", result)
	}
	if s.createCount() != 1 || s.publishCount() != 1 {
		t.Fatalf("mutations: create=%d publish=%d, want 1/1", s.createCount(), s.publishCount())
	}

	replanned, err := p.PlanFinalization(context.Background(), nil)
	if err != nil {
		t.Fatalf("second PlanFinalization: %v", err)
	}
	if replanned != nil {
		t.Fatalf("second plan = %+v, want no finalization", replanned)
	}
}

func TestConditionalPublicationSkipsVersionWhenMutationConvergesDraftToLive(t *testing.T) {
	t.Parallel()

	s := newFinalizeServer(t)
	s.liveVersion = "8"
	p := newFinalizeProvider(t, s)
	if err := p.Configure(resource.Attributes{"publish": true, "environment": "live"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	planned, err := p.PlanFinalization(context.Background(), []provider.PendingChange{{
		Address: resource.Address{Provider: matomo.Name, Type: matomo.TypeTag, Name: "trial_started"},
		Action:  "update",
	}})
	if err != nil {
		t.Fatalf("PlanFinalization: %v", err)
	}
	if planned == nil || !planned.Conditional {
		t.Fatalf("planned = %+v, want conditional publication", planned)
	}

	result, err := p.Finalize(context.Background(), *planned)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if result.Changed || len(result.Details) != 1 || result.Details[0] != "no publication required" {
		t.Fatalf("result = %+v, want no publication required", result)
	}
	if s.createCount() != 0 || s.publishCount() != 0 {
		t.Fatalf("mutations: create=%d publish=%d, want 0/0", s.createCount(), s.publishCount())
	}
}

func TestPublicationWithoutPendingChangesIsDefiniteWhenDraftDiffers(t *testing.T) {
	t.Parallel()

	s := newFinalizeServer(t)
	s.liveVersion = "8"
	s.liveVariableName = "previousName"
	p := newFinalizeProvider(t, s)
	if err := p.Configure(resource.Attributes{"publish": true, "environment": "live"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	planned, err := p.PlanFinalization(context.Background(), nil)
	if err != nil {
		t.Fatalf("PlanFinalization: %v", err)
	}
	if planned == nil || planned.Conditional {
		t.Fatalf("planned = %+v, want definite publication", planned)
	}
}

func TestPublicationWithoutPendingChangesIsOmittedWhenDraftMatchesLive(t *testing.T) {
	t.Parallel()

	s := newFinalizeServer(t)
	s.liveVersion = "8"
	p := newFinalizeProvider(t, s)
	if err := p.Configure(resource.Attributes{"publish": true, "environment": "live"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	planned, err := p.PlanFinalization(context.Background(), nil)
	if err != nil {
		t.Fatalf("PlanFinalization: %v", err)
	}
	if planned != nil {
		t.Fatalf("planned = %+v, want no publication", planned)
	}
}

func TestPublicationDisabledDoesNotPlan(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{})
	if err := p.Configure(resource.Attributes{"publish": false}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	planned, err := p.PlanFinalization(context.Background(), []provider.PendingChange{{
		Address: resource.Address{Provider: matomo.Name, Type: matomo.TypeTag, Name: "trial_started"},
		Action:  "update",
	}})
	if err != nil {
		t.Fatalf("PlanFinalization: %v", err)
	}
	if planned != nil {
		t.Fatalf("planned = %+v, want nil", planned)
	}
}

func TestPublicationCapabilityFailurePreventsMutation(t *testing.T) {
	t.Parallel()

	s := newFinalizeServer(t)
	s.publishable = []string{"dev"}
	p := newFinalizeProvider(t, s)
	if err := p.Configure(resource.Attributes{"publish": true, "environment": "live"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	_, err := p.PlanFinalization(context.Background(), []provider.PendingChange{{
		Address: resource.Address{Provider: matomo.Name, Type: matomo.TypeTag, Name: "trial_started"},
		Action:  "update",
	}})
	if err == nil || !strings.Contains(err.Error(), "cannot publish") {
		t.Fatalf("PlanFinalization = %v, want capability error", err)
	}
	if s.createCount() != 0 || s.publishCount() != 0 {
		t.Fatalf("capability failure mutated: create=%d publish=%d", s.createCount(), s.publishCount())
	}
}

func TestPublicationFailureReportsCreatedVersion(t *testing.T) {
	t.Parallel()

	s := newFinalizeServer(t)
	s.failPublish = true
	p := newFinalizeProvider(t, s)
	if err := p.Configure(resource.Attributes{"publish": true}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	planned := provider.FinalizationPlan{
		Address: resource.Address{Provider: matomo.Name, Type: "container", Name: "main"},
		Action:  "publish",
		Target:  "live",
	}
	result, err := p.Finalize(context.Background(), planned)
	if err == nil || !strings.Contains(err.Error(), "publish container version") {
		t.Fatalf("Finalize = %v, want publish failure", err)
	}
	if len(result.Details) == 0 || !strings.Contains(result.Details[0], "version 10 created") {
		t.Fatalf("result details = %v, want created version detail", result.Details)
	}
	if strings.Contains(err.Error(), "test-secret-token") {
		t.Fatalf("secret leaked in error %q", err.Error())
	}
	if s.createCount() != 1 || s.publishCount() != 1 {
		t.Fatalf("mutations: create=%d publish=%d, want 1/1", s.createCount(), s.publishCount())
	}
}

func TestApplyPublicationFailureReportsPartialConvergence(t *testing.T) {
	t.Parallel()

	s := newFinalizeServer(t)
	s.failPublish = true
	p := newFinalizeProvider(t, s)
	if err := p.Configure(resource.Attributes{"publish": true}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	_, err = apply.Run(context.Background(), nil, nil, st, &out, reg)
	if err == nil {
		t.Fatal("Run succeeded, want publication failure")
	}
	if !apply.IsPartial(err) {
		t.Fatalf("publication failure after version create was not partial: %v", err)
	}
	if !strings.Contains(err.Error(), "publish") {
		t.Fatalf("error = %q, want publish action", err)
	}
	if !strings.Contains(err.Error(), "were not rolled back") && !strings.Contains(err.Error(), "may already have changed") {
		t.Fatalf("error = %q, want partial convergence guidance", err)
	}
	if !strings.Contains(out.String(), "version 10 created") {
		t.Fatalf("created-version detail missing:\n%s", out.String())
	}
	if strings.Contains(out.String(), "Apply complete!") {
		t.Fatalf("failed publication claimed apply complete:\n%s", out.String())
	}
	if strings.Contains(err.Error(), "test-secret-token") || strings.Contains(out.String(), "test-secret-token") {
		t.Fatal("secret leaked in apply diagnostic")
	}
}

func TestApplyPublicationCreateFailureIsNotPartial(t *testing.T) {
	t.Parallel()

	s := newFinalizeServer(t)
	s.failCreate = true
	p := newFinalizeProvider(t, s)
	if err := p.Configure(resource.Attributes{"publish": true}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}

	_, err = apply.Run(context.Background(), nil, nil, st, &bytes.Buffer{}, reg)
	if err == nil {
		t.Fatal("Run succeeded, want version create failure")
	}
	if apply.IsPartial(err) {
		t.Fatalf("version create failure classified as partial: %v", err)
	}
	if !strings.Contains(err.Error(), "create container version") {
		t.Fatalf("error = %q, want create container version", err)
	}
}

func TestPublicationVersionCreationFailureDoesNotPublish(t *testing.T) {
	t.Parallel()

	s := newFinalizeServer(t)
	s.failCreate = true
	p := newFinalizeProvider(t, s)
	if err := p.Configure(resource.Attributes{"publish": true}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	planned := provider.FinalizationPlan{
		Address: resource.Address{Provider: matomo.Name, Type: "container", Name: "main"},
		Action:  "publish",
		Target:  "live",
	}
	_, err := p.Finalize(context.Background(), planned)
	if err == nil || !strings.Contains(err.Error(), "create container version") {
		t.Fatalf("Finalize = %v, want create failure", err)
	}
	if strings.Contains(err.Error(), "test-secret-token") {
		t.Fatalf("secret leaked in error %q", err.Error())
	}
	if s.createCount() != 1 || s.publishCount() != 0 {
		t.Fatalf("mutations: create=%d publish=%d, want 1/0", s.createCount(), s.publishCount())
	}
}

func TestPublicationMalformedCapabilityResponsePreventsMutation(t *testing.T) {
	t.Parallel()

	s := newFinalizeServer(t)
	s.malformedPublishable = true
	p := newFinalizeProvider(t, s)
	if err := p.Configure(resource.Attributes{"publish": true}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	_, err := p.PlanFinalization(context.Background(), []provider.PendingChange{{
		Address: resource.Address{Provider: matomo.Name, Type: matomo.TypeTag, Name: "trial_started"},
		Action:  "update",
	}})
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("PlanFinalization = %v, want malformed response", err)
	}
	if s.createCount() != 0 || s.publishCount() != 0 {
		t.Fatalf("malformed preflight mutated: create=%d publish=%d", s.createCount(), s.publishCount())
	}
}

func TestConfigurePublicationValidation(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{})
	for _, tc := range []struct {
		name string
		cfg  resource.Attributes
	}{
		{name: "publish type", cfg: resource.Attributes{"publish": "yes"}},
		{name: "environment type", cfg: resource.Attributes{"environment": true}},
		{name: "empty environment", cfg: resource.Attributes{"environment": "  "}},
		{name: "unknown", cfg: resource.Attributes{"autoPublish": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := p.Configure(tc.cfg); err == nil {
				t.Fatalf("Configure(%v) succeeded, want error", tc.cfg)
			}
		})
	}
}

type finalizeServer struct {
	mu                   sync.Mutex
	server               *httptest.Server
	publishable          []string
	liveVersion          string
	creates              int
	publishes            int
	failCreate           bool
	failPublish          bool
	malformedPublishable bool
	liveVariableName     string
}

func newFinalizeServer(t *testing.T) *finalizeServer {
	t.Helper()
	s := &finalizeServer{publishable: []string{"live", "dev"}}
	s.server = httptest.NewServer(s)
	t.Cleanup(s.server.Close)
	return s
}

func newFinalizeProvider(t *testing.T, s *finalizeServer) *matomo.Provider {
	t.Helper()
	return matomo.NewWithHTTPClient(client.Config{
		BaseURL:     s.server.URL,
		TokenAuth:   "test-secret-token",
		SiteID:      "3",
		ContainerID: "6OMh6taM",
		HTTPClient:  s.server.Client(),
	}, s.server.Client())
}

func (s *finalizeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	vals, _ := url.ParseQuery(string(body))
	switch vals.Get("method") {
	case "API.getMatomoVersion":
		_, _ = io.WriteString(w, `"5.2.0"`)
	case "TagManager.getAvailableEnvironmentsWithPublishCapability":
		s.mu.Lock()
		ids := append([]string(nil), s.publishable...)
		malformed := s.malformedPublishable
		s.mu.Unlock()
		if malformed {
			_, _ = io.WriteString(w, `"oops"`)
			return
		}
		writeEnvironments(w, ids)
	case "TagManager.getAvailableEnvironments":
		writeEnvironments(w, []string{"live", "dev"})
	case "TagManager.getContainer":
		s.mu.Lock()
		live := s.liveVersion
		s.mu.Unlock()
		out := map[string]any{
			"idcontainer": "6OMh6taM",
			"idsite":      3,
			"name":        "Website",
			"draft":       map[string]any{"idcontainerversion": 9},
			"releases":    []any{},
		}
		if live != "" {
			out["releases"] = []any{map[string]any{"idcontainerversion": live, "environment": "live"}}
		}
		_ = json.NewEncoder(w).Encode(out)
	case "TagManager.getContainerVariables":
		version := vals.Get("idContainerVersion")
		id := "2"
		name := "userId"
		if version != "9" {
			id = "102"
			s.mu.Lock()
			if s.liveVariableName != "" {
				name = s.liveVariableName
			}
			s.mu.Unlock()
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"idvariable":         id,
			"idcontainerversion": version,
			"type":               "DataLayer",
			"name":               name,
			"parameters":         map[string]any{"dataLayerName": "userId"},
		}})
	case "TagManager.getContainerTriggers", "TagManager.getContainerTags":
		_, _ = io.WriteString(w, `[]`)
	case "TagManager.createContainerVersion":
		s.mu.Lock()
		s.creates++
		fail := s.failCreate
		s.mu.Unlock()
		if fail {
			_, _ = io.WriteString(w, `{"result":"error","message":"cannot create test-secret-token"}`)
			return
		}
		_, _ = io.WriteString(w, `10`)
	case "TagManager.publishContainerVersion":
		s.mu.Lock()
		s.publishes++
		fail := s.failPublish
		if !fail {
			s.liveVersion = vals.Get("idContainerVersion")
		}
		s.mu.Unlock()
		if fail {
			_, _ = io.WriteString(w, `{"result":"error","message":"cannot publish test-secret-token"}`)
			return
		}
		_, _ = io.WriteString(w, `1`)
	default:
		_, _ = io.WriteString(w, `{"result":"error","message":"unexpected method"}`)
	}
}

func writeEnvironments(w http.ResponseWriter, ids []string) {
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, map[string]any{"id": id, "name": strings.ToUpper(id)})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *finalizeServer) createCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creates
}

func (s *finalizeServer) publishCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.publishes
}

func TestPublicationEnabledConnectionCheckRequiresSiteAndContainer(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  client.Config
		want string
	}{
		{
			name: "site",
			cfg: client.Config{
				BaseURL:     "https://matomo.example.com",
				TokenAuth:   "secret",
				ContainerID: "containerA",
			},
			want: matomo.EnvSiteID,
		},
		{
			name: "container",
			cfg: client.Config{
				BaseURL:   "https://matomo.example.com",
				TokenAuth: "secret",
				SiteID:    "3",
			},
			want: matomo.EnvContainerID,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := matomo.New(tc.cfg)
			if err := p.Configure(resource.Attributes{"publish": true}); err != nil {
				t.Fatalf("Configure: %v", err)
			}
			err := p.CheckConnection(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("CheckConnection = %v, want %s", err, tc.want)
			}
		})
	}
}
