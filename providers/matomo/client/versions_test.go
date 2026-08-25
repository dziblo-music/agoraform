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

func TestPublishContainerVersionAcceptsReleaseIDs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{name: "number", body: `12`},
		{name: "string", body: `"7"`},
		{name: "wrapped number", body: `{"value":4}`},
		{name: "wrapped string", body: `{"value":"9"}`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got url.Values
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				got, _ = url.ParseQuery(string(body))
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(srv.Close)

			c := mustPublishClient(t, srv)
			if err := c.TagManager().PublishContainerVersion(context.Background(), "10", "live"); err != nil {
				t.Fatalf("PublishContainerVersion: %v", err)
			}
			if got.Get("method") != "TagManager.publishContainerVersion" {
				t.Fatalf("method = %q", got.Get("method"))
			}
			if got.Get("idContainerVersion") != "10" || got.Get("environment") != "live" {
				t.Fatalf("params = %v", got)
			}
		})
	}
}

func TestPublishContainerVersionRejectsUnconfirmedResponses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{name: "empty body", body: ""},
		{name: "null", body: "null"},
		{name: "empty string", body: `""`},
		{name: "boolean", body: "true"},
		{name: "non-numeric string", body: `"live"`},
		{name: "float", body: "1.5"},
		{name: "array", body: `[1]`},
		{name: "object", body: `{"idcontainerrelease":1}`},
		{name: "empty object", body: `{}`},
		{name: "wrapped empty", body: `{"value":""}`},
		{name: "wrapped null", body: `{"value":null}`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(srv.Close)

			c := mustPublishClient(t, srv)
			err := c.TagManager().PublishContainerVersion(context.Background(), "10", "live")
			if err == nil {
				t.Fatal("PublishContainerVersion succeeded, want uncertain error")
			}
			if !client.IsUncertainOutcome(err) {
				t.Fatalf("error = %v, want uncertain outcome", err)
			}
			if !strings.Contains(err.Error(), "uncertain") {
				t.Fatalf("error = %q, want uncertain publication outcome", err)
			}
			if !strings.Contains(err.Error(), "do not create another version") {
				t.Fatalf("error = %q, want guidance against creating another version", err)
			}
			assertNoSecret(t, err.Error())
		})
	}
}

func TestPublishContainerVersionAPIErrorRedactsToken(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		msg, _ := json.Marshal(map[string]string{
			"result":  "error",
			"message": "cannot publish " + testToken,
		})
		_, _ = w.Write(msg)
	}))
	t.Cleanup(srv.Close)

	c := mustPublishClient(t, srv)
	err := c.TagManager().PublishContainerVersion(context.Background(), "10", "live")
	if err == nil {
		t.Fatal("PublishContainerVersion succeeded, want API error")
	}
	if client.IsUncertainOutcome(err) {
		t.Fatalf("API error classified as uncertain: %v", err)
	}
	if !strings.Contains(err.Error(), "cannot publish") {
		t.Fatalf("error = %q, want API message", err)
	}
	assertNoSecret(t, err.Error())
}

func TestPublishContainerVersionPreRequestErrorsAreNotUncertain(t *testing.T) {
	t.Parallel()

	c := mustClient(t, "https://matomo.example.com", testToken)
	cases := []struct {
		name        string
		versionID   string
		environment string
		want        string
	}{
		{name: "version", versionID: "  ", environment: "live", want: "idContainerVersion"},
		{name: "environment", versionID: "10", environment: "  ", want: "environment"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := c.TagManager().PublishContainerVersion(context.Background(), tc.versionID, tc.environment)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("PublishContainerVersion = %v, want %s", err, tc.want)
			}
			if client.IsUncertainOutcome(err) {
				t.Fatalf("pre-request error classified as uncertain: %v", err)
			}
		})
	}
}

func mustPublishClient(t *testing.T, srv *httptest.Server) *client.Client {
	t.Helper()
	c, err := client.New(client.Config{
		BaseURL:     srv.URL,
		TokenAuth:   testToken,
		SiteID:      "3",
		ContainerID: "containerA",
		HTTPClient:  srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
