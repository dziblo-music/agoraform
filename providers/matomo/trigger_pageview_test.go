package matomo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo"
)

func TestValidatePageViewAndHistoryChangeTriggers(t *testing.T) {
	t.Parallel()

	p := testTriggerProvider(t, newTriggerServer(t))
	cases := []struct {
		name  string
		res   string
		attrs resource.Attributes
	}{
		{
			name:  "pageView",
			res:   "pageview",
			attrs: resource.Attributes{matomo.AttrType: "pageView"},
		},
		{
			name: "pageView named",
			res:  "pageview",
			attrs: resource.Attributes{
				matomo.AttrType: "pageView",
				matomo.AttrName: "Pageview",
			},
		},
		{
			name:  "historyChange",
			res:   "route_change",
			attrs: resource.Attributes{matomo.AttrType: "historyChange"},
		},
		{
			name: "historyChange named",
			res:  "route_change",
			attrs: resource.Attributes{
				matomo.AttrType: "historyChange",
				matomo.AttrName: "History Change",
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := p.Validate(context.Background(), triggerResource(t, tc.res, tc.attrs)); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestValidatePageViewAndHistoryChangeTriggerErrors(t *testing.T) {
	t.Parallel()

	p := testTriggerProvider(t, newTriggerServer(t))
	cases := []struct {
		name  string
		res   string
		attrs resource.Attributes
		want  string
	}{
		{
			name:  "pageView with event",
			res:   "pageview",
			attrs: resource.Attributes{matomo.AttrType: "pageView", matomo.AttrEvent: "mtm.PageView"},
			want:  `attribute "event" is only supported when type is "customEvent"`,
		},
		{
			name:  "historyChange with event",
			res:   "route_change",
			attrs: resource.Attributes{matomo.AttrType: "historyChange", matomo.AttrEvent: "mtm.HistoryChange"},
			want:  `attribute "event" is only supported when type is "customEvent"`,
		},
		{
			name:  "matomo native pageView casing",
			res:   "pageview",
			attrs: resource.Attributes{matomo.AttrType: "PageView"},
			want:  "pageView",
		},
		{
			name:  "matomo native historyChange casing",
			res:   "route_change",
			attrs: resource.Attributes{matomo.AttrType: "HistoryChange"},
			want:  "historyChange",
		},
		{
			name:  "empty pageView name",
			res:   "pageview",
			attrs: resource.Attributes{matomo.AttrType: "pageView", matomo.AttrName: ""},
			want:  "non-empty",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := triggerResource(t, tc.res, tc.attrs)
			err := p.Validate(context.Background(), res)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err, tc.want)
			}
			assertNoProviderSecret(t, err.Error())
		})
	}
}

func TestCreatePageViewTriggerOmitsParameters(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	p := testTriggerProvider(t, srv)
	res := triggerResource(t, "pageview", resource.Attributes{matomo.AttrType: "pageView"})

	live, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID == "" {
		t.Fatal("create returned empty identity")
	}
	vals := srv.lastCreateValues()
	if vals.Get("type") != "PageView" {
		t.Fatalf("create type = %q, want PageView", vals.Get("type"))
	}
	if vals.Get("name") != "pageview" {
		t.Fatalf("create name = %q, want address name", vals.Get("name"))
	}
	if vals.Get("parameters[eventName]") != "" {
		t.Fatalf("pageView trigger must not send eventName: %v", vals)
	}
	if live.Attributes[matomo.AttrType] != "pageView" {
		t.Fatalf("type = %v", live.Attributes[matomo.AttrType])
	}
	if _, ok := live.Attributes[matomo.AttrEvent]; ok {
		t.Fatalf("event leaked onto pageView trigger: %v", live.Attributes[matomo.AttrEvent])
	}
}

func TestCreateHistoryChangeTrigger(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	p := testTriggerProvider(t, srv)
	res := triggerResource(t, "route_change", resource.Attributes{
		matomo.AttrType: "historyChange",
		matomo.AttrName: "History Change",
	})

	live, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	vals := srv.lastCreateValues()
	if vals.Get("type") != "HistoryChange" {
		t.Fatalf("create type = %q, want HistoryChange", vals.Get("type"))
	}
	if vals.Get("name") != "History Change" {
		t.Fatalf("create name = %q", vals.Get("name"))
	}
	if vals.Get("parameters[eventName]") != "" {
		t.Fatalf("historyChange trigger must not send eventName: %v", vals)
	}
	if live.Attributes[matomo.AttrType] != "historyChange" {
		t.Fatalf("type = %v", live.Attributes[matomo.AttrType])
	}
}

func TestReadPageViewTrigger(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.seed(apiTrigger{ID: 11, Name: "Pageview", Type: "PageView", Description: "initial load"})
	p := testTriggerProvider(t, srv)

	live, err := p.Read(context.Background(), triggerResource(t, "pageview", resource.Attributes{
		matomo.AttrType: "pageView",
		matomo.AttrName: "Pageview",
	}))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "11" {
		t.Fatalf("identity = %q, want 11", live.Identity.ID)
	}
	if live.Attributes[matomo.AttrType] != "pageView" {
		t.Fatalf("type = %v", live.Attributes[matomo.AttrType])
	}
	if _, ok := live.Attributes[matomo.AttrEvent]; ok {
		t.Fatal("pageView trigger must omit event")
	}
	if live.Computed["description"] != "initial load" {
		t.Fatalf("description = %v", live.Computed["description"])
	}
}

func TestPlanPageViewTriggerUnchanged(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.seed(apiTrigger{ID: 11, Name: "pageview", Type: "PageView"})
	p := testTriggerProvider(t, srv)

	got := mustPlanTrigger(t, p, triggerResource(t, "pageview", resource.Attributes{matomo.AttrType: "pageView"}))
	if got.HasChanges() {
		t.Fatalf("equivalent pageView trigger produced changes: %+v", got.Changes)
	}
}

func TestPlanHistoryChangeTriggerUpdateName(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.seed(apiTrigger{ID: 12, Name: "History Change", Type: "HistoryChange"})
	p := testTriggerProvider(t, srv)

	res := triggerResource(t, "route_change", resource.Attributes{
		matomo.AttrType: "historyChange",
		matomo.AttrName: "SPA History Change",
	})
	res.Identity = resource.Identity{ID: "12"}
	got := mustPlanTrigger(t, p, res)
	change := changeByAddr(t, got, "matomo.trigger.route_change")
	if change.Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want update", change)
	}
}

func TestImportPageViewTrigger(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.seed(apiTrigger{ID: 11, Name: "Pageview", Type: "PageView"})
	p := testTriggerProvider(t, srv)

	live, err := p.Import(context.Background(), mustTriggerAddress(t, "pageview"), "11")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if live.Identity.ID != "11" {
		t.Fatalf("identity = %q, want 11", live.Identity.ID)
	}
	if live.Attributes[matomo.AttrType] != "pageView" {
		t.Fatalf("type = %v", live.Attributes[matomo.AttrType])
	}
	if live.Attributes[matomo.AttrName] != "Pageview" {
		t.Fatalf("name = %v", live.Attributes[matomo.AttrName])
	}
	if _, ok := live.Attributes[matomo.AttrEvent]; ok {
		t.Fatal("imported pageView trigger must omit event")
	}
	if err := p.Validate(context.Background(), resource.Resource{Address: live.Address, Attributes: live.Attributes.Clone()}); err != nil {
		t.Fatalf("imported attributes must validate: %v", err)
	}
}

func TestReadTriggerUnsupportedRemoteTypeStillRejected(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.seed(apiTrigger{ID: 3, Name: "All Elements Click", Type: "AllElementsClick"})
	p := testTriggerProvider(t, srv)

	_, err := p.Read(context.Background(), triggerResource(t, "click", resource.Attributes{
		matomo.AttrType: "pageView",
		matomo.AttrName: "All Elements Click",
	}))
	if err == nil {
		t.Fatal("expected unsupported type error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported remote type must not look like not found")
	}
	if !strings.Contains(err.Error(), "AllElementsClick") {
		t.Fatalf("error = %q, want remote type", err)
	}
}
