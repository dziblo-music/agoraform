package matomo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/graph"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo"
)

func userIDDataLayerAttrs() resource.Attributes {
	return resource.Attributes{
		matomo.AttrType: "dataLayer",
		matomo.AttrKey:  "userId",
		matomo.AttrName: "User ID",
	}
}

func TestCreateReadUpdateMatomoConfigurationUserID(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	p := testVariableProvider(t, srv)
	userID := variableResource(t, "user_id", userIDDataLayerAttrs())
	if _, err := p.Create(context.Background(), userID); err != nil {
		t.Fatalf("Create data layer: %v", err)
	}

	res := variableResource(t, "config", configVariableAttrs(resource.Attributes{
		matomo.AttrUserID: variableRef(t, "user_id"),
	}))
	live, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatalf("Create config: %v", err)
	}
	if srv.lastCreateValues().Get("parameters[userId]") != "{{User ID}}" {
		t.Fatalf("create userId = %v, want {{User ID}}", srv.lastCreateValues())
	}
	ref, ok := resource.AsRef(live.Attributes[matomo.AttrUserID])
	if !ok || ref.Address.String() != "matomo.variable.user_id" {
		t.Fatalf("created userId = %#v", live.Attributes[matomo.AttrUserID])
	}

	got, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	ref, ok = resource.AsRef(got.Attributes[matomo.AttrUserID])
	if !ok || ref.Address.String() != "matomo.variable.user_id" {
		t.Fatalf("read userId = %#v", got.Attributes[matomo.AttrUserID])
	}

	updated, err := p.Update(context.Background(), res, got)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if srv.lastUpdateValues().Get("parameters[userId]") != "{{User ID}}" {
		t.Fatalf("update userId = %v", srv.lastUpdateValues())
	}
	ref, ok = resource.AsRef(updated.Attributes[matomo.AttrUserID])
	if !ok || ref.Address.String() != "matomo.variable.user_id" {
		t.Fatalf("updated userId = %#v", updated.Attributes[matomo.AttrUserID])
	}
}

func TestCreateMatomoConfigurationUserIDRequiresBoundVariable(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	p := testVariableProvider(t, srv)
	_, err := p.Create(context.Background(), variableResource(t, "config", configVariableAttrs(resource.Attributes{
		matomo.AttrUserID: variableRef(t, "user_id"),
	})))
	if err == nil || !strings.Contains(err.Error(), "has no Tag Manager name") {
		t.Fatalf("Create = %v, want unresolved variable error", err)
	}
	if srv.createCount() != 0 {
		t.Fatalf("creates = %d, want 0", srv.createCount())
	}
}

func TestCreateMatomoConfigurationUserIDRejectsRemoteNonDataLayer(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{ID: 2, Name: "User ID", Type: "Cookie", Key: "userId"})
	p := testVariableProvider(t, srv)
	_, err := p.Create(context.Background(), variableResource(t, "config", configVariableAttrs(resource.Attributes{
		matomo.AttrUserID: resource.Resolved{
			Address:  mustVariableAddress(t, "user_id"),
			Identity: resource.Identity{ID: "2"},
		},
	})))
	if err == nil || !strings.Contains(err.Error(), "dataLayer") {
		t.Fatalf("Create = %v, want dataLayer type rejection", err)
	}
	if srv.createCount() != 0 {
		t.Fatalf("creates = %d, want 0", srv.createCount())
	}
}

func TestPlanMatomoConfigurationUserIDUnchangedEquivalentRemote(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{ID: 2, Name: "User ID", Type: "DataLayer", Key: "userId"})
	srv.seed(apiVariable{
		ID:   20,
		Name: "Matomo Configuration",
		Type: "MatomoConfiguration",
		Parameters: map[string]any{
			"matomoUrl":          "https://matomo.example.com",
			"idSite":             "1",
			"enableLinkTracking": true,
			"userId":             "{{User ID}}",
			"domains":            []any{"example.com"},
		},
	})
	p := testVariableProvider(t, srv)
	got := mustPlanVariables(t, p,
		variableResource(t, "user_id", userIDDataLayerAttrs()),
		variableResource(t, "config", configVariableAttrs(resource.Attributes{
			matomo.AttrEnableLinkTracking: true,
			matomo.AttrUserID:             variableRef(t, "user_id"),
		})),
	)
	change := changeByAddr(t, got, "matomo.variable.config")
	if change.Action != plan.ActionUnchanged {
		t.Fatalf("config change = %+v, want unchanged", change)
	}
}

func TestPlanMatomoConfigurationAnonymousUserIDUnchanged(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{
		ID:   20,
		Name: "Matomo Configuration",
		Type: "MatomoConfiguration",
		Parameters: map[string]any{
			"matomoUrl": "https://matomo.example.com",
			"idSite":    "1",
			"userId":    "",
		},
	})
	p := testVariableProvider(t, srv)
	got := mustPlanVariable(t, p, variableResource(t, "config", configVariableAttrs(nil)))
	if got.HasChanges() {
		t.Fatalf("anonymous remote produced changes: %+v", got.Changes)
	}
}

func TestPlanMatomoConfigurationMissingUserIDIsUpdate(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{ID: 2, Name: "User ID", Type: "DataLayer", Key: "userId"})
	srv.seed(apiVariable{
		ID:   20,
		Name: "Matomo Configuration",
		Type: "MatomoConfiguration",
		Parameters: map[string]any{
			"matomoUrl": "https://matomo.example.com",
			"idSite":    "1",
		},
	})
	p := testVariableProvider(t, srv)
	got := mustPlanVariables(t, p,
		variableResource(t, "user_id", userIDDataLayerAttrs()),
		variableResource(t, "config", configVariableAttrs(resource.Attributes{
			matomo.AttrUserID: variableRef(t, "user_id"),
		})),
	)
	change := changeByAddr(t, got, "matomo.variable.config")
	if change.Action != plan.ActionUpdate {
		t.Fatalf("config change = %+v, want update", change)
	}
}

func TestUpdateMatomoConfigurationUserIDPreservesUnmanagedParameters(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{ID: 2, Name: "User ID", Type: "DataLayer", Key: "userId"})
	srv.seed(apiVariable{
		ID:   20,
		Name: "Matomo Configuration",
		Type: "MatomoConfiguration",
		Parameters: map[string]any{
			"matomoUrl":                 "https://matomo.example.com",
			"idSite":                    "1",
			"enableLinkTracking":        true,
			"userId":                    "",
			"enableDoNotTrack":          true,
			"crossDomainLinkingTimeout": 180,
			"domains":                   []any{"example.com", "shop.example.com"},
		},
	})
	p := testVariableProvider(t, srv)
	if _, err := p.Read(context.Background(), variableResource(t, "user_id", userIDDataLayerAttrs())); err != nil {
		t.Fatalf("Read data layer: %v", err)
	}

	desired := variableResource(t, "config", configVariableAttrs(resource.Attributes{
		matomo.AttrEnableLinkTracking: false,
		matomo.AttrUserID:             variableRef(t, "user_id"),
	}))
	if _, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "20"},
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got := srv.lastUpdateValues()
	if got.Get("parameters[userId]") != "{{User ID}}" {
		t.Fatalf("managed userId not updated: %v", got)
	}
	if got.Get("parameters[enableLinkTracking]") != "0" {
		t.Fatalf("managed field not updated: %v", got)
	}
	if got.Get("parameters[enableDoNotTrack]") != "1" {
		t.Fatalf("unowned bool dropped: %v", got)
	}
	if got.Get("parameters[domains][0]") != "example.com" || got.Get("parameters[domains][1]") != "shop.example.com" {
		t.Fatalf("unowned array dropped: %v", got)
	}
}

func TestUpdateMatomoConfigurationPreservesUndeclaredUserID(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{
		ID:   20,
		Name: "Matomo Configuration",
		Type: "MatomoConfiguration",
		Parameters: map[string]any{
			"matomoUrl":          "https://matomo.example.com",
			"idSite":             "1",
			"enableLinkTracking": true,
			"userId":             "{{User ID}}",
			"domains":            []any{"example.com"},
		},
	})
	p := testVariableProvider(t, srv)
	desired := variableResource(t, "config", configVariableAttrs(resource.Attributes{
		matomo.AttrEnableLinkTracking: false,
	}))
	if _, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "20"},
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got := srv.lastUpdateValues()
	if got.Get("parameters[userId]") != "{{User ID}}" {
		t.Fatalf("undeclared userId not preserved: %v", got)
	}
	if got.Get("parameters[domains][0]") != "example.com" {
		t.Fatalf("unowned array dropped: %v", got)
	}
}

func TestImportMatomoConfigurationReconstructsUserIDRef(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{ID: 2, Name: "User ID", Type: "DataLayer", Key: "userId"})
	srv.seed(apiVariable{
		ID:   20,
		Name: "Matomo Configuration",
		Type: "MatomoConfiguration",
		Parameters: map[string]any{
			"matomoUrl": "https://matomo.example.com",
			"idSite":    "1",
			"userId":    "{{User ID}}",
			"domains":   []any{"example.com"},
		},
	})
	p := testVariableProvider(t, srv)
	p.SetIdentityCatalog(boundIdentityCatalogs(t, map[string]string{
		"matomo.variable.user_id": "2",
	}))

	live, err := p.Import(context.Background(), mustVariableAddress(t, "config"), "20")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	ref, ok := resource.AsRef(live.Attributes[matomo.AttrUserID])
	if !ok || ref.Address.String() != "matomo.variable.user_id" {
		t.Fatalf("imported userId = %#v", live.Attributes[matomo.AttrUserID])
	}
	if _, ok := live.Attributes["domains"]; ok {
		t.Fatal("unowned parameters must not appear in imported attributes")
	}
	if srv.createCount() != 0 {
		t.Fatalf("import mutated remote: creates=%d", srv.createCount())
	}

	res := resource.Resource{
		Address:    live.Address,
		Attributes: live.Attributes,
		Identity:   live.Identity,
	}
	got, err := plan.Build(context.Background(), []resource.Resource{
		variableResource(t, "user_id", userIDDataLayerAttrs()),
		res,
	}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err != nil {
		t.Fatalf("plan after import: %v", err)
	}
	change := changeByAddr(t, got, "matomo.variable.config")
	if change.Action != plan.ActionUnchanged {
		t.Fatalf("post-import plan = %+v", change)
	}
}

func TestImportMatomoConfigurationOmitsUnboundUserID(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{ID: 2, Name: "User ID", Type: "DataLayer", Key: "userId"})
	srv.seed(apiVariable{
		ID:   20,
		Name: "Matomo Configuration",
		Type: "MatomoConfiguration",
		Parameters: map[string]any{
			"matomoUrl": "https://matomo.example.com",
			"idSite":    "1",
			"userId":    "{{User ID}}",
		},
	})
	p := testVariableProvider(t, srv)
	live, err := p.Import(context.Background(), mustVariableAddress(t, "config"), "20")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if _, ok := live.Attributes[matomo.AttrUserID]; ok {
		t.Fatalf("unbound userId was guessed: %#v", live.Attributes[matomo.AttrUserID])
	}
}

func TestImportMatomoConfigurationOmitsAnonymousUserID(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{
		ID:   20,
		Name: "Matomo Configuration",
		Type: "MatomoConfiguration",
		Parameters: map[string]any{
			"matomoUrl": "https://matomo.example.com",
			"idSite":    "1",
			"userId":    "",
		},
	})
	p := testVariableProvider(t, srv)
	live, err := p.Import(context.Background(), mustVariableAddress(t, "config"), "20")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if _, ok := live.Attributes[matomo.AttrUserID]; ok {
		t.Fatalf("anonymous userId leaked into import: %#v", live.Attributes[matomo.AttrUserID])
	}
}

func TestImportMatomoConfigurationOmitsLiteralUserID(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{
		ID:   20,
		Name: "Matomo Configuration",
		Type: "MatomoConfiguration",
		Parameters: map[string]any{
			"matomoUrl": "https://matomo.example.com",
			"idSite":    "1",
			"userId":    "alice@example.com",
		},
	})
	p := testVariableProvider(t, srv)
	live, err := p.Import(context.Background(), mustVariableAddress(t, "config"), "20")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if _, ok := live.Attributes[matomo.AttrUserID]; ok {
		t.Fatalf("literal userId leaked into import: %#v", live.Attributes[matomo.AttrUserID])
	}
	for _, v := range live.Attributes {
		if s, ok := v.(string); ok && strings.Contains(s, "alice@example.com") {
			t.Fatal("imported attributes leaked a user identifier")
		}
	}
}

func TestImportMatomoConfigurationAmbiguousUserID(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{ID: 2, Name: "User ID", Type: "DataLayer", Key: "userId"})
	srv.seed(apiVariable{ID: 8, Name: "User ID", Type: "DataLayer", Key: "user.id"})
	srv.seed(apiVariable{
		ID:   20,
		Name: "Matomo Configuration",
		Type: "MatomoConfiguration",
		Parameters: map[string]any{
			"matomoUrl": "https://matomo.example.com",
			"idSite":    "1",
			"userId":    "{{User ID}}",
		},
	})
	p := testVariableProvider(t, srv)
	p.SetIdentityCatalog(boundIdentityCatalog(t, "matomo.variable.user_id", "2"))
	_, err := p.Import(context.Background(), mustVariableAddress(t, "config"), "20")
	if err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("Import = %v, want ambiguous relationship error", err)
	}
}

func TestImportMatomoConfigurationUserIDWrongRemoteType(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{ID: 2, Name: "User ID", Type: "Cookie", Key: "userId"})
	srv.seed(apiVariable{
		ID:   20,
		Name: "Matomo Configuration",
		Type: "MatomoConfiguration",
		Parameters: map[string]any{
			"matomoUrl": "https://matomo.example.com",
			"idSite":    "1",
			"userId":    "{{User ID}}",
		},
	})
	p := testVariableProvider(t, srv)
	p.SetIdentityCatalog(boundIdentityCatalog(t, "matomo.variable.user_id", "2"))
	_, err := p.Import(context.Background(), mustVariableAddress(t, "config"), "20")
	if err == nil || !strings.Contains(err.Error(), "dataLayer") {
		t.Fatalf("Import = %v, want dataLayer type error", err)
	}
}

func TestReadMatomoConfigurationMalformedUserID(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{
		ID:   20,
		Name: "Matomo Configuration",
		Type: "MatomoConfiguration",
		Parameters: map[string]any{
			"matomoUrl": "https://matomo.example.com",
			"idSite":    "1",
			"userId":    []any{"not-a-string"},
		},
	})
	p := testVariableProvider(t, srv)
	_, err := p.Read(context.Background(), variableResource(t, "config", configVariableAttrs(nil)))
	if err == nil || !strings.Contains(err.Error(), matomo.AttrUserID) {
		t.Fatalf("Read = %v, want unreadable userId", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestDestroyOrderConfigurationBeforeUserID(t *testing.T) {
	t.Parallel()

	g, err := graph.Build([]resource.Resource{
		variableResource(t, "user_id", userIDDataLayerAttrs()),
		variableResource(t, "config", configVariableAttrs(resource.Attributes{
			matomo.AttrUserID: variableRef(t, "user_id"),
		})),
	})
	if err != nil {
		t.Fatalf("graph.Build: %v", err)
	}
	order := g.ReverseOrder()
	if len(order) != 2 {
		t.Fatalf("destroy order = %v", order)
	}
	if order[0].String() != "matomo.variable.config" {
		t.Fatalf("first destroy = %s, want matomo.variable.config", order[0])
	}
	if order[1].String() != "matomo.variable.user_id" {
		t.Fatalf("second destroy = %s, want matomo.variable.user_id", order[1])
	}
}

func TestDestroyMatomoConfigurationWithUserID(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{ID: 2, Name: "User ID", Type: "DataLayer", Key: "userId"})
	srv.seed(apiVariable{
		ID:   20,
		Name: "Matomo Configuration",
		Type: "MatomoConfiguration",
		Parameters: map[string]any{
			"matomoUrl": "https://matomo.example.com",
			"idSite":    "1",
			"userId":    "{{User ID}}",
		},
	})
	p := testVariableProvider(t, srv)

	config := variableResource(t, "config", configVariableAttrs(resource.Attributes{
		matomo.AttrUserID: variableRef(t, "user_id"),
	}))
	config.Identity = resource.Identity{ID: "20"}
	if result, err := p.Destroy(context.Background(), config); err != nil || result.Status != provider.DestroyStatusDestroyed {
		t.Fatalf("config Destroy = (%v, %v)", result, err)
	}

	dataLayer := variableResource(t, "user_id", userIDDataLayerAttrs())
	dataLayer.Identity = resource.Identity{ID: "2"}
	if result, err := p.Destroy(context.Background(), dataLayer); err != nil || result.Status != provider.DestroyStatusDestroyed {
		t.Fatalf("data layer Destroy = (%v, %v)", result, err)
	}
}

func mustPlanVariables(t *testing.T, p *matomo.Provider, resources ...resource.Resource) *plan.Plan {
	t.Helper()
	got, err := plan.Build(context.Background(), resources, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	return got
}
