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
