package client_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestDeleteGoalSendsID(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `true`)
	}))
	t.Cleanup(srv.Close)

	c := mustClient(t, srv.URL, testToken)
	if err := c.Analytics().DeleteGoal(context.Background(), "12"); err != nil {
		t.Fatalf("DeleteGoal: %v", err)
	}
	if got.Get("method") != "Goals.deleteGoal" {
		t.Fatalf("method = %q", got.Get("method"))
	}
	if got.Get("idGoal") != "12" {
		t.Fatalf("form = %v", got)
	}
}

func TestDeleteContainerUsesExplicitID(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `true`)
	}))
	t.Cleanup(srv.Close)

	c := mustTagClient(t, srv)
	if err := c.TagManager().DeleteContainer(context.Background(), "Aa000001"); err != nil {
		t.Fatalf("DeleteContainer: %v", err)
	}
	if got.Get("method") != "TagManager.deleteContainer" {
		t.Fatalf("method = %q", got.Get("method"))
	}
	if got.Get("idContainer") != "Aa000001" {
		t.Fatalf("idContainer = %q", got.Get("idContainer"))
	}
}

func TestDeleteTagManagerDraftEntities(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `true`)
	}))
	t.Cleanup(srv.Close)

	tm := mustTagClient(t, srv).TagManager().ForContainer("6OMh6taM")
	if err := tm.DeleteContainerVariable(context.Background(), "9", "2"); err != nil {
		t.Fatalf("DeleteContainerVariable: %v", err)
	}
	if got.Get("method") != "TagManager.deleteContainerVariable" || got.Get("idVariable") != "2" || got.Get("idContainerVersion") != "9" {
		t.Fatalf("variable form = %v", got)
	}
	if err := tm.DeleteContainerTrigger(context.Background(), "9", "5"); err != nil {
		t.Fatalf("DeleteContainerTrigger: %v", err)
	}
	if got.Get("method") != "TagManager.deleteContainerTrigger" || got.Get("idTrigger") != "5" {
		t.Fatalf("trigger form = %v", got)
	}
	if err := tm.DeleteContainerTag(context.Background(), "9", "7"); err != nil {
		t.Fatalf("DeleteContainerTag: %v", err)
	}
	if got.Get("method") != "TagManager.deleteContainerTag" || got.Get("idTag") != "7" {
		t.Fatalf("tag form = %v", got)
	}
}
