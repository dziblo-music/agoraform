package matomo_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/destroy"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/matomo"
)

func validPageviewTagAttrs(t *testing.T, trigger string) resource.Attributes {
	t.Helper()
	return resource.Attributes{
		matomo.AttrType:         "matomoAnalytics",
		matomo.AttrTrackingType: "pageview",
		matomo.AttrTrigger:      triggerRef(t, trigger),
	}
}

func TestValidatePageviewTag(t *testing.T) {
	t.Parallel()

	p := testTagProvider(t, newTagServer(t))
	cases := []struct {
		name  string
		attrs resource.Attributes
	}{
		{
			name:  "minimal",
			attrs: validPageviewTagAttrs(t, "pageview"),
		},
		{
			name: "documentTitle and customUrl literals",
			attrs: resource.Attributes{
				matomo.AttrType:          "matomoAnalytics",
				matomo.AttrTrackingType:  "pageview",
				matomo.AttrTrigger:       triggerRef(t, "route_change"),
				matomo.AttrDocumentTitle: "{{PageTitle}}",
				matomo.AttrCustomURL:     "{{PageUrl}}",
				matomo.AttrName:          "SPA pageview",
			},
		},
		{
			name: "managed configuration and variable refs",
			attrs: resource.Attributes{
				matomo.AttrType:                "matomoAnalytics",
				matomo.AttrTrackingType:        "pageview",
				matomo.AttrTrigger:             triggerRef(t, "pageview"),
				matomo.AttrDocumentTitle:       variableRef(t, "page_title"),
				matomo.AttrCustomURL:           variableRef(t, "page_url"),
				matomo.AttrMatomoConfiguration: variableRef(t, "config"),
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := p.Validate(context.Background(), tagResource(t, "pageview", tc.attrs)); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestValidatePageviewTagErrors(t *testing.T) {
	t.Parallel()

	p := testTagProvider(t, newTagServer(t))
	addr := mustTagAddress(t, "pageview")
	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name:  "pageview with eventCategory",
			attrs: withTagAttr(validPageviewTagAttrs(t, "pageview"), matomo.AttrEventCategory, "signup"),
			want:  `attribute "eventCategory" is not supported when trackingType is "pageview"`,
		},
		{
			name:  "pageview with eventAction",
			attrs: withTagAttr(validPageviewTagAttrs(t, "pageview"), matomo.AttrEventAction, "trialStarted"),
			want:  `attribute "eventAction" is not supported when trackingType is "pageview"`,
		},
		{
			name:  "event tag with documentTitle",
			attrs: withTagAttr(validTagAttrs(t), matomo.AttrDocumentTitle, "{{PageTitle}}"),
			want:  `attribute "documentTitle" is not supported when trackingType is "event"`,
		},
		{
			name:  "event tag with customUrl",
			attrs: withTagAttr(validTagAttrs(t), matomo.AttrCustomURL, "{{PageUrl}}"),
			want:  `attribute "customUrl" is not supported when trackingType is "event"`,
		},
		{
			name:  "ui trackingType casing",
			attrs: withTagAttr(validPageviewTagAttrs(t, "pageview"), matomo.AttrTrackingType, "pageView"),
			want:  "pageview",
		},
		{
			name:  "unsupported goal trackingType",
			attrs: withTagAttr(validPageviewTagAttrs(t, "pageview"), matomo.AttrTrackingType, "goal"),
			want:  "event, pageview",
		},
		{
			name:  "unsupported initialise trackingType",
			attrs: withTagAttr(validPageviewTagAttrs(t, "pageview"), matomo.AttrTrackingType, "initialise"),
			want:  "event, pageview",
		},
		{
			name:  "empty trackingType",
			attrs: withTagAttr(validTagAttrs(t), matomo.AttrTrackingType, ""),
			want:  "non-empty",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := p.Validate(context.Background(), resource.Resource{Address: addr, Attributes: tc.attrs})
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

func TestCreatePageviewTag(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	p := testTagProvider(t, srv)
	res := tagResource(t, "pageview", resource.Attributes{
		matomo.AttrType:         "matomoAnalytics",
		matomo.AttrTrackingType: "pageview",
		matomo.AttrTrigger:      resolvedTrigger(t, "pageview", "11"),
		matomo.AttrName:         "Pageview",
	})

	live, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID == "" {
		t.Fatal("create returned empty identity")
	}
	vals := srv.lastCreateValues()
	if vals.Get("type") != "Matomo" {
		t.Fatalf("create type = %q, want Matomo", vals.Get("type"))
	}
	if vals.Get("parameters[trackingType]") != "pageview" {
		t.Fatalf("trackingType = %v", vals)
	}
	if vals.Get("parameters[eventCategory]") != "" || vals.Get("parameters[eventAction]") != "" {
		t.Fatalf("pageview tag must not send event fields: %v", vals)
	}
	if vals.Get("parameters[matomoConfig]") != "{{Matomo Configuration}}" {
		t.Fatalf("matomoConfig = %v", vals)
	}
	if vals.Get("fireTriggerIds[0]") != "11" {
		t.Fatalf("fireTriggerIds = %v", vals)
	}
	if live.Attributes[matomo.AttrTrackingType] != "pageview" {
		t.Fatalf("live trackingType = %v", live.Attributes[matomo.AttrTrackingType])
	}
}

func TestCreatePageviewTagCustomTitleAndURL(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	p := testTagProvider(t, srv)
	res := tagResource(t, "route_change", resource.Attributes{
		matomo.AttrType:          "matomoAnalytics",
		matomo.AttrTrackingType:  "pageview",
		matomo.AttrTrigger:       resolvedTrigger(t, "route_change", "12"),
		matomo.AttrDocumentTitle: "{{PageTitle}}",
		matomo.AttrCustomURL:     "{{PageOrigin}}/{{PageHash}}",
		matomo.AttrName:          "SPA route change",
	})

	if _, err := p.Create(context.Background(), res); err != nil {
		t.Fatalf("Create: %v", err)
	}
	vals := srv.lastCreateValues()
	if vals.Get("parameters[documentTitle]") != "{{PageTitle}}" {
		t.Fatalf("documentTitle = %v", vals)
	}
	if vals.Get("parameters[customUrl]") != "{{PageOrigin}}/{{PageHash}}" {
		t.Fatalf("customUrl = %v", vals)
	}
}

func TestCreatePageviewTagUsesManagedConfiguration(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedVariable(apiTagVariable{ID: 20, Name: "Site Config", Type: "MatomoConfiguration"})
	p := testTagProvider(t, srv)
	res := tagResource(t, "pageview", resource.Attributes{
		matomo.AttrType:         "matomoAnalytics",
		matomo.AttrTrackingType: "pageview",
		matomo.AttrTrigger:      resolvedTrigger(t, "pageview", "11"),
		matomo.AttrMatomoConfiguration: resource.Resolved{
			Address:  mustVariableAddress(t, "config"),
			Identity: resource.Identity{ID: "20"},
		},
	})

	if _, err := p.Create(context.Background(), res); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if srv.lastCreateValues().Get("parameters[matomoConfig]") != "{{Site Config}}" {
		t.Fatalf("matomoConfig = %v, want {{Site Config}}", srv.lastCreateValues())
	}
}

func TestPlanPageviewTagUnchanged(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 11, Name: "Pageview", Type: "PageView"})
	srv.seedTag(apiTag{
		ID:            21,
		Name:          "Pageview",
		Type:          "Matomo",
		FireTriggerID: 11,
		Parameters: map[string]any{
			"trackingType": "pageview",
			"matomoConfig": map[string]any{"name": "Matomo Configuration", "type": "MatomoConfiguration"},
		},
	})
	p := testTagProvider(t, srv)

	trigger := triggerResource(t, "pageview", resource.Attributes{
		matomo.AttrType: "pageView",
		matomo.AttrName: "Pageview",
	})
	tag := tagResource(t, "pageview", resource.Attributes{
		matomo.AttrType:         "matomoAnalytics",
		matomo.AttrTrackingType: "pageview",
		matomo.AttrTrigger:      triggerRef(t, "pageview"),
		matomo.AttrName:         "Pageview",
	})
	got := mustPlanTag(t, p, trigger, tag)
	if got.HasChanges() {
		t.Fatalf("equivalent pageview tag produced changes: %+v", got.Changes)
	}
}

func TestPlanPageviewTagUpdateCustomURL(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 12, Name: "History Change", Type: "HistoryChange"})
	srv.seedTag(apiTag{
		ID:            22,
		Name:          "SPA route change",
		Type:          "Matomo",
		FireTriggerID: 12,
		Parameters: map[string]any{
			"trackingType":  "pageview",
			"documentTitle": "{{PageTitle}}",
			"customUrl":     "{{PageUrl}}",
			"matomoConfig":  map[string]any{"name": "Matomo Configuration", "type": "MatomoConfiguration"},
		},
	})
	p := testTagProvider(t, srv)

	trigger := triggerResource(t, "route_change", resource.Attributes{
		matomo.AttrType: "historyChange",
		matomo.AttrName: "History Change",
	})
	tag := tagResource(t, "route_change", resource.Attributes{
		matomo.AttrType:          "matomoAnalytics",
		matomo.AttrTrackingType:  "pageview",
		matomo.AttrTrigger:       triggerRef(t, "route_change"),
		matomo.AttrName:          "SPA route change",
		matomo.AttrDocumentTitle: "{{PageTitle}}",
		matomo.AttrCustomURL:     "{{PageOrigin}}/{{PageHash}}",
	})
	got := mustPlanTag(t, p, trigger, tag)
	change := changeByAddr(t, got, "matomo.tag.route_change")
	if change.Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want update", change)
	}
	var urlDiff *plan.AttributeDiff
	for i := range change.Diffs {
		if change.Diffs[i].Path == matomo.AttrCustomURL {
			urlDiff = &change.Diffs[i]
		}
	}
	if urlDiff == nil || urlDiff.Before != "{{PageUrl}}" || urlDiff.After != "{{PageOrigin}}/{{PageHash}}" {
		t.Fatalf("customUrl diff = %+v", change.Diffs)
	}
}

func TestPlanSeparateInitialAndRouteChangeTagsAreNotDuplicates(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	p := testTagProvider(t, srv)

	pageTrigger := triggerResource(t, "pageview", resource.Attributes{matomo.AttrType: "pageView"})
	historyTrigger := triggerResource(t, "route_change", resource.Attributes{matomo.AttrType: "historyChange"})
	pageTag := tagResource(t, "pageview", validPageviewTagAttrs(t, "pageview"))
	routeTag := tagResource(t, "route_change", resource.Attributes{
		matomo.AttrType:         "matomoAnalytics",
		matomo.AttrTrackingType: "pageview",
		matomo.AttrTrigger:      triggerRef(t, "route_change"),
		matomo.AttrName:         "SPA route change",
	})

	got := mustPlanTag(t, p, pageTrigger, historyTrigger, pageTag, routeTag)
	if !hasAction(got, "matomo.trigger.pageview", plan.ActionCreate) {
		t.Fatalf("missing pageView trigger create: %+v", got.Changes)
	}
	if !hasAction(got, "matomo.trigger.route_change", plan.ActionCreate) {
		t.Fatalf("missing historyChange trigger create: %+v", got.Changes)
	}
	if !hasAction(got, "matomo.tag.pageview", plan.ActionCreate) {
		t.Fatalf("missing initial pageview tag create: %+v", got.Changes)
	}
	if !hasAction(got, "matomo.tag.route_change", plan.ActionCreate) {
		t.Fatalf("missing route-change pageview tag create: %+v", got.Changes)
	}
}

func TestReadPageviewTagUnsupportedTrackingType(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 11, Name: "Pageview", Type: "PageView"})
	srv.seedTag(apiTag{
		ID:            21,
		Name:          "Pageview",
		Type:          "Matomo",
		FireTriggerID: 11,
		Parameters: map[string]any{
			"trackingType": "goal",
			"idGoal":       "1",
			"matomoConfig": map[string]any{"name": "Matomo Configuration", "type": "MatomoConfiguration"},
		},
	})
	p := testTagProvider(t, srv)

	_, err := p.Read(context.Background(), tagResource(t, "pageview", resource.Attributes{
		matomo.AttrType:         "matomoAnalytics",
		matomo.AttrTrackingType: "pageview",
		matomo.AttrTrigger:      triggerRef(t, "pageview"),
		matomo.AttrName:         "Pageview",
	}))
	if err == nil {
		t.Fatal("expected unsupported trackingType error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported trackingType must not look like not found")
	}
	if !strings.Contains(err.Error(), "goal") {
		t.Fatalf("error = %q, want remote trackingType", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadPageviewTagMissingTrackingTypeDefaultsToPageview(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 11, Name: "Pageview", Type: "PageView"})
	srv.seedTag(apiTag{
		ID:            21,
		Name:          "Pageview",
		Type:          "Matomo",
		FireTriggerID: 11,
		Parameters: map[string]any{
			"matomoConfig": map[string]any{"name": "Matomo Configuration", "type": "MatomoConfiguration"},
		},
	})
	p := testTagProvider(t, srv)

	trigger := triggerResource(t, "pageview", resource.Attributes{
		matomo.AttrType: "pageView",
		matomo.AttrName: "Pageview",
	})
	if _, err := p.Read(context.Background(), trigger); err != nil {
		t.Fatalf("Read trigger: %v", err)
	}
	live, err := p.Read(context.Background(), tagResource(t, "pageview", resource.Attributes{
		matomo.AttrType:         "matomoAnalytics",
		matomo.AttrTrackingType: "pageview",
		matomo.AttrTrigger:      triggerRef(t, "pageview"),
		matomo.AttrName:         "Pageview",
	}))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Attributes[matomo.AttrTrackingType] != "pageview" {
		t.Fatalf("trackingType = %v, want pageview default", live.Attributes[matomo.AttrTrackingType])
	}
}

func TestImportPageviewTagReconstructsRefs(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedVariable(apiTagVariable{ID: 20, Name: "Site Config", Type: "MatomoConfiguration"})
	srv.seedTrigger(apiTagTrigger{ID: 11, Name: "Pageview", Type: "PageView"})
	srv.seedTag(apiTag{
		ID:            21,
		Name:          "Pageview",
		Type:          "Matomo",
		FireTriggerID: 11,
		Parameters: map[string]any{
			"trackingType":  "pageview",
			"documentTitle": "{{PageTitle}}",
			"customUrl":     "{{userId}}",
			"matomoConfig":  "Site Config",
		},
	})
	p := testTagProvider(t, srv)
	srv.seedVariable(apiTagVariable{ID: 30, Name: "userId", Type: "DataLayer", Key: "userId"})
	p.SetIdentityCatalog(boundIdentityCatalogs(t, map[string]string{
		"matomo.trigger.pageview": "11",
		"matomo.variable.config":  "20",
		"matomo.variable.user_id": "30",
	}))

	live, err := p.Import(context.Background(), mustTagAddress(t, "pageview"), "21")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if live.Attributes[matomo.AttrTrackingType] != "pageview" {
		t.Fatalf("trackingType = %v", live.Attributes[matomo.AttrTrackingType])
	}
	ref, ok := resource.AsRef(live.Attributes[matomo.AttrTrigger])
	if !ok || ref.Address.String() != "matomo.trigger.pageview" {
		t.Fatalf("trigger = %v, want $ref matomo.trigger.pageview", live.Attributes[matomo.AttrTrigger])
	}
	cfg, ok := resource.AsRef(live.Attributes[matomo.AttrMatomoConfiguration])
	if !ok || cfg.Address.String() != "matomo.variable.config" {
		t.Fatalf("matomoConfiguration = %v, want $ref matomo.variable.config", live.Attributes[matomo.AttrMatomoConfiguration])
	}
	if live.Attributes[matomo.AttrDocumentTitle] != "{{PageTitle}}" {
		t.Fatalf("unmanaged PageTitle template = %v", live.Attributes[matomo.AttrDocumentTitle])
	}
	customURL, ok := resource.AsRef(live.Attributes[matomo.AttrCustomURL])
	if !ok || customURL.Address.String() != "matomo.variable.user_id" {
		t.Fatalf("customUrl = %v, want reconstructed variable $ref", live.Attributes[matomo.AttrCustomURL])
	}
	if err := p.Validate(context.Background(), resource.Resource{Address: live.Address, Attributes: live.Attributes.Clone()}); err != nil {
		t.Fatalf("imported attributes must validate: %v", err)
	}
}

func TestApplyPageviewTagAfterTriggerThenDestroyTagFirst(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	p := testTagProvider(t, srv)
	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}

	pageTrigger := triggerResource(t, "pageview", resource.Attributes{matomo.AttrType: "pageView"})
	historyTrigger := triggerResource(t, "route_change", resource.Attributes{matomo.AttrType: "historyChange"})
	pageTag := tagResource(t, "pageview", validPageviewTagAttrs(t, "pageview"))
	routeTag := tagResource(t, "route_change", resource.Attributes{
		matomo.AttrType:         "matomoAnalytics",
		matomo.AttrTrackingType: "pageview",
		matomo.AttrTrigger:      triggerRef(t, "route_change"),
		matomo.AttrName:         "SPA route change",
	})
	resources := []resource.Resource{pageTag, routeTag, pageTrigger, historyTrigger}

	var out bytes.Buffer
	result, err := apply.Run(context.Background(), resources, func(resource.Address) (provider.Provider, error) {
		return p, nil
	}, st, &out)
	if err != nil {
		t.Fatalf("apply.Run: %v", err)
	}
	if result.Created != 4 {
		t.Fatalf("result = %+v, want 4 created", result)
	}
	progress := out.String()
	if strings.Index(progress, "matomo.trigger.pageview: created") > strings.Index(progress, "matomo.tag.pageview: created") {
		t.Fatalf("pageview tag created before its trigger:\n%s", progress)
	}
	if strings.Index(progress, "matomo.trigger.route_change: created") > strings.Index(progress, "matomo.tag.route_change: created") {
		t.Fatalf("route-change tag created before its trigger:\n%s", progress)
	}

	got := mustPlanTag(t, p, resources...)
	if got.HasChanges() {
		t.Fatalf("plan after apply produced changes: %+v", got.Changes)
	}

	var destroyOut bytes.Buffer
	destroyed, err := destroy.Run(context.Background(), resources, lookupMatomo(p), st, &destroyOut, nil)
	if err != nil {
		t.Fatalf("destroy.Run: %v", err)
	}
	if destroyed.Destroyed != 4 {
		t.Fatalf("destroy result = %+v, want 4 destroyed", destroyed)
	}
	destroyProgress := destroyOut.String()
	pageTagIdx := strings.Index(destroyProgress, "matomo.tag.pageview: destroyed")
	pageTrigIdx := strings.Index(destroyProgress, "matomo.trigger.pageview: destroyed")
	routeTagIdx := strings.Index(destroyProgress, "matomo.tag.route_change: destroyed")
	routeTrigIdx := strings.Index(destroyProgress, "matomo.trigger.route_change: destroyed")
	if pageTagIdx < 0 || pageTrigIdx < 0 || pageTagIdx > pageTrigIdx {
		t.Fatalf("pageview destroy order:\n%s", destroyProgress)
	}
	if routeTagIdx < 0 || routeTrigIdx < 0 || routeTagIdx > routeTrigIdx {
		t.Fatalf("route-change destroy order:\n%s", destroyProgress)
	}
}

func TestPlanFinalizationVisibleForPageviewTagCreate(t *testing.T) {
	t.Parallel()

	s := newFinalizeServer(t)
	p := newFinalizeProvider(t, s)
	if err := p.Configure(resource.Attributes{"publish": true, "environment": "live"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	planned, err := p.PlanFinalization(context.Background(), []provider.PendingChange{{
		Address: resource.Address{Provider: matomo.Name, Type: matomo.TypeTag, Name: "pageview"},
		Action:  "create",
	}})
	if err != nil {
		t.Fatalf("PlanFinalization: %v", err)
	}
	if planned == nil || planned.Action != "publish" {
		t.Fatalf("planned = %+v, want publish for pageview tag create", planned)
	}

	result, err := p.Finalize(context.Background(), *planned)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if !result.Changed {
		t.Fatalf("result = %+v, want changed", result)
	}

	replanned, err := p.PlanFinalization(context.Background(), nil)
	if err != nil {
		t.Fatalf("second PlanFinalization: %v", err)
	}
	if replanned != nil {
		t.Fatalf("second plan = %+v, want no duplicate version", replanned)
	}
}
