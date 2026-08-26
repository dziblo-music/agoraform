package client_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
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
	if got := config.Parameters["enableLinkTracking"]; got != "true" {
		t.Fatalf("enableLinkTracking = %q, want true", got)
	}
	if got := config.Parameters["crossDomainLinkingTimeout"]; got != "180" {
		t.Fatalf("crossDomainLinkingTimeout = %q, want 180", got)
	}
	if got := config.Parameters["domains"]; got != `["example.com","shop.example.com"]` {
		t.Fatalf("domains = %q", got)
	}
	if got := config.Parameters["customDimensions"]; got != `[{"index":1,"value":"member"}]` {
		t.Fatalf("customDimensions = %q", got)
	}
	if got := config.Parameters["customData"]; got != `{"source":"release-test"}` {
		t.Fatalf("customData = %q", got)
	}

	managed := vars[1]
	if managed.IDVariable != "19" || managed.Type != "DataLayer" || managed.Name != "Trial user ID" {
		t.Fatalf("managed variable = %+v", managed)
	}
	if got := managed.Parameters["dataLayerName"]; got != "userId" {
		t.Fatalf("dataLayerName = %q, want userId", got)
	}
}
