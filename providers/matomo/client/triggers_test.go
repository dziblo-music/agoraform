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

func TestGetContainerTriggersArray(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `[{
			"idtrigger": 5,
			"idcontainerversion": 9,
			"idsite": 3,
			"type": "CustomEvent",
			"name": "trialStarted",
			"status": "active",
			"parameters": {"eventName": "trialStarted"},
			"conditions": []
		}]`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	triggers, err := c.TagManager().GetContainerTriggers(context.Background(), "9")
	if err != nil {
		t.Fatalf("GetContainerTriggers: %v", err)
	}
	if got.Get("idContainerVersion") != "9" {
		t.Fatalf("idContainerVersion = %q", got.Get("idContainerVersion"))
	}
	if len(triggers) != 1 {
		t.Fatalf("len(triggers) = %d, want 1", len(triggers))
	}
	tr := triggers[0]
	if tr.IDTrigger != "5" || tr.Type != "CustomEvent" || tr.Name != "trialStarted" {
		t.Fatalf("trigger = %+v", tr)
	}
	if tr.Parameters["eventName"] != "trialStarted" {
		t.Fatalf("parameters = %v", tr.Parameters)
	}
}

func TestGetContainerTriggersEmpty(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[]`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	triggers, err := c.TagManager().GetContainerTriggers(context.Background(), "9")
	if err != nil {
		t.Fatalf("GetContainerTriggers: %v", err)
	}
	if len(triggers) != 0 {
		t.Fatalf("len(triggers) = %d, want 0", len(triggers))
	}
}

func TestAddContainerTrigger(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `7`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	id, err := c.TagManager().AddContainerTrigger(context.Background(), "9", client.TriggerInput{
		Type: "CustomEvent",
		Name: "trialStarted",
		Parameters: map[string]string{
			"eventName": "trialStarted",
		},
	})
	if err != nil {
		t.Fatalf("AddContainerTrigger: %v", err)
	}
	if id != "7" {
		t.Fatalf("id = %q, want 7", id)
	}
	if got.Get("method") != "TagManager.addContainerTrigger" {
		t.Fatalf("method = %q", got.Get("method"))
	}
	if got.Get("type") != "CustomEvent" || got.Get("name") != "trialStarted" {
		t.Fatalf("form = %v", got)
	}
	if got.Get("parameters[eventName]") != "trialStarted" {
		t.Fatalf("parameters = %v", got)
	}
	if got.Get("idContainer") != "6OMh6taM" {
		t.Fatalf("idContainer = %q", got.Get("idContainer"))
	}
}

func TestUpdateContainerTriggerPreservesUnmanagedFields(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `null`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	err := c.TagManager().UpdateContainerTrigger(context.Background(), "9", "4", client.TriggerInput{
		Type: "CustomEvent",
		Name: "Trial Started",
		Parameters: map[string]string{
			"eventName": "trial_started",
		},
	}, client.TriggerPreservedFields{
		Conditions: json.RawMessage(`[{"actual":"PageUrl","comparison":"equals","expected":"https://example.com"}]`),
	})
	if err != nil {
		t.Fatalf("UpdateContainerTrigger: %v", err)
	}
	if got.Get("type") != "" {
		t.Fatal("update must not send type")
	}
	if got.Get("idTrigger") != "4" || got.Get("name") != "Trial Started" {
		t.Fatalf("form = %v", got)
	}
	if got.Get("parameters[eventName]") != "trial_started" {
		t.Fatalf("parameters = %v", got)
	}
	if got.Get("conditions[0][actual]") != "PageUrl" || got.Get("conditions[0][expected]") != "https://example.com" {
		t.Fatalf("conditions = %v", got)
	}
}

func TestGetContainerTriggersMalformed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `"oops secret-token"`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	_, err := c.TagManager().GetContainerTriggers(context.Background(), "9")
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("GetContainerTriggers = %v, want malformed", err)
	}
	assertNoSecret(t, err.Error())
}
