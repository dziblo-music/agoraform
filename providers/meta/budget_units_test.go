package meta

import "testing"

func TestBudgetAttributeNamesMakeMinorUnitsExplicit(t *testing.T) {
	t.Parallel()

	if AttrDailyBudget != "dailyBudgetMinorUnits" {
		t.Fatalf("AttrDailyBudget = %q, want dailyBudgetMinorUnits", AttrDailyBudget)
	}
	if AttrLifetimeBudget != "lifetimeBudgetMinorUnits" {
		t.Fatalf("AttrLifetimeBudget = %q, want lifetimeBudgetMinorUnits", AttrLifetimeBudget)
	}
}

func TestLegacyAmbiguousBudgetAttributeNamesAreUnsupported(t *testing.T) {
	t.Parallel()

	for resourceType, attrs := range map[string]map[string]struct{}{
		"campaign": supportedCampaignAttrs,
		"ad_set":   supportedAdSetAttrs,
	} {
		for _, legacy := range []string{"dailyBudget", "lifetimeBudget"} {
			if _, ok := attrs[legacy]; ok {
				t.Fatalf("%s still supports ambiguous legacy attribute %q", resourceType, legacy)
			}
		}
	}
}
