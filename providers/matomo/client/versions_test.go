package client_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestGetContainerReleases(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{
			"idcontainer": "6OMh6taM",
			"idsite": 3,
			"name": "Website",
			"draft": {"idcontainerversion": 9},
			"releases": [
				{"idcontainerversion": 12, "environment": "live"},
				{"idcontainerversion": 8, "environment": "dev"}
			]
		}`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	container, err := c.TagManager().GetContainer(context.Background())
	if err != nil {
		t.Fatalf("GetContainer: %v", err)
	}
	live, ok := container.ReleaseFor("live")
	if !ok || live.IDContainerVersion != "12" {
		t.Fatalf("live release = %+v ok=%v", live, ok)
	}
	if _, ok := container.ReleaseFor("staging"); ok {
		t.Fatal("expected no staging release")
	}
}

func TestGetAvailableEnvironments(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[{"id":"live","name":"Live"},{"id":"dev","name":"Dev"}]`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	envs, err := c.TagManager().GetAvailableEnvironments(context.Background())
	if err != nil {
		t.Fatalf("GetAvailableEnvironments: %v", err)
	}
	if len(envs) != 2 || envs[0].ID != "live" || envs[1].ID != "dev" {
		t.Fatalf("envs = %+v", envs)
	}
}

func TestGetAvailableEnvironmentsMalformed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `"oops `+testToken+`"`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	_, err := c.TagManager().GetAvailableEnvironments(context.Background())
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("GetAvailableEnvironments = %v, want malformed", err)
	}
	assertNoSecret(t, err.Error())
}

func TestCreateContainerVersion(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `15`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	id, err := c.TagManager().CreateContainerVersion(context.Background(), "agoraform-20260825T193800Z", "")
	if err != nil {
		t.Fatalf("CreateContainerVersion: %v", err)
	}
	if id != "15" {
		t.Fatalf("id = %q, want 15", id)
	}
	if got.Get("method") != "TagManager.createContainerVersion" {
		t.Fatalf("method = %q", got.Get("method"))
	}
	if got.Get("name") != "agoraform-20260825T193800Z" {
		t.Fatalf("name = %q", got.Get("name"))
	}
	if got.Get("idContainer") != "6OMh6taM" {
		t.Fatalf("idContainer = %q", got.Get("idContainer"))
	}
}

func TestCreateContainerVersionWrappedID(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"value":16}`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	id, err := c.TagManager().CreateContainerVersion(context.Background(), "v1", "from draft")
	if err != nil {
		t.Fatalf("CreateContainerVersion: %v", err)
	}
	if id != "16" {
		t.Fatalf("id = %q, want 16", id)
	}
}

func TestCreateContainerVersionMalformed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"hello":"`+testToken+`"}`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	_, err := c.TagManager().CreateContainerVersion(context.Background(), "v1", "")
	if err == nil || !strings.Contains(err.Error(), "unexpected") {
		t.Fatalf("CreateContainerVersion = %v, want unexpected payload", err)
	}
	assertNoSecret(t, err.Error())
}

func TestCreateContainerVersionAPIError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"result":"error","message":"duplicate name `+testToken+`"}`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	_, err := c.TagManager().CreateContainerVersion(context.Background(), "v1", "")
	if err == nil || !strings.Contains(err.Error(), "duplicate name") {
		t.Fatalf("CreateContainerVersion = %v, want API error", err)
	}
	assertNoSecret(t, err.Error())
}

func TestPublishContainerVersion(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `4`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	if err := c.TagManager().PublishContainerVersion(context.Background(), "15", "live"); err != nil {
		t.Fatalf("PublishContainerVersion: %v", err)
	}
	if got.Get("method") != "TagManager.publishContainerVersion" {
		t.Fatalf("method = %q", got.Get("method"))
	}
	if got.Get("idContainerVersion") != "15" || got.Get("environment") != "live" {
		t.Fatalf("form = %v", got)
	}
}

func TestPublishContainerVersionNullOK(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `null`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	if err := c.TagManager().PublishContainerVersion(context.Background(), "15", "live"); err != nil {
		t.Fatalf("PublishContainerVersion: %v", err)
	}
}

func TestPublishContainerVersionAPIError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"result":"error","message":"cannot publish `+testToken+`"}`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	err := c.TagManager().PublishContainerVersion(context.Background(), "15", "live")
	if err == nil || !strings.Contains(err.Error(), "cannot publish") {
		t.Fatalf("PublishContainerVersion = %v, want API error", err)
	}
	assertNoSecret(t, err.Error())
}

func TestCreateContainerVersionRequiresName(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no HTTP call expected")
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	_, err := c.TagManager().CreateContainerVersion(context.Background(), "  ", "")
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("CreateContainerVersion = %v, want name required", err)
	}
}

func TestPublishContainerVersionRequiresArgs(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no HTTP call expected")
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	err := c.TagManager().PublishContainerVersion(context.Background(), "", "live")
	if err == nil || !strings.Contains(err.Error(), "idContainerVersion") {
		t.Fatalf("PublishContainerVersion = %v, want id required", err)
	}
	err = c.TagManager().PublishContainerVersion(context.Background(), "15", " ")
	if err == nil || !strings.Contains(err.Error(), "environment") {
		t.Fatalf("PublishContainerVersion = %v, want environment required", err)
	}
}

func TestNilTagManagerVersionHelpers(t *testing.T) {
	t.Parallel()

	var tm *client.TagManager
	if _, err := tm.GetAvailableEnvironments(context.Background()); err == nil {
		t.Fatal("expected nil client error")
	}
	if _, err := tm.CreateContainerVersion(context.Background(), "v1", ""); err == nil {
		t.Fatal("expected nil client error")
	}
	if err := tm.PublishContainerVersion(context.Background(), "1", "live"); err == nil {
		t.Fatal("expected nil client error")
	}
}
