package meta

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMoneyAttributesUseAccountCurrencyUnits(t *testing.T) {
	t.Parallel()

	for attr, want := range map[string]string{
		AttrDailyBudget:    "dailyBudget",
		AttrLifetimeBudget: "lifetimeBudget",
		AttrBidAmount:      "bidAmount",
	} {
		if attr != want {
			t.Errorf("attribute name = %q, want %q", attr, want)
		}
	}
	for _, legacy := range []string{"dailyBudgetMinorUnits", "lifetimeBudgetMinorUnits"} {
		for name, attrs := range map[string]map[string]struct{}{
			"campaign": supportedCampaignAttrs,
			"ad_set":   supportedAdSetAttrs,
		} {
			if _, ok := attrs[legacy]; ok {
				t.Errorf("%s still supports minimum-denomination attribute %q", name, legacy)
			}
		}
	}
}

func TestParseAmountAcceptsAccountCurrencyUnits(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		input any
		want  amount
	}{
		{"integer", 20, amount(2000)},
		{"int64", int64(20), amount(2000)},
		{"float", 20.5, amount(2050)},
		{"float cents", 20.55, amount(2055)},
		{"string", "20.50", amount(2050)},
		{"json number", json.Number("0.99"), amount(99)},
		{"leading dot", ".99", amount(99)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAmount(tc.input)
			if err != nil || got != tc.want {
				t.Fatalf("parseAmount(%v) = %v, %v; want %v", tc.input, got, err, tc.want)
			}
		})
	}
}

func TestParseAmountRejectsUnusableValues(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		input    any
		contains string
	}{
		{"zero", 0, "greater than 0"},
		{"negative", -20, "greater than 0"},
		{"sub-cent float", 20.555, "at most 2 decimal places"},
		{"sub-cent string", "20.555", "at most 2 decimal places"},
		{"words", "twenty", "account-currency units"},
		{"exponent", "2e3", "account-currency units"},
		{"boolean", true, "account-currency units"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseAmount(tc.input); err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("parseAmount(%v) error = %v, want %q", tc.input, err, tc.contains)
			}
		})
	}
}

func TestAmountRoundTripsThroughEachCurrencyOffset(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		currency string
		declared any
		minimum  int64
		value    any
	}{
		{"USD", 20, 2000, int64(20)},
		{"USD", 20.5, 2050, 20.5},
		{"EUR", "12.34", 1234, 12.34},
		{"JPY", 2000, 2000, int64(2000)},
		{"KRW", 5000, 5000, int64(5000)},
		{"BHD", 20, 2000, int64(20)},
	} {
		t.Run(tc.currency, func(t *testing.T) {
			parsed, err := parseAmount(tc.declared)
			if err != nil {
				t.Fatal(err)
			}
			minimum, err := parsed.minimumDenomination(tc.currency)
			if err != nil || minimum != tc.minimum {
				t.Fatalf("minimumDenomination = %d, %v; want %d", minimum, err, tc.minimum)
			}
			back, err := amountFromMinimumDenomination(minimum, tc.currency)
			if err != nil {
				t.Fatal(err)
			}
			if back != parsed {
				t.Fatalf("round trip = %v, want %v", back, parsed)
			}
			if got := back.value(); got != tc.value {
				t.Fatalf("value = %#v, want %#v", got, tc.value)
			}
		})
	}
}

func TestAmountRejectsFractionsOfAZeroDecimalCurrency(t *testing.T) {
	t.Parallel()

	parsed, err := parseAmount(2000.5)
	if err != nil {
		t.Fatal(err)
	}
	_, err = parsed.minimumDenomination("JPY")
	if err == nil || !strings.Contains(err.Error(), "JPY amounts must be whole numbers") {
		t.Fatalf("error = %v, want a JPY precision failure", err)
	}
}

func TestUnsupportedCurrencyIsRejectedRatherThanGuessed(t *testing.T) {
	t.Parallel()

	if _, err := currencyOffset("XYZ"); err == nil || !strings.Contains(err.Error(), "not a supported Meta advertising currency") {
		t.Fatalf("error = %v, want an unsupported-currency failure", err)
	}
}
