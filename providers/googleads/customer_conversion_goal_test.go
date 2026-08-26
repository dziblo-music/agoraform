package googleads_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestValidateCustomerConversionGoalValid(t *testing.T) {
	t.Parallel()

	p, _ := testConversionActionProvider(t, nil)
	res := customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: true,
	})
	if err := p.Validate(context.Background(), res); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateCustomerConversionGoalWithConversionActionRef(t *testing.T) {
	t.Parallel()

	p, _ := testConversionActionProvider(t, nil)
	res := customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory:         "SIGNUP",
		googleads.AttrOrigin:           "WEBSITE",
		googleads.AttrBiddable:         true,
		googleads.AttrConversionAction: conversionActionRef(t, "trial_started"),
	})
	if err := p.Validate(context.Background(), res); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateCustomerConversionGoalErrors(t *testing.T) {
	t.Parallel()

	p, _ := testConversionActionProvider(t, nil)
	addr := mustCustomerConversionGoalAddress(t, "signup")

	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name:  "missing category",
			attrs: resource.Attributes{googleads.AttrOrigin: "WEBSITE", googleads.AttrBiddable: true},
			want:  "missing required attribute \"category\"",
		},
		{
			name:  "missing origin",
			attrs: resource.Attributes{googleads.AttrCategory: "SIGNUP", googleads.AttrBiddable: true},
			want:  "missing required attribute \"origin\"",
		},
		{
			name:  "missing biddable",
			attrs: resource.Attributes{googleads.AttrCategory: "SIGNUP", googleads.AttrOrigin: "WEBSITE"},
			want:  "missing required attribute \"biddable\"",
		},
		{
			name: "unknown category",
			attrs: resource.Attributes{
				googleads.AttrCategory: "PHONE_CALL_LEAD",
				googleads.AttrOrigin:   "WEBSITE",
				googleads.AttrBiddable: true,
			},
			want: "category",
		},
		{
			name: "unsupported origin",
			attrs: resource.Attributes{
				googleads.AttrCategory: "SIGNUP",
				googleads.AttrOrigin:   "APP",
				googleads.AttrBiddable: true,
			},
			want: "WEBSITE",
		},
		{
			name: "google hosted origin",
			attrs: resource.Attributes{
				googleads.AttrCategory: "SIGNUP",
				googleads.AttrOrigin:   "GOOGLE_HOSTED",
				googleads.AttrBiddable: true,
			},
			want: "WEBSITE",
		},
		{
			name: "computed resourceName",
			attrs: resource.Attributes{
				googleads.AttrCategory: "SIGNUP",
				googleads.AttrOrigin:   "WEBSITE",
				googleads.AttrBiddable: true,
				"resourceName":         "customers/1234567890/customerConversionGoals/SIGNUP~WEBSITE",
			},
			want: "computed",
		},
		{
			name: "computed id",
			attrs: resource.Attributes{
				googleads.AttrCategory: "SIGNUP",
				googleads.AttrOrigin:   "WEBSITE",
				googleads.AttrBiddable: true,
				"id":                   "SIGNUP~WEBSITE",
			},
			want: "computed",
		},
		{
			name: "unsupported attribute",
			attrs: resource.Attributes{
				googleads.AttrCategory: "SIGNUP",
				googleads.AttrOrigin:   "WEBSITE",
				googleads.AttrBiddable: true,
				"campaign":             "brand",
			},
			want: "unsupported attribute",
		},
		{
			name: "conversionAction string",
			attrs: resource.Attributes{
				googleads.AttrCategory:         "SIGNUP",
				googleads.AttrOrigin:           "WEBSITE",
				googleads.AttrBiddable:         true,
				googleads.AttrConversionAction: "googleads.conversion_action.trial_started",
			},
			want: "resource reference",
		},
		{
			name: "conversionAction wrong type",
			attrs: resource.Attributes{
				googleads.AttrCategory:         "SIGNUP",
				googleads.AttrOrigin:           "WEBSITE",
				googleads.AttrBiddable:         true,
				googleads.AttrConversionAction: resource.Ref{Address: mustCustomerConversionGoalAddress(t, "other")},
			},
			want: "googleads.conversion_action",
		},
		{
			name: "biddable not bool",
			attrs: resource.Attributes{
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

func TestValidateCustomerConversionGoalRequiresCustomerID(t *testing.T) {
	t.Parallel()

	p := googleads.New(googleads.Config{
		DeveloperToken: testDeveloperToken,
		ClientID:       testClientID,
		ClientSecret:   testClientSecret,
		RefreshToken:   testRefreshToken,
		BaseURL:        "https://googleads.example.com",
	})
	err := p.Validate(context.Background(), customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: true,
	}))
	if err == nil {
		t.Fatal("expected customer id error")
	}
	if !strings.Contains(err.Error(), googleads.EnvCustomerID) {
		t.Fatalf("error = %q, want %s", err, googleads.EnvCustomerID)
	}
}

func TestReadCustomerConversionGoalSuccess(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seedGoal(map[string]any{
		"category": "SIGNUP",
		"origin":   "WEBSITE",
		"biddable": true,
	})
	p, _ := testConversionActionProvider(t, fake)

	live, err := p.Read(context.Background(), customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: true,
	}))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "SIGNUP~WEBSITE" {
		t.Fatalf("identity = %q, want SIGNUP~WEBSITE", live.Identity.ID)
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
	if live.Computed["id"] != "SIGNUP~WEBSITE" {
		t.Fatalf("computed id = %v", live.Computed["id"])
	}
	if live.Computed["resourceName"] != "customers/"+testCustomerID+"/customerConversionGoals/SIGNUP~WEBSITE" {
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

func TestReadCustomerConversionGoalNotFound(t *testing.T) {
	t.Parallel()

	p, _ := testConversionActionProvider(t, newConversionActionFake())
	_, err := p.Read(context.Background(), customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: true,
	}))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadCustomerConversionGoalBoundIdentity(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seedGoal(map[string]any{"category": "PURCHASE", "origin": "WEBSITE", "biddable": false})
	fake.seedGoal(map[string]any{"category": "SIGNUP", "origin": "WEBSITE", "biddable": true})
	p, _ := testConversionActionProvider(t, fake)

	res := customerConversionGoalResource(t, "purchase", resource.Attributes{
		googleads.AttrCategory: "PURCHASE",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: false,
	})
	res.Identity = resource.Identity{ID: "PURCHASE~WEBSITE"}
	live, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "PURCHASE~WEBSITE" {
		t.Fatalf("identity = %q, want bound PURCHASE~WEBSITE", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrBiddable] != false {
		t.Fatalf("biddable = %v, want identity-bound false", live.Attributes[googleads.AttrBiddable])
	}
}

func TestReadCustomerConversionGoalBoundIdentityMismatch(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seedGoal(map[string]any{"category": "SIGNUP", "origin": "WEBSITE", "biddable": true})
	p, _ := testConversionActionProvider(t, fake)

	res := customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory: "PURCHASE",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: true,
	})
	res.Identity = resource.Identity{ID: "SIGNUP~WEBSITE"}
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

func TestReadCustomerConversionGoalAPIError(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.searchStatus = http.StatusForbidden
	p, _ := testConversionActionProvider(t, fake)

	_, err := p.Read(context.Background(), customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: true,
	}))
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadCustomerConversionGoalMalformedResponse(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.searchBody = `{"results":[{"customerConversionGoal":"oops ` + testAccessToken + `"}]}`
	p, _ := testConversionActionProvider(t, fake)

	_, err := p.Read(context.Background(), customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: true,
	}))
	if err == nil {
		t.Fatal("expected malformed response error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("malformed response must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadCustomerConversionGoalRejectsUnsupportedOrigin(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.searchBody = `{"results":[{"customerConversionGoal":{"resourceName":"customers/` + testCustomerID + `/customerConversionGoals/SIGNUP~APP","category":"SIGNUP","origin":"APP","biddable":true}}]}`
	p, _ := testConversionActionProvider(t, fake)

	_, err := p.Read(context.Background(), customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: true,
	}))
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

func TestCreateCustomerConversionGoalAdoptsProviderCreatedGoal(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seedGoal(map[string]any{
		"category": "SIGNUP",
		"origin":   "WEBSITE",
		"biddable": false,
	})
	p, _ := testConversionActionProvider(t, fake)

	live, err := p.Create(context.Background(), customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: true,
	}))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID != "SIGNUP~WEBSITE" {
		t.Fatalf("identity = %q", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrBiddable] != true {
		t.Fatalf("biddable = %v, want true", live.Attributes[googleads.AttrBiddable])
	}
	assertGoalMutateUpdateOnly(t, fake.lastMutate)
}

func TestCreateCustomerConversionGoalNoOpWhenBiddableMatches(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seedGoal(map[string]any{
		"category": "SIGNUP",
		"origin":   "WEBSITE",
		"biddable": true,
	})
	p, _ := testConversionActionProvider(t, fake)

	live, err := p.Create(context.Background(), customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory: "signup",
		googleads.AttrOrigin:   "website",
		googleads.AttrBiddable: true,
	}))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID != "SIGNUP~WEBSITE" {
		t.Fatalf("identity = %q", live.Identity.ID)
	}
	if fake.lastMutate != "" {
		t.Fatalf("matching biddable still mutated: %s", fake.lastMutate)
	}
}

func TestCreateCustomerConversionGoalMissingIsActionable(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	p, _ := testConversionActionProvider(t, fake)

	_, err := p.Create(context.Background(), customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: true,
	}))
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
	if fake.lastMutate != "" {
		t.Fatalf("missing goal sent mutate: %s", fake.lastMutate)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestCreateCustomerConversionGoalRejectsBoundIdentity(t *testing.T) {
	t.Parallel()

	p, _ := testConversionActionProvider(t, newConversionActionFake())
	res := customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: true,
	})
	res.Identity = resource.Identity{ID: "SIGNUP~WEBSITE"}
	_, err := p.Create(context.Background(), res)
	if err == nil {
		t.Fatal("expected bound identity error")
	}
	if !strings.Contains(err.Error(), "persisted identity") {
		t.Fatalf("error = %q", err)
	}
}

func TestUpdateCustomerConversionGoalBiddable(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seedGoal(map[string]any{
		"category": "SIGNUP",
		"origin":   "WEBSITE",
		"biddable": true,
	})
	p, _ := testConversionActionProvider(t, fake)

	desired := customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: false,
	})
	live, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:    desired.Address,
		Identity:   resource.Identity{ID: "SIGNUP~WEBSITE"},
		Attributes: resource.Attributes{googleads.AttrCategory: "SIGNUP", googleads.AttrOrigin: "WEBSITE", googleads.AttrBiddable: true},
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

func TestUpdateCustomerConversionGoalNoOp(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seedGoal(map[string]any{
		"category": "SIGNUP",
		"origin":   "WEBSITE",
		"biddable": true,
	})
	p, _ := testConversionActionProvider(t, fake)

	desired := customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: true,
	})
	live, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:    desired.Address,
		Identity:   resource.Identity{ID: "SIGNUP~WEBSITE"},
		Attributes: resource.Attributes{googleads.AttrCategory: "SIGNUP", googleads.AttrOrigin: "WEBSITE", googleads.AttrBiddable: true},
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

func TestUpdateCustomerConversionGoalAPIError(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seedGoal(map[string]any{"category": "SIGNUP", "origin": "WEBSITE", "biddable": true})
	fake.mutateStatus = http.StatusBadRequest
	p, _ := testConversionActionProvider(t, fake)

	_, err := p.Update(context.Background(), customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: false,
	}), resource.RemoteResource{
		Address:    mustCustomerConversionGoalAddress(t, "signup"),
		Identity:   resource.Identity{ID: "SIGNUP~WEBSITE"},
		Attributes: resource.Attributes{googleads.AttrCategory: "SIGNUP", googleads.AttrOrigin: "WEBSITE", googleads.AttrBiddable: true},
	})
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestImportCustomerConversionGoal(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seedGoal(map[string]any{"category": "SIGNUP", "origin": "WEBSITE", "biddable": true})
	p, _ := testConversionActionProvider(t, fake)
	addr := mustCustomerConversionGoalAddress(t, "signup")

	live, err := p.Import(context.Background(), addr, "SIGNUP~WEBSITE")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if live.Identity.ID != "SIGNUP~WEBSITE" {
		t.Fatalf("identity = %q", live.Identity.ID)
	}

	named, err := p.Import(context.Background(), addr, "customers/"+testCustomerID+"/customerConversionGoals/SIGNUP~WEBSITE")
	if err != nil {
		t.Fatalf("Import by resource name: %v", err)
	}
	if named.Identity.ID != "SIGNUP~WEBSITE" {
		t.Fatalf("resource-name import identity = %q", named.Identity.ID)
	}

	normalized, err := p.Import(context.Background(), addr, "signup~website")
	if err != nil {
		t.Fatalf("Import normalized: %v", err)
	}
	if normalized.Identity.ID != "SIGNUP~WEBSITE" {
		t.Fatalf("normalized import identity = %q", normalized.Identity.ID)
	}
}

func TestImportCustomerConversionGoalNotFound(t *testing.T) {
	t.Parallel()

	p, _ := testConversionActionProvider(t, newConversionActionFake())
	_, err := p.Import(context.Background(), mustCustomerConversionGoalAddress(t, "signup"), "SIGNUP~WEBSITE")
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Import = %v, want ErrNotFound", err)
	}
}

func TestPlanCustomerConversionGoalCreateWhenMissing(t *testing.T) {
	t.Parallel()

	p, _ := testConversionActionProvider(t, newConversionActionFake())
	res := customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: true,
	})
	got := mustPlanCustomerConversionGoal(t, p, res)
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionCreate {
		t.Fatalf("change = %+v, want create", got.Changes)
	}
}

func TestPlanCustomerConversionGoalUnchangedEquivalentRemote(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seedGoal(map[string]any{"category": "SIGNUP", "origin": "WEBSITE", "biddable": true})
	p, _ := testConversionActionProvider(t, fake)

	res := customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory: "signup",
		googleads.AttrOrigin:   "website",
		googleads.AttrBiddable: true,
	})
	got := mustPlanCustomerConversionGoal(t, p, res)
	if got.HasChanges() {
		t.Fatalf("equivalent remote produced changes: %+v", got.Changes)
	}
	if got.Changes[0].Identity.ID != "SIGNUP~WEBSITE" {
		t.Fatalf("identity = %q", got.Changes[0].Identity.ID)
	}
}

func TestNormalizeCustomerConversionGoalOmitsConversionActionRef(t *testing.T) {
	t.Parallel()

	p, _ := testConversionActionProvider(t, nil)
	desired := customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory:         "SIGNUP",
		googleads.AttrOrigin:           "WEBSITE",
		googleads.AttrBiddable:         true,
		googleads.AttrConversionAction: conversionActionRef(t, "trial_started"),
	})
	live := resource.RemoteResource{
		Address:    desired.Address,
		Identity:   resource.Identity{ID: "SIGNUP~WEBSITE"},
		Attributes: resource.Attributes{googleads.AttrCategory: "SIGNUP", googleads.AttrOrigin: "WEBSITE", googleads.AttrBiddable: true},
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

func TestPlanCustomerConversionGoalUpdateBiddable(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seedGoal(map[string]any{"category": "SIGNUP", "origin": "WEBSITE", "biddable": false})
	p, _ := testConversionActionProvider(t, fake)

	res := customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: true,
	})
	got := mustPlanCustomerConversionGoal(t, p, res)
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want update", got.Changes)
	}
}

func TestPlanCustomerConversionGoalBoundIdentityMissingIsStale(t *testing.T) {
	t.Parallel()

	p, _ := testConversionActionProvider(t, newConversionActionFake())
	res := customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory: "SIGNUP",
		googleads.AttrOrigin:   "WEBSITE",
		googleads.AttrBiddable: true,
	})
	_, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, customerConversionGoalIdentities{res.Address.String(): {ID: "SIGNUP~WEBSITE"}})
	if err == nil {
		t.Fatal("expected stale identity error")
	}
	if !strings.Contains(err.Error(), "SIGNUP~WEBSITE") {
		t.Fatalf("error = %q, want persisted identity", err)
	}
}

func TestApplyCustomerConversionGoalAfterConversionActionCreate(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	p, _ := testConversionActionProvider(t, fake)

	action := conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:     "Trial Started",
		googleads.AttrCategory: "SIGNUP",
	})
	goal := customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory:         "SIGNUP",
		googleads.AttrOrigin:           "WEBSITE",
		googleads.AttrBiddable:         false,
		googleads.AttrConversionAction: conversionActionRef(t, "trial_started"),
	})
	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}

	result, err := apply.Run(context.Background(), []resource.Resource{goal, action}, func(resource.Address) (provider.Provider, error) {
		return p, nil
	}, st, nil)
	if err != nil {
		t.Fatalf("apply.Run: %v", err)
	}
	if result.Created != 2 {
		t.Fatalf("result = %+v, want 2 created", result)
	}
	if len(fake.mutates) < 2 || fake.mutates[0] != "conversionActions" {
		t.Fatalf("mutates = %v, want conversion action before goal", fake.mutates)
	}
	goalMutates := 0
	for _, collection := range fake.mutates {
		if collection == "customerConversionGoals" {
			goalMutates++
		}
	}
	if goalMutates != 1 {
		t.Fatalf("customerConversionGoals mutates = %d, want 1", goalMutates)
	}
	assertGoalMutateUpdateOnly(t, fake.lastMutate)

	live, err := p.Read(context.Background(), goal)
	if err != nil {
		t.Fatalf("Read after apply: %v", err)
	}
	if live.Attributes[googleads.AttrBiddable] != false {
		t.Fatalf("biddable = %v, want false after adopt+update", live.Attributes[googleads.AttrBiddable])
	}

	second, err := plan.BuildWithState(context.Background(), []resource.Resource{action, goal}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("second plan: %v", err)
	}
	if second.HasChanges() {
		t.Fatalf("second plan has changes: %+v", second.Changes)
	}
}

func TestApplyCustomerConversionGoalOrdersConversionActionFirst(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	p, _ := testConversionActionProvider(t, fake)
	action := conversionActionResource(t, "trial_started", resource.Attributes{
		googleads.AttrName:     "Trial Started",
		googleads.AttrCategory: "SIGNUP",
	})
	goal := customerConversionGoalResource(t, "signup", resource.Attributes{
		googleads.AttrCategory:         "SIGNUP",
		googleads.AttrOrigin:           "WEBSITE",
		googleads.AttrBiddable:         true,
		googleads.AttrConversionAction: conversionActionRef(t, "trial_started"),
	})
	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := apply.Run(context.Background(), []resource.Resource{goal, action}, func(resource.Address) (provider.Provider, error) {
		return p, nil
	}, st, nil); err != nil {
		t.Fatalf("apply.Run: %v", err)
	}
	if len(fake.mutates) == 0 || fake.mutates[0] != "conversionActions" {
		t.Fatalf("mutates = %v, want conversionActions first", fake.mutates)
	}
}

func mustPlanCustomerConversionGoal(t *testing.T, p *googleads.Provider, res resource.Resource) *plan.Plan {
	t.Helper()
	got, err := plan.Build(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	return got
}

func customerConversionGoalResource(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{
		Address:    mustCustomerConversionGoalAddress(t, name),
		Attributes: attrs,
	}
}

func mustCustomerConversionGoalAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("googleads.customer_conversion_goal." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func conversionActionRef(t *testing.T, name string) resource.Ref {
	t.Helper()
	return resource.Ref{Address: mustConversionActionAddress(t, name)}
}

type customerConversionGoalIdentities map[string]resource.Identity

func (m customerConversionGoalIdentities) Identity(addr resource.Address) (resource.Identity, bool, error) {
	id, ok := m[addr.String()]
	return id, ok, nil
}

func assertGoalMutateUpdateOnly(t *testing.T, raw string) {
	t.Helper()
	if strings.TrimSpace(raw) == "" {
		t.Fatal("expected goal mutate body")
	}
	var req struct {
		Operations []map[string]any `json:"operations"`
	}
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("mutate JSON: %v (%s)", err, raw)
	}
	if len(req.Operations) == 0 {
		t.Fatalf("mutate missing operations: %s", raw)
	}
	for i, op := range req.Operations {
		if _, ok := op["create"]; ok {
			t.Fatalf("operation %d attempted unsupported create: %s", i, raw)
		}
		if _, ok := op["remove"]; ok {
			t.Fatalf("operation %d attempted unsupported remove: %s", i, raw)
		}
		if _, ok := op["delete"]; ok {
			t.Fatalf("operation %d attempted unsupported delete: %s", i, raw)
		}
		if _, ok := op["update"]; !ok {
			t.Fatalf("operation %d missing update: %s", i, raw)
		}
	}
}
