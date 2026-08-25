package client_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestGetAvailableEnvironmentsUsesExactAPISignature(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `[{"id":"live","name":"Live"}]`)
	}))
	t.Cleanup(srv.Close)

	c, err := client.New(client.Config{
		BaseURL:     srv.URL,
		TokenAuth:   "secret",
		SiteID:      "3",
		ContainerID: "containerA",
		HTTPClient:  srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	envs, err := c.TagManager().GetAvailableEnvironments(context.Background())
	if err != nil {
		t.Fatalf("GetAvailableEnvironments: %v", err)
	}
	if len(envs) != 1 || envs[0].ID != "live" {
		t.Fatalf("environments = %+v", envs)
	}
	if got.Get("method") != "TagManager.getAvailableEnvironments" {
		t.Fatalf("method = %q", got.Get("method"))
	}
	if got.Get("idSite") != "" || got.Get("idContainer") != "" {
		t.Fatalf("unexpected endpoint args: idSite=%q idContainer=%q", got.Get("idSite"), got.Get("idContainer"))
	}
}

func TestGetPublishableEnvironmentsSendsSiteOnly(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `[{"id":"live","name":"Live"}]`)
	}))
	t.Cleanup(srv.Close)

	c, err := client.New(client.Config{
		BaseURL:     srv.URL,
		TokenAuth:   "secret",
		SiteID:      "3",
		ContainerID: "containerA",
		HTTPClient:  srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	envs, err := c.TagManager().GetAvailableEnvironmentsWithPublishCapability(context.Background())
	if err != nil {
		t.Fatalf("GetAvailableEnvironmentsWithPublishCapability: %v", err)
	}
	if len(envs) != 1 || envs[0].ID != "live" {
		t.Fatalf("environments = %+v", envs)
	}
	if got.Get("method") != "TagManager.getAvailableEnvironmentsWithPublishCapability" {
		t.Fatalf("method = %q", got.Get("method"))
	}
	if got.Get("idSite") != "3" {
		t.Fatalf("idSite = %q, want 3", got.Get("idSite"))
	}
	if got.Get("idContainer") != "" {
		t.Fatalf("idContainer = %q, want omitted", got.Get("idContainer"))
	}
}
