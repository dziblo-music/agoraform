package resource

import "testing"

func TestParseAddressRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []string{
		"matomo.goal.trial_started",
		"fake.widget.homepage",
		"googleads.conversion_action.purchase",
		"a.b.c",
		"provider_one.type2.name_3",
	}
	for _, in := range cases {
		in := in
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			addr, err := ParseAddress(in)
			if err != nil {
				t.Fatalf("ParseAddress(%q) unexpected error: %v", in, err)
			}
			if got := addr.String(); got != in {
				t.Fatalf("String() = %q, want %q", got, in)
			}
			if err := addr.Validate(); err != nil {
				t.Fatalf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestParseAddressInvalid(t *testing.T) {
	t.Parallel()

	cases := []string{
		"",
		"   ",
		"matomo.goal",
		"matomo.goal.trial.started",
		"matomo..trial_started",
		".goal.trial_started",
		"matomo.goal.",
		"Matomo.goal.trial_started",
		"matomo.Goal.trial_started",
		"matomo.goal.Trial_started",
		"matomo.goal.trial-started",
		"1matomo.goal.name",
		"matomo.2goal.name",
		"matomo.goal.3name",
		"matomo.goal.trial started",
		"matomo.goal.trial.started.extra",
	}
	for _, in := range cases {
		in := in
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			if _, err := ParseAddress(in); err == nil {
				t.Fatalf("ParseAddress(%q) succeeded, want error", in)
			}
		})
	}
}

func TestAddressValidateZero(t *testing.T) {
	t.Parallel()

	var addr Address
	if !addr.IsZero() {
		t.Fatal("zero Address should report IsZero")
	}
	if err := addr.Validate(); err == nil {
		t.Fatal("zero Address should fail Validate")
	}
}
