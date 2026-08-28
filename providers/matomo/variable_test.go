package matomo_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

const variableContainerID = "6OMh6taM"

func TestValidateVariableValid(t *testing.T) {
	t.Parallel()

	p := testVariableProvider(t, newVariableServer(t))
	cases := []struct {
		name  string
		attrs resource.Attributes
	}{
		{
			name: "key only",
			attrs: resource.Attributes{
				matomo.AttrType: "dataLayer",
				matomo.AttrKey:  "userId",
			},
		},
		{
			name: "display name with internal space",
			attrs: resource.Attributes{
				matomo.AttrType: "dataLayer",
				matomo.AttrKey:  "userId",
				matomo.AttrName: "User ID",
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := variableResource(t, "user_id", tc.attrs)
			if err := p.Validate(context.Background(), res); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestValidateVariableErrors(t *testing.T) {
	t.Parallel()

	p := testVariableProvider(t, newVariableServer(t))
	addr := mustVariableAddress(t, "user_id")

	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name:  "missing type",
			attrs: resource.Attributes{matomo.AttrKey: "userId"},
			want:  "missing required attribute \"type\"",
		},
		{
			name:  "missing key",
			attrs: resource.Attributes{matomo.AttrType: "dataLayer"},
			want:  "missing required attribute \"key\"",
		},
		{
			name:  "empty key",
			attrs: resource.Attributes{matomo.AttrType: "dataLayer", matomo.AttrKey: ""},
			want:  "non-empty",
		},
		{
			name:  "unsupported type",
			attrs: resource.Attributes{matomo.AttrType: "cookie", matomo.AttrKey: "userId"},
			want:  "dataLayer",
		},
		{
			name:  "matomo native type casing",
			attrs: resource.Attributes{matomo.AttrType: "DataLayer", matomo.AttrKey: "userId"},
			want:  "dataLayer",
		},
		{
			name:  "computed idvariable",
			attrs: resource.Attributes{matomo.AttrType: "dataLayer", matomo.AttrKey: "userId", "idvariable": "1"},
			want:  "computed",
		},
		{
			name:  "manifest identity",
			attrs: resource.Attributes{matomo.AttrType: "dataLayer", matomo.AttrKey: "userId", matomo.AttrIDVariable: "1"},
			want:  "not configurable",
		},
		{
			name:  "unknown attribute",
			attrs: resource.Attributes{matomo.AttrType: "dataLayer", matomo.AttrKey: "userId", "selector": ".user"},
			want:  "unsupported attribute",
		},
		{
			name:  "empty name",
			attrs: resource.Attributes{matomo.AttrType: "dataLayer", matomo.AttrKey: "userId", matomo.AttrName: ""},
			want:  "non-empty",
		},
		{
			name:  "leading whitespace key",
			attrs: resource.Attributes{matomo.AttrType: "dataLayer", matomo.AttrKey: " userId"},
			want:  `attribute "key" must not have leading or trailing whitespace`,
		},
		{
			name:  "trailing whitespace key",
			attrs: resource.Attributes{matomo.AttrType: "dataLayer", matomo.AttrKey: "userId "},
			want:  `attribute "key" must not have leading or trailing whitespace`,
		},
		{
			name:  "whitespace only key",
			attrs: resource.Attributes{matomo.AttrType: "dataLayer", matomo.AttrKey: "   "},
			want:  `attribute "key" must not have leading or trailing whitespace`,
		},
		{
			name: "leading and trailing whitespace name",
			attrs: resource.Attributes{
				matomo.AttrType: "dataLayer",
				matomo.AttrKey:  "userId",
				matomo.AttrName: " User ID ",
			},
			want: `attribute "name" must not have leading or trailing whitespace`,
		},
		{
			name: "whitespace only name",
			attrs: resource.Attributes{
				matomo.AttrType: "dataLayer",
				matomo.AttrKey:  "userId",
				matomo.AttrName: "   ",
			},
			want: `attribute "name" must not have leading or trailing whitespace`,
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
			assertNoProviderSecret(t, err.Error())
		})
	}
}

func TestValidateVariableWhitespaceIsNotNormalized(t *testing.T) {
	t.Parallel()

	p := testVariableProvider(t, newVariableServer(t))
	res := variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "dataLayer",
		matomo.AttrKey:  " userId",
	})
	if err := p.Validate(context.Background(), res); err == nil {
		t.Fatal("expected validation error; leading whitespace must not be trimmed and accepted")
	}

	_, err := p.Create(context.Background(), res)
	if err == nil {
		t.Fatal("expected create to fail validation")
	}
	if !strings.Contains(err.Error(), "leading or trailing whitespace") {
		t.Fatalf("error = %q, want whitespace rejection", err)
	}
}

func TestValidateVariableEffectiveNameLength(t *testing.T) {
	t.Parallel()

	p := testVariableProvider(t, newVariableServer(t))
	key255 := strings.Repeat("k", matomo.MaxVariableNameLen)
	key256 := strings.Repeat("k", matomo.MaxVariableNameLen+1)
	key300 := strings.Repeat("k", matomo.MaxDataLayerKeyLen)
	name255 := strings.Repeat("n", matomo.MaxVariableNameLen)
	name256 := strings.Repeat("n", matomo.MaxVariableNameLen+1)

	cases := []struct {
		name    string
		attrs   resource.Attributes
		wantErr string
	}{
		{
			name: "key length 255 without name",
			attrs: resource.Attributes{
				matomo.AttrType: "dataLayer",
				matomo.AttrKey:  key255,
			},
		},
		{
			name: "key length 256 without name",
			attrs: resource.Attributes{
				matomo.AttrType: "dataLayer",
				matomo.AttrKey:  key256,
			},
			wantErr: matomo.AttrKey,
		},
		{
			name: "key length 300 with short name",
			attrs: resource.Attributes{
				matomo.AttrType: "dataLayer",
				matomo.AttrKey:  key300,
				matomo.AttrName: "User ID",
			},
		},
		{
			name: "explicit name length 256",
			attrs: resource.Attributes{
				matomo.AttrType: "dataLayer",
				matomo.AttrKey:  "userId",
				matomo.AttrName: name256,
			},
			wantErr: matomo.AttrName,
		},
		{
			name: "explicit name length 255",
			attrs: resource.Attributes{
				matomo.AttrType: "dataLayer",
				matomo.AttrKey:  "userId",
				matomo.AttrName: name255,
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := p.Validate(context.Background(), variableResource(t, "user_id", tc.attrs))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), "255") {
				t.Fatalf("error = %q, want 255-character limit", err)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want attribute %q", err, tc.wantErr)
			}
			if !strings.Contains(err.Error(), "matomo.variable.user_id") {
				t.Fatalf("error = %q, want address", err)
			}
			assertNoProviderSecret(t, err.Error())
		})
	}
}

func TestValidateVariableRequiresContainerID(t *testing.T) {
	t.Parallel()

	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:   "https://matomo.example.com",
		TokenAuth: providerToken,
		SiteID:    "3",
	}, http.DefaultClient)
	err := p.Validate(context.Background(), variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "dataLayer",
		matomo.AttrKey:  "userId",
	}))
	if err == nil {
		t.Fatal("expected container id error")
	}
	if !strings.Contains(err.Error(), matomo.EnvContainerID) {
		t.Fatalf("error = %q, want %s", err, matomo.EnvContainerID)
	}
}

func TestValidateVariableRequiresSiteID(t *testing.T) {
	t.Parallel()

	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:     "https://matomo.example.com",
		TokenAuth:   providerToken,
		ContainerID: variableContainerID,
	}, http.DefaultClient)
	err := p.Validate(context.Background(), variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "dataLayer",
		matomo.AttrKey:  "userId",
	}))
	if err == nil {
		t.Fatal("expected site id error")
	}
	if !strings.Contains(err.Error(), matomo.EnvSiteID) {
		t.Fatalf("error = %q, want %s", err, matomo.EnvSiteID)
	}
}

func TestValidateGoalDoesNotRequireContainerID(t *testing.T) {
	t.Parallel()

	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:   "https://matomo.example.com",
		TokenAuth: providerToken,
		SiteID:    "3",
	}, http.DefaultClient)
	err := p.Validate(context.Background(), goalResource(t, "trial_started", resource.Attributes{
		matomo.AttrName:           "Trial Started",
		matomo.AttrMatchAttribute: "manually",
	}))
	if err != nil {
		t.Fatalf("goal without container must remain valid: %v", err)
	}
}

func TestReadVariableSuccess(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{
		ID:           4,
		Name:         "userId",
		Type:         "DataLayer",
		Key:          "userId",
		Description:  "from data layer",
		DefaultValue: "",
		Status:       "active",
		Version:      "9",
		IDSite:       "3",
	})
	p := testVariableProvider(t, srv)

	live, err := p.Read(context.Background(), variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "dataLayer",
		matomo.AttrKey:  "userId",
	}))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "4" {
		t.Fatalf("identity = %q, want 4", live.Identity.ID)
	}
	if live.Attributes[matomo.AttrType] != "dataLayer" {
		t.Fatalf("type = %v", live.Attributes[matomo.AttrType])
	}
	if live.Attributes[matomo.AttrKey] != "userId" {
		t.Fatalf("key = %v", live.Attributes[matomo.AttrKey])
	}
	if live.Computed["idvariable"] != "4" {
		t.Fatalf("computed idvariable = %v", live.Computed["idvariable"])
	}
	if live.Computed["description"] != "from data layer" {
		t.Fatalf("computed description = %v", live.Computed["description"])
	}
	if _, ok := live.Attributes["description"]; ok {
		t.Fatal("description must not appear in comparable attributes")
	}
}

func TestReadVariableNotFound(t *testing.T) {
	t.Parallel()

	p := testVariableProvider(t, newVariableServer(t))
	_, err := p.Read(context.Background(), variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "dataLayer",
		matomo.AttrKey:  "userId",
	}))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadVariableDuplicateName(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{ID: 1, Name: "userId", Type: "DataLayer", Key: "userId"})
	srv.seed(apiVariable{ID: 8, Name: "userId", Type: "DataLayer", Key: "user.id"})
	p := testVariableProvider(t, srv)

	_, err := p.Read(context.Background(), variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "dataLayer",
		matomo.AttrKey:  "userId",
	}))
	if err == nil {
		t.Fatal("expected duplicate name error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("duplicate names must not look like not found")
	}
	if !strings.Contains(err.Error(), "multiple remote variables") {
		t.Fatalf("error = %q", err)
	}
}

func TestCreateVariable(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	p := testVariableProvider(t, srv)
	res := variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "dataLayer",
		matomo.AttrKey:  "userId",
	})

	live, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID == "" {
		t.Fatal("create returned empty identity")
	}
	if live.Attributes[matomo.AttrKey] != "userId" {
		t.Fatalf("key = %v", live.Attributes[matomo.AttrKey])
	}
	if srv.lastCreateValues().Get("type") != "DataLayer" {
		t.Fatalf("create type = %q, want DataLayer", srv.lastCreateValues().Get("type"))
	}
	if srv.lastCreateValues().Get("name") != "userId" {
		t.Fatalf("create name = %q, want defaulted key", srv.lastCreateValues().Get("name"))
	}
	if srv.lastCreateValues().Get("parameters[dataLayerName]") != "userId" {
		t.Fatalf("create parameters = %v", srv.lastCreateValues())
	}

	got, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read after create: %v", err)
	}
	if got.Identity.ID != live.Identity.ID {
		t.Fatalf("read id = %q, want %q", got.Identity.ID, live.Identity.ID)
	}
}

func TestUpdateVariable(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{
		ID:           8,
		Name:         "userId",
		Type:         "DataLayer",
		Key:          "oldKey",
		Description:  "keep me",
		DefaultValue: "anon",
	})
	p := testVariableProvider(t, srv)

	desired := variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "dataLayer",
		matomo.AttrKey:  "userId",
		matomo.AttrName: "User ID",
	})
	live, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "8"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if live.Identity.ID != "8" {
		t.Fatalf("identity = %q, want 8", live.Identity.ID)
	}
	if live.Attributes[matomo.AttrKey] != "userId" {
		t.Fatalf("key = %v", live.Attributes[matomo.AttrKey])
	}
	if live.Attributes[matomo.AttrName] != "User ID" {
		t.Fatalf("name = %v", live.Attributes[matomo.AttrName])
	}
	if srv.lastUpdateValues().Get("description") != "keep me" {
		t.Fatalf("update dropped description: %v", srv.lastUpdateValues())
	}
	if srv.lastUpdateValues().Get("defaultValue") != "anon" {
		t.Fatalf("update dropped defaultValue: %v", srv.lastUpdateValues())
	}
	if srv.lastUpdateValues().Get("type") != "" {
		t.Fatal("update must not send type")
	}
}

func TestPlanVariableCreateWhenMissing(t *testing.T) {
	t.Parallel()

	p := testVariableProvider(t, newVariableServer(t))
	res := variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "dataLayer",
		matomo.AttrKey:  "userId",
	})

	got := mustPlanVariable(t, p, res)
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionCreate {
		t.Fatalf("change = %+v, want create", got.Changes)
	}
}

func TestPlanVariableUnchangedEquivalentRemote(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{
		ID:           2,
		Name:         "userId",
		Type:         "DataLayer",
		Key:          "userId",
		Description:  "ignored",
		DefaultValue: "",
		Status:       "active",
		Version:      "9",
		IDSite:       "3",
	})
	p := testVariableProvider(t, srv)

	res := variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "dataLayer",
		matomo.AttrKey:  "userId",
	})
	got := mustPlanVariable(t, p, res)
	if got.HasChanges() {
		t.Fatalf("equivalent remote produced changes: %+v", got.Changes)
	}
	if got.Changes[0].Identity.ID != "2" {
		t.Fatalf("identity = %q, want 2", got.Changes[0].Identity.ID)
	}
}

func TestPlanVariableUnchangedWithDisplayName(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{
		ID:     2,
		Name:   "User ID",
		Type:   "DataLayer",
		Key:    "userId",
		Status: "active",
	})
	p := testVariableProvider(t, srv)

	res := variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "dataLayer",
		matomo.AttrKey:  "userId",
		matomo.AttrName: "User ID",
	})
	got := mustPlanVariable(t, p, res)
	if got.HasChanges() {
		t.Fatalf("equivalent display name produced changes: %+v", got.Changes)
	}
}

func TestPlanVariableUpdateConfigurableField(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{ID: 2, Name: "userId", Type: "DataLayer", Key: "old"})
	p := testVariableProvider(t, srv)

	res := variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "dataLayer",
		matomo.AttrKey:  "userId",
	})
	got := mustPlanVariable(t, p, res)
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want update", got.Changes)
	}
	var keyDiff *plan.AttributeDiff
	for i := range got.Changes[0].Diffs {
		if got.Changes[0].Diffs[i].Path == matomo.AttrKey {
			keyDiff = &got.Changes[0].Diffs[i]
		}
	}
	if keyDiff == nil || keyDiff.Before != "old" || keyDiff.After != "userId" {
		t.Fatalf("key diff = %+v", got.Changes[0].Diffs)
	}
}

func TestPlanVariableIgnoresComputedFields(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{
		ID:           5,
		Name:         "userId",
		Type:         "DataLayer",
		Key:          "userId",
		Description:  "computed",
		DefaultValue: "none",
		LookupTable:  `[{"comparison":"equals","match_value":"a","out_value":"b"}]`,
		Status:       "active",
		Version:      "9",
		IDSite:       "3",
	})
	p := testVariableProvider(t, srv)

	res := variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "dataLayer",
		matomo.AttrKey:  "userId",
	})
	got := mustPlanVariable(t, p, res)
	if got.HasChanges() {
		t.Fatalf("computed fields produced changes: %+v", got.Changes)
	}
}

func TestReadVariableAPIError(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.fail("TagManager.getContainerVariables", `{"result":"error","message":"Unable to authenticate with `+providerToken+`"}`)
	p := testVariableProvider(t, srv)

	_, err := p.Read(context.Background(), variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "dataLayer",
		matomo.AttrKey:  "userId",
	}))
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	if !strings.Contains(err.Error(), "authenticate") && !strings.Contains(err.Error(), "TagManager.getContainerVariables") {
		t.Fatalf("error = %q, want API diagnostic", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadVariableMalformedResponse(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.malformed("TagManager.getContainerVariables", `"oops `+providerToken+`"`)
	p := testVariableProvider(t, srv)

	_, err := p.Read(context.Background(), variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "dataLayer",
		matomo.AttrKey:  "userId",
	}))
	if err == nil {
		t.Fatal("expected malformed error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("malformed response must not be ErrNotFound")
	}
	if !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("error = %q, want malformed", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestCreateVariableValidatesBeforeMutation(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	p := testVariableProvider(t, srv)
	_, err := p.Create(context.Background(), variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "dataLayer",
	}))
	if err == nil {
		t.Fatal("expected validation error")
	}
	if srv.createCount() != 0 {
		t.Fatalf("creates = %d, want 0", srv.createCount())
	}
}

func TestImportVariable(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{ID: 1, Name: "userId", Type: "DataLayer", Key: "userId"})
	p := testVariableProvider(t, srv)
	addr := mustVariableAddress(t, "user_id")
	live, err := p.Import(context.Background(), addr, "1")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if live.Identity.ID != "1" {
		t.Fatalf("identity = %q, want 1", live.Identity.ID)
	}
	if live.Attributes[matomo.AttrKey] != "userId" {
		t.Fatalf("key = %v", live.Attributes[matomo.AttrKey])
	}
	if _, ok := live.Attributes["idvariable"]; ok {
		t.Fatal("imported attributes must omit computed identity")
	}
}

func TestReadVariableUsesBoundIDInsteadOfNameDiscovery(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{ID: 12, Name: "userId", Type: "DataLayer", Key: "userId"})
	srv.seed(apiVariable{ID: 99, Name: "other", Type: "DataLayer", Key: "other"})
	p := testVariableProvider(t, srv)

	res := variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "dataLayer",
		matomo.AttrKey:  "other",
		matomo.AttrName: "other",
	})
	res.Identity = resource.Identity{ID: "12"}

	live, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "12" {
		t.Fatalf("identity = %q, want bound 12", live.Identity.ID)
	}
}

func TestReadVariableStaleIdentity(t *testing.T) {
	t.Parallel()

	p := testVariableProvider(t, newVariableServer(t))
	res := variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "dataLayer",
		matomo.AttrKey:  "userId",
	})
	res.Identity = resource.Identity{ID: "12"}

	_, err := p.Read(context.Background(), res)
	if !errors.Is(err, state.ErrStaleIdentity) {
		t.Fatalf("Read = %v, want ErrStaleIdentity", err)
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("stale identity must not look like a create candidate")
	}
}

func TestReadVariableUnsupportedRemoteType(t *testing.T) {
	t.Parallel()

	srv := newVariableServer(t)
	srv.seed(apiVariable{ID: 3, Name: "userId", Type: "Cookie", Key: "userId"})
	p := testVariableProvider(t, srv)

	_, err := p.Read(context.Background(), variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "dataLayer",
		matomo.AttrKey:  "userId",
	}))
	if err == nil {
		t.Fatal("expected unsupported type error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported remote type must not look like not found")
	}
	if !strings.Contains(err.Error(), "Cookie") {
		t.Fatalf("error = %q, want remote type", err)
	}
}

func TestVariableManifestCompatibility(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "agoraform.yaml")
	contents := `apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.goal.trial_started
    attributes:
      name: Trial Started
      matchAttribute: event_action
      pattern: trialStarted
  - address: matomo.variable.user_id
    attributes:
      type: dataLayer
      key: userId
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	m, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if len(m.Resources) != 2 {
		t.Fatalf("len(resources) = %d, want 2", len(m.Resources))
	}
	if m.Resources[0].Address.String() != "matomo.goal.trial_started" {
		t.Fatalf("goal address = %s", m.Resources[0].Address)
	}
	if m.Resources[1].Address.String() != "matomo.variable.user_id" {
		t.Fatalf("variable address = %s", m.Resources[1].Address)
	}
	if m.Resources[1].Attributes[matomo.AttrType] != "dataLayer" {
		t.Fatalf("variable type = %v", m.Resources[1].Attributes[matomo.AttrType])
	}

	srv := newVariableServer(t)
	p := testVariableProvider(t, srv)
	if err := p.Validate(context.Background(), m.Resources[0]); err != nil {
		t.Fatalf("existing goal resource must remain valid: %v", err)
	}
	if err := p.Validate(context.Background(), m.Resources[1]); err != nil {
		t.Fatalf("variable resource: %v", err)
	}
}

func TestVariableResourceTypesRegistered(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{BaseURL: "https://matomo.example.com", TokenAuth: providerToken, SiteID: "1"})
	if !provider.Supports(p, matomo.TypeVariable) {
		t.Fatal("matomo.variable must be registered")
	}
	if !provider.Supports(p, matomo.TypeGoal) {
		t.Fatal("matomo.goal must remain registered")
	}
}

type apiVariable struct {
	ID           int
	Name         string
	Type         string
	Key          string
	Description  string
	DefaultValue string
	LookupTable  string
	Status       string
	Version      string
	IDSite       string
	Parameters   map[string]any
}

type variableServer struct {
	mu         sync.Mutex
	nextID     int
	version    string
	variables  map[int]apiVariable
	fails      map[string]string
	creates    int
	updates    int
	lastCreate url.Values
	lastUpdate url.Values
	server     *httptest.Server
}

func newVariableServer(t *testing.T) *variableServer {
	t.Helper()
	s := &variableServer{
		nextID:    1,
		version:   "9",
		variables: make(map[int]apiVariable),
		fails:     make(map[string]string),
	}
	s.server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.server.Close)
	return s
}

func (s *variableServer) seed(v apiVariable) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.ID == 0 {
		v.ID = s.nextID
		s.nextID++
	}
	if v.ID >= s.nextID {
		s.nextID = v.ID + 1
	}
	if v.Type == "" {
		v.Type = "DataLayer"
	}
	if v.Status == "" {
		v.Status = "active"
	}
	if v.Version == "" {
		v.Version = s.version
	}
	if v.IDSite == "" {
		v.IDSite = "3"
	}
	s.variables[v.ID] = v
}

func (s *variableServer) fail(method, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fails[method] = body
}

func (s *variableServer) malformed(method, body string) {
	s.fail(method, body)
}

func (s *variableServer) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	vals, err := url.ParseQuery(string(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	method := vals.Get("method")

	s.mu.Lock()
	failBody, fail := s.fails[method]
	s.mu.Unlock()
	if fail {
		_, _ = io.WriteString(w, failBody)
		return
	}

	switch method {
	case "API.getMatomoVersion":
		_, _ = io.WriteString(w, `"5.2.0"`)
	case "TagManager.getContainer":
		s.writeContainer(w)
	case "TagManager.getContainerVariables":
		s.writeVariables(w)
	case "TagManager.addContainerVariable":
		s.addVariable(w, vals)
	case "TagManager.updateContainerVariable":
		s.updateVariable(w, vals)
	default:
		_, _ = io.WriteString(w, `{"result":"error","message":"unknown method"}`)
	}
}

func (s *variableServer) writeContainer(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = io.WriteString(w, `{"idcontainer":"`+variableContainerID+`","idsite":3,"name":"Website","draft":{"idcontainerversion":`+s.version+`}}`)
}

func (s *variableServer) writeVariables(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.variables) == 0 {
		_, _ = io.WriteString(w, `[]`)
		return
	}
	out := make([]map[string]any, 0, len(s.variables))
	for id, v := range s.variables {
		params := v.Parameters
		if params == nil {
			params = map[string]any{}
			if v.Key != "" {
				params["dataLayerName"] = v.Key
			}
		}
		item := map[string]any{
			"idvariable":         strconv.Itoa(id),
			"idcontainerversion": v.Version,
			"idsite":             v.IDSite,
			"type":               v.Type,
			"name":               v.Name,
			"status":             v.Status,
			"description":        v.Description,
			"default_value":      v.DefaultValue,
			"parameters":         params,
		}
		if v.LookupTable != "" {
			var table any
			if err := json.Unmarshal([]byte(v.LookupTable), &table); err == nil {
				item["lookup_table"] = table
			} else {
				item["lookup_table"] = []any{}
			}
		} else {
			item["lookup_table"] = []any{}
		}
		out = append(out, item)
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *variableServer) addVariable(w http.ResponseWriter, vals url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creates++
	s.lastCreate = vals
	id := s.nextID
	s.nextID++
	s.variables[id] = apiVariable{
		ID:         id,
		Name:       vals.Get("name"),
		Type:       vals.Get("type"),
		Key:        vals.Get("parameters[dataLayerName]"),
		Status:     "active",
		Version:    s.version,
		IDSite:     "3",
		Parameters: formVariableParameters(vals),
	}
	_, _ = io.WriteString(w, strconv.Itoa(id))
}

func (s *variableServer) updateVariable(w http.ResponseWriter, vals url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates++
	s.lastUpdate = vals
	id, err := strconv.Atoi(vals.Get("idVariable"))
	if err != nil {
		_, _ = io.WriteString(w, `{"result":"error","message":"invalid idVariable"}`)
		return
	}
	v, ok := s.variables[id]
	if !ok {
		_, _ = io.WriteString(w, `{"result":"error","message":"variable not found"}`)
		return
	}
	v.Name = vals.Get("name")
	if key := vals.Get("parameters[dataLayerName]"); key != "" {
		v.Key = key
	}
	v.Parameters = mergeFormVariableParameters(v.Parameters, vals)
	if desc := vals.Get("description"); desc != "" || vals.Has("description") {
		v.Description = desc
	}
	if def := vals.Get("defaultValue"); def != "" || vals.Has("defaultValue") {
		v.DefaultValue = def
	}
	s.variables[id] = v
	_, _ = io.WriteString(w, `null`)
}

func (s *variableServer) lastCreateValues() url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastCreate
}

func (s *variableServer) lastUpdateValues() url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastUpdate
}

func formVariableParameters(vals url.Values) map[string]any {
	out := map[string]any{}
	for key, values := range vals {
		name, rest, ok := parseFormParamKey(key)
		if !ok || len(values) == 0 {
			continue
		}
		setNestedParam(out, append([]string{name}, rest...), values[0])
	}
	return out
}

func mergeFormVariableParameters(existing map[string]any, vals url.Values) map[string]any {
	out := map[string]any{}
	for k, v := range existing {
		out[k] = v
	}
	for k, v := range formVariableParameters(vals) {
		out[k] = v
	}
	return out
}

func parseFormParamKey(key string) (string, []string, bool) {
	const prefix = "parameters["
	if !strings.HasPrefix(key, prefix) || !strings.HasSuffix(key, "]") {
		return "", nil, false
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(key, prefix), "]")
	parts := strings.Split(inner, "][")
	if len(parts) == 0 || parts[0] == "" {
		return "", nil, false
	}
	return parts[0], parts[1:], true
}

func setNestedParam(out map[string]any, path []string, value string) {
	if len(path) == 0 {
		return
	}
	key := path[0]
	if len(path) == 1 {
		out[key] = value
		return
	}
	child, _ := out[key].(map[string]any)
	if child == nil {
		child = map[string]any{}
		out[key] = child
	}
	setNestedParam(child, path[1:], value)
}

func (s *variableServer) createCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creates
}

func testVariableProvider(t *testing.T, srv *variableServer) *matomo.Provider {
	t.Helper()
	return matomo.NewWithHTTPClient(client.Config{
		BaseURL:     srv.server.URL,
		TokenAuth:   providerToken,
		SiteID:      "3",
		ContainerID: variableContainerID,
		HTTPClient:  srv.server.Client(),
	}, srv.server.Client())
}

func variableResource(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{
		Address:    mustVariableAddress(t, name),
		Attributes: attrs,
	}
}

func mustVariableAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("matomo.variable." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func mustPlanVariable(t *testing.T, p *matomo.Provider, res resource.Resource) *plan.Plan {
	t.Helper()
	got, err := plan.Build(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	return got
}
