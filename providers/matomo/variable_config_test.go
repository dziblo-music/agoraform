package matomo_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo"
)

func configVariableAttrs(extra resource.Attributes) resource.Attributes {
	attrs := resource.Attributes{
		matomo.AttrType:      "matomoConfiguration",
		matomo.AttrName:      "Matomo Configuration",
		matomo.AttrMatomoURL: "https://matomo.example.com",
		matomo.AttrSiteID:    1,
	}
	for k, v := range extra {
		attrs[k] = v
	}
	return attrs
}

func TestValidateMatomoConfigurationVariableValid(t *testing.T) {
	t.Parallel()

	p := testVariableProvider(t, newVariableServer(t))
	cases := []struct {
		name  string
		attrs resource.Attributes
	}{
		{name: "required fields", attrs: configVariableAttrs(nil)},
		{
			name: "link tracking enabled",
			attrs: configVariableAttrs(resource.Attributes{
				matomo.AttrEnableLinkTracking: true,
			}),
		},
		{
			name: "site id as string",
			attrs: configVariableAttrs(resource.Attributes{
				matomo.AttrSiteID: "12",
			}),
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := p.Validate(context.Background(), variableResource(t, "config", tc.attrs)); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestValidateMatomoConfigurationVariableErrors(t *testing.T) {
	t.Parallel()

	p := testVariableProvider(t, newVariableServer(t))
	addr := mustVariableAddress(t, "config")
	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name:  "missing name",
			attrs: resource.Attributes{matomo.AttrType: "matomoConfiguration", matomo.AttrMatomoURL: "https://matomo.example.com", matomo.AttrSiteID: 1},
			want:  "missing required attribute \"name\"",
		},
		{
			name:  "missing matomoUrl",
			attrs: resource.Attributes{matomo.AttrType: "matomoConfiguration", matomo.AttrName: "Matomo Configuration", matomo.AttrSiteID: 1},
			want:  "missing required attribute \"matomoUrl\"",
		},
		{
			name:  "missing siteId",
			attrs: resource.Attributes{matomo.AttrType: "matomoConfiguration", matomo.AttrName: "Matomo Configuration", matomo.AttrMatomoURL: "https://matomo.example.com"},
			want:  "missing required attribute \"siteId\"",
		},
		{
			name:  "empty name",
			attrs: configVariableAttrs(resource.Attributes{matomo.AttrName: ""}),
			want:  "non-empty",
		},
		{
			name:  "relative url",
			attrs: configVariableAttrs(resource.Attributes{matomo.AttrMatomoURL: "/matomo"}),
			want:  "http or https URL",
		},
		{
			name:  "ftp url",
			attrs: configVariableAttrs(resource.Attributes{matomo.AttrMatomoURL: "ftp://matomo.example.com"}),
			want:  "http or https URL",
		},
		{
			name:  "credential url",
			attrs: configVariableAttrs(resource.Attributes{matomo.AttrMatomoURL: "https://user:secret-token@matomo.example.com"}),
			want:  "must not contain credentials",
		},
		{
			name:  "zero siteId",
			attrs: configVariableAttrs(resource.Attributes{matomo.AttrSiteID: 0}),
			want:  "positive site identifier",
		},
		{
			name:  "negative siteId",
			attrs: configVariableAttrs(resource.Attributes{matomo.AttrSiteID: -3}),
			want:  "positive site identifier",
		},
		{
			name:  "fractional siteId",
			attrs: configVariableAttrs(resource.Attributes{matomo.AttrSiteID: 1.5}),
			want:  "positive site identifier",
		},
		{
			name:  "key not used",
			attrs: configVariableAttrs(resource.Attributes{matomo.AttrKey: "userId"}),
			want:  "not used when",
		},
		{
			name: "dataLayer rejects config fields",
			attrs: resource.Attributes{
				matomo.AttrType:      "dataLayer",
				matomo.AttrKey:       "userId",
				matomo.AttrMatomoURL: "https://matomo.example.com",
			},
			want: "not used when",
		},
		{
			name:  "native type casing",
			attrs: resource.Attributes{matomo.AttrType: "MatomoConfiguration", matomo.AttrName: "Matomo Configuration", matomo.AttrMatomoURL: "https://matomo.example.com", matomo.AttrSiteID: 1},
			want:  "matomoConfiguration",
		},
		{
			name:  "enableLinkTracking not bool",
			attrs: configVariableAttrs(resource.Attributes{matomo.AttrEnableLinkTracking: "sometimes"}),
			want:  "boolean",
		},
		{
			name:  "open-ended parameter map",
			attrs: configVariableAttrs(resource.Attributes{"domains": []any{"example.com"}}),
			want:  "unsupported attribute",
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
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
			if !strings.Contains(err.Error(), addr.String()) {
				t.Fatalf("error = %q, want address", err.Error())
			}
			if strings.Contains(err.Error(), "secret-token") {
				t.Fatalf("error leaked secret: %q", err)
			}
			assertNoProviderSecret(t, err.Error())
		})
	}
}

func TestReadCreateUpdateMatomoConfigurationVariable(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	p := testVariableProvider(t, srv)
	res := variableResource(t, "config", configVariableAttrs(resource.Attributes{
		matomo.AttrEnableLinkTracking: true,
	}))

	if _, err := p.Read(context.Background(), res); !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read missing = %v, want ErrNotFound", err)
	}

	live, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID == "" {
		t.Fatal("create returned empty identity")
	}
	if srv.lastCreateValues().Get("type") != "MatomoConfiguration" {
		t.Fatalf("create type = %q", srv.lastCreateValues().Get("type"))
	}
	if srv.lastCreateValues().Get("name") != "Matomo Configuration" {
		t.Fatalf("create name = %q", srv.lastCreateValues().Get("name"))
	}
	if srv.lastCreateValues().Get("parameters[matomoUrl]") != "https://matomo.example.com" {
		t.Fatalf("create matomoUrl = %v", srv.lastCreateValues())
	}
	if srv.lastCreateValues().Get("parameters[idSite]") != "1" {
		t.Fatalf("create idSite = %v", srv.lastCreateValues())
	}
	if srv.lastCreateValues().Get("parameters[enableLinkTracking]") != "1" {
		t.Fatalf("create enableLinkTracking = %v", srv.lastCreateValues())
	}
	if srv.lastCreateValues().Get("parameters[dataLayerName]") != "" {
		t.Fatal("create must not send dataLayerName")
	}

	got, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read after create: %v", err)
	}
	if got.Identity.ID != live.Identity.ID {
		t.Fatalf("read id = %q, want %q", got.Identity.ID, live.Identity.ID)
	}
	if got.Attributes[matomo.AttrType] != "matomoConfiguration" {
		t.Fatalf("type = %v", got.Attributes[matomo.AttrType])
	}
	if got.Attributes[matomo.AttrMatomoURL] != "https://matomo.example.com" {
		t.Fatalf("matomoUrl = %v", got.Attributes[matomo.AttrMatomoURL])
	}

	updated, err := p.Update(context.Background(), variableResource(t, "config", configVariableAttrs(resource.Attributes{
		matomo.AttrEnableLinkTracking: false,
		matomo.AttrSiteID:             2,
	})), got)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Identity.ID != got.Identity.ID {
		t.Fatalf("update id = %q", updated.Identity.ID)
	}
	if srv.lastUpdateValues().Get("type") != "" {
		t.Fatal("update must not send type")
	}
	if srv.lastUpdateValues().Get("parameters[idSite]") != "2" {
		t.Fatalf("update idSite = %v", srv.lastUpdateValues())
	}
	if srv.lastUpdateValues().Get("parameters[enableLinkTracking]") != "0" {
		t.Fatalf("update enableLinkTracking = %v", srv.lastUpdateValues())
	}
}

func TestUpdateMatomoConfigurationPreservesUnmanagedParameters(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{
		ID:   20,
		Name: "Matomo Configuration",
		Type: "MatomoConfiguration",
		Parameters: map[string]any{
			"matomoUrl":                 "https://matomo.example.com",
			"idSite":                    "1",
			"enableLinkTracking":        true,
			"enableDoNotTrack":          true,
			"crossDomainLinkingTimeout": 180,
			"domains":                   []any{"example.com", "shop.example.com"},
			"customDimensions":          []any{map[string]any{"index": 1, "value": "member"}},
			"customData":                map[string]any{"source": "release-test"},
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
	if got.Get("parameters[enableLinkTracking]") != "0" {
		t.Fatalf("managed field not updated: %v", got)
	}
	if got.Get("parameters[enableDoNotTrack]") != "1" {
		t.Fatalf("unowned bool dropped: %v", got)
	}
	if got.Get("parameters[crossDomainLinkingTimeout]") != "180" {
		t.Fatalf("unowned number dropped: %v", got)
	}
	if got.Get("parameters[domains][0]") != "example.com" || got.Get("parameters[domains][1]") != "shop.example.com" {
		t.Fatalf("unowned array dropped: %v", got)
	}
	if got.Get("parameters[customDimensions][0][index]") != "1" || got.Get("parameters[customDimensions][0][value]") != "member" {
		t.Fatalf("unowned object array dropped: %v", got)
	}
	if got.Get("parameters[customData][source]") != "release-test" {
		t.Fatalf("unowned object dropped: %v", got)
	}
}

func TestPlanMatomoConfigurationUnchangedEquivalentRemote(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{
		ID:   20,
		Name: "Matomo Configuration",
		Type: "MatomoConfiguration",
		Parameters: map[string]any{
			"matomoUrl":          "https://matomo.example.com",
			"idSite":             json.Number("1"),
			"enableLinkTracking": "1",
			"domains":            []any{"example.com"},
		},
	})
	p := testVariableProvider(t, srv)
	res := variableResource(t, "config", configVariableAttrs(resource.Attributes{
		matomo.AttrEnableLinkTracking: true,
	}))
	got := mustPlanVariable(t, p, res)
	if got.HasChanges() {
		t.Fatalf("equivalent remote produced changes: %+v", got.Changes)
	}
}

func TestPlanMatomoConfigurationOmitsUndeclaredLinkTracking(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{
		ID:   20,
		Name: "Matomo Configuration",
		Type: "MatomoConfiguration",
		Parameters: map[string]any{
			"matomoUrl":          "https://matomo.example.com",
			"idSite":             1,
			"enableLinkTracking": false,
		},
	})
	p := testVariableProvider(t, srv)
	got := mustPlanVariable(t, p, variableResource(t, "config", configVariableAttrs(nil)))
	if got.HasChanges() {
		t.Fatalf("omitted optional field produced changes: %+v", got.Changes)
	}
}

func TestImportMatomoConfigurationVariable(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{
		ID:   20,
		Name: "Matomo Configuration",
		Type: "MatomoConfiguration",
		Parameters: map[string]any{
			"matomoUrl":          "https://matomo.example.com/",
			"idSite":             "3",
			"enableLinkTracking": true,
			"domains":            []any{"example.com"},
		},
	})
	p := testVariableProvider(t, srv)
	addr := mustVariableAddress(t, "config")
	live, err := p.Import(context.Background(), addr, "20")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if live.Identity.ID != "20" {
		t.Fatalf("identity = %q", live.Identity.ID)
	}
	if live.Attributes[matomo.AttrType] != "matomoConfiguration" {
		t.Fatalf("type = %v", live.Attributes[matomo.AttrType])
	}
	if live.Attributes[matomo.AttrName] != "Matomo Configuration" {
		t.Fatalf("name = %v", live.Attributes[matomo.AttrName])
	}
	if live.Attributes[matomo.AttrMatomoURL] != "https://matomo.example.com/" {
		t.Fatalf("matomoUrl = %v", live.Attributes[matomo.AttrMatomoURL])
	}
	if _, ok := live.Attributes["domains"]; ok {
		t.Fatal("unowned parameters must not appear in imported attributes")
	}
	if _, ok := live.Attributes["idvariable"]; ok {
		t.Fatal("imported attributes must omit computed identity")
	}
	if srv.createCount() != 0 {
		t.Fatalf("import mutated remote: creates=%d", srv.createCount())
	}

	res := resource.Resource{Address: addr, Attributes: live.Attributes, Identity: live.Identity}
	got, err := plan.Build(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err != nil {
		t.Fatalf("plan after import: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("post-import plan = %+v", got.Changes)
	}
}

func TestReadMatomoConfigurationMalformedEnableLinkTracking(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{
		ID:   20,
		Name: "Matomo Configuration",
		Type: "MatomoConfiguration",
		Parameters: map[string]any{
			"matomoUrl":          "https://matomo.example.com",
			"idSite":             "1",
			"enableLinkTracking": "sometimes",
		},
	})
	p := testVariableProvider(t, srv)
	_, err := p.Read(context.Background(), variableResource(t, "config", configVariableAttrs(nil)))
	if err == nil {
		t.Fatal("expected unreadable enableLinkTracking error")
	}
	if !strings.Contains(err.Error(), matomo.AttrEnableLinkTracking) {
		t.Fatalf("error = %q", err)
	}
	assertNoProviderSecret(t, err.Error())
}
