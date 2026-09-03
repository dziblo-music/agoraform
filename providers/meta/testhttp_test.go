package meta_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/meta"
	"github.com/dziblo-music/agoraform/providers/meta/client"
)

const (
	testToken     = "EAAB-secret-conversion-token"
	testAccountID = "123456789012345"
	testPixelID   = "111222333444555"
	testConvID    = "998877665544332"
)

type graphObject map[string]any

type graphServer struct {
	t *testing.T

	mu            sync.Mutex
	pixels        map[string]graphObject
	accountPixels map[string]bool
	convs         map[string]graphObject
	posts         int
	deletes       int
	requests      []string
}

func newGraphServer(t *testing.T) *graphServer {
	t.Helper()
	return &graphServer{
		t:             t,
		pixels:        map[string]graphObject{},
		accountPixels: map[string]bool{},
		convs:         map[string]graphObject{},
	}
}

func (s *graphServer) seedPixel(id, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pixels[id] = graphObject{"id": id, "name": name, "is_unavailable": false}
	s.accountPixels[id] = true
}

func (s *graphServer) seedForeignPixel(id, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pixels[id] = graphObject{"id": id, "name": name, "is_unavailable": false}
}

func (s *graphServer) seedConversion(id string, fields graphObject) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := graphObject{
		"id":          id,
		"account_id":  testAccountID,
		"is_archived": false,
	}
	for k, v := range fields {
		item[k] = v
	}
	s.convs[id] = item
}

func (s *graphServer) start() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(s.serve))
}

func (s *graphServer) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, r.Method+" "+r.URL.Path)
	if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
		s.t.Errorf("Authorization = %q", got)
	}
	if strings.Contains(r.URL.RawQuery, "access_token") || strings.Contains(r.URL.RawQuery, testToken) {
		s.t.Errorf("token leaked in query %q", r.URL.RawQuery)
	}
	if !strings.HasPrefix(r.URL.Path, "/"+client.Version+"/") {
		http.NotFound(w, r)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/"+client.Version+"/")
	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodGet && path == "act_"+testAccountID+"/adspixels":
		s.writeList(w, s.accountPixelValues())
	case r.Method == http.MethodGet && path == "act_"+testAccountID+"/customconversions":
		s.writeList(w, mapsValues(s.convs))
	case r.Method == http.MethodPost && path == "act_"+testAccountID+"/customconversions":
		s.posts++
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		id := testConvID
		item := graphObject{
			"id":                id,
			"account_id":        testAccountID,
			"name":              r.Form.Get("name"),
			"custom_event_type": r.Form.Get("custom_event_type"),
			"rule":              r.Form.Get("rule"),
			"pixel":             graphObject{"id": r.Form.Get("event_source_id")},
			"event_source_type": "pixel",
			"is_archived":       false,
		}
		if value := r.Form.Get("default_conversion_value"); value != "" {
			item["default_conversion_value"] = json.Number(value)
		}
		s.convs[id] = item
		_, _ = io.WriteString(w, `{"id":"`+id+`"}`)
	case r.Method == http.MethodGet && s.pixels[path] != nil:
		if strings.Contains(r.URL.Query().Get("fields"), "code") {
			s.t.Errorf("pixel read requested secret-bearing code field")
		}
		writeJSON(w, s.pixels[path])
	case r.Method == http.MethodGet && s.convs[path] != nil:
		writeJSON(w, s.convs[path])
	case r.Method == http.MethodPost && s.convs[path] != nil:
		s.posts++
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		item := s.convs[path]
		if name := r.Form.Get("name"); name != "" {
			item["name"] = name
		}
		if value := r.Form.Get("default_conversion_value"); value != "" {
			item["default_conversion_value"] = value
		}
		s.convs[path] = item
		_, _ = io.WriteString(w, `{"success":true}`)
	case r.Method == http.MethodDelete && s.convs[path] != nil:
		s.deletes++
		item := s.convs[path]
		item["is_archived"] = true
		s.convs[path] = item
		_, _ = io.WriteString(w, `{"success":true}`)
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"message":"Unsupported get request","code":803}}`)
	}
}

func (s *graphServer) accountPixelValues() []graphObject {
	out := make([]graphObject, 0, len(s.accountPixels))
	for id := range s.accountPixels {
		if item := s.pixels[id]; item != nil {
			out = append(out, item)
		}
	}
	return out
}

func (s *graphServer) writeList(w http.ResponseWriter, items []graphObject) {
	writeJSON(w, map[string]any{"data": items})
}

func (s *graphServer) mutationCounts() (posts, deletes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.posts, s.deletes
}

func mapsValues(in map[string]graphObject) []graphObject {
	out := make([]graphObject, 0, len(in))
	for _, item := range in {
		out = append(out, item)
	}
	return out
}

func writeJSON(w http.ResponseWriter, v any) {
	_ = json.NewEncoder(w).Encode(v)
}

func testProvider(t *testing.T, server *httptest.Server) *meta.Provider {
	t.Helper()
	return meta.NewWithHTTPClient(meta.Config{
		AccessToken: testToken,
		AdAccountID: testAccountID,
		BaseURL:     server.URL,
		Timeout:     time.Second,
	}, server.Client())
}

func pixelAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("meta.pixel." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func conversionAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("meta.custom_conversion." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func pixelResource(t *testing.T, name string) resource.Resource {
	t.Helper()
	return resource.Resource{
		Address:    pixelAddress(t, name),
		Attributes: resource.Attributes{meta.AttrName: "Website"},
	}
}

func conversionResource(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	return resource.Resource{Address: conversionAddress(t, name), Attributes: attrs}
}

func websiteRule() map[string]any {
	return map[string]any{
		"and": []any{
			map[string]any{"event": map[string]any{"eq": "StartTrial"}},
		},
	}
}

func websiteConversionAttrs(t *testing.T) resource.Attributes {
	t.Helper()
	return resource.Attributes{
		meta.AttrName:      "Trial Started",
		meta.AttrEventType: "START_TRIAL",
		meta.AttrRule:      websiteRule(),
		meta.AttrPixel:     resource.Ref{Address: pixelAddress(t, "website")},
	}
}

type staticCatalog map[string]resource.Address

func (c staticCatalog) AddressByRemoteID(providerName, resourceType, remoteID string) (resource.Address, bool, error) {
	addr, ok := c[providerName+"/"+resourceType+"/"+remoteID]
	return addr, ok, nil
}
