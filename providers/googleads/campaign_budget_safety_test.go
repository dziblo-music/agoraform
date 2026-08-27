package googleads_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestValidateCampaignBudgetRejectsAcceleratedForSearch(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignBudgetProvider(t, nil)
	res := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand daily budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
		googleads.AttrDeliveryMethod:   "ACCELERATED",
	})

	err := p.Validate(context.Background(), res)
	if err == nil {
		t.Fatal("expected accelerated delivery validation error")
	}
	if !strings.Contains(err.Error(), "STANDARD") || !strings.Contains(err.Error(), "Search") {
		t.Fatalf("error = %q, want Search/Standard guidance", err)
	}
}

func TestPlanCampaignBudgetRejectsSharedToDedicated(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{
		"id":               "31",
		"name":             "Shared budget",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": true,
		"period":           "DAILY",
		"type":             "STANDARD",
	})
	p, _ := testCampaignBudgetProvider(t, fake)
	res := campaignBudgetResource(t, "shared", resource.Attributes{
		googleads.AttrName:             "Shared budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	})

	_, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, campaignBudgetIdentities{res.Address.String(): {ID: "31"}})
	if err == nil {
		t.Fatal("expected shared-to-dedicated planning error")
	}
	if !strings.Contains(err.Error(), "true to false") || !strings.Contains(err.Error(), "never become non-shared") {
		t.Fatalf("error = %q, want one-way sharing guidance", err)
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("plan mutated remote: %v", fake.mutates)
	}
}

func TestUpdateCampaignBudgetRejectsSharedToDedicatedBeforeMutation(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{
		"id":               "32",
		"name":             "Shared budget",
		"amountMicros":     "50000000",
		"explicitlyShared": true,
	})
	p, _ := testCampaignBudgetProvider(t, fake)
	desired := campaignBudgetResource(t, "shared", resource.Attributes{
		googleads.AttrName:             "Shared budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	})

	_, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "32"},
	})
	if err == nil {
		t.Fatal("expected shared-to-dedicated update error")
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("invalid transition mutated remote: %v", fake.mutates)
	}
}

func TestPlanDedicatedCampaignBudgetIgnoresCampaignSyncedName(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{
		"id":               "33",
		"name":             "Campaign synced name",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
	})
	p, _ := testCampaignBudgetProvider(t, fake)
	res := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Manifest budget name",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: false,
	})

	got, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, campaignBudgetIdentities{res.Address.String(): {ID: "33"}})
	if err != nil {
		t.Fatalf("plan.BuildWithState: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("campaign-synchronized dedicated budget name produced drift: %+v", got.Changes)
	}
}

func TestUpdateDedicatedCampaignBudgetSendsOnlyChangedAmount(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{
		"id":               "34",
		"name":             "Campaign synced name",
		"amountMicros":     "50000000",
		"deliveryMethod":   "STANDARD",
		"explicitlyShared": false,
	})
	p, _ := testCampaignBudgetProvider(t, fake)
	desired := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Manifest budget name",
		googleads.AttrAmount:           75,
		googleads.AttrExplicitlyShared: false,
	})

	live, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "34"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if strings.Contains(fake.lastMutate, `"name":`) {
		t.Fatalf("dedicated amount-only update rewrote Google-managed name: %s", fake.lastMutate)
	}
	if strings.Contains(fake.lastMutate, `"explicitlyShared":`) {
		t.Fatalf("dedicated amount-only update resent unchanged sharing field: %s", fake.lastMutate)
	}
	if !strings.Contains(fake.lastMutate, `"updateMask":"amountMicros"`) && !strings.Contains(fake.lastMutate, `"updateMask": "amountMicros"`) {
		t.Fatalf("update mask = %s, want amountMicros only", fake.lastMutate)
	}
	if live.Attributes[googleads.AttrName] != "Campaign synced name" {
		t.Fatalf("live name = %v, want Google-managed campaign-synced name", live.Attributes[googleads.AttrName])
	}
	if live.Attributes[googleads.AttrAmount] != int64(75) {
		t.Fatalf("live amount = %v, want 75", live.Attributes[googleads.AttrAmount])
	}
}

func TestUpdateCampaignBudgetToSharedIncludesNameInSameMutation(t *testing.T) {
	t.Parallel()

	fake := newCampaignBudgetFake()
	fake.seed(map[string]any{
		"id":               "35",
		"name":             "Brand budget",
		"amountMicros":     "50000000",
		"explicitlyShared": false,
	})
	p, _ := testCampaignBudgetProvider(t, fake)
	desired := campaignBudgetResource(t, "brand", resource.Attributes{
		googleads.AttrName:             "Brand budget",
		googleads.AttrAmount:           50,
		googleads.AttrExplicitlyShared: true,
	})

	_, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "35"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !strings.Contains(fake.lastMutate, `"name":"Brand budget"`) && !strings.Contains(fake.lastMutate, `"name": "Brand budget"`) {
		t.Fatalf("non-shared to shared update must include name: %s", fake.lastMutate)
	}
	if !strings.Contains(fake.lastMutate, "explicitlyShared") {
		t.Fatalf("non-shared to shared update missing explicitlyShared: %s", fake.lastMutate)
	}
}
