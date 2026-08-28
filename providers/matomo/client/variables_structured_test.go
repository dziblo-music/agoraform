package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestGetContainerVariablesAllowsStructuredParameters(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[
			{
				"idvariable": 20,
				"idcontainerversion": 9,
				"idsite": 3,
				"type": "MatomoConfiguration",
				"name": "Matomo Configuration",
				"status": "active",
				"parameters": {
					"matomoUrl": "https://matomo.example.com/",
					"idSite": "3",
					"enableLinkTracking": true,
					"crossDomainLinkingTimeout": 180,
					"domains": ["example.com", "shop.example.com"],
					"customDimensions": [{"index": 1, "value": "member"}],
					"customData": {"source": "release-test"}
				},
				"default_value": "",
				"lookup_table": []
			},
			{
				"idvariable": 19,
				"idcontainerversion": 9,
				"idsite": 3,
				"type": "DataLayer",
				"name": "Trial user ID",
				"status": "active",
				"parameters": {"dataLayerName": "userId"},
				"default_value": "",
				"lookup_table": []
			}
		]`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	vars, err := c.TagManager().GetContainerVariables(context.Background(), "9")
	if err != nil {
		t.Fatalf("GetContainerVariables: %v", err)
	}
	if len(vars) != 2 {
		t.Fatalf("len(vars) = %d, want 2", len(vars))
	}

	config := vars[0]
	if config.IDVariable != "20" || config.Type != "MatomoConfiguration" {
		t.Fatalf("config variable = %+v", config)
	}
	if got := config.Parameters["enableLinkTracking"]; got != true {
		t.Fatalf("enableLinkTracking = %#v, want true", got)
	}
	if got := config.Parameters["crossDomainLinkingTimeout"]; got != float64(180) {
		t.Fatalf("crossDomainLinkingTimeout = %#v, want 180", got)
	}
	if got := config.Parameters["domains"]; fmt.Sprint(got) != "[example.com shop.example.com]" {
		t.Fatalf("domains = %#v", got)
	}
	gotDims, _ := json.Marshal(config.Parameters["customDimensions"])
	if string(gotDims) != `[{"index":1,"value":"member"}]` {
		t.Fatalf("customDimensions = %s", gotDims)
	}
	gotData, _ := json.Marshal(config.Parameters["customData"])
	if string(gotData) != `{"source":"release-test"}` {
		t.Fatalf("customData = %s", gotData)
	}

	managed := vars[1]
	if managed.IDVariable != "19" || managed.Type != "DataLayer" || managed.Name != "Trial user ID" {
		t.Fatalf("managed variable = %+v", managed)
	}
	if got := managed.Parameters["dataLayerName"]; got != "userId" {
		t.Fatalf("dataLayerName = %q, want userId", got)
	}
}

func TestUpdateContainerVariablePreservesStructuredParameters(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `null`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	err := c.TagManager().UpdateContainerVariable(context.Background(), "9", "20", client.VariableInput{
		Type: "MatomoConfiguration",
		Name: "Matomo Configuration",
		Parameters: map[string]any{
			"matomoUrl":          "https://matomo.example.com",
			"idSite":             "1",
			"enableLinkTracking": true,
		},
	}, client.VariablePreservedFields{
		Parameters: map[string]any{
			"matomoUrl":                 "https://old.example.com",
			"idSite":                    "9",
			"enableLinkTracking":        false,
			"enableDoNotTrack":          true,
			"crossDomainLinkingTimeout": 180,
			"domains":                   []any{"example.com", "shop.example.com"},
			"customDimensions":          []any{map[string]any{"index": 1, "value": "member"}},
			"customData":                map[string]any{"source": "release-test"},
		},
	})
	if err != nil {
		t.Fatalf("UpdateContainerVariable: %v", err)
	}
	if got.Get("parameters[matomoUrl]") != "https://matomo.example.com" {
		t.Fatalf("matomoUrl = %v", got)
	}
	if got.Get("parameters[idSite]") != "1" {
		t.Fatalf("idSite = %v", got)
	}
	if got.Get("parameters[enableLinkTracking]") != "1" {
		t.Fatalf("enableLinkTracking = %v", got)
	}
	if got.Get("parameters[enableDoNotTrack]") != "1" {
		t.Fatalf("enableDoNotTrack dropped: %v", got)
	}
	if got.Get("parameters[crossDomainLinkingTimeout]") != "180" {
		t.Fatalf("crossDomainLinkingTimeout dropped: %v", got)
	}
	if got.Get("parameters[domains][0]") != "example.com" || got.Get("parameters[domains][1]") != "shop.example.com" {
		t.Fatalf("domains dropped: %v", got)
	}
	if got.Get("parameters[customDimensions][0][index]") != "1" || got.Get("parameters[customDimensions][0][value]") != "member" {
		t.Fatalf("customDimensions dropped: %v", got)
	}
	if got.Get("parameters[customData][source]") != "release-test" {
		t.Fatalf("customData dropped: %v", got)
	}
}

func TestCloneJSONMapRejectsUnencodableParameters(t *testing.T) {
	t.Parallel()

	_, err := client.CloneJSONMap(map[string]any{"bad": make(chan int)})
	if err == nil {
		t.Fatal("expected unencodable parameters to fail")
	}
}
