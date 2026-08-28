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

const triggerContainerID = "6OMh6taM"

func TestValidateTriggerValid(t *testing.T) {
	t.Parallel()

	p := testTriggerProvider(t, newTriggerServer(t))
	cases := []struct {
		name  string
		attrs resource.Attributes
	}{
		{
			name: "event only",
			attrs: resource.Attributes{
				matomo.AttrType:  "customEvent",
				matomo.AttrEvent: "trialStarted",
			},
		},
		{
			name: "display name with internal space",
			attrs: resource.Attributes{
				matomo.AttrType:  "customEvent",
				matomo.AttrEvent: "trialStarted",
				matomo.AttrName:  "Trial Started",
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := triggerResource(t, "trial_started", tc.attrs)
			if err := p.Validate(context.Background(), res); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestValidateTriggerErrors(t *testing.T) {
	t.Parallel()

	p := testTriggerProvider(t, newTriggerServer(t))
	addr := mustTriggerAddress(t, "trial_started")

	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name:  "missing type",
			attrs: resource.Attributes{matomo.AttrEvent: "trialStarted"},
			want:  "missing required attribute \"type\"",
		},
		{
			name:  "missing event",
			attrs: resource.Attributes{matomo.AttrType: "customEvent"},
			want:  "missing required attribute \"event\"",
		},
		{
			name:  "empty event",
			attrs: resource.Attributes{matomo.AttrType: "customEvent", matomo.AttrEvent: ""},
			want:  "non-empty",
		},
		{
			name:  "unsupported type",
			attrs: resource.Attributes{matomo.AttrType: "pageView", matomo.AttrEvent: "trialStarted"},
			want:  "customEvent",
		},
		{
			name:  "matomo native type casing",
			attrs: resource.Attributes{matomo.AttrType: "CustomEvent", matomo.AttrEvent: "trialStarted"},
			want:  "customEvent",
		},
		{
			name:  "computed idtrigger",
			attrs: resource.Attributes{matomo.AttrType: "customEvent", matomo.AttrEvent: "trialStarted", "idtrigger": "1"},
			want:  "computed",
		},
		{
			name:  "manifest identity",
			attrs: resource.Attributes{matomo.AttrType: "customEvent", matomo.AttrEvent: "trialStarted", matomo.AttrIDTrigger: "1"},
			want:  "not configurable",
		},
		{
			name:  "computed conditions",
			attrs: resource.Attributes{matomo.AttrType: "customEvent", matomo.AttrEvent: "trialStarted", "conditions": "[]"},
			want:  "computed",
		},
		{
			name:  "unknown attribute",
			attrs: resource.Attributes{matomo.AttrType: "customEvent", matomo.AttrEvent: "trialStarted", "delay": "100"},
			want:  "unsupported attribute",
		},
		{
			name:  "empty name",
			attrs: resource.Attributes{matomo.AttrType: "customEvent", matomo.AttrEvent: "trialStarted", matomo.AttrName: ""},
			want:  "non-empty",
		},
		{
			name:  "leading whitespace event",
			attrs: resource.Attributes{matomo.AttrType: "customEvent", matomo.AttrEvent: " trialStarted"},
			want:  `attribute "event" must not have leading or trailing whitespace`,
		},
		{
			name:  "trailing whitespace event",
			attrs: resource.Attributes{matomo.AttrType: "customEvent", matomo.AttrEvent: "trialStarted "},
			want:  `attribute "event" must not have leading or trailing whitespace`,
		},
		{
			name:  "whitespace only event",
			attrs: resource.Attributes{matomo.AttrType: "customEvent", matomo.AttrEvent: "   "},
			want:  `attribute "event" must not have leading or trailing whitespace`,
		},
		{
			name: "leading and trailing whitespace name",
			attrs: resource.Attributes{
				matomo.AttrType:  "customEvent",
				matomo.AttrEvent: "trialStarted",
				matomo.AttrName:  " Trial Started ",
			},
			want: `attribute "name" must not have leading or trailing whitespace`,
		},
		{
			name: "whitespace only name",
			attrs: resource.Attributes{
				matomo.AttrType:  "customEvent",
				matomo.AttrEvent: "trialStarted",
				matomo.AttrName:  "   ",
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

func TestValidateTriggerWhitespaceIsNotNormalized(t *testing.T) {
	t.Parallel()

	p := testTriggerProvider(t, newTriggerServer(t))
	res := triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: " trialStarted",
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

func TestValidateTriggerEffectiveNameLength(t *testing.T) {
	t.Parallel()

	p := testTriggerProvider(t, newTriggerServer(t))
	event255 := strings.Repeat("e", matomo.MaxTriggerNameLen)
	event256 := strings.Repeat("e", matomo.MaxTriggerNameLen+1)
	event300 := strings.Repeat("e", matomo.MaxEventNameLen)
	name255 := strings.Repeat("n", matomo.MaxTriggerNameLen)
	name256 := strings.Repeat("n", matomo.MaxTriggerNameLen+1)

	cases := []struct {
		name    string
		attrs   resource.Attributes
		wantErr string
	}{
		{
			name: "event length 255 without name",
			attrs: resource.Attributes{
				matomo.AttrType:  "customEvent",
				matomo.AttrEvent: event255,
			},
		},
		{
			name: "event length 256 without name",
			attrs: resource.Attributes{
				matomo.AttrType:  "customEvent",
				matomo.AttrEvent: event256,
			},
			wantErr: matomo.AttrEvent,
		},
		{
			name: "event length 300 with short name",
			attrs: resource.Attributes{
				matomo.AttrType:  "customEvent",
				matomo.AttrEvent: event300,
				matomo.AttrName:  "Trial Started",
			},
		},
		{
			name: "explicit name length 256",
			attrs: resource.Attributes{
				matomo.AttrType:  "customEvent",
				matomo.AttrEvent: "trialStarted",
				matomo.AttrName:  name256,
			},
			wantErr: matomo.AttrName,
		},
		{
			name: "explicit name length 255",
			attrs: resource.Attributes{
				matomo.AttrType:  "customEvent",
				matomo.AttrEvent: "trialStarted",
				matomo.AttrName:  name255,
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := p.Validate(context.Background(), triggerResource(t, "trial_started", tc.attrs))
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
			if !strings.Contains(err.Error(), "matomo.trigger.trial_started") {
				t.Fatalf("error = %q, want address", err)
			}
			assertNoProviderSecret(t, err.Error())
		})
	}
}

func TestValidateTriggerRequiresContainerID(t *testing.T) {
	t.Parallel()

	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:   "https://matomo.example.com",
		TokenAuth: providerToken,
		SiteID:    "3",
	}, http.DefaultClient)
	err := p.Validate(context.Background(), triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
	}))
	if err == nil {
		t.Fatal("expected container id error")
	}
	if !strings.Contains(err.Error(), matomo.EnvContainerID) {
		t.Fatalf("error = %q, want %s", err, matomo.EnvContainerID)
	}
}

func TestValidateTriggerRequiresSiteID(t *testing.T) {
	t.Parallel()

	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:     "https://matomo.example.com",
		TokenAuth:   providerToken,
		ContainerID: triggerContainerID,
	}, http.DefaultClient)
	err := p.Validate(context.Background(), triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
	}))
	if err == nil {
		t.Fatal("expected site id error")
	}
	if !strings.Contains(err.Error(), matomo.EnvSiteID) {
		t.Fatalf("error = %q, want %s", err, matomo.EnvSiteID)
	}
}

func TestReadTriggerSuccess(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.seed(apiTrigger{
		ID:          4,
		Name:        "trialStarted",
		Type:        "CustomEvent",
		Event:       "trialStarted",
		Description: "custom event",
		Status:      "active",
		Version:     "9",
		IDSite:      "3",
	})
	p := testTriggerProvider(t, srv)

	live, err := p.Read(context.Background(), triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
	}))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "4" {
		t.Fatalf("identity = %q, want 4", live.Identity.ID)
	}
	if live.Attributes[matomo.AttrType] != "customEvent" {
		t.Fatalf("type = %v", live.Attributes[matomo.AttrType])
	}
	if live.Attributes[matomo.AttrEvent] != "trialStarted" {
		t.Fatalf("event = %v", live.Attributes[matomo.AttrEvent])
	}
	if live.Computed["idtrigger"] != "4" {
		t.Fatalf("computed idtrigger = %v", live.Computed["idtrigger"])
	}
	if live.Computed["description"] != "custom event" {
		t.Fatalf("computed description = %v", live.Computed["description"])
	}
	if _, ok := live.Attributes["description"]; ok {
		t.Fatal("description must not appear in comparable attributes")
	}
}

func TestReadTriggerNotFound(t *testing.T) {
	t.Parallel()

	p := testTriggerProvider(t, newTriggerServer(t))
	_, err := p.Read(context.Background(), triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
	}))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadTriggerDuplicateName(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.seed(apiTrigger{ID: 1, Name: "trialStarted", Type: "CustomEvent", Event: "trialStarted"})
	srv.seed(apiTrigger{ID: 8, Name: "trialStarted", Type: "CustomEvent", Event: "trial.started"})
	p := testTriggerProvider(t, srv)

	_, err := p.Read(context.Background(), triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
	}))
	if err == nil {
		t.Fatal("expected duplicate name error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("duplicate names must not look like not found")
	}
	if !strings.Contains(err.Error(), "multiple remote triggers") {
		t.Fatalf("error = %q", err)
	}
}

func TestCreateTrigger(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	p := testTriggerProvider(t, srv)
	res := triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
	})

	live, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID == "" {
		t.Fatal("create returned empty identity")
	}
	if live.Attributes[matomo.AttrEvent] != "trialStarted" {
		t.Fatalf("event = %v", live.Attributes[matomo.AttrEvent])
	}
	if srv.lastCreateValues().Get("type") != "CustomEvent" {
		t.Fatalf("create type = %q, want CustomEvent", srv.lastCreateValues().Get("type"))
	}
	if srv.lastCreateValues().Get("name") != "trialStarted" {
		t.Fatalf("create name = %q, want defaulted event", srv.lastCreateValues().Get("name"))
	}
	if srv.lastCreateValues().Get("parameters[eventName]") != "trialStarted" {
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

func TestUpdateTrigger(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.seed(apiTrigger{
		ID:          8,
		Name:        "trialStarted",
		Type:        "CustomEvent",
		Event:       "oldEvent",
		Description: "keep me",
		Conditions:  `[{"actual":"PageUrl","comparison":"equals","expected":"https://example.com"}]`,
	})
	p := testTriggerProvider(t, srv)

	desired := triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
		matomo.AttrName:  "Trial Started",
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
	if live.Attributes[matomo.AttrEvent] != "trialStarted" {
		t.Fatalf("event = %v", live.Attributes[matomo.AttrEvent])
	}
	if live.Attributes[matomo.AttrName] != "Trial Started" {
		t.Fatalf("name = %v", live.Attributes[matomo.AttrName])
	}
	if srv.lastUpdateValues().Get("conditions[0][actual]") != "PageUrl" {
		t.Fatalf("update dropped conditions: %v", srv.lastUpdateValues())
	}
	if srv.lastUpdateValues().Get("type") != "" {
		t.Fatal("update must not send type")
	}
}

func TestPlanTriggerCreateWhenMissing(t *testing.T) {
	t.Parallel()

	p := testTriggerProvider(t, newTriggerServer(t))
	res := triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
	})

	got := mustPlanTrigger(t, p, res)
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionCreate {
		t.Fatalf("change = %+v, want create", got.Changes)
	}
}

func TestPlanTriggerUnchangedEquivalentRemote(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.seed(apiTrigger{
		ID:          2,
		Name:        "trialStarted",
		Type:        "CustomEvent",
		Event:       "trialStarted",
		Description: "ignored",
		Status:      "active",
		Version:     "9",
		IDSite:      "3",
	})
	p := testTriggerProvider(t, srv)

	res := triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
	})
	got := mustPlanTrigger(t, p, res)
	if got.HasChanges() {
		t.Fatalf("equivalent remote produced changes: %+v", got.Changes)
	}
	if got.Changes[0].Identity.ID != "2" {
		t.Fatalf("identity = %q, want 2", got.Changes[0].Identity.ID)
	}
}

func TestPlanTriggerUnchangedWithDisplayName(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.seed(apiTrigger{
		ID:     2,
		Name:   "Trial Started",
		Type:   "CustomEvent",
		Event:  "trialStarted",
		Status: "active",
	})
	p := testTriggerProvider(t, srv)

	res := triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
		matomo.AttrName:  "Trial Started",
	})
	got := mustPlanTrigger(t, p, res)
	if got.HasChanges() {
		t.Fatalf("equivalent display name produced changes: %+v", got.Changes)
	}
}

func TestPlanTriggerUpdateConfigurableField(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.seed(apiTrigger{ID: 2, Name: "trialStarted", Type: "CustomEvent", Event: "old"})
	p := testTriggerProvider(t, srv)

	res := triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
	})
	got := mustPlanTrigger(t, p, res)
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want update", got.Changes)
	}
	var eventDiff *plan.AttributeDiff
	for i := range got.Changes[0].Diffs {
		if got.Changes[0].Diffs[i].Path == matomo.AttrEvent {
			eventDiff = &got.Changes[0].Diffs[i]
		}
	}
	if eventDiff == nil || eventDiff.Before != "old" || eventDiff.After != "trialStarted" {
		t.Fatalf("event diff = %+v", got.Changes[0].Diffs)
	}
}

func TestPlanTriggerIgnoresComputedFields(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.seed(apiTrigger{
		ID:          5,
		Name:        "trialStarted",
		Type:        "CustomEvent",
		Event:       "trialStarted",
		Description: "computed",
		Conditions:  `[{"actual":"PageUrl","comparison":"equals","expected":"https://example.com"}]`,
		Status:      "active",
		Version:     "9",
		IDSite:      "3",
	})
	p := testTriggerProvider(t, srv)

	res := triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
	})
	got := mustPlanTrigger(t, p, res)
	if got.HasChanges() {
		t.Fatalf("computed fields produced changes: %+v", got.Changes)
	}
}

func TestReadTriggerAPIError(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.fail("TagManager.getContainerTriggers", `{"result":"error","message":"Unable to authenticate with `+providerToken+`"}`)
	p := testTriggerProvider(t, srv)

	_, err := p.Read(context.Background(), triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
	}))
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	if !strings.Contains(err.Error(), "authenticate") && !strings.Contains(err.Error(), "TagManager.getContainerTriggers") {
		t.Fatalf("error = %q, want API diagnostic", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadTriggerMalformedResponse(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.malformed("TagManager.getContainerTriggers", `"oops `+providerToken+`"`)
	p := testTriggerProvider(t, srv)

	_, err := p.Read(context.Background(), triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
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

func TestCreateTriggerValidatesBeforeMutation(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	p := testTriggerProvider(t, srv)
	_, err := p.Create(context.Background(), triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType: "customEvent",
	}))
	if err == nil {
		t.Fatal("expected validation error")
	}
	if srv.createCount() != 0 {
		t.Fatalf("creates = %d, want 0", srv.createCount())
	}
}

func TestImportTrigger(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.seed(apiTrigger{ID: 1, Name: "trialStarted", Type: "CustomEvent", Event: "trialStarted"})
	p := testTriggerProvider(t, srv)
	addr := mustTriggerAddress(t, "trial_started")
	live, err := p.Import(context.Background(), addr, "1")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if live.Identity.ID != "1" {
		t.Fatalf("identity = %q, want 1", live.Identity.ID)
	}
	if live.Attributes[matomo.AttrEvent] != "trialStarted" {
		t.Fatalf("event = %v", live.Attributes[matomo.AttrEvent])
	}
	if _, ok := live.Attributes["idtrigger"]; ok {
		t.Fatal("imported attributes must omit computed identity")
	}
}

func TestReadTriggerUsesBoundIDInsteadOfNameDiscovery(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.seed(apiTrigger{ID: 12, Name: "trialStarted", Type: "CustomEvent", Event: "trialStarted"})
	srv.seed(apiTrigger{ID: 99, Name: "other", Type: "CustomEvent", Event: "other"})
	p := testTriggerProvider(t, srv)

	res := triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "other",
		matomo.AttrName:  "other",
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

func TestReadTriggerStaleIdentity(t *testing.T) {
	t.Parallel()

	p := testTriggerProvider(t, newTriggerServer(t))
	res := triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
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

func TestReadTriggerUnsupportedRemoteType(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.seed(apiTrigger{ID: 3, Name: "trialStarted", Type: "PageView", Event: "trialStarted"})
	p := testTriggerProvider(t, srv)

	_, err := p.Read(context.Background(), triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
	}))
	if err == nil {
		t.Fatal("expected unsupported type error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported remote type must not look like not found")
	}
	if !strings.Contains(err.Error(), "PageView") {
		t.Fatalf("error = %q, want remote type", err)
	}
}

func TestTriggerManifestCompatibility(t *testing.T) {
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
  - address: matomo.trigger.trial_started
    attributes:
      type: customEvent
      event: trialStarted
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	m, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if len(m.Resources) != 3 {
		t.Fatalf("len(resources) = %d, want 3", len(m.Resources))
	}
	if m.Resources[0].Address.String() != "matomo.goal.trial_started" {
		t.Fatalf("goal address = %s", m.Resources[0].Address)
	}
	if m.Resources[1].Address.String() != "matomo.variable.user_id" {
		t.Fatalf("variable address = %s", m.Resources[1].Address)
	}
	if m.Resources[2].Address.String() != "matomo.trigger.trial_started" {
		t.Fatalf("trigger address = %s", m.Resources[2].Address)
	}
	if m.Resources[2].Attributes[matomo.AttrType] != "customEvent" {
		t.Fatalf("trigger type = %v", m.Resources[2].Attributes[matomo.AttrType])
	}

	srv := newTriggerServer(t)
	p := testTriggerProvider(t, srv)
	if err := p.Validate(context.Background(), m.Resources[0]); err != nil {
		t.Fatalf("existing goal resource must remain valid: %v", err)
	}
	if err := p.Validate(context.Background(), m.Resources[1]); err != nil {
		t.Fatalf("existing variable resource must remain valid: %v", err)
	}
	if err := p.Validate(context.Background(), m.Resources[2]); err != nil {
		t.Fatalf("trigger resource: %v", err)
	}
}

func TestTriggerResourceTypesRegistered(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{BaseURL: "https://matomo.example.com", TokenAuth: providerToken, SiteID: "1"})
	if !provider.Supports(p, matomo.TypeTrigger) {
		t.Fatal("matomo.trigger must be registered")
	}
	if !provider.Supports(p, matomo.TypeVariable) {
		t.Fatal("matomo.variable must remain registered")
	}
	if !provider.Supports(p, matomo.TypeGoal) {
		t.Fatal("matomo.goal must remain registered")
	}
}

type apiTrigger struct {
	ID          int
	Name        string
	Type        string
	Event       string
	Description string
	Conditions  string
	Status      string
	Version     string
	IDSite      string
}

type triggerServer struct {
	mu         sync.Mutex
	nextID     int
	version    string
	triggers   map[int]apiTrigger
	fails      map[string]string
	creates    int
	updates    int
	lastCreate url.Values
	lastUpdate url.Values
	server     *httptest.Server
}

func newTriggerServer(t *testing.T) *triggerServer {
	t.Helper()
	s := &triggerServer{
		nextID:   1,
		version:  "9",
		triggers: make(map[int]apiTrigger),
		fails:    make(map[string]string),
	}
	s.server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.server.Close)
	return s
}

func (s *triggerServer) seed(tr apiTrigger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tr.ID == 0 {
		tr.ID = s.nextID
		s.nextID++
	}
	if tr.ID >= s.nextID {
		s.nextID = tr.ID + 1
	}
	if tr.Type == "" {
		tr.Type = "CustomEvent"
	}
	if tr.Status == "" {
		tr.Status = "active"
	}
	if tr.Version == "" {
		tr.Version = s.version
	}
	if tr.IDSite == "" {
		tr.IDSite = "3"
	}
	s.triggers[tr.ID] = tr
}

func (s *triggerServer) fail(method, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fails[method] = body
}

func (s *triggerServer) malformed(method, body string) {
	s.fail(method, body)
}

func (s *triggerServer) serve(w http.ResponseWriter, r *http.Request) {
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
	case "TagManager.getContainerTriggers":
		s.writeTriggers(w)
	case "TagManager.addContainerTrigger":
		s.addTrigger(w, vals)
	case "TagManager.updateContainerTrigger":
		s.updateTrigger(w, vals)
	case "TagManager.deleteContainerTrigger":
		s.deleteTrigger(w, vals)
	default:
		_, _ = io.WriteString(w, `{"result":"error","message":"unknown method"}`)
	}
}

func (s *triggerServer) writeContainer(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = io.WriteString(w, `{"idcontainer":"`+triggerContainerID+`","idsite":3,"name":"Website","draft":{"idcontainerversion":`+s.version+`}}`)
}

func (s *triggerServer) writeTriggers(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.triggers) == 0 {
		_, _ = io.WriteString(w, `[]`)
		return
	}
	out := make([]map[string]any, 0, len(s.triggers))
	for id, tr := range s.triggers {
		item := map[string]any{
			"idtrigger":          strconv.Itoa(id),
			"idcontainerversion": tr.Version,
			"idsite":             tr.IDSite,
			"type":               tr.Type,
			"name":               tr.Name,
			"status":             tr.Status,
			"description":        tr.Description,
			"parameters":         map[string]any{"eventName": tr.Event},
		}
		if tr.Conditions != "" {
			var conditions any
			if err := json.Unmarshal([]byte(tr.Conditions), &conditions); err == nil {
				item["conditions"] = conditions
			} else {
				item["conditions"] = []any{}
			}
		} else {
			item["conditions"] = []any{}
		}
		out = append(out, item)
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *triggerServer) addTrigger(w http.ResponseWriter, vals url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creates++
	s.lastCreate = vals
	id := s.nextID
	s.nextID++
	s.triggers[id] = apiTrigger{
		ID:      id,
		Name:    vals.Get("name"),
		Type:    vals.Get("type"),
		Event:   vals.Get("parameters[eventName]"),
		Status:  "active",
		Version: s.version,
		IDSite:  "3",
	}
	_, _ = io.WriteString(w, strconv.Itoa(id))
}

func (s *triggerServer) updateTrigger(w http.ResponseWriter, vals url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates++
	s.lastUpdate = vals
	id, err := strconv.Atoi(vals.Get("idTrigger"))
	if err != nil {
		_, _ = io.WriteString(w, `{"result":"error","message":"invalid idTrigger"}`)
		return
	}
	tr, ok := s.triggers[id]
	if !ok {
		_, _ = io.WriteString(w, `{"result":"error","message":"trigger not found"}`)
		return
	}
	tr.Name = vals.Get("name")
	if event := vals.Get("parameters[eventName]"); event != "" {
		tr.Event = event
	}
	s.triggers[id] = tr
	_, _ = io.WriteString(w, `null`)
}

func (s *triggerServer) deleteTrigger(w http.ResponseWriter, vals url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, err := strconv.Atoi(vals.Get("idTrigger"))
	if err != nil {
		_, _ = io.WriteString(w, `{"result":"error","message":"invalid idTrigger"}`)
		return
	}
	if _, ok := s.triggers[id]; !ok {
		_, _ = io.WriteString(w, `{"result":"error","message":"The requested container trigger does not exist"}`)
		return
	}
	delete(s.triggers, id)
	_, _ = io.WriteString(w, `true`)
}

func (s *triggerServer) lastCreateValues() url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastCreate
}

func (s *triggerServer) lastUpdateValues() url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastUpdate
}

func (s *triggerServer) createCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creates
}

func testTriggerProvider(t *testing.T, srv *triggerServer) *matomo.Provider {
	t.Helper()
	return matomo.NewWithHTTPClient(client.Config{
		BaseURL:     srv.server.URL,
		TokenAuth:   providerToken,
		SiteID:      "3",
		ContainerID: triggerContainerID,
		HTTPClient:  srv.server.Client(),
	}, srv.server.Client())
}

func triggerResource(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{
		Address:    mustTriggerAddress(t, name),
		Attributes: attrs,
	}
}

func mustTriggerAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("matomo.trigger." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func mustPlanTrigger(t *testing.T, p *matomo.Provider, res resource.Resource) *plan.Plan {
	t.Helper()
	got, err := plan.Build(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	return got
}
