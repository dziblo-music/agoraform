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

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/importer"
	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestValidateCampaignConversionGoalValid(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignConversionGoalProvider(t, nil)
	res := campaignConversionGoalResource(t, "trial_signup", defaultCampaignConversionGoalAttrs(t))
	if err := p.Validate(context.Background(), res); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateCampaignConversionGoalWithConversionActionRef(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignConversionGoalProvider(t, nil)
	res := campaignConversionGoalResource(t, "trial_signup", defaultCampaignConversionGoalAttrs(t))
	res.Attributes[googleads.AttrConversionAction] = conversionActionRef(t, "trial_started")
	if err := p.Validate(context.Background(), res); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateCampaignConversionGoalErrors(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignConversionGoalProvider(t, nil)
	addr := mustCampaignConversionGoalAddress(t, "trial_signup")
	campaign := campaignRef(t, "brand")

	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name:  "missing campaign",
			attrs: resource.Attributes{googleads.AttrCategory: "SIGNUP", googleads.AttrOrigin: "WEBSITE", googleads.AttrBiddable: true},
			want:  "missing required attribute \"campaign\"",
		},
		{
			name: "campaign not a ref",
			attrs: resource.Attributes{
				googleads.AttrCampaign: "googleads.campaign.brand",
				googleads.AttrCategory: "SIGNUP",
				googleads.AttrOrigin:   "WEBSITE",
				googleads.AttrBiddable: true,
			},
			want: "resource reference",
		},
		{
			name: "campaign wrong type",
			attrs: resource.Attributes{
				googleads.AttrCampaign: conversionActionRef(t, "trial_started"),
				googleads.AttrCategory: "SIGNUP",
				googleads.AttrOrigin:   "WEBSITE",
				googleads.AttrBiddable: true,
			},
			want: "googleads.campaign",
		},
		{
			name:  "missing category",
			attrs: resource.Attributes{googleads.AttrCampaign: campaign, googleads.AttrOrigin: "WEBSITE", googleads.AttrBiddable: true},
			want:  "missing required attribute \"category\"",
		},
		{
			name:  "missing origin",
			attrs: resource.Attributes{googleads.AttrCampaign: campaign, googleads.AttrCategory: "SIGNUP", googleads.AttrBiddable: true},
			want:  "missing required attribute \"origin\"",
		},
		{
			name:  "missing biddable",
			attrs: resource.Attributes{googleads.AttrCampaign: campaign, googleads.AttrCategory: "SIGNUP", googleads.AttrOrigin: "WEBSITE"},
			want:  "missing required attribute \"biddable\"",
		},
		{
			name: "unknown category",
			attrs: resource.Attributes{
				googleads.AttrCampaign: campaign,
				googleads.AttrCategory: "PHONE_CALL_LEAD",
				googleads.AttrOrigin:   "WEBSITE",
				googleads.AttrBiddable: true,
			},
			want: "category",
		},
		{
			name: "unsupported origin",
			attrs: resource.Attributes{
				googleads.AttrCampaign: campaign,
				googleads.AttrCategory: "SIGNUP",
				googleads.AttrOrigin:   "APP",
				googleads.AttrBiddable: true,
			},
			want: "WEBSITE",
		},
		{
			name: "computed resourceName",
			attrs: resource.Attributes{
				googleads.AttrCampaign: campaign,
				googleads.AttrCategory: "SIGNUP",
				googleads.AttrOrigin:   "WEBSITE",
				googleads.AttrBiddable: true,
				"resourceName":         "customers/1234567890/campaignConversionGoals/21~SIGNUP~WEBSITE",
			},
			want: "computed",
		},
		{
			name: "computed id",
			attrs: resource.Attributes{
				googleads.AttrCampaign: campaign,
				googleads.AttrCategory: "SIGNUP",
				googleads.AttrOrigin:   "WEBSITE",
				googleads.AttrBiddable: true,
				"id":                   "21~SIGNUP~WEBSITE",
			},
			want: "computed",
		},
		{
			name: "unsupported attribute",
			attrs: resource.Attributes{
				googleads.AttrCampaign: campaign,
				googleads.AttrCategory: "SIGNUP",
				googleads.AttrOrigin:   "WEBSITE",
				googleads.AttrBiddable: true,
				"customGoal":           "signup",
			},
			want: "unsupported attribute",
		},
		{
			name: "biddable not bool",
			attrs: resource.Attributes{
				googleads.AttrCampaign: campaign,
				googleads.AttrCategory: "SIGNUP",
				googleads.AttrOrigin:   "WEBSITE",
				googleads.AttrBiddable: "maybe",
			},
			want: "boolean",
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

func TestValidateCampaignConversionGoalRequiresCustomerID(t *testing.T) {
	t.Parallel()

	p := googleads.New(googleads.Config{
		DeveloperToken: testDeveloperToken,
		ClientID:       testClientID,
		ClientSecret:   testClientSecret,
		RefreshToken:   testRefreshToken,
		BaseURL:        "https://googleads.example.com",
	})
	err := p.Validate(context.Background(), campaignConversionGoalResource(t, "trial_signup", defaultCampaignConversionGoalAttrs(t)))
	if err == nil {
		t.Fatal("expected customer id error")
	}
	if !strings.Contains(err.Error(), googleads.EnvCustomerID) {
		t.Fatalf("error = %q, want %s", err, googleads.EnvCustomerID)
	}
}

func TestReadCampaignConversionGoalSuccess(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	fake.seedGoal(map[string]any{"campaignId": "21", "category": "SIGNUP", "origin": "WEBSITE", "biddable": true})
	p, _ := testCampaignConversionGoalProvider(t, fake)
	bindCampaignIdentity(t, p, "21")

	res := campaignConversionGoalResource(t, "trial_signup", defaultCampaignConversionGoalAttrs(t))
	res.Identity = resource.Identity{ID: "21~SIGNUP~WEBSITE"}
	live, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "21~SIGNUP~WEBSITE" {
		t.Fatalf("identity = %q, want 21~SIGNUP~WEBSITE", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrCategory] != "SIGNUP" {
		t.Fatalf("category = %v", live.Attributes[googleads.AttrCategory])
	}
	if live.Attributes[googleads.AttrOrigin] != "WEBSITE" {
		t.Fatalf("origin = %v", live.Attributes[googleads.AttrOrigin])
	}
	if live.Attributes[googleads.AttrBiddable] != true {
		t.Fatalf("biddable = %v, want true", live.Attributes[googleads.AttrBiddable])
	}
	ref, ok := resource.AsRef(live.Attributes[googleads.AttrCampaign])
	if !ok || ref.Address != mustCampaignAddress(t, "brand") {
		t.Fatalf("campaign = %#v, want $ref to googleads.campaign.brand", live.Attributes[googleads.AttrCampaign])
	}
	if live.Computed["id"] != "21~SIGNUP~WEBSITE" {
		t.Fatalf("computed id = %v", live.Computed["id"])
	}
	if live.Computed["resourceName"] != "customers/"+testCustomerID+"/campaignConversionGoals/21~SIGNUP~WEBSITE" {
		t.Fatalf("computed resourceName = %v", live.Computed["resourceName"])
	}
	if _, ok := live.Attributes["id"]; ok {
		t.Fatal("id must not appear in comparable attributes")
	}
	if _, ok := live.Attributes["resourceName"]; ok {
		t.Fatal("resourceName must not appear in comparable attributes")
	}
	if _, ok := live.Attributes[googleads.AttrConversionAction]; ok {
		t.Fatal("conversionAction must not appear in live comparable attributes")
	}
}

func TestReadCampaignConversionGoalUnboundWithoutCampaignIdentityIsNotFound(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	fake.seedGoal(map[string]any{"campaignId": "21", "category": "SIGNUP", "origin": "WEBSITE", "biddable": true})
	p, _ := testCampaignConversionGoalProvider(t, fake)

	_, err := p.Read(context.Background(), campaignConversionGoalResource(t, "trial_signup", defaultCampaignConversionGoalAttrs(t)))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound when campaign identity is unknown", err)
	}
}

func TestReadCampaignConversionGoalNotFound(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignConversionGoalProvider(t, newCampaignConversionGoalFake())
	_, err := p.Read(context.Background(), campaignConversionGoalResource(t, "trial_signup", resolvedCampaignConversionGoalAttrs(t, "21")))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadCampaignConversionGoalBoundIdentity(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	fake.seedGoal(map[string]any{"campaignId": "21", "category": "PURCHASE", "origin": "WEBSITE", "biddable": false})
	fake.seedGoal(map[string]any{"campaignId": "21", "category": "SIGNUP", "origin": "WEBSITE", "biddable": true})
	p, _ := testCampaignConversionGoalProvider(t, fake)
	bindCampaignIdentity(t, p, "21")

	res := campaignConversionGoalResource(t, "purchase", resource.Attributes{
		googleads.AttrCampaign: campaignRef(t, "brand"),
		googleads.AttrCategory: "PURCHASE",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: false,
	})
	res.Identity = resource.Identity{ID: "21~PURCHASE~WEBSITE"}
	live, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "21~PURCHASE~WEBSITE" {
		t.Fatalf("identity = %q, want bound 21~PURCHASE~WEBSITE", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrBiddable] != false {
		t.Fatalf("biddable = %v, want identity-bound false", live.Attributes[googleads.AttrBiddable])
	}
}

func TestReadCampaignConversionGoalBoundIdentityMismatch(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	fake.seedGoal(map[string]any{"campaignId": "21", "category": "SIGNUP", "origin": "WEBSITE", "biddable": true})
	p, _ := testCampaignConversionGoalProvider(t, fake)

	res := campaignConversionGoalResource(t, "trial_signup", resource.Attributes{
		googleads.AttrCampaign: campaignRef(t, "brand"),
		googleads.AttrCategory: "PURCHASE",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: true,
	})
	res.Identity = resource.Identity{ID: "21~SIGNUP~WEBSITE"}
	_, err := p.Read(context.Background(), res)
	if err == nil {
		t.Fatal("expected identity mismatch")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("identity mismatch must not look like not found")
	}
	if !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("error = %q, want identity mismatch", err)
	}
}

func TestReadCampaignConversionGoalAPIError(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	fake.searchStatus = http.StatusForbidden
	p, _ := testCampaignConversionGoalProvider(t, fake)

	_, err := p.Read(context.Background(), campaignConversionGoalResource(t, "trial_signup", resolvedCampaignConversionGoalAttrs(t, "21")))
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadCampaignConversionGoalMalformedResponse(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	fake.searchBody = `{"results":[{"campaignConversionGoal":"oops ` + testAccessToken + `"}]}`
	p, _ := testCampaignConversionGoalProvider(t, fake)

	_, err := p.Read(context.Background(), campaignConversionGoalResource(t, "trial_signup", resolvedCampaignConversionGoalAttrs(t, "21")))
	if err == nil {
		t.Fatal("expected malformed response error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("malformed response must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadCampaignConversionGoalRejectsUnsupportedOrigin(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	fake.searchBody = `{"results":[{"campaignConversionGoal":{"resourceName":"customers/` + testCustomerID + `/campaignConversionGoals/21~SIGNUP~APP","campaign":"customers/` + testCustomerID + `/campaigns/21","category":"SIGNUP","origin":"APP","biddable":true}}]}`
	p, _ := testCampaignConversionGoalProvider(t, fake)

	_, err := p.Read(context.Background(), campaignConversionGoalResource(t, "trial_signup", resolvedCampaignConversionGoalAttrs(t, "21")))
	if err == nil {
		t.Fatal("expected unsupported origin error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported origin must not look like not found")
	}
	if !strings.Contains(err.Error(), "WEBSITE") {
		t.Fatalf("error = %q, want WEBSITE guidance", err)
	}
}

func TestCreateCampaignConversionGoalAdoptsProviderCreatedGoal(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	fake.seedGoal(map[string]any{"campaignId": "21", "category": "SIGNUP", "origin": "WEBSITE", "biddable": false})
	p, _ := testCampaignConversionGoalProvider(t, fake)

	live, err := p.Create(context.Background(), campaignConversionGoalResource(t, "trial_signup", resolvedCampaignConversionGoalAttrs(t, "21")))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID != "21~SIGNUP~WEBSITE" {
		t.Fatalf("identity = %q", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrBiddable] != true {
		t.Fatalf("biddable = %v, want true", live.Attributes[googleads.AttrBiddable])
	}
	assertGoalMutateUpdateOnly(t, fake.lastMutate)
}

func TestCreateCampaignConversionGoalNoOpWhenBiddableMatches(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	fake.seedGoal(map[string]any{"campaignId": "21", "category": "SIGNUP", "origin": "WEBSITE", "biddable": true})
	p, _ := testCampaignConversionGoalProvider(t, fake)

	attrs := resolvedCampaignConversionGoalAttrs(t, "21")
	attrs[googleads.AttrCategory] = "signup"
	attrs[googleads.AttrOrigin] = "website"
	live, err := p.Create(context.Background(), campaignConversionGoalResource(t, "trial_signup", attrs))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID != "21~SIGNUP~WEBSITE" {
		t.Fatalf("identity = %q", live.Identity.ID)
	}
	if fake.lastMutate != "" {
		t.Fatalf("matching biddable still mutated: %s", fake.lastMutate)
	}
}

func TestCreateCampaignConversionGoalMissingIsActionable(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	p, _ := testCampaignConversionGoalProvider(t, fake)

	_, err := p.Create(context.Background(), campaignConversionGoalResource(t, "trial_signup", resolvedCampaignConversionGoalAttrs(t, "21")))
	if err == nil {
		t.Fatal("expected missing goal error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("missing provider-created goal must not look like a create candidate")
	}
	if !strings.Contains(err.Error(), "cannot create or delete") {
		t.Fatalf("error = %q, want cannot create or delete guidance", err)
	}
	if !strings.Contains(err.Error(), "conversion_action") {
		t.Fatalf("error = %q, want conversion action guidance", err)
	}
	if !strings.Contains(err.Error(), "campaign") {
		t.Fatalf("error = %q, want campaign guidance", err)
	}
	if fake.lastMutate != "" {
		t.Fatalf("missing goal sent mutate: %s", fake.lastMutate)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestCreateCampaignConversionGoalRejectsBoundIdentity(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignConversionGoalProvider(t, newCampaignConversionGoalFake())
	res := campaignConversionGoalResource(t, "trial_signup", resolvedCampaignConversionGoalAttrs(t, "21"))
	res.Identity = resource.Identity{ID: "21~SIGNUP~WEBSITE"}
	_, err := p.Create(context.Background(), res)
	if err == nil {
		t.Fatal("expected bound identity error")
	}
	if !strings.Contains(err.Error(), "persisted identity") {
		t.Fatalf("error = %q", err)
	}
}

func TestUpdateCampaignConversionGoalBiddable(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	fake.seedGoal(map[string]any{"campaignId": "21", "category": "SIGNUP", "origin": "WEBSITE", "biddable": true})
	p, _ := testCampaignConversionGoalProvider(t, fake)
	bindCampaignIdentity(t, p, "21")

	desired := campaignConversionGoalResource(t, "trial_signup", resource.Attributes{
		googleads.AttrCampaign: campaignRef(t, "brand"),
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: false,
	})
	live, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "21~SIGNUP~WEBSITE"},
		Attributes: resource.Attributes{
			googleads.AttrCampaign: campaignRef(t, "brand"),
			googleads.AttrCategory: "SIGNUP",
			googleads.AttrOrigin:   "WEBSITE",
			googleads.AttrBiddable: true,
		},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if live.Attributes[googleads.AttrBiddable] != false {
		t.Fatalf("biddable = %v, want false", live.Attributes[googleads.AttrBiddable])
	}
	assertGoalMutateUpdateOnly(t, fake.lastMutate)
	if !strings.Contains(fake.lastMutate, "biddable") {
		t.Fatalf("update missing biddable: %s", fake.lastMutate)
	}
}

func TestUpdateCampaignConversionGoalNoOp(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	fake.seedGoal(map[string]any{"campaignId": "21", "category": "SIGNUP", "origin": "WEBSITE", "biddable": true})
	p, _ := testCampaignConversionGoalProvider(t, fake)
	bindCampaignIdentity(t, p, "21")

	desired := campaignConversionGoalResource(t, "trial_signup", defaultCampaignConversionGoalAttrs(t))
	live, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "21~SIGNUP~WEBSITE"},
		Attributes: resource.Attributes{
			googleads.AttrCampaign: campaignRef(t, "brand"),
			googleads.AttrCategory: "SIGNUP",
			googleads.AttrOrigin:   "WEBSITE",
			googleads.AttrBiddable: true,
		},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if live.Attributes[googleads.AttrBiddable] != true {
		t.Fatalf("biddable = %v", live.Attributes[googleads.AttrBiddable])
	}
	if fake.lastMutate != "" {
		t.Fatalf("no-op update mutated: %s", fake.lastMutate)
	}
}

func TestUpdateCampaignConversionGoalAPIError(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	fake.seedGoal(map[string]any{"campaignId": "21", "category": "SIGNUP", "origin": "WEBSITE", "biddable": true})
	fake.mutateStatus = http.StatusBadRequest
	p, _ := testCampaignConversionGoalProvider(t, fake)
	bindCampaignIdentity(t, p, "21")

	_, err := p.Update(context.Background(), campaignConversionGoalResource(t, "trial_signup", resource.Attributes{
		googleads.AttrCampaign: campaignRef(t, "brand"),
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: false,
	}), resource.RemoteResource{
		Address:  mustCampaignConversionGoalAddress(t, "trial_signup"),
		Identity: resource.Identity{ID: "21~SIGNUP~WEBSITE"},
		Attributes: resource.Attributes{
			googleads.AttrCampaign: campaignRef(t, "brand"),
			googleads.AttrCategory: "SIGNUP",
			googleads.AttrOrigin:   "WEBSITE",
			googleads.AttrBiddable: true,
		},
	})
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestImportCampaignConversionGoalRequiresBoundCampaign(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	fake.seedGoal(map[string]any{"campaignId": "21", "category": "SIGNUP", "origin": "WEBSITE", "biddable": true})
	p, _ := testCampaignConversionGoalProvider(t, fake)
	_, err := p.Import(context.Background(), mustCampaignConversionGoalAddress(t, "trial_signup"), "21~SIGNUP~WEBSITE")
	if err == nil {
		t.Fatal("expected missing campaign binding error")
	}
	if !strings.Contains(err.Error(), "googleads.campaign") {
		t.Fatalf("error = %q, want campaign import guidance", err)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("import mutated remote: %v", fake.mutates)
	}
}

func TestImportCampaignConversionGoal(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	fake.seedGoal(map[string]any{"campaignId": "21", "category": "SIGNUP", "origin": "WEBSITE", "biddable": true})
	p, _ := testCampaignConversionGoalProvider(t, fake)
	bindCampaignIdentity(t, p, "21")
	addr := mustCampaignConversionGoalAddress(t, "trial_signup")

	live, err := p.Import(context.Background(), addr, "21~SIGNUP~WEBSITE")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if live.Identity.ID != "21~SIGNUP~WEBSITE" {
		t.Fatalf("identity = %q", live.Identity.ID)
	}
	ref, ok := resource.AsRef(live.Attributes[googleads.AttrCampaign])
	if !ok || ref.Address != mustCampaignAddress(t, "brand") {
		t.Fatalf("campaign = %#v, want reconstructed $ref", live.Attributes[googleads.AttrCampaign])
	}
	if live.Attributes[googleads.AttrCategory] != "SIGNUP" || live.Attributes[googleads.AttrOrigin] != "WEBSITE" {
		t.Fatalf("attributes = %#v", live.Attributes)
	}
	if _, ok := live.Attributes[googleads.AttrConversionAction]; ok {
		t.Fatal("conversionAction $ref must not be reconstructed by import")
	}

	named, err := p.Import(context.Background(), addr, "customers/"+testCustomerID+"/campaignConversionGoals/21~SIGNUP~WEBSITE")
	if err != nil {
		t.Fatalf("Import by resource name: %v", err)
	}
	if named.Identity.ID != "21~SIGNUP~WEBSITE" {
		t.Fatalf("resource-name import identity = %q", named.Identity.ID)
	}

	normalized, err := p.Import(context.Background(), addr, "21~signup~website")
	if err != nil {
		t.Fatalf("Import normalized: %v", err)
	}
	if normalized.Identity.ID != "21~SIGNUP~WEBSITE" {
		t.Fatalf("normalized import identity = %q", normalized.Identity.ID)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("import mutated remote: %v", fake.mutates)
	}
}

func TestNormalizeCampaignConversionGoalImportID(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignConversionGoalProvider(t, nil)
	addr := mustCampaignConversionGoalAddress(t, "trial_signup")

	got, err := p.NormalizeImportID(addr, "21~signup~website")
	if err != nil || got != "21~SIGNUP~WEBSITE" {
		t.Fatalf("case alias = (%q, %v), want 21~SIGNUP~WEBSITE", got, err)
	}
	got, err = p.NormalizeImportID(addr, "customers/"+testCustomerID+"/campaignConversionGoals/21~SIGNUP~WEBSITE")
	if err != nil || got != "21~SIGNUP~WEBSITE" {
		t.Fatalf("resource name = (%q, %v), want 21~SIGNUP~WEBSITE", got, err)
	}

	_, err = p.NormalizeImportID(addr, "not-an-id")
	if err == nil || !strings.Contains(err.Error(), "CAMPAIGN_ID~CATEGORY~ORIGIN") {
		t.Fatalf("invalid id error = %v", err)
	}
	_, err = p.NormalizeImportID(addr, "21~SIGNUP~APP")
	if err == nil || !strings.Contains(err.Error(), "WEBSITE") {
		t.Fatalf("unsupported origin error = %v", err)
	}
	_, err = p.NormalizeImportID(addr, "customers/0000000000/campaignConversionGoals/21~SIGNUP~WEBSITE")
	if err == nil || !strings.Contains(err.Error(), "does not match configured") {
		t.Fatalf("wrong customer error = %v", err)
	}
}

func TestImportCampaignConversionGoalNotFound(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignConversionGoalProvider(t, newCampaignConversionGoalFake())
	_, err := p.Import(context.Background(), mustCampaignConversionGoalAddress(t, "trial_signup"), "21~SIGNUP~WEBSITE")
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Import = %v, want ErrNotFound", err)
	}
}

func TestImportCampaignConversionGoalUnsupportedOrigin(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignConversionGoalProvider(t, newCampaignConversionGoalFake())
	_, err := p.Import(context.Background(), mustCampaignConversionGoalAddress(t, "trial_signup"), "21~SIGNUP~APP")
	if err == nil {
		t.Fatal("expected unsupported origin error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported origin must not look like not found")
	}
	if !strings.Contains(err.Error(), "WEBSITE") {
		t.Fatalf("error = %q, want WEBSITE guidance", err)
	}
}

func TestImportCampaignConversionGoalThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	fake.seedBudget(map[string]any{
		"id":               "11",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
	})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
	fake.seedGoal(map[string]any{"campaignId": "21", "category": "SIGNUP", "origin": "WEBSITE", "biddable": true})
	p, _ := testCampaignConversionGoalProvider(t, fake)

	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignBudgetAddress(t, "brand"), resource.Identity{ID: "11"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	got, err := importer.Run(context.Background(), mustCampaignConversionGoalAddress(t, "trial_signup"), "21~signup~website", lookupGoogleAds(p), st)
	if err != nil {
		t.Fatalf("importer.Run: %v", err)
	}
	if got.Identity.ID != "21~SIGNUP~WEBSITE" {
		t.Fatalf("canonical identity = %q", got.Identity.ID)
	}
	if strings.Contains(got.YAML, "resourceName") || strings.Contains(got.YAML, "conversionAction") {
		t.Fatalf("generated YAML leaked computed or reconstructed fields:\n%s", got.YAML)
	}
	if !strings.Contains(got.YAML, "$ref: googleads.campaign.brand") {
		t.Fatalf("generated YAML missing campaign $ref:\n%s", got.YAML)
	}

	combined := campaignStackYAML(t) + strings.TrimPrefix(got.YAML, "apiVersion: agoraform.io/v1alpha1\nresources:\n")
	parsed, err := manifest.Parse([]byte(combined), "generated")
	if err != nil {
		t.Fatalf("generated YAML: %v\n%s", err, combined)
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
}

func TestPlanCampaignConversionGoalCreateWhenMissing(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	seedCampaignStack(fake)
	p, _ := testCampaignConversionGoalProvider(t, fake)

	got := mustPlanCampaignConversionGoalStack(t, p, defaultCampaignConversionGoalAttrs(t))
	goalChange := changeFor(t, got, mustCampaignConversionGoalAddress(t, "trial_signup"))
	if goalChange.Action != plan.ActionCreate || goalChange.Operation != string(provider.MissingResourceAdopt) {
		t.Fatalf("goal change = %+v, want adopt", goalChange)
	}
}

func TestPlanCampaignConversionGoalUnchangedEquivalentRemote(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	seedCampaignStack(fake)
	fake.seedGoal(map[string]any{"campaignId": "21", "category": "SIGNUP", "origin": "WEBSITE", "biddable": true})
	p, _ := testCampaignConversionGoalProvider(t, fake)

	attrs := defaultCampaignConversionGoalAttrs(t)
	attrs[googleads.AttrCategory] = "signup"
	attrs[googleads.AttrOrigin] = "website"
	got := mustPlanCampaignConversionGoalStack(t, p, attrs)
	if got.HasChanges() {
		t.Fatalf("equivalent remote produced changes: %+v", got.Changes)
	}
	goalChange := changeFor(t, got, mustCampaignConversionGoalAddress(t, "trial_signup"))
	if goalChange.Identity.ID != "21~SIGNUP~WEBSITE" {
		t.Fatalf("identity = %q", goalChange.Identity.ID)
	}
}

func TestNormalizeCampaignConversionGoalOmitsConversionActionRef(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignConversionGoalProvider(t, nil)
	desired := campaignConversionGoalResource(t, "trial_signup", defaultCampaignConversionGoalAttrs(t))
	desired.Attributes[googleads.AttrConversionAction] = conversionActionRef(t, "trial_started")
	live := resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "21~SIGNUP~WEBSITE"},
		Attributes: resource.Attributes{
			googleads.AttrCampaign: campaignRef(t, "brand"),
			googleads.AttrCategory: "SIGNUP",
			googleads.AttrOrigin:   "WEBSITE",
			googleads.AttrBiddable: true,
		},
	}
	want, got, err := p.NormalizeComparable(desired, &live)
	if err != nil {
		t.Fatalf("NormalizeComparable: %v", err)
	}
	if _, ok := want[googleads.AttrConversionAction]; ok {
		t.Fatal("conversionAction $ref must not be comparable desired state")
	}
	if _, ok := got[googleads.AttrConversionAction]; ok {
		t.Fatal("conversionAction $ref must not be comparable live state")
	}
}

func TestPlanCampaignConversionGoalUpdateBiddable(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	seedCampaignStack(fake)
	fake.seedGoal(map[string]any{"campaignId": "21", "category": "SIGNUP", "origin": "WEBSITE", "biddable": false})
	p, _ := testCampaignConversionGoalProvider(t, fake)

	got := mustPlanCampaignConversionGoalStack(t, p, defaultCampaignConversionGoalAttrs(t))
	goalChange := changeFor(t, got, mustCampaignConversionGoalAddress(t, "trial_signup"))
	if goalChange.Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want update", goalChange)
	}
}

func TestPlanCampaignConversionGoalBoundIdentityMissingIsStale(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	seedCampaignStack(fake)
	p, _ := testCampaignConversionGoalProvider(t, fake)

	resources := campaignConversionGoalStack(t, defaultCampaignConversionGoalAttrs(t))
	_, err := plan.BuildWithState(context.Background(), resources, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, campaignConversionGoalIdentities{
		mustCampaignConversionGoalAddress(t, "trial_signup").String(): {ID: "21~SIGNUP~WEBSITE"},
	})
	if err == nil {
		t.Fatal("expected stale identity error")
	}
	if !strings.Contains(err.Error(), "21~SIGNUP~WEBSITE") {
		t.Fatalf("error = %q, want persisted identity", err)
	}
}

func TestApplyCampaignConversionGoalAfterCampaignAndConversionAction(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	seedCampaignStack(fake)
	p, _ := testCampaignConversionGoalProvider(t, fake)

	resources := append(campaignConversionGoalStack(t, resource.Attributes{
		googleads.AttrCampaign:         campaignRef(t, "brand"),
		googleads.AttrCategory:         "SIGNUP",
		googleads.AttrOrigin:           "WEBSITE",
		googleads.AttrBiddable:         false,
		googleads.AttrConversionAction: conversionActionRef(t, "trial_started"),
	}), conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:     "Trial Started",
		googleads.AttrCategory: "SIGNUP",
	}))
	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}

	result, err := apply.Run(context.Background(), resources, func(resource.Address) (provider.Provider, error) {
		return p, nil
	}, st, nil)
	if err != nil {
		t.Fatalf("apply.Run: %v", err)
	}
	if result.Created != 2 {
		t.Fatalf("result = %+v, want conversion action create and goal adopt", result)
	}

	if len(fake.mutates) < 2 || fake.mutates[0] != "conversionActions" {
		t.Fatalf("mutates = %v, want conversion action before goal", fake.mutates)
	}
	goalMutates := 0
	for _, collection := range fake.mutates {
		if collection == "campaignConversionGoals" {
			goalMutates++
		}
		if collection == "campaigns" || collection == "campaignBudgets" {
			t.Fatalf("seeded campaign stack was mutated: %v", fake.mutates)
		}
	}
	if goalMutates != 1 {
		t.Fatalf("campaignConversionGoals mutates = %d, want 1", goalMutates)
	}
	assertGoalMutateUpdateOnly(t, fake.lastMutate)

	goal := campaignConversionGoalResource(t, "trial_signup", defaultCampaignConversionGoalAttrs(t))
	live, err := p.Read(context.Background(), goal)
	if err != nil {
		t.Fatalf("Read after apply: %v", err)
	}
	if live.Attributes[googleads.AttrBiddable] != false {
		t.Fatalf("biddable = %v, want false after adopt+update", live.Attributes[googleads.AttrBiddable])
	}

	second, err := plan.BuildWithState(context.Background(), resources, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("second plan: %v", err)
	}
	if second.HasChanges() {
		t.Fatalf("second plan has changes: %+v", second.Changes)
	}
}

func TestApplyCampaignConversionGoalOrdersDependenciesFirst(t *testing.T) {
	t.Parallel()

	fake := newCampaignConversionGoalFake()
	seedCampaignStack(fake)
	p, _ := testCampaignConversionGoalProvider(t, fake)
	resources := append(campaignConversionGoalStack(t, resource.Attributes{
		googleads.AttrCampaign:         campaignRef(t, "brand"),
		googleads.AttrCategory:         "SIGNUP",
		googleads.AttrOrigin:           "WEBSITE",
		googleads.AttrBiddable:         true,
		googleads.AttrConversionAction: conversionActionRef(t, "trial_started"),
	}), conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:     "Trial Started",
		googleads.AttrCategory: "SIGNUP",
	}))
	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := apply.Run(context.Background(), resources, func(resource.Address) (provider.Provider, error) {
		return p, nil
	}, st, nil); err != nil {
		t.Fatalf("apply.Run: %v", err)
	}
	if len(fake.mutates) == 0 || fake.mutates[0] != "conversionActions" {
		t.Fatalf("mutates = %v, want conversionActions first", fake.mutates)
	}
}

func defaultCampaignConversionGoalAttrs(t *testing.T) resource.Attributes {
	t.Helper()
	return resource.Attributes{
		googleads.AttrCampaign: campaignRef(t, "brand"),
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: true,
	}
}

func resolvedCampaignConversionGoalAttrs(t *testing.T, campaignID string) resource.Attributes {
	t.Helper()
	attrs := defaultCampaignConversionGoalAttrs(t)
	attrs[googleads.AttrCampaign] = resource.Resolved{
		Address:  mustCampaignAddress(t, "brand"),
		Identity: resource.Identity{ID: campaignID},
		Outputs:  resource.Attributes{"resourceName": "customers/" + testCustomerID + "/campaigns/" + campaignID},
	}
	return attrs
}

func campaignConversionGoalResource(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{
		Address:    mustCampaignConversionGoalAddress(t, name),
		Attributes: attrs,
	}
}

func mustCampaignConversionGoalAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("googleads.campaign_conversion_goal." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func campaignRef(t *testing.T, name string) resource.Ref {
	t.Helper()
	return resource.Ref{Address: mustCampaignAddress(t, name)}
}

func bindCampaignIdentity(t *testing.T, p *googleads.Provider, campaignID string) {
	t.Helper()
	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: campaignID}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)
}

func campaignConversionGoalStack(t *testing.T, goalAttrs resource.Attributes) []resource.Resource {
	t.Helper()
	return []resource.Resource{
		campaignBudgetResource(t, "brand", resource.Attributes{
			googleads.AttrName:             "Brand daily budget",
			googleads.AttrAmount:           50,
			googleads.AttrExplicitlyShared: false,
		}),
		campaignResource(t, "brand", defaultCampaignAttrs(t)),
		campaignConversionGoalResource(t, "trial_signup", goalAttrs),
	}
}

func campaignStackYAML(t *testing.T) string {
	t.Helper()
	return `apiVersion: agoraform.io/v1alpha1
resources:
  - address: googleads.campaign_budget.brand
    attributes:
      name: Brand daily budget
      amount: 50
      explicitlyShared: false
  - address: googleads.campaign.brand
    attributes:
      name: Brand
      budget:
        $ref: googleads.campaign_budget.brand
      bidding:
        strategy: MANUAL_CPC
`
}

func seedCampaignStack(fake *campaignConversionGoalFake) {
	fake.seedBudget(map[string]any{
		"id":               "11",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
	})
	fake.seedCampaign(sampleSearchCampaign("21", "Brand", "11"))
}

func mustPlanCampaignConversionGoalStack(t *testing.T, p *googleads.Provider, goalAttrs resource.Attributes) *plan.Plan {
	t.Helper()
	got, err := plan.Build(context.Background(), campaignConversionGoalStack(t, goalAttrs), func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	return got
}

func changeFor(t *testing.T, got *plan.Plan, addr resource.Address) plan.Change {
	t.Helper()
	for _, change := range got.Changes {
		if change.Address == addr {
			return change
		}
	}
	t.Fatalf("missing change for %s: %+v", addr, got.Changes)
	return plan.Change{}
}

type campaignConversionGoalIdentities map[string]resource.Identity

func (m campaignConversionGoalIdentities) Identity(addr resource.Address) (resource.Identity, bool, error) {
	id, ok := m[addr.String()]
	return id, ok, nil
}

func testCampaignConversionGoalProvider(t *testing.T, fake *campaignConversionGoalFake) (*googleads.Provider, *httptest.Server) {
	t.Helper()
	if fake == nil {
		fake = newCampaignConversionGoalFake()
	}
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	t.Cleanup(srv.Close)
	cfg := validConfig(srv.URL)
	cfg.TokenURL = srv.URL + "/oauth/token"
	p := googleads.NewWithHTTPClient(cfg, srv.Client())
	return p, srv
}

type campaignConversionGoalFake struct {
	mu sync.Mutex

	nextCampaignID int64
	nextBudgetID   int64
	nextActionID   int64
	goals          map[string]map[string]any
	campaigns      map[string]map[string]any
	budgets        map[string]map[string]any
	actions        map[string]map[string]any

	searchStatus int
	searchBody   string
	mutateStatus int
	mutateBody   string

	lastQuery  string
	lastMutate string
	mutates    []string
}

func newCampaignConversionGoalFake() *campaignConversionGoalFake {
	return &campaignConversionGoalFake{
		nextCampaignID: 20,
		nextBudgetID:   10,
		nextActionID:   100,
		goals:          map[string]map[string]any{},
		campaigns:      map[string]map[string]any{},
		budgets:        map[string]map[string]any{},
		actions:        map[string]map[string]any{},
	}
}

func (f *campaignConversionGoalFake) seedGoal(goal map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storeGoalLocked(cloneMap(goal))
}

func (f *campaignConversionGoalFake) seedCampaign(campaign map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storeCampaignLocked(cloneMap(campaign))
}

func (f *campaignConversionGoalFake) seedBudget(budget map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cloned := cloneMap(budget)
	id := stringify(cloned["id"])
	if stringify(cloned["resourceName"]) == "" {
		cloned["resourceName"] = "customers/" + testCustomerID + "/campaignBudgets/" + id
	}
	if stringify(cloned["period"]) == "" {
		cloned["period"] = "DAILY"
	}
	if stringify(cloned["type"]) == "" {
		cloned["type"] = "STANDARD"
	}
	f.budgets[id] = cloned
}

func (f *campaignConversionGoalFake) storeGoalLocked(goal map[string]any) {
	campaignID := stringify(goal["campaignId"])
	if campaignID == "" {
		campaignID = strings.TrimPrefix(stringify(goal["campaign"]), "customers/"+testCustomerID+"/campaigns/")
	}
	category := strings.ToUpper(strings.TrimSpace(stringify(goal["category"])))
	origin := strings.ToUpper(strings.TrimSpace(stringify(goal["origin"])))
	if origin == "" {
		origin = "WEBSITE"
		goal["origin"] = origin
	}
	id := campaignID + "~" + category + "~" + origin
	if stringify(goal["resourceName"]) == "" {
		goal["resourceName"] = "customers/" + testCustomerID + "/campaignConversionGoals/" + id
	}
	if stringify(goal["campaign"]) == "" {
		goal["campaign"] = "customers/" + testCustomerID + "/campaigns/" + campaignID
	}
	if _, ok := goal["biddable"]; !ok {
		goal["biddable"] = true
	}
	goal["campaignId"] = campaignID
	goal["category"] = category
	goal["origin"] = origin
	f.goals[id] = goal
}

func (f *campaignConversionGoalFake) storeCampaignLocked(campaign map[string]any) {
	id := stringify(campaign["id"])
	if stringify(campaign["resourceName"]) == "" {
		campaign["resourceName"] = "customers/" + testCustomerID + "/campaigns/" + id
	}
	if stringify(campaign["advertisingChannelType"]) == "" {
		campaign["advertisingChannelType"] = "SEARCH"
	}
	if stringify(campaign["status"]) == "" {
		campaign["status"] = "PAUSED"
	}
	f.campaigns[id] = campaign
	f.ensureCampaignGoalsLocked(id)
}

func (f *campaignConversionGoalFake) storeActionLocked(action map[string]any) {
	id := stringify(action["id"])
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
	f.actions[id] = action
	f.ensureActionGoalsLocked(stringify(action["category"]), stringify(action["origin"]))
}

func (f *campaignConversionGoalFake) ensureCampaignGoalsLocked(campaignID string) {
	for _, action := range f.actions {
		f.ensureGoalLocked(campaignID, stringify(action["category"]), stringify(action["origin"]))
	}
}

func (f *campaignConversionGoalFake) ensureActionGoalsLocked(category, origin string) {
	for campaignID := range f.campaigns {
		f.ensureGoalLocked(campaignID, category, origin)
	}
}

func (f *campaignConversionGoalFake) ensureGoalLocked(campaignID, category, origin string) {
	category = strings.ToUpper(strings.TrimSpace(category))
	origin = strings.ToUpper(strings.TrimSpace(origin))
	if campaignID == "" || category == "" {
		return
	}
	if origin == "" {
		origin = "WEBSITE"
	}
	id := campaignID + "~" + category + "~" + origin
	if _, ok := f.goals[id]; ok {
		return
	}
	f.storeGoalLocked(map[string]any{
		"campaignId": campaignID,
		"category":   category,
		"origin":     origin,
		"biddable":   true,
	})
}

func (f *campaignConversionGoalFake) handler(w http.ResponseWriter, r *http.Request) {
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
		query := strings.ToLower(req.Query)
		switch {
		case strings.Contains(query, "from campaign_conversion_goal"):
			_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchGoalsLocked(req.Query)})
		case strings.Contains(query, "from conversion_action"):
			_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchActionsLocked(req.Query)})
		case strings.Contains(query, "from campaign_budget"):
			_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchBudgetsLocked(req.Query)})
		case strings.Contains(query, "from campaign"):
			_ = json.NewEncoder(w).Encode(map[string]any{"results": f.searchCampaignsLocked(req.Query)})
		case strings.Contains(query, "from customer"):
			_, _ = io.WriteString(w, `{"results":[{"customer":{"id":"`+testCustomerID+`"}}]}`)
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
		}
		return
	}
	if strings.Contains(r.URL.Path, ":mutate") {
		f.lastMutate = string(body)
		collection := mutateCollection(r.URL.Path)
		f.mutates = append(f.mutates, collection)
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
		resourceName, err := f.mutateLocked(collection, body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"resourceName": resourceName}}})
		return
	}
	http.NotFound(w, r)
}

func mutateCollection(path string) string {
	switch {
	case strings.Contains(path, "/campaignConversionGoals:mutate"):
		return "campaignConversionGoals"
	case strings.Contains(path, "/conversionActions:mutate"):
		return "conversionActions"
	case strings.Contains(path, "/campaignBudgets:mutate"):
		return "campaignBudgets"
	case strings.Contains(path, "/campaigns:mutate"):
		return "campaigns"
	case strings.Contains(path, "/adGroups:mutate"):
		return "adGroups"
	default:
		return "unknown"
	}
}

func (f *campaignConversionGoalFake) searchGoalsLocked(query string) []any {
	var out []any
	for _, goal := range f.goals {
		if matchesCampaignConversionGoalQuery(query, goal) {
			out = append(out, map[string]any{"campaignConversionGoal": cloneMap(goal)})
		}
	}
	return out
}

func (f *campaignConversionGoalFake) searchCampaignsLocked(query string) []any {
	var out []any
	for _, campaign := range f.campaigns {
		if matchesCampaignQuery(query, campaign) {
			out = append(out, map[string]any{"campaign": cloneMap(campaign)})
		}
	}
	return out
}

func (f *campaignConversionGoalFake) searchBudgetsLocked(query string) []any {
	var out []any
	for _, budget := range f.budgets {
		if matchesCampaignBudgetQuery(query, budget) {
			out = append(out, map[string]any{"campaignBudget": cloneMap(budget)})
		}
	}
	return out
}

func (f *campaignConversionGoalFake) searchActionsLocked(query string) []any {
	var out []any
	for _, action := range f.actions {
		if matchesConversionActionQuery(query, action) {
			out = append(out, map[string]any{"conversionAction": cloneMap(action)})
		}
	}
	return out
}

func (f *campaignConversionGoalFake) mutateLocked(collection string, body []byte) (string, error) {
	var req struct {
		Operations []map[string]any `json:"operations"`
	}
	if err := json.Unmarshal(body, &req); err != nil || len(req.Operations) == 0 {
		return "", errors.New("malformed mutate")
	}
	op := req.Operations[0]
	switch collection {
	case "campaignConversionGoals":
		return f.mutateGoalLocked(op)
	case "campaigns":
		return f.mutateCampaignLocked(op)
	case "campaignBudgets":
		return f.mutateBudgetLocked(op)
	case "conversionActions":
		return f.mutateActionLocked(op)
	default:
		return "", errors.New("unsupported mutate")
	}
}

func (f *campaignConversionGoalFake) mutateGoalLocked(op map[string]any) (string, error) {
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
	id := strings.TrimPrefix(resourceName, "customers/"+testCustomerID+"/campaignConversionGoals/")
	current, ok := f.goals[id]
	if !ok {
		return "", errors.New("missing campaign conversion goal")
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

func (f *campaignConversionGoalFake) mutateCampaignLocked(op map[string]any) (string, error) {
	raw, ok := op["create"]
	if !ok {
		return "", errors.New("unsupported campaign mutate")
	}
	campaign, _ := raw.(map[string]any)
	created := cloneMap(campaign)
	f.nextCampaignID++
	id := strconv.FormatInt(f.nextCampaignID, 10)
	created["id"] = id
	if stringify(created["advertisingChannelType"]) == "" {
		created["advertisingChannelType"] = "SEARCH"
	}
	if stringify(created["status"]) == "" {
		created["status"] = "PAUSED"
	}
	if stringify(created["biddingStrategyType"]) == "" {
		if _, ok := created["manualCpc"]; ok {
			created["biddingStrategyType"] = "MANUAL_CPC"
		}
	}
	f.storeCampaignLocked(created)
	return stringify(created["resourceName"]), nil
}

func (f *campaignConversionGoalFake) mutateBudgetLocked(op map[string]any) (string, error) {
	raw, ok := op["create"]
	if !ok {
		return "", errors.New("unsupported budget mutate")
	}
	budget, _ := raw.(map[string]any)
	created := cloneMap(budget)
	f.nextBudgetID++
	id := strconv.FormatInt(f.nextBudgetID, 10)
	created["id"] = id
	if stringify(created["period"]) == "" {
		created["period"] = "DAILY"
	}
	if stringify(created["type"]) == "" {
		created["type"] = "STANDARD"
	}
	if stringify(created["resourceName"]) == "" {
		created["resourceName"] = "customers/" + testCustomerID + "/campaignBudgets/" + id
	}
	f.budgets[id] = created
	return stringify(created["resourceName"]), nil
}

func (f *campaignConversionGoalFake) mutateActionLocked(op map[string]any) (string, error) {
	raw, ok := op["create"]
	if !ok {
		return "", errors.New("unsupported conversion action mutate")
	}
	action, _ := raw.(map[string]any)
	created := cloneMap(action)
	f.nextActionID++
	id := strconv.FormatInt(f.nextActionID, 10)
	created["id"] = id
	if stringify(created["countingType"]) == "" {
		created["countingType"] = "MANY_PER_CLICK"
	}
	f.storeActionLocked(created)
	return stringify(created["resourceName"]), nil
}

func matchesCampaignConversionGoalQuery(query string, goal map[string]any) bool {
	if strings.Contains(query, "campaign_conversion_goal.campaign = ") {
		want := extractGAQLString(query, "campaign_conversion_goal.campaign = ")
		if want != "" && want != stringify(goal["campaign"]) {
			return false
		}
	}
	if strings.Contains(query, "campaign_conversion_goal.category = ") {
		want := extractGAQLString(query, "campaign_conversion_goal.category = ")
		if want != "" && want != stringify(goal["category"]) {
			return false
		}
	}
	if strings.Contains(query, "campaign_conversion_goal.origin = ") {
		want := extractGAQLString(query, "campaign_conversion_goal.origin = ")
		if want != "" && want != stringify(goal["origin"]) {
			return false
		}
	}
	return true
}
