package googleads_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/internal/graph"
	"github.com/dziblo-music/agoraform/internal/importer"
	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestValidateCampaignBudgetValid(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignBudgetProvider(t, nil)
	res := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
		googleads.AttrDeliveryMethod:   "STANDARD",
	})
	if err := p.Validate(context.Background(), res); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateCampaignBudgetErrors(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignBudgetProvider(t, nil)
	addr := mustCampaignBudgetAddress(t, "brand")

	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name: "missing name",
			attrs: resource.Attributes{
				googleads.AttrAmount:           50,
				googleads.AttrExplicitlyShared: false,
			},
			want: "missing required attribute \"name\"",
		},
		{
			name: "missing amount",
			attrs: resource.Attributes{
				googleads.AttrName:             "Brand daily budget",
				googleads.AttrExplicitlyShared: false,
			},
			want: "missing required attribute \"amount\"",
		},
		{
			name: "missing explicitlyShared",
			attrs: resource.Attributes{
				googleads.AttrName:   "Brand daily budget",
				googleads.AttrAmount: 50,
			},
			want: "missing required attribute \"explicitlyShared\"",
		},
		{
			name: "zero amount",
			attrs: resource.Attributes{
				googleads.AttrName:             "Brand daily budget",
				googleads.AttrAmount:           0,
				googleads.AttrExplicitlyShared: false,
			},
			want: "greater than 0",
		},
		{
			name: "negative amount",
			attrs: resource.Attributes{
				googleads.AttrName:             "Brand daily budget",
				googleads.AttrAmount:           -1,
				googleads.AttrExplicitlyShared: false,
			},
			want: "greater than 0",
		},
		{
			name: "too many decimal places",
			attrs: resource.Attributes{
				googleads.AttrName:             "Brand daily budget",
				googleads.AttrAmount:           "50.1234567",
				googleads.AttrExplicitlyShared: false,
			},
			want: "at most 6 decimal places",
		},
		{
			name: "computed id",
			attrs: resource.Attributes{
				googleads.AttrName:             "Brand daily budget",
				googleads.AttrAmount:           50,
				googleads.AttrExplicitlyShared: false,
				"id":                           "1",
			},
			want: "computed",
		},
		{
			name: "computed amountMicros",
			attrs: resource.Attributes{
				googleads.AttrName:             "Brand daily budget",
				googleads.AttrAmount:           50,
				googleads.AttrExplicitlyShared: false,
				"amountMicros":                 50000000,
			},
			want: "computed",
		},
		{
			name: "computed period",
			attrs: resource.Attributes{
				googleads.AttrName:             "Brand daily budget",
				googleads.AttrAmount:           50,
				googleads.AttrExplicitlyShared: false,
				"period":                       "DAILY",
			},
			want: "computed",
		},
		{
			name: "unsupported attribute",
			attrs: resource.Attributes{
				googleads.AttrName:             "Brand daily budget",
				googleads.AttrAmount:           50,
				googleads.AttrExplicitlyShared: false,
				"alignedBiddingStrategy":       "1",
			},
			want: "unsupported attribute",
		},
		{
			name: "invalid delivery method",
			attrs: resource.Attributes{
				googleads.AttrName:             "Brand daily budget",
				googleads.AttrAmount:           50,
				googleads.AttrExplicitlyShared: false,
				googleads.AttrDeliveryMethod:   "FAST",
			},
			want: "deliveryMethod",
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
				t.Fatalf("error = %q, want address", err)
			}
			assertNoProviderSecret(t, err.Error())
		})
	}
}

func TestValidateCampaignBudgetRequiresCustomerID(t *testing.T) {
	t.Parallel()

	p := googleads.New(googleads.Config{
		DeveloperToken: testDeveloperToken,
		ClientID:       testClientID,
		ClientSecret:   testClientSecret,
		RefreshToken:   testRefreshToken,
		BaseURL:        "https://googleads.example.com",
	})
	err := p.Validate(context.Background(), campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	}))
	if err == nil {
		t.Fatal("expected customer id error")
	}
	if !strings.Contains(err.Error(), googleads.EnvCustomerID) {
		t.Fatalf("error = %q, want %s", err, googleads.EnvCustomerID)
	}
}

func TestReadCampaignBudgetSuccess(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{
		"id":               "11",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
		"status":           "ENABLED",
		"referenceCount":   "0",
	})
	p, _ := testCampaignBudgetProvider(t, fake)

	live, err := p.Read(context.Background(), campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
		googleads.AttrDeliveryMethod:   "STANDARD",
	}))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.ID != "11" {
		t.Fatalf("identity = %q, want 11", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrName] != "Brand daily budget" {
		t.Fatalf("name = %v", live.Attributes[googleads.AttrName])
	}
	if live.Attributes[googleads.AttrAmount] != int64(50) {
		t.Fatalf("amount = %v (%T), want 50", live.Attributes[googleads.AttrAmount], live.Attributes[googleads.AttrAmount])
	}
	if live.Attributes[googleads.AttrExplicitlyShared] != false {
		t.Fatalf("explicitlyShared = %v", live.Attributes[googleads.AttrExplicitlyShared])
	}
	if live.Computed["id"] != "11" {
		t.Fatalf("computed id = %v", live.Computed["id"])
	}
	if live.Computed["amountMicros"] != int64(50000000) {
		t.Fatalf("computed amountMicros = %v", live.Computed["amountMicros"])
	}
	if live.Computed["period"] != "DAILY" {
		t.Fatalf("computed period = %v", live.Computed["period"])
	}
	if _, ok := live.Attributes["id"]; ok {
		t.Fatal("id must not appear in comparable attributes")
	}
	if _, ok := live.Attributes["amountMicros"]; ok {
		t.Fatal("amountMicros must not appear in comparable attributes")
	}
	if _, ok := live.Attributes["resourceName"]; ok {
		t.Fatal("resourceName must not appear in comparable attributes")
	}
}

func TestReadCampaignBudgetNotFound(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignBudgetProvider(t, newCampaignBudgetFake())
	_, err := p.Read(context.Background(), campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	}))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadCampaignBudgetDuplicateName(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{"id": "1", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false})
	fake.seed(map[string]any{"id": "4", "name": "Brand daily budget", "amountMicros": "25000000", "explicitlyShared": true})
	p, _ := testCampaignBudgetProvider(t, fake)

	_, err := p.Read(context.Background(), campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	}))
	if err == nil {
		t.Fatal("expected duplicate name error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("duplicate names must not look like not found")
	}
	if !strings.Contains(err.Error(), "multiple remote campaign budgets") {
		t.Fatalf("error = %q", err)
	}
}

func TestReadCampaignBudgetBoundIdentity(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{"id": "5", "name": "Other", "amountMicros": "10000000", "explicitlyShared": true})
	fake.seed(map[string]any{"id": "9", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false})
	p, _ := testCampaignBudgetProvider(t, fake)

	res := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
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

func TestReadCampaignBudgetBoundIdentityMissing(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{"id": "9", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false})
	p, _ := testCampaignBudgetProvider(t, fake)

	res := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	})
	res.Identity = resource.Identity{ID: "99"}
	_, err := p.Read(context.Background(), res)
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
}

func TestReadCampaignBudgetRejectsCustomPeriod(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{
		"id":               "3",
		"name":             "Lifetime budget",
		"amountMicros":     "50000000",
		"explicitlyShared": false,
		"period":           "CUSTOM_PERIOD",
		"type":             "STANDARD",
	})
	p, _ := testCampaignBudgetProvider(t, fake)

	_, err := p.Read(context.Background(), campaignBudgetResource(t, "lifetime", resource.Attributes{
		googleads.AttrName:             "Lifetime budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	}))
	if err == nil {
		t.Fatal("expected unsupported period error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported period must not look like not found")
	}
	if !strings.Contains(err.Error(), "DAILY") {
		t.Fatalf("error = %q, want DAILY guidance", err)
	}
}

func TestReadCampaignBudgetRejectsNonStandardType(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{
		"id":               "3",
		"name":             "Smart budget",
		"amountMicros":     "50000000",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "SMART_CAMPAIGN",
	})
	p, _ := testCampaignBudgetProvider(t, fake)

	_, err := p.Read(context.Background(), campaignBudgetResource(t, "smart", resource.Attributes{
		googleads.AttrName:             "Smart budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	}))
	if err == nil {
		t.Fatal("expected unsupported type error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported type must not look like not found")
	}
	if !strings.Contains(err.Error(), "STANDARD") {
		t.Fatalf("error = %q, want STANDARD guidance", err)
	}
}

func TestReadCampaignBudgetAPIError(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.searchStatus = http.StatusForbidden
	p, _ := testCampaignBudgetProvider(t, fake)

	_, err := p.Read(context.Background(), campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	}))
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestReadCampaignBudgetMalformedResponse(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.searchBody = `{"results":[{"campaignBudget":"oops ` + testAccessToken + `"}]}`
	p, _ := testCampaignBudgetProvider(t, fake)

	_, err := p.Read(context.Background(), campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	}))
	if err == nil {
		t.Fatal("expected malformed response error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("malformed response must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestCreateCampaignBudget(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	p, _ := testCampaignBudgetProvider(t, fake)
	res := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50.25,
		googleads.AttrExplicitlyShared: false,
		googleads.AttrDeliveryMethod:   "STANDARD",
	})

	live, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if live.Identity.ID == "" {
		t.Fatal("create returned empty identity")
	}
	if live.Attributes[googleads.AttrName] != "Brand daily budget" {
		t.Fatalf("name = %v", live.Attributes[googleads.AttrName])
	}
	if live.Computed["period"] != "DAILY" {
		t.Fatalf("period = %v, want DAILY", live.Computed["period"])
	}
	if live.Computed["type"] != "STANDARD" {
		t.Fatalf("type = %v, want STANDARD", live.Computed["type"])
	}
	if live.Computed["amountMicros"] != int64(50250000) {
		t.Fatalf("amountMicros = %v, want 50250000", live.Computed["amountMicros"])
	}
	if !strings.Contains(fake.lastMutate, `"period":"DAILY"`) && !strings.Contains(fake.lastMutate, `"period": "DAILY"`) {
		t.Fatalf("create mutate missing DAILY period: %s", fake.lastMutate)
	}
	if !strings.Contains(fake.lastMutate, `"explicitlyShared":false`) && !strings.Contains(fake.lastMutate, `"explicitlyShared": false`) {
		t.Fatalf("create mutate missing dedicated budget: %s", fake.lastMutate)
	}
	if !strings.Contains(fake.lastMutate, "50250000") {
		t.Fatalf("create mutate missing normalized micros: %s", fake.lastMutate)
	}

	got, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read after create: %v", err)
	}
	if got.Identity.ID != live.Identity.ID {
		t.Fatalf("read id = %q, want %q", got.Identity.ID, live.Identity.ID)
	}
}

func TestUpdateCampaignBudget(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{
		"id":               "8",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
	})
	p, _ := testCampaignBudgetProvider(t, fake)

	desired := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           75,
		googleads.AttrExplicitlyShared: false,
		googleads.AttrDeliveryMethod:   "ACCELERATED",
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
	if live.Attributes[googleads.AttrAmount] != int64(75) {
		t.Fatalf("amount = %v, want 75", live.Attributes[googleads.AttrAmount])
	}
	if live.Attributes[googleads.AttrDeliveryMethod] != "ACCELERATED" {
		t.Fatalf("deliveryMethod = %v, want ACCELERATED", live.Attributes[googleads.AttrDeliveryMethod])
	}
	if !strings.Contains(fake.lastMutate, "updateMask") {
		t.Fatalf("update missing updateMask: %s", fake.lastMutate)
	}
	if !strings.Contains(fake.lastMutate, "75000000") {
		t.Fatalf("update missing normalized micros: %s", fake.lastMutate)
	}
}

func TestCreateCampaignBudgetAPIError(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.mutateStatus = http.StatusBadRequest
	p, _ := testCampaignBudgetProvider(t, fake)

	_, err := p.Create(context.Background(), campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	}))
	if err == nil {
		t.Fatal("expected API error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("API failure must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestUpdateCampaignBudgetAPIError(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{"id": "8", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false})
	fake.mutateStatus = http.StatusBadRequest
	p, _ := testCampaignBudgetProvider(t, fake)

	_, err := p.Update(context.Background(), campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           75,
		googleads.AttrExplicitlyShared: false,
	}), resource.RemoteResource{
		Address:  mustCampaignBudgetAddress(t, "brand"),
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

func TestImportCampaignBudget(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{
		"id":               "12",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
		"status":           "ENABLED",
		"referenceCount":   "1",
	})
	p, _ := testCampaignBudgetProvider(t, fake)
	addr := mustCampaignBudgetAddress(t, "brand")

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
	if live.Attributes[googleads.AttrName] != "Brand daily budget" {
		t.Fatalf("name = %v", live.Attributes[googleads.AttrName])
	}
	for _, key := range []string{"id", "resourceName", "amountMicros", "period", "type", "status", "referenceCount"} {
		if _, ok := live.Attributes[key]; ok {
			t.Fatalf("computed %s leaked into attributes: %#v", key, live.Attributes)
		}
	}
	if live.Computed["id"] != "12" {
		t.Fatalf("computed id = %v", live.Computed["id"])
	}

	named, err := p.Import(context.Background(), addr, "customers/"+testCustomerID+"/campaignBudgets/12")
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

func TestImportCampaignBudgetNotFound(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignBudgetProvider(t, newCampaignBudgetFake())
	_, err := p.Import(context.Background(), mustCampaignBudgetAddress(t, "brand"), "12")
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Import = %v, want ErrNotFound", err)
	}
}

func TestNormalizeCampaignBudgetImportID(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignBudgetProvider(t, nil)
	addr := mustCampaignBudgetAddress(t, "brand")

	got, err := p.NormalizeImportID(addr, "12")
	if err != nil || got != "12" {
		t.Fatalf("numeric = (%q, %v), want 12", got, err)
	}
	got, err = p.NormalizeImportID(addr, "customers/"+testCustomerID+"/campaignBudgets/12")
	if err != nil || got != "12" {
		t.Fatalf("resource name = (%q, %v), want 12", got, err)
	}

	_, err = p.NormalizeImportID(addr, "abc")
	if err == nil || !strings.Contains(err.Error(), "not a valid Google Ads campaign budget id") {
		t.Fatalf("invalid id error = %v", err)
	}
	_, err = p.NormalizeImportID(addr, "customers/0000000000/campaignBudgets/12")
	if err == nil || !strings.Contains(err.Error(), "does not match configured") {
		t.Fatalf("wrong customer error = %v", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestImportCampaignBudgetInvalidID(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignBudgetProvider(t, newCampaignBudgetFake())
	_, err := p.Import(context.Background(), mustCampaignBudgetAddress(t, "brand"), "abc")
	if err == nil || !strings.Contains(err.Error(), "not a valid Google Ads campaign budget id") {
		t.Fatalf("Import = %v, want invalid id", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestImportCampaignBudgetUnsupportedPeriod(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{
		"id":               "3",
		"name":             "Lifetime budget",
		"amountMicros":     "50000000",
		"explicitlyShared": false,
		"period":           "CUSTOM_PERIOD",
		"type":             "STANDARD",
	})
	p, _ := testCampaignBudgetProvider(t, fake)
	_, err := p.Import(context.Background(), mustCampaignBudgetAddress(t, "lifetime"), "3")
	if err == nil {
		t.Fatal("expected unsupported period error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported period must not look like not found")
	}
	if !strings.Contains(err.Error(), "DAILY") {
		t.Fatalf("error = %q, want DAILY guidance", err)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("unsupported import mutated remote: %v", fake.mutates)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestImportCampaignBudgetThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{
		"id":               "12",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
		"status":           "ENABLED",
		"referenceCount":   "1",
	})
	p, _ := testCampaignBudgetProvider(t, fake)
	addr := mustCampaignBudgetAddress(t, "brand")

	st := mustGoogleAdsImportStore(t)
	got, err := importer.Run(context.Background(), addr, "customers/"+testCustomerID+"/campaignBudgets/12", lookupGoogleAds(p), st)
	if err != nil {
		t.Fatalf("importer.Run: %v", err)
	}
	if got.Identity.ID != "12" {
		t.Fatalf("canonical identity = %q, want 12", got.Identity.ID)
	}
	assertNoProviderSecret(t, got.YAML)
	for _, leak := range []string{"resourceName", "amountMicros", "period", "type", "status", "referenceCount", "id:", testAccessToken} {
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

func TestPlanCampaignBudgetCreateWhenMissing(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignBudgetProvider(t, newCampaignBudgetFake())
	res := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	})

	got := mustPlanCampaignBudget(t, p, res)
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionCreate {
		t.Fatalf("change = %+v, want create", got.Changes)
	}
}

func TestPlanCampaignBudgetUnchangedEquivalentRemote(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{
		"id":               "2",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
		"status":           "ENABLED",
		"referenceCount":   "0",
	})
	p, _ := testCampaignBudgetProvider(t, fake)

	res := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
		googleads.AttrDeliveryMethod:   "standard",
	})
	got := mustPlanCampaignBudget(t, p, res)
	if got.HasChanges() {
		t.Fatalf("equivalent remote produced changes: %+v", got.Changes)
	}
	if got.Changes[0].Identity.ID != "2" {
		t.Fatalf("identity = %q, want 2", got.Changes[0].Identity.ID)
	}
}

func TestPlanCampaignBudgetAmountAliasesAreEquivalent(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{
		"id":               "2",
		"name":             "Brand daily budget",
		"amountMicros":     "50250000",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
	})
	p, _ := testCampaignBudgetProvider(t, fake)

	for _, amount := range []any{50.25, "50.25", "50.250000"} {
		res := campaignBudgetResource(t, "brand", resource.Attributes{
			googleads.AttrName:             "Brand daily budget",
			googleads.AttrAmount:           amount,
			googleads.AttrExplicitlyShared: false,
		})
		got := mustPlanCampaignBudget(t, p, res)
		if got.HasChanges() {
			t.Fatalf("amount %v (%T) produced changes: %+v", amount, amount, got.Changes)
		}
	}
}

func TestPlanCampaignBudgetUpdateConfigurableField(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{
		"id":               "2",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
	})
	p, _ := testCampaignBudgetProvider(t, fake)

	res := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           80,
		googleads.AttrExplicitlyShared: false,
	})
	got := mustPlanCampaignBudget(t, p, res)
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want update", got.Changes)
	}
	var amountDiff *plan.AttributeDiff
	for i := range got.Changes[0].Diffs {
		if got.Changes[0].Diffs[i].Path == googleads.AttrAmount {
			amountDiff = &got.Changes[0].Diffs[i]
		}
	}
	if amountDiff == nil {
		t.Fatalf("amount diff missing: %+v", got.Changes[0].Diffs)
	}
}

func TestPlanCampaignBudgetIgnoresComputedFields(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{
		"id":               "5",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
		"status":           "ENABLED",
		"referenceCount":   "2",
	})
	p, _ := testCampaignBudgetProvider(t, fake)

	res := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	})
	got := mustPlanCampaignBudget(t, p, res)
	if got.HasChanges() {
		t.Fatalf("computed fields produced changes: %+v", got.Changes)
	}
}

func TestPlanCampaignBudgetOmittedDeliveryMethodIsNoOp(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{
		"id":               "5",
		"name":             "Brand daily budget",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
		"period":           "DAILY",
		"type":             "STANDARD",
	})
	p, _ := testCampaignBudgetProvider(t, fake)

	res := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	})
	got := mustPlanCampaignBudget(t, p, res)
	if got.HasChanges() {
		t.Fatalf("omitted delivery method produced changes: %+v", got.Changes)
	}
}

type campaignBudgetIdentities map[string]resource.Identity

func (m campaignBudgetIdentities) Identity(addr resource.Address) (resource.Identity, bool, error) {
	id, ok := m[addr.String()]
	return id, ok, nil
}

func TestPlanCampaignBudgetBoundIdentityMissingIsStale(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignBudgetProvider(t, newCampaignBudgetFake())
	res := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	})
	_, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, campaignBudgetIdentities{res.Address.String(): {ID: "99"}})
	if err == nil {
		t.Fatal("expected stale identity error")
	}
	if !strings.Contains(err.Error(), "99") {
		t.Fatalf("error = %q, want persisted identity", err)
	}
}

func TestCampaignBudgetIsLogicalReferenceTarget(t *testing.T) {
	t.Parallel()

	budget := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	})
	campaignAddr, err := resource.ParseAddress("googleads.campaign.brand")
	if err != nil {
		t.Fatal(err)
	}
	campaign := resource.Resource{
		Address: campaignAddr,
		Attributes: resource.Attributes{
			googleads.AttrName: "Brand",
			"budget":           resource.Ref{Address: budget.Address},
		},
	}

	g, err := graph.Build([]resource.Resource{campaign, budget})
	if err != nil {
		t.Fatalf("graph.Build: %v", err)
	}
	deps := g.Dependencies(campaignAddr)
	if len(deps) != 1 || deps[0] != budget.Address {
		t.Fatalf("campaign dependencies = %v, want [%s]", deps, budget.Address)
	}
}

func mustPlanCampaignBudget(t *testing.T, p *googleads.Provider, res resource.Resource) *plan.Plan {
	t.Helper()
	got, err := plan.Build(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	return got
}

func campaignBudgetResource(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{
		Address:    mustCampaignBudgetAddress(t, name),
		Attributes: attrs,
	}
}

func mustCampaignBudgetAddress(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("googleads.campaign_budget." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func testCampaignBudgetProvider(t *testing.T, fake *campaignBudgetFake) (*googleads.Provider, *httptest.Server) {
	t.Helper()
	if fake == nil {
		fake = newCampaignBudgetFake()
	}
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	t.Cleanup(srv.Close)
	cfg := validConfig(srv.URL)
	cfg.TokenURL = srv.URL + "/oauth/token"
	p := googleads.NewWithHTTPClient(cfg, srv.Client())
	return p, srv
}

type campaignBudgetFake struct {
	mu sync.Mutex

	nextID int64
	byID   map[string]map[string]any

	searchStatus int
	searchBody   string
	mutateStatus int
	mutateBody   string

	lastQuery  string
	lastMutate string
	mutates    []string
}

func newCampaignBudgetFake() *campaignBudgetFake {
	return &campaignBudgetFake{
		byID: map[string]map[string]any{},
	}
}

func (f *campaignBudgetFake) seed(budget map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storeLocked(cloneMap(budget))
}

func (f *campaignBudgetFake) storeLocked(budget map[string]any) {
	id := stringify(budget["id"])
	if stringify(budget["resourceName"]) == "" {
		budget["resourceName"] = "customers/" + testCustomerID + "/campaignBudgets/" + id
	}
	if stringify(budget["period"]) == "" {
		budget["period"] = "DAILY"
	}
	if stringify(budget["type"]) == "" {
		budget["type"] = "STANDARD"
	}
	f.byID[id] = budget
}

func (f *campaignBudgetFake) handler(w http.ResponseWriter, r *http.Request) {
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
		if strings.Contains(strings.ToLower(req.Query), "from customer") && !strings.Contains(strings.ToLower(req.Query), "from campaign_budget") {
			_, _ = io.WriteString(w, `{"results":[{"customer":{"id":"`+testCustomerID+`"}}]}`)
			return
		}
		results := f.searchLocked(req.Query)
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
		return
	}

	if strings.Contains(r.URL.Path, "/campaignBudgets:mutate") {
		f.lastMutate = string(body)
		f.mutates = append(f.mutates, "campaignBudgets")
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

func (f *campaignBudgetFake) searchLocked(query string) []any {
	var out []any
	for _, budget := range f.byID {
		if matchesCampaignBudgetQuery(query, budget) {
			out = append(out, map[string]any{"campaignBudget": cloneMap(budget)})
		}
	}
	return out
}

func (f *campaignBudgetFake) mutateLocked(body []byte) (string, error) {
	var req struct {
		Operations []map[string]any `json:"operations"`
	}
	if err := json.Unmarshal(body, &req); err != nil || len(req.Operations) == 0 {
		return "", errors.New("malformed mutate")
	}
	op := req.Operations[0]
	if raw, ok := op["create"]; ok {
		budget, _ := raw.(map[string]any)
		created := cloneMap(budget)
		f.nextID++
		id := strconv.FormatInt(f.nextID, 10)
		created["id"] = id
		if stringify(created["period"]) == "" {
			created["period"] = "DAILY"
		}
		if stringify(created["type"]) == "" {
			created["type"] = "STANDARD"
		}
		f.storeLocked(created)
		return stringify(created["resourceName"]), nil
	}
	if raw, ok := op["update"]; ok {
		budget, _ := raw.(map[string]any)
		resourceName := stringify(budget["resourceName"])
		id := strings.TrimPrefix(resourceName, "customers/"+testCustomerID+"/campaignBudgets/")
		current, ok := f.byID[id]
		if !ok {
			return "", errors.New("missing campaign budget")
		}
		merged := cloneMap(current)
		for k, v := range budget {
			if k == "resourceName" {
				continue
			}
			merged[k] = v
		}
		f.storeLocked(merged)
		return resourceName, nil
	}
	return "", errors.New("unsupported mutate")
}

func matchesCampaignBudgetQuery(query string, budget map[string]any) bool {
	id := stringify(budget["id"])
	name := stringify(budget["name"])
	if strings.Contains(query, "campaign_budget.id = ") {
		want := strings.TrimSpace(query[strings.Index(query, "campaign_budget.id = ")+len("campaign_budget.id = "):])
		if i := strings.IndexAny(want, " \n"); i >= 0 {
			want = want[:i]
		}
		return want == id
	}
	if strings.Contains(query, "campaign_budget.name = ") {
		start := strings.Index(query, "campaign_budget.name = ") + len("campaign_budget.name = ")
		rest := strings.TrimSpace(query[start:])
		rest = strings.Trim(rest, "'")
		return rest == name
	}
	return true
}
