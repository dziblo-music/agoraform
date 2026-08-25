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

func TestGetContainerTagsArray(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `[{
			"idtag": 7,
			"idcontainerversion": 9,
			"idsite": 3,
			"type": "Matomo",
			"name": "trialStarted",
			"status": "active",
			"fireTriggerIds": [5],
			"blockTriggerIds": [],
			"fireLimit": "unlimited",
			"parameters": {
				"trackingType": "event",
				"eventCategory": "signup",
				"eventAction": "trialStarted",
				"matomoConfig": {"name": "Matomo Configuration", "type": "MatomoConfiguration"}
			}
		}]`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	tags, err := c.TagManager().GetContainerTags(context.Background(), "9")
	if err != nil {
		t.Fatalf("GetContainerTags: %v", err)
	}
	if got.Get("idContainerVersion") != "9" {
		t.Fatalf("idContainerVersion = %q", got.Get("idContainerVersion"))
	}
	if len(tags) != 1 {
		t.Fatalf("len(tags) = %d, want 1", len(tags))
	}
	tag := tags[0]
	if tag.IDTag != "7" || tag.Type != "Matomo" || tag.Name != "trialStarted" {
		t.Fatalf("tag = %+v", tag)
	}
	if len(tag.FireTriggerIDs) != 1 || tag.FireTriggerIDs[0] != "5" {
		t.Fatalf("fireTriggerIds = %v", tag.FireTriggerIDs)
	}
	if tag.Parameters["eventCategory"] != "signup" {
		t.Fatalf("parameters = %v", tag.Parameters)
	}
}

func TestGetContainerTagsSnakeCaseTriggers(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[{
			"idtag": "3",
			"type": "Matomo",
			"name": "signup",
			"fire_trigger_ids": [8],
			"block_trigger_ids": [2],
			"fire_limit": "once_page",
			"parameters": {"trackingType": "event", "eventCategory": "a", "eventAction": "b"}
		}]`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	tags, err := c.TagManager().GetContainerTags(context.Background(), "9")
	if err != nil {
		t.Fatalf("GetContainerTags: %v", err)
	}
	if len(tags) != 1 || tags[0].FireTriggerIDs[0] != "8" || tags[0].BlockTriggerIDs[0] != "2" {
		t.Fatalf("tag = %+v", tags)
	}
	if tags[0].FireLimit != "once_page" {
		t.Fatalf("fireLimit = %q", tags[0].FireLimit)
	}
}

func TestGetContainerTagsEmpty(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[]`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	tags, err := c.TagManager().GetContainerTags(context.Background(), "9")
	if err != nil {
		t.Fatalf("GetContainerTags: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("len(tags) = %d, want 0", len(tags))
	}
}

func TestAddContainerTag(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `11`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	id, err := c.TagManager().AddContainerTag(context.Background(), "9", client.TagInput{
		Type:           "Matomo",
		Name:           "trialStarted",
		FireTriggerIDs: []string{"5"},
		Parameters: map[string]any{
			"trackingType":  "event",
			"eventCategory": "signup",
			"eventAction":   "trialStarted",
			"matomoConfig":  "{{Matomo Configuration}}",
		},
	})
	if err != nil {
		t.Fatalf("AddContainerTag: %v", err)
	}
	if id != "11" {
		t.Fatalf("id = %q, want 11", id)
	}
	if got.Get("method") != "TagManager.addContainerTag" {
		t.Fatalf("method = %q", got.Get("method"))
	}
	if got.Get("type") != "Matomo" || got.Get("name") != "trialStarted" {
		t.Fatalf("form = %v", got)
	}
	if got.Get("fireTriggerIds[0]") != "5" {
		t.Fatalf("fireTriggerIds = %v", got)
	}
	if got.Get("parameters[eventCategory]") != "signup" || got.Get("parameters[trackingType]") != "event" {
		t.Fatalf("parameters = %v", got)
	}
	if got.Get("idContainer") != "6OMh6taM" {
		t.Fatalf("idContainer = %q", got.Get("idContainer"))
	}
}

func TestUpdateContainerTagPreservesUnmanagedFields(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `null`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	err := c.TagManager().UpdateContainerTag(context.Background(), "9", "7", client.TagInput{
		Type:           "Matomo",
		Name:           "Trial Started",
		FireTriggerIDs: []string{"5"},
		Parameters: map[string]any{
			"trackingType":  "event",
			"eventCategory": "signup",
			"eventAction":   "trial_started",
		},
	}, client.TagPreservedFields{
		Description:     "keep me",
		BlockTriggerIDs: []string{"2"},
		FireLimit:       "once_page",
		FireDelay:       "100",
		Priority:        "10",
		Parameters: map[string]any{
			"matomoConfig": map[string]any{"name": "Matomo Configuration", "type": "MatomoConfiguration"},
		},
	})
	if err != nil {
		t.Fatalf("UpdateContainerTag: %v", err)
	}
	if got.Get("type") != "" {
		t.Fatal("update must not send type")
	}
	if got.Get("idTag") != "7" || got.Get("name") != "Trial Started" {
		t.Fatalf("form = %v", got)
	}
	if got.Get("fireTriggerIds[0]") != "5" {
		t.Fatalf("fireTriggerIds = %v", got)
	}
	if got.Get("blockTriggerIds[0]") != "2" {
		t.Fatalf("blockTriggerIds = %v", got)
	}
	if got.Get("description") != "keep me" || got.Get("fireLimit") != "once_page" {
		t.Fatalf("preserved = %v", got)
	}
	if got.Get("parameters[matomoConfig]") != "{{Matomo Configuration}}" {
		t.Fatalf("matomoConfig = %v", got)
	}
	if got.Get("parameters[eventAction]") != "trial_started" {
		t.Fatalf("parameters = %v", got)
	}
}

func TestGetContainerTagsMalformed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `"oops secret-token"`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	_, err := c.TagManager().GetContainerTags(context.Background(), "9")
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("GetContainerTags = %v, want malformed", err)
	}
	assertNoSecret(t, err.Error())
}

func TestNormalizeMatomoConfig(t *testing.T) {
	t.Parallel()

	if got := client.NormalizeMatomoConfig("{{Matomo Configuration}}"); got != "{{Matomo Configuration}}" {
		t.Fatalf("already wrapped = %v", got)
	}
	if got := client.NormalizeMatomoConfig("Matomo Configuration"); got != "{{Matomo Configuration}}" {
		t.Fatalf("name = %v", got)
	}
	got := client.NormalizeMatomoConfig(map[string]any{"name": "Matomo Configuration", "type": "MatomoConfiguration"})
	if got != "{{Matomo Configuration}}" {
		t.Fatalf("object = %v", got)
	}
}
