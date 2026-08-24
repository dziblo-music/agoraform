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

func TestGetGoalsKeyedObject(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{
			"1": {
				"idgoal": "1",
				"idsite": 3,
				"name": "Trial Started",
				"match_attribute": "event_action",
				"pattern": "trialStarted",
				"pattern_type": "exact",
				"case_sensitive": 0,
				"revenue": "0"
			}
		}`)
	}))
	t.Cleanup(srv.Close)

	c := mustClient(t, srv.URL, testToken)
	goals, err := c.Analytics().GetGoals(context.Background())
	if err != nil {
		t.Fatalf("GetGoals: %v", err)
	}
	if len(goals) != 1 {
		t.Fatalf("len(goals) = %d, want 1", len(goals))
	}
	g := goals[0]
	if g.IDGoal != "1" || g.IDSite != "3" || g.Name != "Trial Started" {
		t.Fatalf("goal = %+v", g)
	}
	if g.MatchAttribute != "event_action" || g.Pattern != "trialStarted" || g.PatternType != "exact" {
		t.Fatalf("match fields = %+v", g)
	}
	if g.CaseSensitive != "0" {
		t.Fatalf("case_sensitive = %q, want 0", g.CaseSensitive)
	}
}

func TestGetGoalsEmptyArray(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[]`)
	}))
	t.Cleanup(srv.Close)

	c := mustClient(t, srv.URL, testToken)
	goals, err := c.Analytics().GetGoals(context.Background())
	if err != nil {
		t.Fatalf("GetGoals: %v", err)
	}
	if len(goals) != 0 {
		t.Fatalf("len(goals) = %d, want 0", len(goals))
	}
}

func TestGetGoalsArrayAndSingleObject(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		vals, _ := url.ParseQuery(string(body))
		if vals.Get("format") != "JSON" {
			t.Errorf("format = %q", vals.Get("format"))
		}
		switch vals.Get("idSite") {
		case "array":
			_, _ = io.WriteString(w, `[{"idgoal":2,"name":"A","match_attribute":"url","pattern":"/a","pattern_type":"contains"}]`)
		default:
			_, _ = io.WriteString(w, `{"idgoal":"9","name":"Solo","match_attribute":"manually"}`)
		}
	}))
	t.Cleanup(srv.Close)

	arrayClient, err := client.New(client.Config{
		BaseURL:    srv.URL,
		TokenAuth:  testToken,
		SiteID:     "array",
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	goals, err := arrayClient.Analytics().GetGoals(context.Background())
	if err != nil {
		t.Fatalf("array GetGoals: %v", err)
	}
	if len(goals) != 1 || goals[0].IDGoal != "2" || goals[0].Name != "A" {
		t.Fatalf("array goals = %+v", goals)
	}

	solo := mustClient(t, srv.URL, testToken)
	goals, err = solo.Analytics().GetGoals(context.Background())
	if err != nil {
		t.Fatalf("single GetGoals: %v", err)
	}
	if len(goals) != 1 || goals[0].IDGoal != "9" || goals[0].Name != "Solo" {
		t.Fatalf("single goal = %+v", goals)
	}
}

func TestGetGoalsMalformedPayload(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `"not-goals"`)
	}))
	t.Cleanup(srv.Close)

	c := mustClient(t, srv.URL, testToken)
	_, err := c.Analytics().GetGoals(context.Background())
	if err == nil {
		t.Fatal("expected malformed error")
	}
	if !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("error = %q, want malformed", err)
	}
	assertNoSecret(t, err.Error())
}

func TestAddGoalReturnsID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "number", body: `12`, want: "12"},
		{name: "string", body: `"7"`, want: "7"},
		{name: "wrapped", body: `{"value":4}`, want: "4"},
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

			c := mustClient(t, srv.URL, testToken)
			id, err := c.Analytics().AddGoal(context.Background(), client.GoalInput{
				Name:           "Trial Started",
				MatchAttribute: "event_action",
				Pattern:        "trialStarted",
				PatternType:    "exact",
			})
			if err != nil {
				t.Fatalf("AddGoal: %v", err)
			}
			if id != tc.want {
				t.Fatalf("id = %q, want %q", id, tc.want)
			}
			if got.Get("method") != "Goals.addGoal" {
				t.Fatalf("method = %q", got.Get("method"))
			}
			if got.Get("name") != "Trial Started" || got.Get("matchAttribute") != "event_action" {
				t.Fatalf("params = %v", got)
			}
			if strings.Contains(got.Encode(), testToken) && got.Get("token_auth") != testToken {
				t.Fatal("token leaked outside token_auth")
			}
		})
	}
}

func TestAddGoalMalformedID(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"nope":true}`)
	}))
	t.Cleanup(srv.Close)

	c := mustClient(t, srv.URL, testToken)
	_, err := c.Analytics().AddGoal(context.Background(), client.GoalInput{Name: "X", MatchAttribute: "manually"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unexpected") {
		t.Fatalf("error = %q, want unexpected payload", err)
	}
}

func TestUpdateGoalSendsID(t *testing.T) {
	t.Parallel()

	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(body))
		_, _ = io.WriteString(w, `null`)
	}))
	t.Cleanup(srv.Close)

	c := mustClient(t, srv.URL, testToken)
	if err := c.Analytics().UpdateGoal(context.Background(), "3", client.GoalInput{
		Name:           "Signup",
		MatchAttribute: "event_name",
		Pattern:        "signed_up",
		PatternType:    "contains",
	}); err != nil {
		t.Fatalf("UpdateGoal: %v", err)
	}
	if got.Get("method") != "Goals.updateGoal" {
		t.Fatalf("method = %q", got.Get("method"))
	}
	if got.Get("idGoal") != "3" {
		t.Fatalf("idGoal = %q, want 3", got.Get("idGoal"))
	}
	if got.Get("name") != "Signup" {
		t.Fatalf("name = %q", got.Get("name"))
	}
}

func TestUpdateGoalRequiresID(t *testing.T) {
	t.Parallel()

	c := mustClient(t, "https://matomo.example.com", testToken)
	err := c.Analytics().UpdateGoal(context.Background(), "  ", client.GoalInput{Name: "X"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "idGoal") {
		t.Fatalf("error = %q, want idGoal", err)
	}
}

func TestGoalAPIErrorRedactsToken(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		msg, _ := json.Marshal(map[string]string{
			"result":  "error",
			"message": "Unable to authenticate with " + testToken,
		})
		_, _ = w.Write(msg)
	}))
	t.Cleanup(srv.Close)

	c := mustClient(t, srv.URL, testToken)
	_, err := c.Analytics().GetGoals(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	assertNoSecret(t, err.Error())
}
