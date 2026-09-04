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
	testToken      = "EAAB-secret-conversion-token"
	testAccountID  = "123456789012345"
	testPixelID    = "111222333444555"
	testConvID     = "998877665544332"
	testCampaignID = "777888999000111"
	testAdSetID    = "222333444555666"
)

type graphObject map[string]any

type graphServer struct {
	t *testing.T

	mu                 sync.Mutex
	pixels             map[string]graphObject
	accountPixels      map[string]bool
	convs              map[string]graphObject
	campaigns          map[string]graphObject
	adSets             map[string]graphObject
	posts              int
	deletes            int
	requests           []string
	adSetCreateFailure bool
}

func newGraphServer(t *testing.T) *graphServer {
	t.Helper()
	return &graphServer{
		t:             t,
		pixels:        map[string]graphObject{},
		accountPixels: map[string]bool{},
		convs:         map[string]graphObject{},
		campaigns:     map[string]graphObject{},
		adSets:        map[string]graphObject{},
	}
}

func (s *graphServer) seedAdSet(id string, fields graphObject) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := graphObject{
		"id": id, "account_id": testAccountID, "campaign_id": testCampaignID,
		"name": "Instagram", "status": "PAUSED", "configured_status": "PAUSED", "effective_status": "PAUSED",
		"billing_event": "IMPRESSIONS", "optimization_goal": "OFFSITE_CONVERSIONS",
		"bid_strategy": "LOWEST_COST_WITHOUT_CAP", "destination_type": "WEBSITE",
		"promoted_object": graphObject{"pixel_id": testPixelID, "custom_conversion_id": testConvID},
		"targeting":       graphObject{"geo_locations": graphObject{"countries": []string{"US"}}, "age_min": 18, "age_max": 65},
	}
	for k, v := range fields {
		item[k] = v
	}
	s.adSets[id] = item
}

func (s *graphServer) seedCampaign(id string, fields graphObject) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := graphObject{
		"id": id, "account_id": testAccountID, "status": "PAUSED",
		"configured_status": "PAUSED", "effective_status": "PAUSED",
		"buying_type": "AUCTION", "special_ad_categories": []string{},
		"is_adset_budget_sharing_enabled": false,
	}
	for k, v := range fields {
		item[k] = v
	}
	s.campaigns[id] = item
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
	case r.Method == http.MethodGet && path == "me/permissions":
		s.writeList(w, []graphObject{{"permission": "ads_management", "status": "granted"}})
	case r.Method == http.MethodGet && path == "act_"+testAccountID:
		writeJSON(w, graphObject{"id": testAccountID, "account_status": 1})
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
	case r.Method == http.MethodPost && path == "act_"+testAccountID+"/campaigns":
		s.posts++
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var categories []string
		if err := json.Unmarshal([]byte(r.Form.Get("special_ad_categories")), &categories); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		item := graphObject{
			"id": testCampaignID, "account_id": testAccountID, "name": r.Form.Get("name"),
			"objective": r.Form.Get("objective"), "status": r.Form.Get("status"),
			"configured_status": r.Form.Get("status"), "effective_status": r.Form.Get("status"),
			"buying_type": r.Form.Get("buying_type"), "special_ad_categories": categories,
		}
		if value := r.Form.Get("daily_budget"); value != "" {
			item["daily_budget"] = value
		}
		if value := r.Form.Get("lifetime_budget"); value != "" {
			item["lifetime_budget"] = value
		}
		if value := r.Form.Get("bid_strategy"); value != "" {
			item["bid_strategy"] = value
		}
		_, sharingSet := r.Form["is_adset_budget_sharing_enabled"]
		hasCampaignBudget := r.Form.Get("daily_budget") != "" || r.Form.Get("lifetime_budget") != ""
		if sharingSet == hasCampaignBudget {
			http.Error(w, "is_adset_budget_sharing_enabled must be explicit only for ad-set budgets", http.StatusBadRequest)
			return
		}
		if sharingSet {
			item["is_adset_budget_sharing_enabled"] = r.Form.Get("is_adset_budget_sharing_enabled") == "true"
		}
		s.campaigns[testCampaignID] = item
		_, _ = io.WriteString(w, `{"id":"`+testCampaignID+`"}`)
	case r.Method == http.MethodPost && path == "act_"+testAccountID+"/adsets":
		s.posts++
		if s.adSetCreateFailure {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"error":{"message":"temporary ad set failure","code":1,"is_transient":true}}`)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var promoted, targeting graphObject
		if err := json.Unmarshal([]byte(r.Form.Get("promoted_object")), &promoted); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal([]byte(r.Form.Get("targeting")), &targeting); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		item := graphObject{"id": testAdSetID, "account_id": testAccountID, "campaign_id": r.Form.Get("campaign_id"), "name": r.Form.Get("name"), "status": r.Form.Get("status"), "configured_status": r.Form.Get("status"), "effective_status": r.Form.Get("status"), "billing_event": r.Form.Get("billing_event"), "optimization_goal": r.Form.Get("optimization_goal"), "bid_strategy": r.Form.Get("bid_strategy"), "destination_type": r.Form.Get("destination_type"), "promoted_object": promoted, "targeting": targeting}
		for _, key := range []string{"daily_budget", "lifetime_budget", "start_time", "end_time", "bid_amount"} {
			if value := r.Form.Get(key); value != "" {
				item[key] = value
			}
		}
		s.adSets[testAdSetID] = item
		_, _ = io.WriteString(w, `{"id":"`+testAdSetID+`"}`)
	case r.Method == http.MethodGet && s.pixels[path] != nil:
		if strings.Contains(r.URL.Query().Get("fields"), "code") {
			s.t.Errorf("pixel read requested secret-bearing code field")
		}
		writeJSON(w, s.pixels[path])
	case r.Method == http.MethodGet && s.convs[path] != nil:
		writeJSON(w, s.convs[path])
	case r.Method == http.MethodGet && s.campaigns[path] != nil:
		writeJSON(w, s.campaigns[path])
	case r.Method == http.MethodGet && s.adSets[path] != nil:
		writeJSON(w, s.adSets[path])
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
	case r.Method == http.MethodPost && s.campaigns[path] != nil:
		s.posts++
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		item := s.campaigns[path]
		for formKey, itemKey := range map[string]string{"name": "name", "status": "status", "daily_budget": "daily_budget", "lifetime_budget": "lifetime_budget", "bid_strategy": "bid_strategy"} {
			if value := r.Form.Get(formKey); value != "" {
				item[itemKey] = value
				if formKey == "status" {
					item["configured_status"] = value
					item["effective_status"] = value
				}
			}
		}
		if value := r.Form.Get("special_ad_categories"); value != "" {
			var categories []string
			if err := json.Unmarshal([]byte(value), &categories); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			item["special_ad_categories"] = categories
		}
		if values, ok := r.Form["is_adset_budget_sharing_enabled"]; ok && len(values) > 0 {
			item["is_adset_budget_sharing_enabled"] = values[0] == "true"
		}
		s.campaigns[path] = item
		_, _ = io.WriteString(w, `{"success":true}`)
	case r.Method == http.MethodPost && s.adSets[path] != nil:
		s.posts++
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		item := s.adSets[path]
		for _, key := range []string{"name", "status", "daily_budget", "lifetime_budget", "end_time", "bid_strategy", "bid_amount"} {
			if value := r.Form.Get(key); value != "" {
				item[key] = value
				if key == "status" {
					item["configured_status"] = value
					item["effective_status"] = value
				}
			}
		}
		if value := r.Form.Get("targeting"); value != "" {
			var targeting graphObject
			if err := json.Unmarshal([]byte(value), &targeting); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			item["targeting"] = targeting
		}
		s.adSets[path] = item
		_, _ = io.WriteString(w, `{"success":true}`)
	case r.Method == http.MethodDelete && s.convs[path] != nil:
		s.deletes++
		item := s.convs[path]
		item["is_archived"] = true
		s.convs[path] = item
		_, _ = io.WriteString(w, `{"success":true}`)
	case r.Method == http.MethodDelete && s.campaigns[path] != nil:
		s.deletes++
		item := s.campaigns[path]
		item["status"] = "DELETED"
		item["configured_status"] = "DELETED"
		item["effective_status"] = "DELETED"
		s.campaigns[path] = item
		_, _ = io.WriteString(w, `{"success":true}`)
	case r.Method == http.MethodDelete && s.adSets[path] != nil:
		s.deletes++
		item := s.adSets[path]
		item["status"] = "DELETED"
		item["configured_status"] = "DELETED"
		item["effective_status"] = "DELETED"
		s.adSets[path] = item
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

func campaignAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("meta.campaign." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func campaignResource(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{Address: campaignAddress(t, name), Attributes: attrs}
}

func adSetAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("meta.ad_set." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func adSetResource(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{Address: adSetAddress(t, name), Attributes: attrs}
}

func standardCampaignAttrs() resource.Attributes {
	return resource.Attributes{
		meta.AttrName: "Acquisition", meta.AttrObjective: "OUTCOME_SALES",
		meta.AttrSpecialAdCategories: []any{},
	}
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
