package googleads_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestValidateCampaignTargetROASRange(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignProvider(t, nil)
	cases := []struct {
		name     string
		strategy string
		value    float64
		wantErr  bool
	}{
		{name: "target roas below minimum", strategy: "TARGET_ROAS", value: 0.009, wantErr: true},
		{name: "target roas minimum", strategy: "TARGET_ROAS", value: 0.01},
		{name: "target roas maximum", strategy: "TARGET_ROAS", value: 1000},
		{name: "target roas above maximum", strategy: "TARGET_ROAS", value: 1000.001, wantErr: true},
		{name: "maximize value clear", strategy: "MAXIMIZE_CONVERSION_VALUE", value: 0},
		{name: "maximize value below minimum", strategy: "MAXIMIZE_CONVERSION_VALUE", value: 0.009, wantErr: true},
		{name: "maximize value minimum", strategy: "MAXIMIZE_CONVERSION_VALUE", value: 0.01},
		{name: "maximize value maximum", strategy: "MAXIMIZE_CONVERSION_VALUE", value: 1000},
		{name: "maximize value above maximum", strategy: "MAXIMIZE_CONVERSION_VALUE", value: 1000.001, wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			attrs := defaultCampaignAttrs(t)
			attrs[googleads.AttrBidding] = map[string]any{
				"strategy":   tc.strategy,
				"targetRoas": tc.value,
			}
			err := p.Validate(context.Background(), campaignResource(t, "brand", attrs))
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "targetRoas") {
					t.Fatalf("Validate error = %v, want targetRoas range error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestValidateCampaignCalendarDates(t *testing.T) {
	t.Parallel()

	p, _ := testCampaignProvider(t, nil)
	cases := []struct {
		name    string
		date    string
		wantErr bool
	}{
		{name: "invalid day", date: "2026-02-31", wantErr: true},
		{name: "invalid month", date: "2026-13-01", wantErr: true},
		{name: "non leap day", date: "2027-02-29", wantErr: true},
		{name: "valid leap day", date: "2028-02-29"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			attrs := defaultCampaignAttrs(t)
			attrs[googleads.AttrStartDate] = tc.date
			err := p.Validate(context.Background(), campaignResource(t, "brand", attrs))
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "valid calendar date") {
					t.Fatalf("Validate error = %v, want calendar-date error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestUpdateCampaignClearsTrackingFields(t *testing.T) {
	t.Parallel()

	fake := newCampaignFake()
	fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false})
	campaign := sampleSearchCampaign("21", "Brand", "11")
	campaign["trackingUrlTemplate"] = "https://tracker.example/{lpurl}"
	campaign["finalUrlSuffix"] = "utm_source=google"
	fake.seedCampaign(campaign)
	p, _ := testCampaignProvider(t, fake)

	desired := campaignResource(t, "brand", resource.Attributes{
		googleads.AttrName: "Brand",
		googleads.AttrBudget: resource.Resolved{
			Address:  mustCampaignBudgetAddress(t, "brand"),
			Identity: resource.Identity{ID: "11"},
			Outputs:  resource.Attributes{"resourceName": "customers/" + testCustomerID + "/campaignBudgets/11"},
		},
		googleads.AttrBidding:             map[string]any{"strategy": "MANUAL_CPC"},
		googleads.AttrTrackingUrlTemplate: "",
		googleads.AttrFinalUrlSuffix:      "",
	})
	actual := resource.RemoteResource{Address: desired.Address, Identity: resource.Identity{ID: "21"}}

	if _, err := p.Update(context.Background(), desired, actual); err != nil {
		t.Fatalf("Update: %v", err)
	}
	for _, want := range []string{"trackingUrlTemplate", "finalUrlSuffix"} {
		if !strings.Contains(fake.lastMutate, want) {
			t.Fatalf("mutate missing %s clear: %s", want, fake.lastMutate)
		}
	}
	if !strings.Contains(fake.lastMutate, `"trackingUrlTemplate":""`) || !strings.Contains(fake.lastMutate, `"finalUrlSuffix":""`) {
		t.Fatalf("mutate did not send explicit empty-string clears: %s", fake.lastMutate)
	}
	mutates := len(fake.mutates)
	if _, err := p.Update(context.Background(), desired, actual); err != nil {
		t.Fatalf("second Update: %v", err)
	}
	if len(fake.mutates) != mutates {
		t.Fatalf("equivalent cleared state mutated again: before=%d after=%d", mutates, len(fake.mutates))
	}
}

func TestUpdateCampaignClearsOptionalBiddingTargets(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		apiType    string
		remoteKey  string
		remoteBody map[string]any
		desired    map[string]any
		maskPath   string
	}{
		{
			name:       "maximize clicks cpc ceiling",
			apiType:    "TARGET_SPEND",
			remoteKey:  "targetSpend",
			remoteBody: map[string]any{"cpcBidCeilingMicros": "3000000"},
			desired:    map[string]any{"strategy": "MAXIMIZE_CLICKS", "cpcBidCeiling": 0},
			maskPath:   "targetSpend.cpcBidCeilingMicros",
		},
		{
			name:       "maximize conversions target cpa",
			apiType:    "MAXIMIZE_CONVERSIONS",
			remoteKey:  "maximizeConversions",
			remoteBody: map[string]any{"targetCpaMicros": "25000000"},
			desired:    map[string]any{"strategy": "MAXIMIZE_CONVERSIONS", "targetCpa": 0},
			maskPath:   "maximizeConversions.targetCpaMicros",
		},
		{
			name:       "maximize conversion value target roas",
			apiType:    "MAXIMIZE_CONVERSION_VALUE",
			remoteKey:  "maximizeConversionValue",
			remoteBody: map[string]any{"targetRoas": 2.5},
			desired:    map[string]any{"strategy": "MAXIMIZE_CONVERSION_VALUE", "targetRoas": 0},
			maskPath:   "maximizeConversionValue.targetRoas",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := newCampaignFake()
			fake.seedBudget(map[string]any{"id": "11", "name": "Brand daily budget", "amountMicros": "50000000", "explicitlyShared": false})
			campaign := sampleSearchCampaign("21", "Brand", "11")
			delete(campaign, "manualCpc")
			campaign["biddingStrategyType"] = tc.apiType
			campaign[tc.remoteKey] = tc.remoteBody
			fake.seedCampaign(campaign)
			p, _ := testCampaignProvider(t, fake)

			desired := campaignResource(t, "brand", resource.Attributes{
				googleads.AttrName: "Brand",
				googleads.AttrBudget: resource.Resolved{
					Address:  mustCampaignBudgetAddress(t, "brand"),
					Identity: resource.Identity{ID: "11"},
					Outputs:  resource.Attributes{"resourceName": "customers/" + testCustomerID + "/campaignBudgets/11"},
				},
				googleads.AttrBidding: tc.desired,
			})
			actual := resource.RemoteResource{Address: desired.Address, Identity: resource.Identity{ID: "21"}}

			if _, err := p.Update(context.Background(), desired, actual); err != nil {
				t.Fatalf("Update: %v", err)
			}
			if !strings.Contains(fake.lastMutate, tc.maskPath) {
				t.Fatalf("mutate missing clear mask %s: %s", tc.maskPath, fake.lastMutate)
			}
			mutates := len(fake.mutates)
			if _, err := p.Update(context.Background(), desired, actual); err != nil {
				t.Fatalf("second Update: %v", err)
			}
			if len(fake.mutates) != mutates {
				t.Fatalf("equivalent cleared state mutated again: before=%d after=%d", mutates, len(fake.mutates))
			}
		})
	}
}
