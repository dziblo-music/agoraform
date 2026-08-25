package client_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestGetContainerDraftVersion(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{
			"idcontainer": "6OMh6taM",
			"idsite": 3,
			"name": "Website",
			"draft": {"idcontainerversion": 9}
		}`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	container, err := c.TagManager().GetContainer(context.Background())
	if err != nil {
		t.Fatalf("GetContainer: %v", err)
	}
	if container.IDContainer != "6OMh6taM" || container.IDSite != "3" || container.DraftVersion != "9" {
		t.Fatalf("container = %+v", container)
	}

	version, err := c.TagManager().DraftVersion(context.Background())
	if err != nil {
		t.Fatalf("DraftVersion: %v", err)
	}
	if version != "9" {
		t.Fatalf("version = %q, want 9", version)
	}
}

func TestGetContainerMalformed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `"oops"`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	_, err := c.TagManager().GetContainer(context.Background())
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("GetContainer = %v, want malformed", err)
	}
	assertNoSecret(t, err.Error())
}

func TestGetContainerVariablesArray(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `[{
			"idvariable": 4,
			"idcontainerversion": 9,
			"idsite": 3,
			"type": "DataLayer",
			"name": "userId",
			"status": "active",
			"parameters": {"dataLayerName": "userId"},
			"default_value": "",
			"lookup_table": []
		}]`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	vars, err := c.TagManager().GetContainerVariables(context.Background(), "9")
	if err != nil {
		t.Fatalf("GetContainerVariables: %v", err)
	}
	if got.Get("idContainerVersion") != "9" {
		t.Fatalf("idContainerVersion = %q", got.Get("idContainerVersion"))
	}
	if len(vars) != 1 {
		t.Fatalf("len(vars) = %d, want 1", len(vars))
	}
	v := vars[0]
	if v.IDVariable != "4" || v.Type != "DataLayer" || v.Name != "userId" {
		t.Fatalf("variable = %+v", v)
	}
	if v.Parameters["dataLayerName"] != "userId" {
		t.Fatalf("parameters = %v", v.Parameters)
	}
}

func TestGetContainerVariablesEmpty(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[]`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	vars, err := c.TagManager().GetContainerVariables(context.Background(), "9")
	if err != nil {
		t.Fatalf("GetContainerVariables: %v", err)
	}
	if len(vars) != 0 {
		t.Fatalf("len(vars) = %d, want 0", len(vars))
	}
}

func TestAddContainerVariable(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `7`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	id, err := c.TagManager().AddContainerVariable(context.Background(), "9", client.VariableInput{
		Type: "DataLayer",
		Name: "userId",
		Parameters: map[string]string{
			"dataLayerName": "userId",
		},
	})
	if err != nil {
		t.Fatalf("AddContainerVariable: %v", err)
	}
	if id != "7" {
		t.Fatalf("id = %q, want 7", id)
	}
	if got.Get("method") != "TagManager.addContainerVariable" {
		t.Fatalf("method = %q", got.Get("method"))
	}
	if got.Get("type") != "DataLayer" || got.Get("name") != "userId" {
		t.Fatalf("form = %v", got)
	}
	if got.Get("parameters[dataLayerName]") != "userId" {
		t.Fatalf("parameters = %v", got)
	}
	if got.Get("idContainer") != "6OMh6taM" {
		t.Fatalf("idContainer = %q", got.Get("idContainer"))
	}
}

func TestUpdateContainerVariablePreservesUnmanagedFields(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `null`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	err := c.TagManager().UpdateContainerVariable(context.Background(), "9", "4", client.VariableInput{
		Type: "DataLayer",
		Name: "User ID",
		Parameters: map[string]string{
			"dataLayerName": "user_id",
		},
	}, client.VariablePreservedFields{
		Description:  "from data layer",
		DefaultValue: "anonymous",
		LookupTable:  json.RawMessage(`[{"comparison":"equals","match_value":"x","out_value":"y"}]`),
	})
	if err != nil {
		t.Fatalf("UpdateContainerVariable: %v", err)
	}
	if got.Get("type") != "" {
		t.Fatal("update must not send type")
	}
	if got.Get("idVariable") != "4" || got.Get("name") != "User ID" {
		t.Fatalf("form = %v", got)
	}
	if got.Get("description") != "from data layer" || got.Get("defaultValue") != "anonymous" {
		t.Fatalf("preserved = %v", got)
	}
	if got.Get("lookupTable[0][comparison]") != "equals" || got.Get("lookupTable[0][out_value]") != "y" {
		t.Fatalf("lookupTable = %v", got)
	}
}

func TestGetContainerVariablesMalformed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `"oops secret-token"`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	_, err := c.TagManager().GetContainerVariables(context.Background(), "9")
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("GetContainerVariables = %v, want malformed", err)
	}
	assertNoSecret(t, err.Error())
}

func mustTagClient(t *testing.T, srv *httptest.Server) *client.Client {
	t.Helper()
	c, err := client.New(client.Config{
		BaseURL:     srv.URL,
		TokenAuth:   testToken,
		SiteID:      "3",
		ContainerID: "6OMh6taM",
		HTTPClient:  srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
