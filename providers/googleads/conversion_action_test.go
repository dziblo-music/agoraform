package googleads_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/internal/importer"
	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestValidateConversionActionValid(t *testing.T) {
	t.Parallel()

	p, _ := testConversionActionProvider(t, nil)
	res := conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:           "Trial Started",
		googleads.AttrCategory:       "SIGNUP",
		googleads.AttrValue:          0,
		googleads.AttrCount:          "ONE",
		googleads.AttrPrimaryForGoal: true,
	})
	if err := p.Validate(context.Background(), res); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestConversionActionOutputs(t *testing.T) {
	t.Parallel()

	p, _ := testConversionActionProvider(t, nil)
	specs := p.Outputs(googleads.TypeConversionAction)
	if len(specs) != 2 {
		t.Fatalf("Outputs = %+v, want conversionId and conversionLabel", specs)
	}
	id, ok := provider.FindOutput(specs, googleads.OutputConversionID)
	if !ok || id.Sensitive || id.Kind != provider.OutputKindString {
		t.Fatalf("conversionId spec = (%v, %v)", id, ok)
	}
	label, ok := provider.FindOutput(specs, googleads.OutputConversionLabel)
	if !ok || label.Sensitive || label.Kind != provider.OutputKindString {
		t.Fatalf("conversionLabel spec = (%v, %v)", label, ok)
	}
	if _, ok := provider.FindOutput(specs, "tagSnippets"); ok {
		t.Fatal("tagSnippets must not be a selectable output")
	}
	if _, ok := provider.FindOutput(specs, "id"); ok {
		t.Fatal("provider-native id must not be a selectable output")
	}
	if specs := p.Outputs(googleads.TypeCampaign); len(specs) != 0 {
		t.Fatalf("campaign outputs = %+v, want none in this issue", specs)
	}

	action := conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:     "Trial Started",
		googleads.AttrCategory: "SIGNUP",
	})
	consumer := resource.Resource{
		Address: mustParseOutputAddr(t, "alt.note.banner"),
		Attributes: resource.Attributes{
			"text": resource.Ref{Address: action.Address, Output: googleads.OutputConversionID},
		},
	}
	if err := provider.ValidateOutputRefs([]resource.Resource{action, consumer}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}); err != nil {
		t.Fatalf("declared conversionId should be selectable: %v", err)
	}

	consumer.Attributes["text"] = resource.Ref{Address: action.Address, Output: "tagSnippets"}
	err := provider.ValidateOutputRefs([]resource.Resource{action, consumer}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err == nil || !strings.Contains(err.Error(), "no declared output") {
		t.Fatalf("tagSnippets should be rejected: %v", err)
	}
}

func TestValidateConversionActionErrors(t *testing.T) {
	t.Parallel()

	p, _ := testConversionActionProvider(t, nil)
	addr := mustConversionActionAddress(t, "trial_started")

	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name:  "missing name",
			attrs: resource.Attributes{googleads.AttrCategory: "SIGNUP"},
			want:  "missing required attribute \"name\"",
		},
		{
			name:  "missing category",
			attrs: resource.Attributes{googleads.AttrName: "Trial Started"},
			want:  "missing required attribute \"category\"",
		},
		{
			name: "unknown category",
			attrs: resource.Attributes{
				googleads.AttrName:     "Trial Started",
				googleads.AttrCategory: "PHONE_CALL_LEAD",
			},
			want: "category",
		},
		{
			name: "computed type",
			attrs: resource.Attributes{
				googleads.AttrName:     "Trial Started",
				googleads.AttrCategory: "SIGNUP",
				"type":                 "WEBPAGE",
			},
			want: "computed",
		},
		{
			name: "computed id",
			attrs: resource.Attributes{
				googleads.AttrName:     "Trial Started",
				googleads.AttrCategory: "SIGNUP",
				"id":                   "1",
			},
			want: "computed",
		},
		{
			name: "unsupported attribute",
			attrs: resource.Attributes{
				googleads.AttrName:     "Trial Started",
				googleads.AttrCategory: "SIGNUP",
				"campaign":             "brand",
			},
			want: "unsupported attribute",
		},
		{
			name: "invalid count",
			attrs: resource.Attributes{
				googleads.AttrName:     "Trial Started",
				googleads.AttrCategory: "SIGNUP",
				googleads.AttrCount:    "ONCE",
			},
			want: "ONE or MANY",
		},
		{
			name: "negative value",
			attrs: resource.Attributes{
				googleads.AttrName:     "Trial Started",
				googleads.AttrCategory: "SIGNUP",
				googleads.AttrValue:    -1,
			},
			want: "greater than or equal to 0",
		},
		{
			name: "invalid currency",
			attrs: resource.Attributes{
				googleads.AttrName:     "Trial Started",
				googleads.AttrCategory: "SIGNUP",
				googleads.AttrCurrency: "US",
			},
			want: "3-letter currency code",
		},
		{
			name: "lookback too large",
			attrs: resource.Attributes{
				googleads.AttrName:                           "Trial Started",
				googleads.AttrCategory:                       "SIGNUP",
				googleads.AttrClickThroughLookbackWindowDays: 120,
			},
			want: "between 1 and 90",
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
			if !strings.Contains(err.Error(), addr.String()) {
				t.Fatalf("error = %q, want address", err)
			}
			assertNoProviderSecret(t, err.Error())
		})
	}
}

func TestValidateConversionActionRequiresCustomerID(t *testing.T) {
	t.Parallel()

	p := googleads.New(googleads.Config{
		DeveloperToken: testDeveloperToken,
		ClientID:       testClientID,
		ClientSecret:   testClientSecret,
		RefreshToken:   testRefreshToken,
		BaseURL:        "https://googleads.example.com",
	})
	err := p.Validate(context.Background(), conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:     "Trial Started",
		googleads.AttrCategory: "SIGNUP",
	}))
	if err == nil {
		t.Fatal("expected customer id error")
	}
	if !strings.Contains(err.Error(), googleads.EnvCustomerID) {
		t.Fatalf("error = %q, want %s", err, googleads.EnvCustomerID)
	}
}

func TestReadConversionActionSuccess(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seed(map[string]any{
		"id":             "11",
		"name":           "Trial Started",
		"category":       "SIGNUP",
		"status":         "ENABLED",
		"type":           "WEBPAGE",
		"origin":         "WEBSITE",
		"countingType":   "ONE_PER_CLICK",
		"primaryForGoal": true,
		"valueSettings": map[string]any{
			"defaultValue":          0.0,
			"alwaysUseDefaultValue": true,
		},
		"clickThroughLookbackWindowDays": "30",
		"viewThroughLookbackWindowDays":  "1",
		"tagSnippets": []any{
			map[string]any{
				"type":         "WEBPAGE",
				"pageFormat":   "HTML",
				"eventSnippet": "gtag('event', 'conversion', {'send_to': 'AW-9988776655/AbC-D_efG-h12_34-567'});",
			},
		},
	})
	p, _ := testConversionActionProvider(t, fake)

	live, err := p.Read(context.Background(), conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:           "Trial Started",
		googleads.AttrCategory:       "SIGNUP",
		googleads.AttrValue:          0,
		googleads.AttrCount:          "ONE",
		googleads.AttrPrimaryForGoal: true,
	}))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "11" {
		t.Fatalf("identity = %q, want 11", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrName] != "Trial Started" {
		t.Fatalf("name = %v", live.Attributes[googleads.AttrName])
	}
	if live.Attributes[googleads.AttrCount] != "ONE" {
		t.Fatalf("count = %v, want ONE", live.Attributes[googleads.AttrCount])
	}
	if live.Computed["id"] != "11" {
		t.Fatalf("computed id = %v", live.Computed["id"])
	}
	if live.Computed["conversionId"] != "9988776655" {
		t.Fatalf("computed conversionId = %v", live.Computed["conversionId"])
	}
	if live.Computed["conversionLabel"] != "AbC-D_efG-h12_34-567" {
		t.Fatalf("computed conversionLabel = %v", live.Computed["conversionLabel"])
	}
	if _, ok := live.Attributes["id"]; ok {
		t.Fatal("id must not appear in comparable attributes")
	}
	if _, ok := live.Attributes["resourceName"]; ok {
		t.Fatal("resourceName must not appear in comparable attributes")
	}
}

func TestReadConversionActionNotFound(t *testing.T) {
	t.Parallel()

	p, _ := testConversionActionProvider(t, newConversionActionFake())
	_, err := p.Read(context.Background(), conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:     "Trial Started",
		googleads.AttrCategory: "SIGNUP",
	}))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadConversionActionDuplicateName(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seed(map[string]any{"id": "1", "name": "Trial Started", "category": "SIGNUP", "type": "WEBPAGE"})
	fake.seed(map[string]any{"id": "4", "name": "Trial Started", "category": "PURCHASE", "type": "WEBPAGE"})
	p, _ := testConversionActionProvider(t, fake)

	_, err := p.Read(context.Background(), conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:     "Trial Started",
		googleads.AttrCategory: "SIGNUP",
	}))
	if err == nil {
		t.Fatal("expected duplicate name error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("duplicate names must not look like not found")
	}
	if !strings.Contains(err.Error(), "multiple remote conversion actions") {
		t.Fatalf("error = %q", err)
	}
}

func TestReadConversionActionBoundIdentity(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seed(map[string]any{"id": "5", "name": "Other", "category": "PURCHASE", "type": "WEBPAGE"})
	fake.seed(map[string]any{"id": "9", "name": "Trial Started", "category": "SIGNUP", "type": "WEBPAGE"})
	p, _ := testConversionActionProvider(t, fake)

	res := conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:     "Trial Started",
		googleads.AttrCategory: "SIGNUP",
	})
	res.Identity = resource.Identity{ID: "5"}
	live, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "5" {
		t.Fatalf("identity = %q, want bound id 5 rather than name match 9", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrName] != "Other" {
		t.Fatalf("name = %v, want identity-bound remote name", live.Attributes[googleads.AttrName])
	}
}

func TestReadConversionActionBoundIdentityMissing(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seed(map[string]any{"id": "9", "name": "Trial Started", "category": "SIGNUP", "type": "WEBPAGE"})
	p, _ := testConversionActionProvider(t, fake)

	res := conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:     "Trial Started",
		googleads.AttrCategory: "SIGNUP",
	})
	res.Identity = resource.Identity{ID: "99"}
	_, err := p.Read(context.Background(), res)
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
}

func TestReadConversionActionRejectsNonWebpage(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seed(map[string]any{"id": "3", "name": "App Install", "category": "DOWNLOAD", "type": "GOOGLE_PLAY_DOWNLOAD"})
	p, _ := testConversionActionProvider(t, fake)

	_, err := p.Read(context.Background(), conversionActionResource(t, "app_install", resource.Attributes{
		googleads.AttrName:     "App Install",
		googleads.AttrCategory: "DOWNLOAD",
	}))
	if err == nil {
		t.Fatal("expected unsupported type error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported type must not look like not found")
	}
	if !strings.Contains(err.Error(), "WEBPAGE") {
		t.Fatalf("error = %q, want WEBPAGE guidance", err)
	}
}

func TestReadConversionActionAPIError(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.searchStatus = http.StatusForbidden
	p, _ := testConversionActionProvider(t, fake)

	_, err := p.Read(context.Background(), conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:     "Trial Started",
		googleads.AttrCategory: "SIGNUP",
	}))
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadConversionActionMalformedResponse(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.searchBody = `{"results":[{"conversionAction":"oops ` + testAccessToken + `"}]}`
	p, _ := testConversionActionProvider(t, fake)

	_, err := p.Read(context.Background(), conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:     "Trial Started",
		googleads.AttrCategory: "SIGNUP",
	}))
	if err == nil {
		t.Fatal("expected malformed response error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("malformed response must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestCreateConversionAction(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	p, _ := testConversionActionProvider(t, fake)
	res := conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:           "Trial Started",
		googleads.AttrCategory:       "SIGNUP",
		googleads.AttrValue:          0,
		googleads.AttrCount:          "ONE",
		googleads.AttrPrimaryForGoal: true,
	})

	live, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID == "" {
		t.Fatal("create returned empty identity")
	}
	if live.Attributes[googleads.AttrName] != "Trial Started" {
		t.Fatalf("name = %v", live.Attributes[googleads.AttrName])
	}
	if live.Computed["type"] != "WEBPAGE" {
		t.Fatalf("type = %v, want WEBPAGE", live.Computed["type"])
	}
	if !strings.Contains(fake.lastMutate, `"type":"WEBPAGE"`) && !strings.Contains(fake.lastMutate, `"type": "WEBPAGE"`) {
		t.Fatalf("create mutate missing WEBPAGE type: %s", fake.lastMutate)
	}

	got, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read after create: %v", err)
	}
	if got.Identity.ID != live.Identity.ID {
		t.Fatalf("read id = %q, want %q", got.Identity.ID, live.Identity.ID)
	}
}

func TestUpdateConversionAction(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seed(map[string]any{
		"id":           "8",
		"name":         "Trial Started",
		"category":     "SIGNUP",
		"type":         "WEBPAGE",
		"countingType": "MANY_PER_CLICK",
		"status":       "ENABLED",
	})
	p, _ := testConversionActionProvider(t, fake)

	desired := conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:     "Trial Started",
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrCount:    "ONE",
		googleads.AttrStatus:   "HIDDEN",
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
	if live.Attributes[googleads.AttrCount] != "ONE" {
		t.Fatalf("count = %v, want ONE", live.Attributes[googleads.AttrCount])
	}
	if live.Attributes[googleads.AttrStatus] != "HIDDEN" {
		t.Fatalf("status = %v, want HIDDEN", live.Attributes[googleads.AttrStatus])
	}
	if !strings.Contains(fake.lastMutate, "updateMask") {
		t.Fatalf("update missing updateMask: %s", fake.lastMutate)
	}
}

func TestCreateConversionActionAPIError(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.mutateStatus = http.StatusBadRequest
	p, _ := testConversionActionProvider(t, fake)

	_, err := p.Create(context.Background(), conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:     "Trial Started",
		googleads.AttrCategory: "SIGNUP",
	}))
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestUpdateConversionActionAPIError(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seed(map[string]any{"id": "8", "name": "Trial Started", "category": "SIGNUP", "type": "WEBPAGE"})
	fake.mutateStatus = http.StatusBadRequest
	p, _ := testConversionActionProvider(t, fake)

	_, err := p.Update(context.Background(), conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:     "Trial Started",
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrCount:    "ONE",
	}), resource.RemoteResource{
		Address:  mustConversionActionAddress(t, "trial_started"),
		Identity: resource.Identity{ID: "8"},
	})
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestImportConversionAction(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seed(map[string]any{
		"id":       "12",
		"name":     "Trial Started",
		"category": "SIGNUP",
		"type":     "WEBPAGE",
		"status":   "ENABLED",
		"tagSnippets": []any{
			map[string]any{"eventSnippet": "gtag('event', 'conversion', {'send_to': 'AW-9988776655/AbC-D_efG-h12_34-567'});"},
		},
	})
	p, _ := testConversionActionProvider(t, fake)
	addr := mustConversionActionAddress(t, "trial_started")

	live, err := p.Import(context.Background(), addr, "12")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if live.Identity.ID != "12" {
		t.Fatalf("identity = %q, want 12", live.Identity.ID)
	}
	if live.Address != addr {
		t.Fatalf("address = %s", live.Address)
	}
	if live.Attributes[googleads.AttrName] != "Trial Started" {
		t.Fatalf("name = %v", live.Attributes[googleads.AttrName])
	}
	for _, key := range []string{"id", "resourceName", "type", "origin", "ownerCustomer", "tagSnippets", "conversionId", "conversionLabel"} {
		if _, ok := live.Attributes[key]; ok {
			t.Fatalf("computed %s leaked into attributes: %#v", key, live.Attributes)
		}
	}
	if live.Computed["id"] != "12" {
		t.Fatalf("computed id = %v", live.Computed["id"])
	}
	if live.Computed["conversionId"] != "9988776655" {
		t.Fatalf("conversionId = %v", live.Computed["conversionId"])
	}

	named, err := p.Import(context.Background(), addr, "customers/"+testCustomerID+"/conversionActions/12")
	if err != nil {
		t.Fatalf("Import by resource name: %v", err)
	}
	if named.Identity.ID != "12" {
		t.Fatalf("resource-name import identity = %q, want numeric 12", named.Identity.ID)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("import mutated remote: %v", fake.mutates)
	}
}

func TestImportConversionActionNotFound(t *testing.T) {
	t.Parallel()

	p, _ := testConversionActionProvider(t, newConversionActionFake())
	_, err := p.Import(context.Background(), mustConversionActionAddress(t, "trial_started"), "12")
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Import = %v, want ErrNotFound", err)
	}
}

func TestNormalizeConversionActionImportID(t *testing.T) {
	t.Parallel()

	p, _ := testConversionActionProvider(t, nil)
	addr := mustConversionActionAddress(t, "trial_started")

	got, err := p.NormalizeImportID(addr, "12")
	if err != nil || got != "12" {
		t.Fatalf("numeric = (%q, %v), want 12", got, err)
	}
	got, err = p.NormalizeImportID(addr, "customers/"+testCustomerID+"/conversionActions/12")
	if err != nil || got != "12" {
		t.Fatalf("resource name = (%q, %v), want 12", got, err)
	}

	_, err = p.NormalizeImportID(addr, "abc")
	if err == nil || !strings.Contains(err.Error(), "not a valid Google Ads conversion action id") {
		t.Fatalf("invalid id error = %v", err)
	}
	_, err = p.NormalizeImportID(addr, "customers/0000000000/conversionActions/12")
	if err == nil || !strings.Contains(err.Error(), "does not match configured") {
		t.Fatalf("wrong customer error = %v", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestImportConversionActionInvalidID(t *testing.T) {
	t.Parallel()

	p, _ := testConversionActionProvider(t, newConversionActionFake())
	_, err := p.Import(context.Background(), mustConversionActionAddress(t, "trial_started"), "abc")
	if err == nil || !strings.Contains(err.Error(), "not a valid Google Ads conversion action id") {
		t.Fatalf("Import = %v, want invalid id", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestImportConversionActionUnsupportedType(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seed(map[string]any{"id": "3", "name": "App Install", "category": "DOWNLOAD", "type": "GOOGLE_PLAY_DOWNLOAD"})
	p, _ := testConversionActionProvider(t, fake)
	_, err := p.Import(context.Background(), mustConversionActionAddress(t, "app_install"), "3")
	if err == nil {
		t.Fatal("expected unsupported type error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported type must not look like not found")
	}
	if !strings.Contains(err.Error(), "WEBPAGE") {
		t.Fatalf("error = %q, want WEBPAGE guidance", err)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("unsupported import mutated remote: %v", fake.mutates)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestImportConversionActionAPIError(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.searchStatus = http.StatusForbidden
	p, _ := testConversionActionProvider(t, fake)
	_, err := p.Import(context.Background(), mustConversionActionAddress(t, "trial_started"), "12")
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestImportConversionActionThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seed(map[string]any{
		"id":             "12",
		"name":           "Trial Started",
		"category":       "SIGNUP",
		"type":           "WEBPAGE",
		"status":         "ENABLED",
		"countingType":   "ONE_PER_CLICK",
		"primaryForGoal": true,
		"valueSettings": map[string]any{
			"defaultValue":          0.0,
			"defaultCurrencyCode":   "USD",
			"alwaysUseDefaultValue": true,
		},
		"tagSnippets": []any{
			map[string]any{"eventSnippet": "gtag('event', 'conversion', {'send_to': 'AW-1/" + testAccessToken + "'});"},
		},
	})
	p, _ := testConversionActionProvider(t, fake)
	addr := mustConversionActionAddress(t, "trial_started")

	st := mustGoogleAdsImportStore(t)
	got, err := importer.Run(context.Background(), addr, "customers/"+testCustomerID+"/conversionActions/12", lookupGoogleAds(p), st)
	if err != nil {
		t.Fatalf("importer.Run: %v", err)
	}
	if got.Identity.ID != "12" {
		t.Fatalf("canonical identity = %q, want 12", got.Identity.ID)
	}
	assertNoProviderSecret(t, got.YAML)
	for _, leak := range []string{"resourceName", "tagSnippets", "conversionId", "conversionLabel", "id:", testAccessToken} {
		if strings.Contains(got.YAML, leak) {
			t.Fatalf("generated YAML leaked %q:\n%s", leak, got.YAML)
		}
	}

	parsed, err := manifest.Parse([]byte(got.YAML), "generated")
	if err != nil {
		t.Fatalf("generated YAML: %v\n%s", err, got.YAML)
	}
	planned, err := plan.BuildWithState(context.Background(), parsed.Resources, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("plan after import: %v", err)
	}
	if planned.HasChanges() {
		t.Fatalf("plan after import has changes: %+v", planned.Changes)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("import/plan mutated remote: %v", fake.mutates)
	}

	again, err := importer.Run(context.Background(), addr, "12", lookupGoogleAds(p), mustGoogleAdsImportStore(t))
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if got.YAML != again.YAML {
		t.Fatalf("YAML differed:\n%s\n---\n%s", got.YAML, again.YAML)
	}
}

func TestPlanConversionActionCreateWhenMissing(t *testing.T) {
	t.Parallel()

	p, _ := testConversionActionProvider(t, newConversionActionFake())
	res := conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:           "Trial Started",
		googleads.AttrCategory:       "SIGNUP",
		googleads.AttrValue:          0,
		googleads.AttrCount:          "ONE",
		googleads.AttrPrimaryForGoal: true,
	})

	got := mustPlanConversionAction(t, p, res)
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionCreate {
		t.Fatalf("change = %+v, want create", got.Changes)
	}
}

func TestPlanConversionActionUnchangedEquivalentRemote(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seed(map[string]any{
		"id":                             "2",
		"name":                           "Trial Started",
		"category":                       "SIGNUP",
		"status":                         "ENABLED",
		"type":                           "WEBPAGE",
		"origin":                         "WEBSITE",
		"countingType":                   "ONE_PER_CLICK",
		"primaryForGoal":                 true,
		"clickThroughLookbackWindowDays": "30",
		"viewThroughLookbackWindowDays":  "1",
		"ownerCustomer":                  "customers/" + testCustomerID,
		"includeInConversionsMetric":     true,
		"valueSettings": map[string]any{
			"defaultValue":          0.0,
			"defaultCurrencyCode":   "USD",
			"alwaysUseDefaultValue": true,
		},
	})
	p, _ := testConversionActionProvider(t, fake)

	res := conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:           "Trial Started",
		googleads.AttrCategory:       "SIGNUP",
		googleads.AttrValue:          0,
		googleads.AttrCount:          "ONE",
		googleads.AttrPrimaryForGoal: true,
	})
	got := mustPlanConversionAction(t, p, res)
	if got.HasChanges() {
		t.Fatalf("equivalent remote produced changes: %+v", got.Changes)
	}
	if got.Changes[0].Identity.ID != "2" {
		t.Fatalf("identity = %q, want 2", got.Changes[0].Identity.ID)
	}
}

func TestPlanConversionActionUpdateConfigurableField(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seed(map[string]any{
		"id":           "2",
		"name":         "Trial Started",
		"category":     "SIGNUP",
		"type":         "WEBPAGE",
		"countingType": "MANY_PER_CLICK",
		"status":       "ENABLED",
	})
	p, _ := testConversionActionProvider(t, fake)

	res := conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:     "Trial Started",
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrCount:    "ONE",
	})
	got := mustPlanConversionAction(t, p, res)
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want update", got.Changes)
	}
	var countDiff *plan.AttributeDiff
	for i := range got.Changes[0].Diffs {
		if got.Changes[0].Diffs[i].Path == googleads.AttrCount {
			countDiff = &got.Changes[0].Diffs[i]
		}
	}
	if countDiff == nil || countDiff.Before != "MANY" || countDiff.After != "ONE" {
		t.Fatalf("count diff = %+v", got.Changes[0].Diffs)
	}
}

func TestPlanConversionActionIgnoresComputedFields(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seed(map[string]any{
		"id":                         "5",
		"name":                       "Trial Started",
		"category":                   "SIGNUP",
		"type":                       "WEBPAGE",
		"origin":                     "WEBSITE",
		"ownerCustomer":              "customers/" + testCustomerID,
		"includeInConversionsMetric": true,
		"tagSnippets": []any{
			map[string]any{"eventSnippet": "gtag('event', 'conversion', {'send_to': 'AW-1/label'});"},
		},
	})
	p, _ := testConversionActionProvider(t, fake)

	res := conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:     "Trial Started",
		googleads.AttrCategory: "SIGNUP",
	})
	got := mustPlanConversionAction(t, p, res)
	if got.HasChanges() {
		t.Fatalf("computed fields produced changes: %+v", got.Changes)
	}
}

func TestPlanConversionActionCreateUsesONEAlias(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seed(map[string]any{
		"id":           "7",
		"name":         "Trial Started",
		"category":     "SIGNUP",
		"type":         "WEBPAGE",
		"countingType": "ONE_PER_CLICK",
	})
	p, _ := testConversionActionProvider(t, fake)

	res := conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:     "Trial Started",
		googleads.AttrCategory: "signup",
		googleads.AttrCount:    "ONE_PER_CLICK",
	})
	got := mustPlanConversionAction(t, p, res)
	if got.HasChanges() {
		t.Fatalf("enum aliases produced changes: %+v", got.Changes)
	}
}

type conversionActionIdentities map[string]resource.Identity

func (m conversionActionIdentities) Identity(addr resource.Address) (resource.Identity, bool, error) {
	id, ok := m[addr.String()]
	return id, ok, nil
}

func TestPlanConversionActionBoundIdentityMissingIsStale(t *testing.T) {
	t.Parallel()

	p, _ := testConversionActionProvider(t, newConversionActionFake())
	res := conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:     "Trial Started",
		googleads.AttrCategory: "SIGNUP",
	})
	_, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, conversionActionIdentities{res.Address.String(): {ID: "99"}})
	if err == nil {
		t.Fatal("expected stale identity error")
	}
	if !strings.Contains(err.Error(), "99") {
		t.Fatalf("error = %q, want persisted identity", err)
	}
}

func mustPlanConversionAction(t *testing.T, p *googleads.Provider, res resource.Resource) *plan.Plan {
	t.Helper()
	got, err := plan.Build(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	return got
}

func mustGoogleAdsImportStore(t *testing.T) *state.Store {
	t.Helper()
	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func lookupGoogleAds(p *googleads.Provider) importer.Lookup {
	return func(resource.Address) (provider.Provider, error) {
		return p, nil
	}
}

func conversionActionResource(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{
		Address:    mustConversionActionAddress(t, name),
		Attributes: attrs,
	}
}

func mustConversionActionAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("googleads.conversion_action." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func mustParseOutputAddr(t *testing.T, s string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress(s)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func assertNoProviderSecret(t *testing.T, s string) {
	t.Helper()
	for _, secret := range []string{testDeveloperToken, testClientID, testClientSecret, testRefreshToken, testAccessToken} {
		if strings.Contains(s, secret) {
			t.Fatalf("secret leaked in %q", s)
		}
	}
}

func testConversionActionProvider(t *testing.T, fake *conversionActionFake) (*googleads.Provider, *httptest.Server) {
	t.Helper()
	if fake == nil {
		fake = newConversionActionFake()
	}
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	t.Cleanup(srv.Close)
	cfg := validConfig(srv.URL)
	cfg.TokenURL = srv.URL + "/oauth/token"
	p := googleads.NewWithHTTPClient(cfg, srv.Client())
	return p, srv
}

type conversionActionFake struct {
	mu sync.Mutex

	nextID int64
	byID   map[string]map[string]any
	goals  map[string]map[string]any

	searchStatus int
	searchBody   string
	mutateStatus int
	mutateBody   string

	lastQuery  string
	lastMutate string
	mutates    []string
}

func newConversionActionFake() *conversionActionFake {
	return &conversionActionFake{
		nextID: 100,
		byID:   map[string]map[string]any{},
		goals:  map[string]map[string]any{},
	}
}

func (f *conversionActionFake) seed(action map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storeLocked(cloneMap(action))
}

func (f *conversionActionFake) seedGoal(goal map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storeGoalLocked(cloneMap(goal))
}

func (f *conversionActionFake) storeLocked(action map[string]any) {
	id := stringify(action["id"])
	if id == "" {
		f.nextID++
		id = strconv.FormatInt(f.nextID, 10)
		action["id"] = id
	}
	if stringify(action["resourceName"]) == "" {
		action["resourceName"] = "customers/" + testCustomerID + "/conversionActions/" + id
	}
	if stringify(action["type"]) == "" {
		action["type"] = "WEBPAGE"
	}
	if stringify(action["origin"]) == "" {
		action["origin"] = "WEBSITE"
	}
	if stringify(action["status"]) == "" {
		action["status"] = "ENABLED"
	}
	if stringify(action["countingType"]) == "" {
		action["countingType"] = "MANY_PER_CLICK"
	}
	if _, ok := action["primaryForGoal"]; !ok {
		action["primaryForGoal"] = true
	}
	if stringify(action["clickThroughLookbackWindowDays"]) == "" {
		action["clickThroughLookbackWindowDays"] = "30"
	}
	if stringify(action["viewThroughLookbackWindowDays"]) == "" {
		action["viewThroughLookbackWindowDays"] = "1"
	}
	f.byID[id] = action
}

func (f *conversionActionFake) storeGoalLocked(goal map[string]any) {
	if f.goals == nil {
		f.goals = map[string]map[string]any{}
	}
	category := strings.ToUpper(strings.TrimSpace(stringify(goal["category"])))
	origin := strings.ToUpper(strings.TrimSpace(stringify(goal["origin"])))
	if origin == "" {
		origin = "WEBSITE"
		goal["origin"] = origin
	}
	id := category + "~" + origin
	if stringify(goal["resourceName"]) == "" {
		goal["resourceName"] = "customers/" + testCustomerID + "/customerConversionGoals/" + id
	}
	if _, ok := goal["biddable"]; !ok {
		goal["biddable"] = true
	}
	goal["category"] = category
	goal["origin"] = origin
	f.goals[id] = goal
}

func (f *conversionActionFake) ensureGoalLocked(category, origin string) {
	category = strings.ToUpper(strings.TrimSpace(category))
	origin = strings.ToUpper(strings.TrimSpace(origin))
	if category == "" {
		return
	}
	if origin == "" {
		origin = "WEBSITE"
	}
	id := category + "~" + origin
	if _, ok := f.goals[id]; ok {
		return
	}
	f.storeGoalLocked(map[string]any{
		"category": category,
		"origin":   origin,
		"biddable": true,
	})
}

func (f *conversionActionFake) searchGoalsLocked(query string) []any {
	var out []any
	for _, goal := range f.goals {
		if matchesCustomerConversionGoalQuery(query, goal) {
			out = append(out, map[string]any{"customerConversionGoal": cloneMap(goal)})
		}
	}
	return out
}

func (f *conversionActionFake) mutateGoalLocked(body []byte) (string, error) {
	var req struct {
		Operations []map[string]any `json:"operations"`
	}
	if err := json.Unmarshal(body, &req); err != nil || len(req.Operations) == 0 {
		return "", errors.New("malformed mutate")
	}
	op := req.Operations[0]
	if _, ok := op["create"]; ok {
		return "", errors.New("unsupported create")
	}
	if _, ok := op["remove"]; ok {
		return "", errors.New("unsupported remove")
	}
	raw, ok := op["update"]
	if !ok {
		return "", errors.New("unsupported mutate")
	}
	goal, _ := raw.(map[string]any)
	resourceName := stringify(goal["resourceName"])
	id := strings.TrimPrefix(resourceName, "customers/"+testCustomerID+"/customerConversionGoals/")
	current, ok := f.goals[id]
	if !ok {
		return "", errors.New("missing customer conversion goal")
	}
	merged := cloneMap(current)
	for k, v := range goal {
		if k == "resourceName" {
			continue
		}
		merged[k] = v
	}
	f.storeGoalLocked(merged)
	return resourceName, nil
}

func matchesCustomerConversionGoalQuery(query string, goal map[string]any) bool {
	category := stringify(goal["category"])
	origin := stringify(goal["origin"])
	if strings.Contains(query, "customer_conversion_goal.category = ") {
		want := extractGAQLString(query, "customer_conversion_goal.category = ")
		if want != "" && want != category {
			return false
		}
	}
	if strings.Contains(query, "customer_conversion_goal.origin = ") {
		want := extractGAQLString(query, "customer_conversion_goal.origin = ")
		if want != "" && want != origin {
			return false
		}
	}
	return true
}

func extractGAQLString(query, prefix string) string {
	start := strings.Index(query, prefix)
	if start < 0 {
		return ""
	}
	rest := strings.TrimSpace(query[start+len(prefix):])
	if rest == "" {
		return ""
	}
	if rest[0] == '\'' {
		rest = rest[1:]
		if i := strings.Index(rest, "'"); i >= 0 {
			rest = rest[:i]
		}
		return rest
	}
	if i := strings.IndexAny(rest, " \n"); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

func (f *conversionActionFake) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	body, _ := io.ReadAll(r.Body)
	if strings.HasSuffix(r.URL.Path, "/oauth/token") {
		writeToken(w)
		return
	}

	if strings.Contains(r.URL.Path, "/googleAds:search") {
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(body, &req)
		f.lastQuery = req.Query
		if f.searchStatus >= 400 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(f.searchStatus)
			_, _ = io.WriteString(w, `{"error":{"code":`+strconv.Itoa(f.searchStatus)+`,"message":"query failed `+testAccessToken+`","status":"PERMISSION_DENIED"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if f.searchBody != "" {
			_, _ = io.WriteString(w, f.searchBody)
			return
		}
		if strings.Contains(strings.ToLower(req.Query), "from customer_conversion_goal") {
			results := f.searchGoalsLocked(req.Query)
			_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
			return
		}
		if strings.Contains(strings.ToLower(req.Query), "from customer") {
			_, _ = io.WriteString(w, `{"results":[{"customer":{"id":"`+testCustomerID+`"}}]}`)
			return
		}
		results := f.searchLocked(req.Query)
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
		return
	}

	if strings.Contains(r.URL.Path, "/customerConversionGoals:mutate") {
		f.lastMutate = string(body)
		f.mutates = append(f.mutates, "customerConversionGoals")
		if f.mutateStatus >= 400 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(f.mutateStatus)
			if f.mutateBody != "" {
				_, _ = io.WriteString(w, f.mutateBody)
				return
			}
			_, _ = io.WriteString(w, `{"error":{"code":400,"message":"mutate failed `+testDeveloperToken+`","status":"INVALID_ARGUMENT"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if f.mutateBody != "" {
			_, _ = io.WriteString(w, f.mutateBody)
			return
		}
		resourceName, err := f.mutateGoalLocked(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []any{map[string]any{"resourceName": resourceName}},
		})
		return
	}

	if strings.Contains(r.URL.Path, "/conversionActions:mutate") {
		f.lastMutate = string(body)
		f.mutates = append(f.mutates, "conversionActions")
		if f.mutateStatus >= 400 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(f.mutateStatus)
			if f.mutateBody != "" {
				_, _ = io.WriteString(w, f.mutateBody)
				return
			}
			_, _ = io.WriteString(w, `{"error":{"code":400,"message":"mutate failed `+testDeveloperToken+`","status":"INVALID_ARGUMENT"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if f.mutateBody != "" {
			_, _ = io.WriteString(w, f.mutateBody)
			return
		}
		resourceName, err := f.mutateLocked(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []any{map[string]any{"resourceName": resourceName}},
		})
		return
	}

	http.NotFound(w, r)
}

func (f *conversionActionFake) searchLocked(query string) []any {
	var out []any
	for _, action := range f.byID {
		if matchesConversionActionQuery(query, action) {
			out = append(out, map[string]any{"conversionAction": cloneMap(action)})
		}
	}
	return out
}

func (f *conversionActionFake) mutateLocked(body []byte) (string, error) {
	var req struct {
		Operations []map[string]any `json:"operations"`
	}
	if err := json.Unmarshal(body, &req); err != nil || len(req.Operations) == 0 {
		return "", errors.New("malformed mutate")
	}
	op := req.Operations[0]
	if raw, ok := op["create"]; ok {
		action, _ := raw.(map[string]any)
		created := cloneMap(action)
		f.nextID++
		id := strconv.FormatInt(f.nextID, 10)
		created["id"] = id
		if stringify(created["countingType"]) == "" {
			created["countingType"] = "MANY_PER_CLICK"
		}
		if _, ok := created["tagSnippets"]; !ok {
			created["tagSnippets"] = []any{
				map[string]any{
					"type":         "WEBPAGE",
					"eventSnippet": "gtag('event', 'conversion', {'send_to': 'AW-1234567890/created-label'});",
				},
			}
		}
		f.storeLocked(created)
		f.ensureGoalLocked(stringify(created["category"]), stringify(created["origin"]))
		return stringify(created["resourceName"]), nil
	}
	if raw, ok := op["update"]; ok {
		action, _ := raw.(map[string]any)
		resourceName := stringify(action["resourceName"])
		id := strings.TrimPrefix(resourceName, "customers/"+testCustomerID+"/conversionActions/")
		current, ok := f.byID[id]
		if !ok {
			return "", errors.New("missing conversion action")
		}
		merged := cloneMap(current)
		for k, v := range action {
			if k == "resourceName" {
				continue
			}
			if k == "valueSettings" {
				src, _ := v.(map[string]any)
				dst, _ := merged["valueSettings"].(map[string]any)
				if dst == nil {
					dst = map[string]any{}
				}
				for sk, sv := range src {
					dst[sk] = sv
				}
				merged["valueSettings"] = dst
				continue
			}
			merged[k] = v
		}
		f.storeLocked(merged)
		return resourceName, nil
	}
	return "", errors.New("unsupported mutate")
}

func matchesConversionActionQuery(query string, action map[string]any) bool {
	id := stringify(action["id"])
	name := stringify(action["name"])
	if strings.Contains(query, "conversion_action.id = ") {
		want := strings.TrimSpace(query[strings.Index(query, "conversion_action.id = ")+len("conversion_action.id = "):])
		if i := strings.IndexAny(want, " \n"); i >= 0 {
			want = want[:i]
		}
		return want == id
	}
	if strings.Contains(query, "conversion_action.name = ") {
		start := strings.Index(query, "conversion_action.name = ") + len("conversion_action.name = ")
		rest := strings.TrimSpace(query[start:])
		rest = strings.Trim(rest, "'")
		return rest == name
	}
	return true
}

func stringify(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatInt(int64(x), 10)
	default:
		return ""
	}
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
