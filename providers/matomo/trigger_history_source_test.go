package matomo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo"
)

func TestValidateHistoryChangeSourceFilter(t *testing.T) {
	t.Parallel()

	p := testTriggerProvider(t, newTriggerServer(t))
	for _, source := range []string{"pushState", "replaceState", "hashchange", "popstate"} {
		source := source
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			res := triggerResource(t, "route_change", resource.Attributes{
				matomo.AttrType:          "historyChange",
				matomo.AttrHistorySource: source,
			})
			if err := p.Validate(context.Background(), res); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestValidateHistoryChangeSourceFilterErrors(t *testing.T) {
	t.Parallel()

	p := testTriggerProvider(t, newTriggerServer(t))
	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name: "pageView rejects historySource",
			attrs: resource.Attributes{
				matomo.AttrType:          "pageView",
				matomo.AttrHistorySource: "pushState",
			},
			want: `only supported when type is "historyChange"`,
		},
		{
			name: "customEvent rejects historySource",
			attrs: resource.Attributes{
				matomo.AttrType:          "customEvent",
				matomo.AttrEvent:         "trialStarted",
				matomo.AttrHistorySource: "pushState",
			},
			want: `only supported when type is "historyChange"`,
		},
		{
			name: "unsupported source",
			attrs: resource.Attributes{
				matomo.AttrType:          "historyChange",
				matomo.AttrHistorySource: "navigation",
			},
			want: "hashchange, popstate, pushState, replaceState",
		},
		{
			name: "source whitespace",
			attrs: resource.Attributes{
				matomo.AttrType:          "historyChange",
				matomo.AttrHistorySource: " pushState",
			},
			want: "leading or trailing whitespace",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := p.Validate(context.Background(), triggerResource(t, "route_change", tc.attrs))
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err, tc.want)
			}
		})
	}
}

func TestCreateHistoryChangeTriggerWithSourceFilter(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	p := testTriggerProvider(t, srv)
	res := triggerResource(t, "route_change", resource.Attributes{
		matomo.AttrType:          "historyChange",
		matomo.AttrName:          "History Change",
		matomo.AttrHistorySource: "pushState",
	})

	if _, err := p.Create(context.Background(), res); err != nil {
		t.Fatalf("Create: %v", err)
	}
	vals := srv.lastCreateValues()
	if vals.Get("conditions[0][actual]") != "HistorySource" {
		t.Fatalf("actual = %q, want HistorySource; values=%v", vals.Get("conditions[0][actual]"), vals)
	}
	if vals.Get("conditions[0][comparison]") != "equals" {
		t.Fatalf("comparison = %q, want equals; values=%v", vals.Get("conditions[0][comparison]"), vals)
	}
	if vals.Get("conditions[0][expected]") != "pushState" {
		t.Fatalf("expected = %q, want pushState; values=%v", vals.Get("conditions[0][expected]"), vals)
	}
}

func TestPlanHistoryChangeTriggerWithSourceFilterUnchanged(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.seed(apiTrigger{
		ID:         12,
		Name:       "History Change",
		Type:       "HistoryChange",
		Conditions: `[{"actual":"HistorySource","comparison":"equals","expected":"pushState"}]`,
	})
	p := testTriggerProvider(t, srv)

	res := triggerResource(t, "route_change", resource.Attributes{
		matomo.AttrType:          "historyChange",
		matomo.AttrName:          "History Change",
		matomo.AttrHistorySource: "pushState",
	})
	got := mustPlanTrigger(t, p, res)
	if got.HasChanges() {
		t.Fatalf("equivalent History Source filter produced changes: %+v", got.Changes)
	}
}

func TestImportHistoryChangeTriggerReconstructsSourceFilter(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.seed(apiTrigger{
		ID:         12,
		Name:       "History Change",
		Type:       "HistoryChange",
		Conditions: `[{"actual":"HistorySource","comparison":"equals","expected":"replaceState"}]`,
	})
	p := testTriggerProvider(t, srv)

	live, err := p.Import(context.Background(), mustTriggerAddress(t, "route_change"), "12")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if live.Attributes[matomo.AttrHistorySource] != "replaceState" {
		t.Fatalf("historySource = %v, want replaceState", live.Attributes[matomo.AttrHistorySource])
	}
	if err := p.Validate(context.Background(), resource.Resource{Address: live.Address, Attributes: live.Attributes.Clone()}); err != nil {
		t.Fatalf("imported attributes must validate: %v", err)
	}
}

func TestUpdateHistoryChangeSourcePreservesUnmanagedConditions(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.seed(apiTrigger{
		ID:   12,
		Name: "History Change",
		Type: "HistoryChange",
		Conditions: `[
			{"actual":"PageUrl","comparison":"contains","expected":"/app"},
			{"actual":"HistorySource","comparison":"equals","expected":"pushState"}
		]`,
	})
	p := testTriggerProvider(t, srv)
	desired := triggerResource(t, "route_change", resource.Attributes{
		matomo.AttrType:          "historyChange",
		matomo.AttrName:          "History Change",
		matomo.AttrHistorySource: "replaceState",
	})

	actual, err := p.Read(context.Background(), desired)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if _, err := p.Update(context.Background(), desired, actual); err != nil {
		t.Fatalf("Update: %v", err)
	}
	vals := srv.lastUpdateValues()
	if vals.Get("conditions[0][actual]") != "PageUrl" || vals.Get("conditions[0][comparison]") != "contains" || vals.Get("conditions[0][expected]") != "/app" {
		t.Fatalf("unmanaged condition not preserved: %v", vals)
	}
	if vals.Get("conditions[1][actual]") != "HistorySource" || vals.Get("conditions[1][comparison]") != "equals" || vals.Get("conditions[1][expected]") != "replaceState" {
		t.Fatalf("managed History Source condition not replaced: %v", vals)
	}
	if vals.Get("conditions[2][actual]") != "" {
		t.Fatalf("unexpected duplicate History Source condition: %v", vals)
	}
}
