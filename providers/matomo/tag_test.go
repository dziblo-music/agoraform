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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

const tagContainerID = "6OMh6taM"

func TestValidateTagValid(t *testing.T) {
	t.Parallel()

	p := testTagProvider(t, newTagServer(t))
	cases := []struct {
		name  string
		attrs resource.Attributes
	}{
		{
			name:  "event fields",
			attrs: validTagAttrs(t),
		},
		{
			name: "display name",
			attrs: resource.Attributes{
				matomo.AttrType:          "matomoAnalytics",
				matomo.AttrTrigger:       triggerRef(t, "trial_started"),
				matomo.AttrEventCategory: "signup",
				matomo.AttrEventAction:   "trialStarted",
				matomo.AttrName:          "Trial Started",
			},
		},
		{
			name: "variable refs on optional fields",
			attrs: resource.Attributes{
				matomo.AttrType:          "matomoAnalytics",
				matomo.AttrTrigger:       triggerRef(t, "trial_started"),
				matomo.AttrEventCategory: "signup",
				matomo.AttrEventAction:   "trialStarted",
				matomo.AttrEventName:     variableRef(t, "user_id"),
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := p.Validate(context.Background(), tagResource(t, "trial_started", tc.attrs)); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestValidateTagErrors(t *testing.T) {
	t.Parallel()

	p := testTagProvider(t, newTagServer(t))
	addr := mustTagAddress(t, "trial_started")

	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name:  "missing type",
			attrs: resource.Attributes{matomo.AttrTrigger: triggerRef(t, "trial_started"), matomo.AttrEventCategory: "signup", matomo.AttrEventAction: "trialStarted"},
			want:  "missing required attribute \"type\"",
		},
		{
			name:  "missing trigger",
			attrs: resource.Attributes{matomo.AttrType: "matomoAnalytics", matomo.AttrEventCategory: "signup", matomo.AttrEventAction: "trialStarted"},
			want:  "missing required attribute \"trigger\"",
		},
		{
			name:  "string trigger",
			attrs: resource.Attributes{matomo.AttrType: "matomoAnalytics", matomo.AttrTrigger: "matomo.trigger.trial_started", matomo.AttrEventCategory: "signup", matomo.AttrEventAction: "trialStarted"},
			want:  "resource reference",
		},
		{
			name:  "trigger refs a variable",
			attrs: resource.Attributes{matomo.AttrType: "matomoAnalytics", matomo.AttrTrigger: variableRef(t, "user_id"), matomo.AttrEventCategory: "signup", matomo.AttrEventAction: "trialStarted"},
			want:  "matomo.trigger",
		},
		{
			name:  "missing eventCategory",
			attrs: resource.Attributes{matomo.AttrType: "matomoAnalytics", matomo.AttrTrigger: triggerRef(t, "trial_started"), matomo.AttrEventAction: "trialStarted"},
			want:  "missing required attribute \"eventCategory\"",
		},
		{
			name:  "empty eventAction",
			attrs: resource.Attributes{matomo.AttrType: "matomoAnalytics", matomo.AttrTrigger: triggerRef(t, "trial_started"), matomo.AttrEventCategory: "signup", matomo.AttrEventAction: ""},
			want:  "non-empty",
		},
		{
			name:  "unsupported type",
			attrs: resource.Attributes{matomo.AttrType: "customHtml", matomo.AttrTrigger: triggerRef(t, "trial_started"), matomo.AttrEventCategory: "signup", matomo.AttrEventAction: "trialStarted"},
			want:  "matomoAnalytics",
		},
		{
			name:  "matomo native type casing",
			attrs: resource.Attributes{matomo.AttrType: "Matomo", matomo.AttrTrigger: triggerRef(t, "trial_started"), matomo.AttrEventCategory: "signup", matomo.AttrEventAction: "trialStarted"},
			want:  "matomoAnalytics",
		},
		{
			name:  "computed idtag",
			attrs: withTagAttr(validTagAttrs(t), "idtag", "1"),
			want:  "computed",
		},
		{
			name:  "manifest identity",
			attrs: withTagAttr(validTagAttrs(t), matomo.AttrIDTag, "1"),
			want:  "not configurable",
		},
		{
			name:  "unknown attribute",
			attrs: withTagAttr(validTagAttrs(t), "html", "<script>"),
			want:  "unsupported attribute",
		},
		{
			name:  "leading whitespace category",
			attrs: withTagAttr(validTagAttrs(t), matomo.AttrEventCategory, " signup"),
			want:  `attribute "eventCategory" must not have leading or trailing whitespace`,
		},
		{
			name:  "eventName refs a trigger",
			attrs: withTagAttr(validTagAttrs(t), matomo.AttrEventName, triggerRef(t, "trial_started")),
			want:  "matomo.variable",
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

func TestValidateTagRequiresContainerID(t *testing.T) {
	t.Parallel()

	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:   "https://matomo.example.com",
		TokenAuth: providerToken,
		SiteID:    "3",
	}, http.DefaultClient)
	err := p.Validate(context.Background(), tagResource(t, "trial_started", validTagAttrs(t)))
	if err == nil {
		t.Fatal("expected container id error")
	}
	if !strings.Contains(err.Error(), matomo.EnvContainerID) {
		t.Fatalf("error = %q, want %s", err, matomo.EnvContainerID)
	}
}

func TestReadTagSuccess(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{
		ID:            7,
		Name:          "trialStarted",
		Type:          "Matomo",
		FireTriggerID: 4,
		Category:      "signup",
		Action:        "trialStarted",
		Description:   "event tag",
	})
	p := testTagProvider(t, srv)

	trigger := triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
	})
	if _, err := p.Read(context.Background(), trigger); err != nil {
		t.Fatalf("Read trigger: %v", err)
	}

	live, err := p.Read(context.Background(), tagResource(t, "trial_started", validTagAttrs(t)))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "7" {
		t.Fatalf("identity = %q, want 7", live.Identity.ID)
	}
	if live.Attributes[matomo.AttrType] != "matomoAnalytics" {
		t.Fatalf("type = %v", live.Attributes[matomo.AttrType])
	}
	if live.Attributes[matomo.AttrEventCategory] != "signup" {
		t.Fatalf("eventCategory = %v", live.Attributes[matomo.AttrEventCategory])
	}
	ref, ok := resource.AsRef(live.Attributes[matomo.AttrTrigger])
	if !ok || ref.Address.String() != "matomo.trigger.trial_started" {
		t.Fatalf("trigger = %v, want logical ref", live.Attributes[matomo.AttrTrigger])
	}
	if live.Computed["idtag"] != "7" {
		t.Fatalf("computed idtag = %v", live.Computed["idtag"])
	}
	if _, ok := live.Attributes["description"]; ok {
		t.Fatal("description must not appear in comparable attributes")
	}
}

func TestReadTagNotFound(t *testing.T) {
	t.Parallel()

	p := testTagProvider(t, newTagServer(t))
	_, err := p.Read(context.Background(), tagResource(t, "trial_started", validTagAttrs(t)))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestCreateTagResolvesTriggerIdentity(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	p := testTagProvider(t, srv)
	res := tagResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:          "matomoAnalytics",
		matomo.AttrTrigger:       resolvedTrigger(t, "trial_started", "4"),
		matomo.AttrEventCategory: "signup",
		matomo.AttrEventAction:   "trialStarted",
	})

	live, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID == "" {
		t.Fatal("create returned empty identity")
	}
	if srv.lastCreateValues().Get("type") != "Matomo" {
		t.Fatalf("create type = %q, want Matomo", srv.lastCreateValues().Get("type"))
	}
	if srv.lastCreateValues().Get("fireTriggerIds[0]") != "4" {
		t.Fatalf("create fireTriggerIds = %v", srv.lastCreateValues())
	}
	if srv.lastCreateValues().Get("parameters[trackingType]") != "event" {
		t.Fatalf("create trackingType = %v", srv.lastCreateValues())
	}
	if srv.lastCreateValues().Get("parameters[matomoConfig]") != "{{Matomo Configuration}}" {
		t.Fatalf("create matomoConfig = %v", srv.lastCreateValues())
	}
	if srv.lastCreateValues().Get("name") != "trialStarted" {
		t.Fatalf("create name = %q, want defaulted eventAction", srv.lastCreateValues().Get("name"))
	}
}

func TestCreateTagResolvesVariableReference(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedVariable(apiTagVariable{ID: 2, Name: "userId", Type: "DataLayer", Key: "userId"})
	p := testTagProvider(t, srv)
	res := tagResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:          "matomoAnalytics",
		matomo.AttrTrigger:       resolvedTrigger(t, "trial_started", "4"),
		matomo.AttrEventCategory: "signup",
		matomo.AttrEventAction:   "trialStarted",
		matomo.AttrEventName: resource.Resolved{
			Address:  mustVariableAddress(t, "user_id"),
			Identity: resource.Identity{ID: "2"},
			Outputs:  resource.Attributes{},
		},
	})

	if _, err := p.Create(context.Background(), res); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if srv.lastCreateValues().Get("parameters[eventName]") != "{{userId}}" {
		t.Fatalf("eventName = %v, want {{userId}}", srv.lastCreateValues())
	}
}

func TestUpdateTag(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{
		ID:            8,
		Name:          "trialStarted",
		Type:          "Matomo",
		FireTriggerID: 4,
		Category:      "old",
		Action:        "trialStarted",
		Description:   "keep me",
		BlockIDs:      []int{2},
		FireLimit:     "once_page",
	})
	p := testTagProvider(t, srv)

	desired := tagResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:          "matomoAnalytics",
		matomo.AttrTrigger:       resolvedTrigger(t, "trial_started", "4"),
		matomo.AttrEventCategory: "signup",
		matomo.AttrEventAction:   "trialStarted",
		matomo.AttrName:          "Trial Started",
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
	if live.Attributes[matomo.AttrEventCategory] != "signup" {
		t.Fatalf("eventCategory = %v", live.Attributes[matomo.AttrEventCategory])
	}
	if srv.lastUpdateValues().Get("type") != "" {
		t.Fatal("update must not send type")
	}
	if srv.lastUpdateValues().Get("description") != "keep me" {
		t.Fatalf("update dropped description: %v", srv.lastUpdateValues())
	}
	if srv.lastUpdateValues().Get("blockTriggerIds[0]") != "2" {
		t.Fatalf("update dropped block triggers: %v", srv.lastUpdateValues())
	}
}

func TestPlanTagCreateWhenMissing(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	p := testTagProvider(t, srv)
	got := mustPlanTag(t, p, trialStartedTrigger(t), tagResource(t, "trial_started", validTagAttrs(t)))
	if !hasAction(got, "matomo.tag.trial_started", plan.ActionCreate) {
		t.Fatalf("change = %+v, want tag create", got.Changes)
	}
}

func TestPlanTagUnchangedEquivalentRemote(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{
		ID:            7,
		Name:          "trialStarted",
		Type:          "Matomo",
		FireTriggerID: 4,
		Category:      "signup",
		Action:        "trialStarted",
		Description:   "ignored",
	})
	p := testTagProvider(t, srv)

	got := mustPlanTag(t, p, trialStartedTrigger(t), tagResource(t, "trial_started", validTagAttrs(t)))
	if got.HasChanges() {
		t.Fatalf("equivalent remote produced changes: %+v", got.Changes)
	}
	tagChange := changeByAddr(t, got, "matomo.tag.trial_started")
	if tagChange.Identity.ID != "7" {
		t.Fatalf("identity = %q, want 7", tagChange.Identity.ID)
	}
	out := plan.Format(got)
	if strings.Contains(out, `"4"`) || strings.Contains(out, "idtag") {
		t.Fatalf("plan leaked provider-native identity:\n%s", out)
	}
}

func TestPlanTagUpdateConfigurableField(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{ID: 7, Name: "trialStarted", Type: "Matomo", FireTriggerID: 4, Category: "old", Action: "trialStarted"})
	p := testTagProvider(t, srv)

	got := mustPlanTag(t, p, trialStartedTrigger(t), tagResource(t, "trial_started", validTagAttrs(t)))
	tagChange := changeByAddr(t, got, "matomo.tag.trial_started")
	if tagChange.Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want update", tagChange)
	}
	var category *plan.AttributeDiff
	for i := range tagChange.Diffs {
		if tagChange.Diffs[i].Path == matomo.AttrEventCategory {
			category = &tagChange.Diffs[i]
		}
	}
	if category == nil || category.Before != "old" || category.After != "signup" {
		t.Fatalf("eventCategory diff = %+v", tagChange.Diffs)
	}
}

func TestPlanTagTriggerChange(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTrigger(apiTagTrigger{ID: 9, Name: "signedUp", Event: "signedUp"})
	srv.seedTag(apiTag{ID: 7, Name: "trialStarted", Type: "Matomo", FireTriggerID: 4, Category: "signup", Action: "trialStarted"})
	p := testTagProvider(t, srv)

	other := triggerResource(t, "signed_up", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "signedUp",
	})
	tag := tagResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:          "matomoAnalytics",
		matomo.AttrTrigger:       triggerRef(t, "signed_up"),
		matomo.AttrEventCategory: "signup",
		matomo.AttrEventAction:   "trialStarted",
	})
	got := mustPlanTag(t, p, trialStartedTrigger(t), other, tag)
	tagChange := changeByAddr(t, got, "matomo.tag.trial_started")
	if tagChange.Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want trigger update", tagChange)
	}
}

func TestPlanTagIgnoresComputedFields(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{
		ID:            5,
		Name:          "trialStarted",
		Type:          "Matomo",
		FireTriggerID: 4,
		Category:      "signup",
		Action:        "trialStarted",
		Description:   "computed",
		FireLimit:     "once_page",
		Status:        "active",
		Version:       "9",
	})
	p := testTagProvider(t, srv)

	got := mustPlanTag(t, p, trialStartedTrigger(t), tagResource(t, "trial_started", validTagAttrs(t)))
	if got.HasChanges() {
		t.Fatalf("computed fields produced changes: %+v", got.Changes)
	}
}

func TestReadTagAPIError(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.fail("TagManager.getContainerTags", `{"result":"error","message":"Unable to authenticate with `+providerToken+`"}`)
	p := testTagProvider(t, srv)

	_, err := p.Read(context.Background(), tagResource(t, "trial_started", validTagAttrs(t)))
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadTagMalformedResponse(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.malformed("TagManager.getContainerTags", `"oops `+providerToken+`"`)
	p := testTagProvider(t, srv)

	_, err := p.Read(context.Background(), tagResource(t, "trial_started", validTagAttrs(t)))
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

func TestCreateTagValidatesBeforeMutation(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	p := testTagProvider(t, srv)
	_, err := p.Create(context.Background(), tagResource(t, "trial_started", resource.Attributes{
		matomo.AttrType: "matomoAnalytics",
	}))
	if err == nil {
		t.Fatal("expected validation error")
	}
	if srv.createCount() != 0 {
		t.Fatalf("creates = %d, want 0", srv.createCount())
	}
}

func TestImportTagReconstructsTriggerRef(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{ID: 1, Name: "trialStarted", Type: "Matomo", FireTriggerID: 4, Category: "signup", Action: "trialStarted"})
	p := testTagProvider(t, srv)
	p.SetIdentityCatalog(boundIdentityCatalog(t, "matomo.trigger.trial_started", "4"))

	live, err := p.Import(context.Background(), mustTagAddress(t, "trial_started"), "1")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if live.Identity.ID != "1" {
		t.Fatalf("identity = %q, want 1", live.Identity.ID)
	}
	ref, ok := resource.AsRef(live.Attributes[matomo.AttrTrigger])
	if !ok || ref.Address.String() != "matomo.trigger.trial_started" {
		t.Fatalf("trigger = %v, want $ref matomo.trigger.trial_started", live.Attributes[matomo.AttrTrigger])
	}
	if live.Attributes[matomo.AttrEventCategory] != "signup" {
		t.Fatalf("eventCategory = %v", live.Attributes[matomo.AttrEventCategory])
	}
	if _, ok := live.Attributes["idtag"]; ok {
		t.Fatal("imported attributes must omit computed identity")
	}
	if _, ok := live.Computed["fire_trigger_ids"]; !ok {
		t.Fatal("computed fire_trigger_ids missing")
	}
	if err := p.Validate(context.Background(), resource.Resource{Address: live.Address, Attributes: live.Attributes.Clone()}); err != nil {
		t.Fatalf("imported attributes must validate: %v", err)
	}
}

func TestImportTagReconstructsVariableRefs(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedVariable(apiTagVariable{ID: 2, Name: "userId", Type: "DataLayer", Key: "userId"})
	srv.seedTag(apiTag{
		ID:            1,
		Name:          "trialStarted",
		Type:          "Matomo",
		FireTriggerID: 4,
		Category:      "{{userId}}",
		Action:        "trialStarted",
		EventName:     "{{userId}}",
	})
	p := testTagProvider(t, srv)
	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustTriggerAddress(t, "trial_started"), resource.Identity{ID: "4"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustVariableAddress(t, "user_id"), resource.Identity{ID: "2"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	live, err := p.Import(context.Background(), mustTagAddress(t, "trial_started"), "1")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	category, ok := resource.AsRef(live.Attributes[matomo.AttrEventCategory])
	if !ok || category.Address.String() != "matomo.variable.user_id" {
		t.Fatalf("eventCategory = %v, want $ref matomo.variable.user_id", live.Attributes[matomo.AttrEventCategory])
	}
	eventName, ok := resource.AsRef(live.Attributes[matomo.AttrEventName])
	if !ok || eventName.Address.String() != "matomo.variable.user_id" {
		t.Fatalf("eventName = %v, want $ref matomo.variable.user_id", live.Attributes[matomo.AttrEventName])
	}
	if live.Attributes[matomo.AttrEventAction] != "trialStarted" {
		t.Fatalf("eventAction = %v, want literal trialStarted", live.Attributes[matomo.AttrEventAction])
	}
}

func TestImportTagKeepsUnboundVariableTemplate(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedVariable(apiTagVariable{ID: 2, Name: "userId", Type: "DataLayer", Key: "userId"})
	srv.seedTag(apiTag{
		ID:            1,
		Name:          "trialStarted",
		Type:          "Matomo",
		FireTriggerID: 4,
		Category:      "{{userId}}",
		Action:        "trialStarted",
	})
	p := testTagProvider(t, srv)
	p.SetIdentityCatalog(boundIdentityCatalog(t, "matomo.trigger.trial_started", "4"))

	live, err := p.Import(context.Background(), mustTagAddress(t, "trial_started"), "1")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if live.Attributes[matomo.AttrEventCategory] != "{{userId}}" {
		t.Fatalf("eventCategory = %v, want unbound template literal", live.Attributes[matomo.AttrEventCategory])
	}
}

func TestImportTagRequiresBoundTrigger(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{ID: 1, Name: "trialStarted", Type: "Matomo", FireTriggerID: 4, Category: "signup", Action: "trialStarted"})
	p := testTagProvider(t, srv)

	_, err := p.Import(context.Background(), mustTagAddress(t, "trial_started"), "1")
	if err == nil {
		t.Fatal("expected unbound trigger error")
	}
	if !strings.Contains(err.Error(), `fire trigger id "4"`) || !strings.Contains(err.Error(), "not bound") {
		t.Fatalf("error = %q, want unbound trigger guidance", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestImportTagRejectsMultipleFireTriggers(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.fail("TagManager.getContainerTags", `[{"idtag":"1","idcontainerversion":"9","idsite":"3","type":"Matomo","name":"trialStarted","status":"active","fire_trigger_ids":["4","5"],"block_trigger_ids":[],"parameters":{"trackingType":"event","eventCategory":"signup","eventAction":"trialStarted","matomoConfig":"{{Matomo Configuration}}"}}]`)

	p := testTagProvider(t, srv)
	p.SetIdentityCatalog(boundIdentityCatalog(t, "matomo.trigger.trial_started", "4"))
	_, err := p.Import(context.Background(), mustTagAddress(t, "trial_started"), "1")
	if err == nil {
		t.Fatal("expected multi-trigger error")
	}
	if !strings.Contains(err.Error(), "exactly one fire trigger") {
		t.Fatalf("error = %q, want multi-trigger guidance", err)
	}
}

func TestImportTagInvalidID(t *testing.T) {
	t.Parallel()

	p := testTagProvider(t, newTagServer(t))
	_, err := p.Import(context.Background(), mustTagAddress(t, "trial_started"), "abc")
	if err == nil {
		t.Fatal("expected invalid id error")
	}
	if !strings.Contains(err.Error(), "valid Matomo tag id") {
		t.Fatalf("error = %q, want invalid id diagnostic", err)
	}
}

func TestImportTagNotFound(t *testing.T) {
	t.Parallel()

	p := testTagProvider(t, newTagServer(t))
	p.SetIdentityCatalog(boundIdentityCatalog(t, "matomo.trigger.trial_started", "4"))
	_, err := p.Import(context.Background(), mustTagAddress(t, "trial_started"), "99")
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Import = %v, want ErrNotFound", err)
	}
}

func TestImportTagThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{ID: 1, Name: "trialStarted", Type: "Matomo", FireTriggerID: 4, Category: "signup", Action: "trialStarted"})
	p := testTagProvider(t, srv)

	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustTriggerAddress(t, "trial_started"), resource.Identity{ID: "4"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	live, err := p.Import(context.Background(), mustTagAddress(t, "trial_started"), "1")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if err := st.RecordImport(live.Address, live.Identity.ID); err != nil {
		t.Fatal(err)
	}

	trigger := trialStartedTrigger(t)
	trigger.Identity = resource.Identity{ID: "4"}
	tag := resource.Resource{Address: live.Address, Identity: live.Identity, Attributes: live.Attributes.Clone()}
	got := mustPlanTag(t, p, trigger, tag)
	if got.HasChanges() {
		t.Fatalf("plan after import produced changes: %+v", got.Changes)
	}
	if srv.createCount() != 0 || srv.updateCount() != 0 {
		t.Fatalf("import path mutated remote: creates=%d updates=%d", srv.createCount(), srv.updateCount())
	}
}

func TestReadTagStaleIdentity(t *testing.T) {
	t.Parallel()

	p := testTagProvider(t, newTagServer(t))
	res := tagResource(t, "trial_started", validTagAttrs(t))
	res.Identity = resource.Identity{ID: "12"}

	_, err := p.Read(context.Background(), res)
	if !errors.Is(err, state.ErrStaleIdentity) {
		t.Fatalf("Read = %v, want ErrStaleIdentity", err)
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("stale identity must not look like a create candidate")
	}
}

func TestApplyTagAfterTrigger(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	p := testTagProvider(t, srv)
	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}

	trigger := trialStartedTrigger(t)
	tag := tagResource(t, "trial_started", validTagAttrs(t))
	var out bytes.Buffer
	result, err := apply.Run(context.Background(), []resource.Resource{tag, trigger}, func(resource.Address) (provider.Provider, error) {
		return p, nil
	}, st, &out)
	if err != nil {
		t.Fatalf("apply.Run: %v", err)
	}
	if result.Created != 2 {
		t.Fatalf("result = %+v, want 2 created", result)
	}
	progress := out.String()
	trig := strings.Index(progress, "matomo.trigger.trial_started: created")
	tg := strings.Index(progress, "matomo.tag.trial_started: created")
	if trig < 0 || tg < 0 || trig > tg {
		t.Fatalf("apply order:\n%s", progress)
	}
	if srv.lastCreateValues().Get("fireTriggerIds[0]") == "" {
		t.Fatalf("tag create missing resolved trigger id: %v", srv.lastCreateValues())
	}

	got := mustPlanTag(t, p, trigger, tag)
	if got.HasChanges() {
		t.Fatalf("plan after apply produced changes: %+v", got.Changes)
	}
}

func TestTagManifestCompatibility(t *testing.T) {
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
  - address: matomo.tag.trial_started
    attributes:
      type: matomoAnalytics
      trigger:
        $ref: matomo.trigger.trial_started
      eventCategory: signup
      eventAction: trialStarted
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	m, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if len(m.Resources) != 4 {
		t.Fatalf("len(resources) = %d, want 4", len(m.Resources))
	}
	if m.Resources[3].Address.String() != "matomo.tag.trial_started" {
		t.Fatalf("tag address = %s", m.Resources[3].Address)
	}
	if _, ok := resource.AsRef(m.Resources[3].Attributes[matomo.AttrTrigger]); !ok {
		t.Fatalf("trigger attr = %v, want $ref", m.Resources[3].Attributes[matomo.AttrTrigger])
	}

	p := testTagProvider(t, newTagServer(t))
	if err := p.Validate(context.Background(), m.Resources[0]); err != nil {
		t.Fatalf("existing goal resource must remain valid: %v", err)
	}
	if err := p.Validate(context.Background(), m.Resources[3]); err != nil {
		t.Fatalf("tag resource: %v", err)
	}
}

func TestTagResourceTypesRegistered(t *testing.T) {
	t.Parallel()

	p := matomo.New(client.Config{BaseURL: "https://matomo.example.com", TokenAuth: providerToken, SiteID: "1"})
	if !provider.Supports(p, matomo.TypeTag) {
		t.Fatal("matomo.tag must be registered")
	}
	if !provider.Supports(p, matomo.TypeTrigger) {
		t.Fatal("matomo.trigger must remain registered")
	}
	if !provider.Supports(p, matomo.TypeVariable) {
		t.Fatal("matomo.variable must remain registered")
	}
	if !provider.Supports(p, matomo.TypeGoal) {
		t.Fatal("matomo.goal must remain registered")
	}
}

type apiTag struct {
	ID            int
	Name          string
	Type          string
	FireTriggerID int
	Category      string
	Action        string
	EventName     string
	MatomoConfig  string
	Description   string
	BlockIDs      []int
	FireLimit     string
	Status        string
	Version       string
	IDSite        string
}

type apiTagTrigger struct {
	ID     int
	Name   string
	Event  string
	Type   string
	Status string
}

type apiTagVariable struct {
	ID     int
	Name   string
	Type   string
	Key    string
	Status string
}

type tagServer struct {
	mu         sync.Mutex
	nextID     int
	version    string
	tags       map[int]apiTag
	triggers   map[int]apiTagTrigger
	variables  map[int]apiTagVariable
	fails      map[string]string
	creates    int
	updates    int
	lastCreate url.Values
	lastUpdate url.Values
	server     *httptest.Server
}

func newTagServer(t *testing.T) *tagServer {
	t.Helper()
	s := &tagServer{
		nextID:    1,
		version:   "9",
		tags:      make(map[int]apiTag),
		triggers:  make(map[int]apiTagTrigger),
		variables: make(map[int]apiTagVariable),
		fails:     make(map[string]string),
	}
	s.variables[1] = apiTagVariable{ID: 1, Name: "Matomo Configuration", Type: "MatomoConfiguration", Status: "active"}
	s.server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.server.Close)
	return s
}

func (s *tagServer) seedTag(tag apiTag) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seedTagLocked(tag)
}

func (s *tagServer) seedTagLocked(tag apiTag) {
	if tag.ID == 0 {
		tag.ID = s.nextID
		s.nextID++
	}
	if tag.ID >= s.nextID {
		s.nextID = tag.ID + 1
	}
	if tag.Type == "" {
		tag.Type = "Matomo"
	}
	if tag.Status == "" {
		tag.Status = "active"
	}
	if tag.Version == "" {
		tag.Version = s.version
	}
	if tag.IDSite == "" {
		tag.IDSite = "3"
	}
	s.tags[tag.ID] = tag
}

func (s *tagServer) seedTrigger(tr apiTagTrigger) {
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
	s.triggers[tr.ID] = tr
}

func (s *tagServer) seedVariable(v apiTagVariable) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.ID == 0 {
		v.ID = s.nextID
		s.nextID++
	}
	if v.ID >= s.nextID {
		s.nextID = v.ID + 1
	}
	if v.Status == "" {
		v.Status = "active"
	}
	s.variables[v.ID] = v
}

func (s *tagServer) fail(method, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fails[method] = body
}

func (s *tagServer) malformed(method, body string) {
	s.fail(method, body)
}

func (s *tagServer) serve(w http.ResponseWriter, r *http.Request) {
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
	case "TagManager.getContainerVariables":
		s.writeVariables(w)
	case "TagManager.getContainerTags":
		s.writeTags(w)
	case "TagManager.addContainerTrigger":
		s.addTrigger(w, vals)
	case "TagManager.addContainerTag":
		s.addTag(w, vals)
	case "TagManager.updateContainerTag":
		s.updateTag(w, vals)
	default:
		_, _ = io.WriteString(w, `{"result":"error","message":"unknown method"}`)
	}
}

func (s *tagServer) writeContainer(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = io.WriteString(w, `{"idcontainer":"`+tagContainerID+`","idsite":3,"name":"Website","draft":{"idcontainerversion":`+s.version+`}}`)
}

func (s *tagServer) writeTriggers(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.triggers) == 0 {
		_, _ = io.WriteString(w, `[]`)
		return
	}
	out := make([]map[string]any, 0, len(s.triggers))
	for id, tr := range s.triggers {
		out = append(out, map[string]any{
			"idtrigger":          strconv.Itoa(id),
			"idcontainerversion": s.version,
			"idsite":             "3",
			"type":               tr.Type,
			"name":               tr.Name,
			"status":             tr.Status,
			"parameters":         map[string]any{"eventName": tr.Event},
			"conditions":         []any{},
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *tagServer) writeVariables(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]map[string]any, 0, len(s.variables))
	for id, v := range s.variables {
		item := map[string]any{
			"idvariable":         strconv.Itoa(id),
			"idcontainerversion": s.version,
			"idsite":             "3",
			"type":               v.Type,
			"name":               v.Name,
			"status":             v.Status,
			"parameters":         map[string]any{},
		}
		if v.Key != "" {
			item["parameters"] = map[string]any{"dataLayerName": v.Key}
		}
		out = append(out, item)
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *tagServer) writeTags(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.tags) == 0 {
		_, _ = io.WriteString(w, `[]`)
		return
	}
	out := make([]map[string]any, 0, len(s.tags))
	for id, tag := range s.tags {
		fire := []any{}
		if tag.FireTriggerID != 0 {
			fire = []any{tag.FireTriggerID}
		}
		block := make([]any, 0, len(tag.BlockIDs))
		for _, bid := range tag.BlockIDs {
			block = append(block, bid)
		}
		configName := tag.MatomoConfig
		if configName == "" {
			configName = "Matomo Configuration"
		}
		item := map[string]any{
			"idtag":              strconv.Itoa(id),
			"idcontainerversion": tag.Version,
			"idsite":             tag.IDSite,
			"type":               tag.Type,
			"name":               tag.Name,
			"status":             tag.Status,
			"description":        tag.Description,
			"fireTriggerIds":     fire,
			"blockTriggerIds":    block,
			"fireLimit":          tag.FireLimit,
			"parameters": map[string]any{
				"trackingType":  "event",
				"eventCategory": tag.Category,
				"eventAction":   tag.Action,
				"eventName":     tag.EventName,
				"matomoConfig":  map[string]any{"name": configName, "type": "MatomoConfiguration"},
			},
		}
		out = append(out, item)
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *tagServer) addTrigger(w http.ResponseWriter, vals url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID
	s.nextID++
	s.triggers[id] = apiTagTrigger{
		ID:     id,
		Name:   vals.Get("name"),
		Event:  vals.Get("parameters[eventName]"),
		Type:   vals.Get("type"),
		Status: "active",
	}
	_, _ = io.WriteString(w, strconv.Itoa(id))
}

func (s *tagServer) addTag(w http.ResponseWriter, vals url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creates++
	s.lastCreate = vals
	id := s.nextID
	s.nextID++
	fireID, _ := strconv.Atoi(vals.Get("fireTriggerIds[0]"))
	s.tags[id] = apiTag{
		ID:            id,
		Name:          vals.Get("name"),
		Type:          vals.Get("type"),
		FireTriggerID: fireID,
		Category:      vals.Get("parameters[eventCategory]"),
		Action:        vals.Get("parameters[eventAction]"),
		EventName:     vals.Get("parameters[eventName]"),
		Status:        "active",
		Version:       s.version,
		IDSite:        "3",
	}
	_, _ = io.WriteString(w, strconv.Itoa(id))
}

func (s *tagServer) updateTag(w http.ResponseWriter, vals url.Values) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates++
	s.lastUpdate = vals
	id, err := strconv.Atoi(vals.Get("idTag"))
	if err != nil {
		_, _ = io.WriteString(w, `{"result":"error","message":"invalid idTag"}`)
		return
	}
	tag, ok := s.tags[id]
	if !ok {
		_, _ = io.WriteString(w, `{"result":"error","message":"tag not found"}`)
		return
	}
	tag.Name = vals.Get("name")
	if category := vals.Get("parameters[eventCategory]"); category != "" {
		tag.Category = category
	}
	if action := vals.Get("parameters[eventAction]"); action != "" {
		tag.Action = action
	}
	if fire := vals.Get("fireTriggerIds[0]"); fire != "" {
		tag.FireTriggerID, _ = strconv.Atoi(fire)
	}
	s.tags[id] = tag
	_, _ = io.WriteString(w, `null`)
}

func (s *tagServer) lastCreateValues() url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastCreate
}

func (s *tagServer) lastUpdateValues() url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastUpdate
}

func (s *tagServer) createCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creates
}

func (s *tagServer) updateCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updates
}

func boundIdentityCatalog(t *testing.T, address, remoteID string) matomo.IdentityCatalog {
	t.Helper()
	return boundIdentityCatalogs(t, map[string]string{address: remoteID})
}

func boundIdentityCatalogs(t *testing.T, ids map[string]string) matomo.IdentityCatalog {
	t.Helper()
	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}
	for address, remoteID := range ids {
		addr, err := resource.ParseAddress(address)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.Bind(addr, resource.Identity{ID: remoteID}); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

func testTagProvider(t *testing.T, srv *tagServer) *matomo.Provider {
	t.Helper()
	return matomo.NewWithHTTPClient(client.Config{
		BaseURL:     srv.server.URL,
		TokenAuth:   providerToken,
		SiteID:      "3",
		ContainerID: tagContainerID,
		HTTPClient:  srv.server.Client(),
	}, srv.server.Client())
}

func tagResource(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{
		Address:    mustTagAddress(t, name),
		Attributes: attrs,
	}
}

func trialStartedTrigger(t *testing.T) resource.Resource {
	t.Helper()
	return triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
	})
}

func validTagAttrs(t *testing.T) resource.Attributes {
	t.Helper()
	return resource.Attributes{
		matomo.AttrType:          "matomoAnalytics",
		matomo.AttrTrigger:       triggerRef(t, "trial_started"),
		matomo.AttrEventCategory: "signup",
		matomo.AttrEventAction:   "trialStarted",
	}
}

func withTagAttr(attrs resource.Attributes, key string, value any) resource.Attributes {
	out := attrs.Clone()
	out[key] = value
	return out
}

func triggerRef(t *testing.T, name string) resource.Ref {
	t.Helper()
	return resource.Ref{Address: mustTriggerAddress(t, name)}
}

func variableRef(t *testing.T, name string) resource.Ref {
	t.Helper()
	return resource.Ref{Address: mustVariableAddress(t, name)}
}

func resolvedTrigger(t *testing.T, name, id string) resource.Resolved {
	t.Helper()
	return resource.Resolved{
		Address:  mustTriggerAddress(t, name),
		Identity: resource.Identity{ID: id},
	}
}

func mustTagAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("matomo.tag." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func mustPlanTag(t *testing.T, p *matomo.Provider, resources ...resource.Resource) *plan.Plan {
	t.Helper()
	got, err := plan.Build(context.Background(), resources, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	return got
}

func changeByAddr(t *testing.T, p *plan.Plan, addr string) plan.Change {
	t.Helper()
	for _, change := range p.Changes {
		if change.Address.String() == addr {
			return change
		}
	}
	t.Fatalf("no change for %s in %+v", addr, p.Changes)
	return plan.Change{}
}

func hasAction(p *plan.Plan, addr string, action plan.Action) bool {
	for _, change := range p.Changes {
		if change.Address.String() == addr && change.Action == action {
			return true
		}
	}
	return false
}
