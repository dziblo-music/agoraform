package client_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
			assertUncertainPublish(t, err)
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

func TestPublishContainerVersionMisleadingAPIErrorIsNotUncertain(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"result":"error","message":"malformed JSON response"}`)
	}))
	t.Cleanup(srv.Close)

	c := mustPublishClient(t, srv)
	err := c.TagManager().PublishContainerVersion(context.Background(), "10", "live")
	if err == nil {
		t.Fatal("PublishContainerVersion succeeded, want API error")
	}
	if client.IsUncertainOutcome(err) {
		t.Fatalf("explicit API error classified as uncertain: %v", err)
	}
	var apiErr *client.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want matomo API error", err)
	}
	if !strings.Contains(err.Error(), "malformed JSON response") {
		t.Fatalf("error = %q, want API message", err)
	}
	assertNoSecret(t, err.Error())
}

func TestPublishContainerVersionReadFailureIsUncertain(t *testing.T) {
	t.Parallel()

	var received atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Store(true)
		_, _ = io.WriteString(w, `12`)
	}))
	t.Cleanup(srv.Close)

	c := mustPublishClientWithTransport(t, srv, failingBodyTransport{
		base: srv.Client().Transport,
		err:  errors.New("connection reset " + testToken),
	})
	err := c.TagManager().PublishContainerVersion(context.Background(), "10", "live")
	if err == nil {
		t.Fatal("PublishContainerVersion succeeded, want read failure")
	}
	if !received.Load() {
		t.Fatal("server did not receive the publish request")
	}
	assertUncertainPublish(t, err)
}

func TestPublishContainerVersionTransportFailureAfterRequestIsUncertain(t *testing.T) {
	t.Parallel()

	var received atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Store(true)
		_, _ = io.WriteString(w, `12`)
	}))
	t.Cleanup(srv.Close)

	c := mustPublishClientWithTransport(t, srv, failingAfterRequestTransport{
		base: srv.Client().Transport,
		err:  errors.New("connection reset " + testToken),
	})
	err := c.TagManager().PublishContainerVersion(context.Background(), "10", "live")
	if err == nil {
		t.Fatal("PublishContainerVersion succeeded, want transport failure")
	}
	if !received.Load() {
		t.Fatal("server did not receive the publish request")
	}
	assertUncertainPublish(t, err)
}

func TestPublishContainerVersionTimeoutAfterRequestIsUncertain(t *testing.T) {
	t.Parallel()

	var received atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Store(true)
		time.Sleep(100 * time.Millisecond)
		_, _ = io.WriteString(w, `12`)
	}))
	t.Cleanup(srv.Close)

	c, err := client.New(client.Config{
		BaseURL:     srv.URL,
		TokenAuth:   testToken,
		SiteID:      "3",
		ContainerID: "containerA",
		HTTPClient: &http.Client{
			Transport: srv.Client().Transport,
			Timeout:   20 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = c.TagManager().PublishContainerVersion(context.Background(), "10", "live")
	if err == nil {
		t.Fatal("PublishContainerVersion succeeded, want timeout")
	}
	if !received.Load() {
		t.Fatal("server did not receive the publish request")
	}
	assertUncertainPublish(t, err)
}

func TestPublishContainerVersionOversizedResponseIsUncertain(t *testing.T) {
	t.Parallel()

	var received atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Store(true)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(bytes.Repeat([]byte("1"), 1<<20+32))
	}))
	t.Cleanup(srv.Close)

	c := mustPublishClient(t, srv)
	err := c.TagManager().PublishContainerVersion(context.Background(), "10", "live")
	if err == nil {
		t.Fatal("PublishContainerVersion succeeded, want oversized response error")
	}
	if !received.Load() {
		t.Fatal("server did not receive the publish request")
	}
	assertUncertainPublish(t, err)
}

func TestPublishContainerVersionPreCanceledContextIsNotUncertain(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	c, err := client.New(client.Config{
		BaseURL:     "https://matomo.example.com",
		TokenAuth:   testToken,
		SiteID:      "3",
		ContainerID: "containerA",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, errors.New("transport should not be called")
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = c.TagManager().PublishContainerVersion(ctx, "10", "live")
	if err == nil {
		t.Fatal("PublishContainerVersion succeeded, want canceled request")
	}
	if client.IsUncertainOutcome(err) {
		t.Fatalf("pre-canceled context classified as uncertain: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("transport called %d times, want 0", calls.Load())
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
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
	return mustPublishClientWithTransport(t, srv, srv.Client().Transport)
}

func mustPublishClientWithTransport(t *testing.T, srv *httptest.Server, rt http.RoundTripper) *client.Client {
	t.Helper()
	c, err := client.New(client.Config{
		BaseURL:     srv.URL,
		TokenAuth:   testToken,
		SiteID:      "3",
		ContainerID: "containerA",
		HTTPClient:  &http.Client{Transport: rt},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func assertUncertainPublish(t *testing.T, err error) {
	t.Helper()
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
}

type failingBodyTransport struct {
	base http.RoundTripper
	err  error
}

func (t failingBodyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	resp.Body = &failingReadCloser{inner: resp.Body, err: t.err}
	return resp, nil
}

type failingAfterRequestTransport struct {
	base http.RoundTripper
	err  error
}

func (t failingAfterRequestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	return nil, t.err
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type failingReadCloser struct {
	inner io.ReadCloser
	err   error
}

func (f *failingReadCloser) Read([]byte) (int, error) {
	return 0, f.err
}

func (f *failingReadCloser) Close() error {
	if f.inner == nil {
		return nil
	}
	return f.inner.Close()
}
