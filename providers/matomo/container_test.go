package matomo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

const testManagedContainerID = "Aa000001"

func TestValidateContainerValid(t *testing.T) {
	t.Parallel()

	p := testContainerProvider(t, newContainerServer(t))
	cases := []struct {
		name  string
		attrs resource.Attributes
	}{
		{
			name: "required fields",
			attrs: resource.Attributes{
				matomo.AttrName:    "Main Website",
				matomo.AttrContext: "web",
			},
		},
		{
			name: "optional description",
			attrs: resource.Attributes{
				matomo.AttrName:        "Main Website",
				matomo.AttrContext:     "web",
				matomo.AttrDescription: "primary container",
			},
		},
		{
			name: "android",
			attrs: resource.Attributes{
				matomo.AttrName:    "Android App",
				matomo.AttrContext: "android",
			},
		},
		{
			name: "ios",
			attrs: resource.Attributes{
				matomo.AttrName:    "iOS App",
				matomo.AttrContext: "ios",
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := containerResource(t, "main", tc.attrs)
			if err := p.Validate(context.Background(), res); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestValidateContainerErrors(t *testing.T) {
	t.Parallel()

	p := testContainerProvider(t, newContainerServer(t))
	addr := mustContainerAddress(t, "main")

	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name:  "missing name",
			attrs: resource.Attributes{matomo.AttrContext: "web"},
			want:  "missing required attribute \"name\"",
		},
		{
			name:  "missing context",
			attrs: resource.Attributes{matomo.AttrName: "Main Website"},
			want:  "missing required attribute \"context\"",
		},
		{
			name:  "empty name",
			attrs: resource.Attributes{matomo.AttrName: "", matomo.AttrContext: "web"},
			want:  "non-empty",
		},
		{
			name:  "unsupported context",
			attrs: resource.Attributes{matomo.AttrName: "Main Website", matomo.AttrContext: "amp"},
			want:  "context",
		},
		{
			name:  "computed idcontainer",
			attrs: resource.Attributes{matomo.AttrName: "Main Website", matomo.AttrContext: "web", "idcontainer": "Aa000001"},
			want:  "computed",
		},
		{
			name:  "manifest identity",
			attrs: resource.Attributes{matomo.AttrName: "Main Website", matomo.AttrContext: "web", matomo.AttrIDContainer: "Aa000001"},
			want:  "not configurable",
		},
		{
			name:  "unknown attribute",
			attrs: resource.Attributes{matomo.AttrName: "Main Website", matomo.AttrContext: "web", "ignoreGtmDataLayer": "1"},
			want:  "computed",
		},
		{
			name:  "unsupported field",
			attrs: resource.Attributes{matomo.AttrName: "Main Website", matomo.AttrContext: "web", "embedCode": "script"},
			want:  "unsupported attribute",
		},
		{
			name:  "leading whitespace name",
			attrs: resource.Attributes{matomo.AttrName: " Main Website", matomo.AttrContext: "web"},
			want:  `attribute "name" must not have leading or trailing whitespace`,
		},
		{
			name: "leading whitespace description",
			attrs: resource.Attributes{
				matomo.AttrName:        "Main Website",
				matomo.AttrContext:     "web",
				matomo.AttrDescription: " primary",
			},
			want: `attribute "description" must not have leading or trailing whitespace`,
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

func TestValidateContainerRequiresSiteID(t *testing.T) {
	t.Parallel()

	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:   "https://matomo.example.com",
		TokenAuth: providerToken,
	}, http.DefaultClient)
	err := p.Validate(context.Background(), containerResource(t, "main", resource.Attributes{
		matomo.AttrName:    "Main Website",
		matomo.AttrContext: "web",
	}))
	if err == nil {
		t.Fatal("expected site id error")
	}
	if !strings.Contains(err.Error(), matomo.EnvSiteID) {
		t.Fatalf("error = %q, want %s", err, matomo.EnvSiteID)
	}
}

func TestValidateContainerNameLength(t *testing.T) {
	t.Parallel()

	p := testContainerProvider(t, newContainerServer(t))
	tooLong := strings.Repeat("n", matomo.MaxContainerNameLen+1)
	err := p.Validate(context.Background(), containerResource(t, "main", resource.Attributes{
		matomo.AttrName:    tooLong,
		matomo.AttrContext: "web",
	}))
	if err == nil || !strings.Contains(err.Error(), "at most") {
		t.Fatalf("Validate = %v, want length error", err)
	}
}

func TestReadContainerSuccess(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	srv.seed(apiContainer{
		ID:          testManagedContainerID,
		Name:        "Main Website",
		Context:     "web",
		Description: "primary",
		Status:      "active",
		Version:     "9",
		IgnoreGtm:   "1",
	})
	p := testContainerProvider(t, srv)

	live, err := p.Read(context.Background(), containerResource(t, "main", resource.Attributes{
		matomo.AttrName:        "Main Website",
		matomo.AttrContext:     "web",
		matomo.AttrDescription: "primary",
	}))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != testManagedContainerID {
		t.Fatalf("identity = %q, want %s", live.Identity.ID, testManagedContainerID)
	}
	if live.Attributes[matomo.AttrName] != "Main Website" {
		t.Fatalf("name = %v", live.Attributes[matomo.AttrName])
	}
	if live.Attributes[matomo.AttrContext] != "web" {
		t.Fatalf("context = %v", live.Attributes[matomo.AttrContext])
	}
	if live.Computed["idcontainer"] != testManagedContainerID {
		t.Fatalf("computed idcontainer = %v", live.Computed["idcontainer"])
	}
	if live.Computed["ignoreGtmDataLayer"] != "1" {
		t.Fatalf("computed ignoreGtmDataLayer = %v", live.Computed["ignoreGtmDataLayer"])
	}
	if _, ok := live.Attributes["ignoreGtmDataLayer"]; ok {
		t.Fatal("unmanaged flags must not appear in comparable attributes")
	}
}

func TestReadContainerNotFound(t *testing.T) {
	t.Parallel()

	p := testContainerProvider(t, newContainerServer(t))
	_, err := p.Read(context.Background(), containerResource(t, "main", defaultContainerAttrs()))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadContainerDuplicateName(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	srv.seed(apiContainer{ID: "Aa000001", Name: "Main Website", Context: "web"})
	srv.seed(apiContainer{ID: "Bb000002", Name: "Main Website", Context: "web"})
	p := testContainerProvider(t, srv)

	_, err := p.Read(context.Background(), containerResource(t, "main", defaultContainerAttrs()))
	if err == nil {
		t.Fatal("expected duplicate name error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("duplicate names must not look like not found")
	}
	if !strings.Contains(err.Error(), "multiple remote containers") {
		t.Fatalf("error = %q", err)
	}
}

func TestReadContainerSkipsDeleted(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	srv.seed(apiContainer{ID: "Dd000009", Name: "Main Website", Context: "web", Status: "deleted"})
	p := testContainerProvider(t, srv)

	_, err := p.Read(context.Background(), containerResource(t, "main", defaultContainerAttrs()))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound for deleted container", err)
	}
}

func TestReadContainerAPIError(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	srv.fail("TagManager.getContainers", `{"result":"error","message":"Unable to authenticate with `+providerToken+`"}`)
	p := testContainerProvider(t, srv)

	_, err := p.Read(context.Background(), containerResource(t, "main", defaultContainerAttrs()))
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadContainerMalformedResponse(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	srv.malformed("TagManager.getContainers", `"oops `+providerToken+`"`)
	p := testContainerProvider(t, srv)

	_, err := p.Read(context.Background(), containerResource(t, "main", defaultContainerAttrs()))
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

func TestReadContainerStaleIdentity(t *testing.T) {
	t.Parallel()

	p := testContainerProvider(t, newContainerServer(t))
	res := containerResource(t, "main", defaultContainerAttrs())
	res.Identity = resource.Identity{ID: "Missing1"}

	_, err := p.Read(context.Background(), res)
	if !errors.Is(err, state.ErrStaleIdentity) {
		t.Fatalf("Read = %v, want ErrStaleIdentity", err)
	}
}

func TestCreateContainer(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	p := testContainerProvider(t, srv)
	res := containerResource(t, "main", resource.Attributes{
		matomo.AttrName:        "Main Website",
		matomo.AttrContext:     "web",
		matomo.AttrDescription: "primary",
	})

	live, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID == "" {
		t.Fatal("create returned empty identity")
	}
	if live.Attributes[matomo.AttrName] != "Main Website" {
		t.Fatalf("name = %v", live.Attributes[matomo.AttrName])
	}
	if live.Attributes[matomo.AttrContext] != "web" {
		t.Fatalf("context = %v", live.Attributes[matomo.AttrContext])
	}

	got, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read after create: %v", err)
	}
	if got.Identity.ID != live.Identity.ID {
		t.Fatalf("read id = %q, want %q", got.Identity.ID, live.Identity.ID)
	}
	create := srv.lastCreateValues()
	if create.Get("context") != "web" || create.Get("name") != "Main Website" || create.Get("description") != "primary" {
		t.Fatalf("create params = %v", create)
	}
	if create.Get("idContainer") != "" {
		t.Fatalf("create sent idContainer = %q", create.Get("idContainer"))
	}
}

func TestCreateContainerValidatesBeforeMutation(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	p := testContainerProvider(t, srv)
	_, err := p.Create(context.Background(), containerResource(t, "main", resource.Attributes{
		matomo.AttrName: "Main Website",
	}))
	if err == nil {
		t.Fatal("expected validation error")
	}
	if srv.createCount() != 0 {
		t.Fatalf("creates = %d, want 0", srv.createCount())
	}
}

func TestUpdateContainer(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	srv.seed(apiContainer{
		ID:          testManagedContainerID,
		Name:        "Main Website",
		Context:     "web",
		Description: "old",
		IgnoreGtm:   "1",
		SyncGtm:     "true",
	})
	p := testContainerProvider(t, srv)

	desired := containerResource(t, "main", resource.Attributes{
		matomo.AttrName:        "Renamed Website",
		matomo.AttrContext:     "web",
		matomo.AttrDescription: "updated",
	})
	live, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: testManagedContainerID},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if live.Identity.ID != testManagedContainerID {
		t.Fatalf("identity = %q", live.Identity.ID)
	}
	if live.Attributes[matomo.AttrName] != "Renamed Website" {
		t.Fatalf("name = %v", live.Attributes[matomo.AttrName])
	}
	if live.Attributes[matomo.AttrDescription] != "updated" {
		t.Fatalf("description = %v", live.Attributes[matomo.AttrDescription])
	}
	update := srv.lastUpdateValues()
	if update.Get("ignoreGtmDataLayer") != "1" {
		t.Fatalf("preserved ignoreGtmDataLayer = %q", update.Get("ignoreGtmDataLayer"))
	}
	if update.Get("activelySyncGtmDataLayer") != "1" {
		t.Fatalf("preserved activelySyncGtmDataLayer = %q", update.Get("activelySyncGtmDataLayer"))
	}
}

func TestUpdateContainerRejectsContextChange(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	srv.seed(apiContainer{ID: testManagedContainerID, Name: "Main Website", Context: "web"})
	p := testContainerProvider(t, srv)

	desired := containerResource(t, "main", resource.Attributes{
		matomo.AttrName:    "Main Website",
		matomo.AttrContext: "android",
	})
	_, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: testManagedContainerID},
	})
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("Update = %v, want immutable context", err)
	}
	if srv.updateCount() != 0 {
		t.Fatalf("updates = %d, want 0", srv.updateCount())
	}
}

func TestPlanContainerCreateWhenMissing(t *testing.T) {
	t.Parallel()

	p := testContainerProvider(t, newContainerServer(t))
	got := mustPlanContainer(t, p, containerResource(t, "main", defaultContainerAttrs()))
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionCreate {
		t.Fatalf("change = %+v, want create", got.Changes)
	}
}

func TestPlanContainerUnchangedEquivalentRemote(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	srv.seed(apiContainer{
		ID:          testManagedContainerID,
		Name:        "Main Website",
		Context:     "web",
		Description: "",
		Status:      "active",
		IgnoreGtm:   "1",
	})
	p := testContainerProvider(t, srv)

	res := containerResource(t, "main", resource.Attributes{
		matomo.AttrName:    "Main Website",
		matomo.AttrContext: "web",
	})
	got := mustPlanContainer(t, p, res)
	if got.HasChanges() {
		t.Fatalf("equivalent remote produced changes: %+v", got.Changes)
	}
	if got.Changes[0].Identity.ID != testManagedContainerID {
		t.Fatalf("identity = %q, want %s", got.Changes[0].Identity.ID, testManagedContainerID)
	}
}

func TestPlanContainerUpdateConfigurableField(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	srv.seed(apiContainer{ID: testManagedContainerID, Name: "Main Website", Context: "web", Description: "old"})
	p := testContainerProvider(t, srv)

	res := containerResource(t, "main", resource.Attributes{
		matomo.AttrName:        "Main Website",
		matomo.AttrContext:     "web",
		matomo.AttrDescription: "updated",
	})
	got := mustPlanContainer(t, p, res)
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want update", got.Changes)
	}
	var description *plan.AttributeDiff
	for i := range got.Changes[0].Diffs {
		if got.Changes[0].Diffs[i].Path == matomo.AttrDescription {
			description = &got.Changes[0].Diffs[i]
		}
	}
	if description == nil || description.Before != "old" || description.After != "updated" {
		t.Fatalf("description diff = %+v", got.Changes[0].Diffs)
	}
}

func TestPlanContainerIgnoresComputedFields(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	srv.seed(apiContainer{
		ID:        testManagedContainerID,
		Name:      "Main Website",
		Context:   "web",
		Status:    "active",
		Version:   "12",
		IgnoreGtm: "1",
		TagFire:   "true",
	})
	p := testContainerProvider(t, srv)

	got := mustPlanContainer(t, p, containerResource(t, "main", defaultContainerAttrs()))
	if got.HasChanges() {
		t.Fatalf("computed fields produced changes: %+v", got.Changes)
	}
}

func TestImportContainer(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	srv.seed(apiContainer{
		ID:          testManagedContainerID,
		Name:        "Main Website",
		Context:     "web",
		Description: "primary",
	})
	p := testContainerProvider(t, srv)
	live, err := p.Import(context.Background(), mustContainerAddress(t, "main"), testManagedContainerID)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if live.Identity.ID != testManagedContainerID {
		t.Fatalf("identity = %q", live.Identity.ID)
	}
	if live.Attributes[matomo.AttrName] != "Main Website" {
		t.Fatalf("name = %v", live.Attributes[matomo.AttrName])
	}
	if live.Attributes[matomo.AttrContext] != "web" {
		t.Fatalf("context = %v", live.Attributes[matomo.AttrContext])
	}
	if _, ok := live.Attributes[matomo.AttrIDContainer]; ok {
		t.Fatal("imported identity must not appear as a configurable attribute")
	}
	if srv.createCount() != 0 || srv.updateCount() != 0 {
		t.Fatalf("import mutated remote: creates=%d updates=%d", srv.createCount(), srv.updateCount())
	}
}

func TestImportContainerThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	srv.seed(apiContainer{ID: testManagedContainerID, Name: "Main Website", Context: "web", Description: "primary"})
	p := testContainerProvider(t, srv)

	live, err := p.Import(context.Background(), mustContainerAddress(t, "main"), testManagedContainerID)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	res := resource.Resource{Address: live.Address, Identity: live.Identity, Attributes: live.Attributes.Clone()}
	got := mustPlanContainer(t, p, res)
	if got.HasChanges() {
		t.Fatalf("plan after import produced changes: %+v", got.Changes)
	}
	if srv.createCount() != 0 || srv.updateCount() != 0 {
		t.Fatalf("import path mutated remote: creates=%d updates=%d", srv.createCount(), srv.updateCount())
	}
}

func TestImportContainerNotFound(t *testing.T) {
	t.Parallel()

	p := testContainerProvider(t, newContainerServer(t))
	_, err := p.Import(context.Background(), mustContainerAddress(t, "main"), "Missing1")
	if err == nil || !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Import = %v, want ErrNotFound", err)
	}
}

func TestImportContainerInvalidID(t *testing.T) {
	t.Parallel()

	p := testContainerProvider(t, newContainerServer(t))
	_, err := p.Import(context.Background(), mustContainerAddress(t, "main"), "bad-id")
	if err == nil || !strings.Contains(err.Error(), "valid Matomo container id") {
		t.Fatalf("Import = %v, want invalid id", err)
	}
}

func TestContainerResourceTypeRegistered(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{BaseURL: "https://matomo.example.com", TokenAuth: providerToken, SiteID: "1"})
	if !provider.Supports(p, matomo.TypeContainer) {
		t.Fatal("matomo.container must be registered")
	}
}

func TestManagedVariableUsesContainerIdentity(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	p := testContainerProvider(t, srv)

	container, err := p.Create(context.Background(), containerResource(t, "main", defaultContainerAttrs()))
	if err != nil {
		t.Fatalf("Create container: %v", err)
	}

	variable := variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType:      "dataLayer",
		matomo.AttrKey:       "userId",
		matomo.AttrContainer: resource.Ref{Address: container.Address},
	})
	live, err := p.Create(context.Background(), variable)
	if err != nil {
		t.Fatalf("Create variable: %v", err)
	}
	if live.Identity.ID == "" {
		t.Fatal("variable identity is empty")
	}
	if srv.lastVariableCreateContainer() != container.Identity.ID {
		t.Fatalf("variable idContainer = %q, want %q", srv.lastVariableCreateContainer(), container.Identity.ID)
	}
}

func TestReadVariableWithoutContainerIdentityIsNotFound(t *testing.T) {
	t.Parallel()

	p := testContainerProvider(t, newContainerServer(t))
	_, err := p.Read(context.Background(), variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType:      "dataLayer",
		matomo.AttrKey:       "userId",
		matomo.AttrContainer: resource.Ref{Address: mustContainerAddress(t, "main")},
	}))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound so plan can create after the container exists", err)
	}
}

func TestCreateVariableWithoutContainerIdentityFails(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	p := testContainerProvider(t, srv)
	_, err := p.Create(context.Background(), variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType:      "dataLayer",
		matomo.AttrKey:       "userId",
		matomo.AttrContainer: resource.Ref{Address: mustContainerAddress(t, "main")},
	}))
	if err == nil {
		t.Fatal("expected create to fail without container identity")
	}
	if srv.variableCreateCount() != 0 {
		t.Fatalf("variable creates = %d, want 0", srv.variableCreateCount())
	}
}

func TestApplyFailedContainerCreatePreventsChildMutations(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	srv.fail("TagManager.addContainer", `{"result":"error","message":"Unable to authenticate with `+providerToken+`"}`)
	p := testContainerProvider(t, srv)

	container := containerResource(t, "main", defaultContainerAttrs())
	variable := variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType:      "dataLayer",
		matomo.AttrKey:       "userId",
		matomo.AttrContainer: resource.Ref{Address: container.Address},
	})

	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}
	_, err = apply.Run(context.Background(), []resource.Resource{container, variable}, func(resource.Address) (provider.Provider, error) {
		return p, nil
	}, st, io.Discard)
	if err == nil {
		t.Fatal("expected apply to fail")
	}
	assertNoProviderSecret(t, err.Error())
	if srv.variableCreateCount() != 0 {
		t.Fatalf("child mutated after failed container create: %d", srv.variableCreateCount())
	}
}

func TestApplyManagedContainerThenChild(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	p := testContainerProvider(t, srv)

	container := containerResource(t, "main", defaultContainerAttrs())
	variable := variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType:      "dataLayer",
		matomo.AttrKey:       "userId",
		matomo.AttrContainer: resource.Ref{Address: container.Address},
	})

	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	result, err := apply.Run(context.Background(), []resource.Resource{container, variable}, func(resource.Address) (provider.Provider, error) {
		return p, nil
	}, st, &out)
	if err != nil {
		t.Fatalf("apply.Run: %v\n%s", err, out.String())
	}
	if result.Created != 2 {
		t.Fatalf("created = %d, want 2", result.Created)
	}
	id, ok, err := st.Identity(container.Address)
	if err != nil || !ok || id.ID == "" {
		t.Fatalf("container identity = (%v,%v,%v)", id, ok, err)
	}
	if srv.lastVariableCreateContainer() != id.ID {
		t.Fatalf("variable idContainer = %q, want bound container %q", srv.lastVariableCreateContainer(), id.ID)
	}
}

func TestImportVariableReconstructsContainerRef(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	srv.seed(apiContainer{ID: testManagedContainerID, Name: "Main Website", Context: "web"})
	srv.seedVariable(testManagedContainerID, apiContainerVariable{ID: 2, Name: "userId", Type: "DataLayer", Key: "userId"})
	p := testContainerProvider(t, srv)

	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustContainerAddress(t, "main"), resource.Identity{ID: testManagedContainerID}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	live, err := p.Import(context.Background(), mustVariableAddress(t, "user_id"), "2")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	ref, ok := resource.AsRef(live.Attributes[matomo.AttrContainer])
	if !ok || ref.Address.String() != "matomo.container.main" {
		t.Fatalf("container ref = %#v, want matomo.container.main", live.Attributes[matomo.AttrContainer])
	}
	if srv.createCount() != 0 || srv.variableCreateCount() != 0 {
		t.Fatal("import mutated remote")
	}
}

func TestExternalContainerModeStillUsesEnvID(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	srv.seed(apiContainer{ID: variableContainerID, Name: "Website", Context: "web"})
	srv.seedVariable(variableContainerID, apiContainerVariable{ID: 2, Name: "userId", Type: "DataLayer", Key: "userId"})
	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:     srv.server.URL,
		TokenAuth:   providerToken,
		SiteID:      "3",
		ContainerID: variableContainerID,
		HTTPClient:  srv.server.Client(),
	}, srv.server.Client())

	live, err := p.Read(context.Background(), variableResource(t, "user_id", resource.Attributes{
		matomo.AttrType: "dataLayer",
		matomo.AttrKey:  "userId",
	}))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "2" {
		t.Fatalf("identity = %q", live.Identity.ID)
	}
	if _, ok := live.Attributes[matomo.AttrContainer]; ok {
		t.Fatal("external mode must not emit a container $ref")
	}
	if srv.lastVariableReadContainer() != variableContainerID {
		t.Fatalf("idContainer = %q, want %s", srv.lastVariableReadContainer(), variableContainerID)
	}
}

type apiContainer struct {
	ID          string
	Name        string
	Context     string
	Description string
	Status      string
	Version     string
	IgnoreGtm   string
	TagFire     string
	SyncGtm     string
}

type apiContainerVariable struct {
	ID   int
	Name string
	Type string
	Key  string
}

type containerServer struct {
	mu                 sync.Mutex
	nextID             int
	containers         map[string]apiContainer
	variables          map[string]map[int]apiContainerVariable
	nextVariable       map[string]int
	fails              map[string]string
	creates            int
	updates            int
	variableCreates    int
	lastCreate         url.Values
	lastUpdate         url.Values
	lastVariableCreate url.Values
	lastReadContainer  string
	server             *httptest.Server
}

func newContainerServer(t *testing.T) *containerServer {
	t.Helper()
	s := &containerServer{
		nextID:       1,
		containers:   make(map[string]apiContainer),
		variables:    make(map[string]map[int]apiContainerVariable),
		nextVariable: make(map[string]int),
		fails:        make(map[string]string),
	}
	s.server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.server.Close)
	return s
}

func (s *containerServer) seed(c apiContainer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.ID == "" {
		c.ID = s.allocateIDLocked()
	}
	if c.Context == "" {
		c.Context = "web"
	}
	if c.Status == "" {
		c.Status = "active"
	}
	if c.Version == "" {
		c.Version = "9"
	}
	s.containers[c.ID] = c
}

func (s *containerServer) seedVariable(containerID string, v apiContainerVariable) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.variables[containerID] == nil {
		s.variables[containerID] = make(map[int]apiContainerVariable)
	}
	if v.ID == 0 {
		s.nextVariable[containerID]++
		v.ID = s.nextVariable[containerID]
	}
	if v.ID >= s.nextVariable[containerID] {
		s.nextVariable[containerID] = v.ID
	}
	if v.Type == "" {
		v.Type = "DataLayer"
	}
	s.variables[containerID][v.ID] = v
}

func (s *containerServer) fail(method, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fails[method] = body
}

func (s *containerServer) malformed(method, body string) {
	s.fail(method, body)
}

func (s *containerServer) serve(w http.ResponseWriter, r *http.Request) {
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
	case "TagManager.getContainers":
		s.writeContainers(w)
	case "TagManager.getContainer":
		s.writeContainer(w, vals.Get("idContainer"))
	case "TagManager.addContainer":
		s.addContainer(w, vals)
	case "TagManager.updateContainer":
		s.updateContainer(w, vals)
	case "TagManager.getContainerVariables":
		s.writeVariables(w, vals.Get("idContainer"))
	case "TagManager.addContainerVariable":
		s.addVariable(w, vals)
	case "TagManager.getAvailableEnvironmentsWithPublishCapability":
		_, _ = io.WriteString(w, `[{"id":"live","name":"Live"}]`)
	default:
		_, _ = io.WriteString(w, `{"result":"error","message":"unknown method"}`)
	}
}

func (s *containerServer) writeContainers(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]map[string]any, 0, len(s.containers))
	for _, c := range s.containers {
		out = append(out, s.containerJSON(c))
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *containerServer) writeContainer(w http.ResponseWriter, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.containers[id]
	if !ok || strings.EqualFold(c.Status, "deleted") {
		_, _ = io.WriteString(w, `false`)
		return
	}
	_ = json.NewEncoder(w).Encode(s.containerJSON(c))
}

func (s *containerServer) containerJSON(c apiContainer) map[string]any {
	return map[string]any{
		"idcontainer":                        c.ID,
		"idsite":                             3,
		"name":                               c.Name,
		"context":                            c.Context,
		"description":                        c.Description,
		"status":                             c.Status,
		"ignoreGtmDataLayer":                 c.IgnoreGtm,
		"isTagFireLimitAllowedInPreviewMode": c.TagFire,
		"activelySyncGtmDataLayer":           c.SyncGtm,
		"draft":                              map[string]any{"idcontainerversion": c.Version},
	}
}

func (s *containerServer) addContainer(w http.ResponseWriter, vals url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creates++
	s.lastCreate = vals
	id := s.allocateIDLocked()
	s.containers[id] = apiContainer{
		ID:          id,
		Name:        vals.Get("name"),
		Context:     vals.Get("context"),
		Description: vals.Get("description"),
		Status:      "active",
		Version:     "9",
	}
	_ = json.NewEncoder(w).Encode(id)
}

func (s *containerServer) updateContainer(w http.ResponseWriter, vals url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates++
	s.lastUpdate = vals
	id := vals.Get("idContainer")
	c, ok := s.containers[id]
	if !ok {
		_, _ = io.WriteString(w, `{"result":"error","message":"The requested container does not exist"}`)
		return
	}
	c.Name = vals.Get("name")
	c.Description = vals.Get("description")
	s.containers[id] = c
	_, _ = io.WriteString(w, `"ok"`)
}

func (s *containerServer) writeVariables(w http.ResponseWriter, containerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastReadContainer = containerID
	vars := s.variables[containerID]
	if len(vars) == 0 {
		_, _ = io.WriteString(w, `[]`)
		return
	}
	out := make([]map[string]any, 0, len(vars))
	for id, v := range vars {
		out = append(out, map[string]any{
			"idvariable":         strconv.Itoa(id),
			"idcontainerversion": "9",
			"idsite":             "3",
			"type":               v.Type,
			"name":               v.Name,
			"status":             "active",
			"parameters":         map[string]any{"dataLayerName": v.Key},
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *containerServer) addVariable(w http.ResponseWriter, vals url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.variableCreates++
	s.lastVariableCreate = vals
	containerID := vals.Get("idContainer")
	if s.variables[containerID] == nil {
		s.variables[containerID] = make(map[int]apiContainerVariable)
	}
	s.nextVariable[containerID]++
	id := s.nextVariable[containerID]
	s.variables[containerID][id] = apiContainerVariable{
		ID:   id,
		Name: vals.Get("name"),
		Type: vals.Get("type"),
		Key:  vals.Get("parameters[dataLayerName]"),
	}
	_, _ = io.WriteString(w, strconv.Itoa(id))
}

func (s *containerServer) allocateIDLocked() string {
	id := "Aa" + strconv.Itoa(1000000 + s.nextID)[1:]
	s.nextID++
	return id
}

func (s *containerServer) lastCreateValues() url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastCreate
}

func (s *containerServer) lastUpdateValues() url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastUpdate
}

func (s *containerServer) lastVariableCreateContainer() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastVariableCreate.Get("idContainer")
}

func (s *containerServer) lastVariableReadContainer() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastReadContainer
}

func (s *containerServer) createCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creates
}

func (s *containerServer) updateCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updates
}

func (s *containerServer) variableCreateCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.variableCreates
}

func testContainerProvider(t *testing.T, srv *containerServer) *matomo.Provider {
	t.Helper()
	return matomo.NewWithHTTPClient(client.Config{
		BaseURL:    srv.server.URL,
		TokenAuth:  providerToken,
		SiteID:     "3",
		HTTPClient: srv.server.Client(),
	}, srv.server.Client())
}

func containerResource(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{
		Address:    mustContainerAddress(t, name),
		Attributes: attrs,
	}
}

func mustContainerAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("matomo.container." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func defaultContainerAttrs() resource.Attributes {
	return resource.Attributes{
		matomo.AttrName:    "Main Website",
		matomo.AttrContext: "web",
	}
}

func mustPlanContainer(t *testing.T, p *matomo.Provider, resources ...resource.Resource) *plan.Plan {
	t.Helper()
	got, err := plan.Build(context.Background(), resources, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	return got
}
