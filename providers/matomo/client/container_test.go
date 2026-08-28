package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestGetContainersArray(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `[{
			"idcontainer": "6OMh6taM",
			"idsite": 3,
			"name": "Website",
			"context": "web",
			"description": "main",
			"draft": {"idcontainerversion": 9}
		}]`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	containers, err := c.TagManager().GetContainers(context.Background())
	if err != nil {
		t.Fatalf("GetContainers: %v", err)
	}
	if len(containers) != 1 || containers[0].IDContainer != "6OMh6taM" || containers[0].Context != "web" {
		t.Fatalf("containers = %+v", containers)
	}
	if got.Get("method") != "TagManager.getContainers" {
		t.Fatalf("method = %q", got.Get("method"))
	}
	if got.Get("idContainer") != "" {
		t.Fatalf("idContainer = %q, want omitted for site listing", got.Get("idContainer"))
	}
	if got.Get("idSite") != "3" {
		t.Fatalf("idSite = %q", got.Get("idSite"))
	}
}

func TestGetContainerNotFoundEmpty(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `false`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	_, err := c.TagManager().GetContainer(context.Background())
	if !errors.Is(err, client.ErrContainerNotFound) {
		t.Fatalf("GetContainer = %v, want ErrContainerNotFound", err)
	}
	assertNoSecret(t, err.Error())
}

func TestGetContainerNotFoundAPIError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"result":"error","message":"The requested container does not exist"}`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	_, err := c.TagManager().ForContainer("missing1").GetContainer(context.Background())
	if !errors.Is(err, client.ErrContainerNotFound) {
		t.Fatalf("GetContainer = %v, want ErrContainerNotFound", err)
	}
	assertNoSecret(t, err.Error())
}

func TestGetContainerAPIFailureIsNotNotFound(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"result":"error","message":"Unable to authenticate with `+testToken+`"}`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	_, err := c.TagManager().GetContainer(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, client.ErrContainerNotFound) {
		t.Fatal("auth failure must not be ErrContainerNotFound")
	}
	assertNoSecret(t, err.Error())
}

func TestForContainerOverridesConfigWithoutMutatingHelper(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `{"idcontainer":"otherCtr","idsite":3,"name":"Other","draft":{"idcontainerversion":2}}`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	base := c.TagManager()
	scoped := base.ForContainer("otherCtr")
	if _, err := scoped.GetContainer(context.Background()); err != nil {
		t.Fatalf("GetContainer: %v", err)
	}
	if got.Get("idContainer") != "otherCtr" {
		t.Fatalf("idContainer = %q, want otherCtr", got.Get("idContainer"))
	}
	if base.ContainerID() != "6OMh6taM" {
		t.Fatalf("base ContainerID = %q, want configured default", base.ContainerID())
	}
	if scoped.ContainerID() != "otherCtr" {
		t.Fatalf("scoped ContainerID = %q, want otherCtr", scoped.ContainerID())
	}
}

func TestAddContainer(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `"AbCd1234"`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	id, err := c.TagManager().AddContainer(context.Background(), client.ContainerInput{
		Context:     "web",
		Name:        "Main Website",
		Description: "primary",
	})
	if err != nil {
		t.Fatalf("AddContainer: %v", err)
	}
	if id != "AbCd1234" {
		t.Fatalf("id = %q", id)
	}
	if got.Get("method") != "TagManager.addContainer" {
		t.Fatalf("method = %q", got.Get("method"))
	}
	if got.Get("idContainer") != "" {
		t.Fatalf("idContainer = %q, want omitted on create", got.Get("idContainer"))
	}
	if got.Get("context") != "web" || got.Get("name") != "Main Website" || got.Get("description") != "primary" {
		t.Fatalf("params = %v", got)
	}
}

func TestAddContainerWrappedID(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"value":"XyZ9abcd"}`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	id, err := c.TagManager().AddContainer(context.Background(), client.ContainerInput{Context: "web", Name: "Site"})
	if err != nil {
		t.Fatalf("AddContainer: %v", err)
	}
	if id != "XyZ9abcd" {
		t.Fatalf("id = %q", id)
	}
}

func TestUpdateContainerPreservesFlags(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `"6OMh6taM"`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	err := c.TagManager().ForContainer("6OMh6taM").UpdateContainer(context.Background(), "6OMh6taM", client.ContainerInput{
		Name:        "Renamed",
		Description: "updated",
	}, client.ContainerPreservedFields{
		IgnoreGtmDataLayer:                 "1",
		IsTagFireLimitAllowedInPreviewMode: "true",
		ActivelySyncGtmDataLayer:           "0",
	})
	if err != nil {
		t.Fatalf("UpdateContainer: %v", err)
	}
	if got.Get("name") != "Renamed" || got.Get("description") != "updated" {
		t.Fatalf("params = %v", got)
	}
	if got.Get("ignoreGtmDataLayer") != "1" {
		t.Fatalf("ignoreGtmDataLayer = %q", got.Get("ignoreGtmDataLayer"))
	}
	if got.Get("isTagFireLimitAllowedInPreviewMode") != "1" {
		t.Fatalf("isTagFireLimitAllowedInPreviewMode = %q", got.Get("isTagFireLimitAllowedInPreviewMode"))
	}
	if got.Get("activelySyncGtmDataLayer") != "0" {
		t.Fatalf("activelySyncGtmDataLayer = %q", got.Get("activelySyncGtmDataLayer"))
	}
}

func TestGetContainerIncludesContextAndFlags(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{
			"idcontainer": "6OMh6taM",
			"idsite": 3,
			"name": "Website",
			"context": "web",
			"description": "desc",
			"status": "active",
			"ignoreGtmDataLayer": 1,
			"isTagFireLimitAllowedInPreviewMode": 0,
			"activelySyncGtmDataLayer": true,
			"draft": {"idcontainerversion": 9},
			"releases": [{"idcontainerversion": 8, "environment": "live"}]
		}`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	container, err := c.TagManager().GetContainer(context.Background())
	if err != nil {
		t.Fatalf("GetContainer: %v", err)
	}
	if container.Context != "web" || container.Description != "desc" {
		t.Fatalf("container = %+v", container)
	}
	if container.IgnoreGtmDataLayer != "1" || container.ActivelySyncGtmDataLayer != "true" {
		t.Fatalf("flags = %+v", container)
	}
	rel, ok := container.ReleaseFor("live")
	if !ok || rel.IDContainerVersion != "8" {
		t.Fatalf("release = %+v ok=%v", rel, ok)
	}
}

func TestAddContainerMalformed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"oops":true}`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	_, err := c.TagManager().AddContainer(context.Background(), client.ContainerInput{Context: "web", Name: "Site"})
	if err == nil || !strings.Contains(err.Error(), "unexpected") {
		t.Fatalf("AddContainer = %v, want unexpected payload", err)
	}
	assertNoSecret(t, err.Error())
}

func TestGetContainersMalformed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `"oops `+testToken+`"`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	_, err := c.TagManager().GetContainers(context.Background())
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("GetContainers = %v, want malformed", err)
	}
	assertNoSecret(t, err.Error())
}

func TestAddContainerJSONRoundTrip(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`"6OMh6taM"`)
	var id string
	if err := json.Unmarshal(raw, &id); err != nil {
		t.Fatal(err)
	}
	if id != "6OMh6taM" {
		t.Fatalf("id = %q", id)
	}
}
