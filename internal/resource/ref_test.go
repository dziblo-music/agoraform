package resource

import (
	"reflect"
	"testing"
)

func TestAsRef(t *testing.T) {
	t.Parallel()

	addr := mustTestAddress(t, "matomo.trigger.trial_started")

	got, ok := AsRef(Ref{Address: addr})
	if !ok || got.Address != addr {
		t.Fatalf("AsRef(Ref) = (%v, %v)", got, ok)
	}

	if _, ok := AsRef("matomo.trigger.trial_started"); ok {
		t.Fatal("plain address-looking string should not be a reference")
	}
	if _, ok := AsRef("trialStarted"); ok {
		t.Fatal("plain string should not be a reference")
	}
	if _, ok := AsRef(1); ok {
		t.Fatal("number should not be a reference")
	}
	if _, ok := AsRef(Ref{}); ok {
		t.Fatal("zero Ref should not be a reference")
	}
}

func TestWalkRefsDeterministicAndNested(t *testing.T) {
	t.Parallel()

	attrs := Attributes{
		"type": "matomoAnalytics",
		"trigger": Ref{
			Address: mustTestAddress(t, "matomo.trigger.trial_started"),
		},
		"meta": map[string]any{
			"primary": Ref{Address: mustTestAddress(t, "matomo.variable.user_id")},
			"extra": []any{
				"not-an-address",
				Ref{Address: mustTestAddress(t, "matomo.trigger.signup")},
			},
		},
	}

	var paths []string
	var addrs []string
	WalkRefs(attrs, func(path string, addr Address) {
		paths = append(paths, path)
		addrs = append(addrs, addr.String())
	})

	wantPaths := []string{"meta.extra[1]", "meta.primary", "trigger"}
	wantAddrs := []string{"matomo.trigger.signup", "matomo.variable.user_id", "matomo.trigger.trial_started"}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("paths = %v, want %v", paths, wantPaths)
	}
	if !reflect.DeepEqual(addrs, wantAddrs) {
		t.Fatalf("addrs = %v, want %v", addrs, wantAddrs)
	}
}

func TestWalkRefsIgnoresAddressLookingStrings(t *testing.T) {
	t.Parallel()

	WalkRefs(Attributes{
		"pattern": "checkout.step.complete",
		"nested": map[string]any{
			"value": "matomo.trigger.trial_started",
		},
	}, func(path string, addr Address) {
		t.Fatalf("unexpected reference at %s -> %s", path, addr)
	})
}

func TestWalkRefsIgnoresNilCallbackAndEmpty(t *testing.T) {
	t.Parallel()

	WalkRefs(Attributes{"title": "Homepage"}, nil)
	WalkRefs(nil, func(string, Address) {
		t.Fatal("nil value should not invoke callback")
	})
}

func TestRefStringAndZero(t *testing.T) {
	t.Parallel()

	if !(Ref{}).IsZero() {
		t.Fatal("empty Ref should be zero")
	}
	ref := Ref{Address: mustTestAddress(t, "fake.widget.homepage")}
	if ref.IsZero() {
		t.Fatal("non-empty Ref should not be zero")
	}
	if ref.String() != "fake.widget.homepage" {
		t.Fatalf("String() = %q", ref.String())
	}
}

func mustTestAddress(t *testing.T, s string) Address {
	t.Helper()
	addr, err := ParseAddress(s)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}
