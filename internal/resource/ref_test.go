package resource

import (
	"fmt"
	"reflect"
	"strings"
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

func TestAsResolved(t *testing.T) {
	t.Parallel()

	addr := mustTestAddress(t, "matomo.trigger.trial_started")
	got, ok := AsResolved(Resolved{
		Address:  addr,
		Identity: Identity{ID: "trigger-1"},
		Outputs:  Attributes{"serial": 3},
	})
	if !ok || got.Address != addr || got.Identity.ID != "trigger-1" {
		t.Fatalf("AsResolved(Resolved) = (%v, %v)", got, ok)
	}
	if got.String() != addr.String() {
		t.Fatalf("Resolved.String() = %q, want logical address", got.String())
	}

	if _, ok := AsResolved(Ref{Address: addr}); ok {
		t.Fatal("Ref should not be AsResolved")
	}
	if _, ok := AsResolved(Resolved{}); ok {
		t.Fatal("zero Resolved should not be AsResolved")
	}
	if _, ok := AsResolved("widget-1"); ok {
		t.Fatal("plain identity string should not be AsResolved")
	}
}

func TestMapRefsReplacesNestedAndDoesNotMutate(t *testing.T) {
	t.Parallel()

	parent := mustTestAddress(t, "fake.widget.homepage")
	other := mustTestAddress(t, "fake.widget.sidebar")
	orig := Attributes{
		"title":  "Banner",
		"parent": Ref{Address: parent},
		"meta": map[string]any{
			"also": []any{Ref{Address: other}, "literal"},
		},
	}

	mapped, err := MapRefs(orig, func(path string, ref Ref) (any, error) {
		return Resolved{
			Address:  ref.Address,
			Identity: Identity{ID: "id-" + ref.Address.Name},
			Outputs:  Attributes{"path": path},
		}, nil
	})
	if err != nil {
		t.Fatalf("MapRefs: %v", err)
	}
	attrs, ok := mapped.(Attributes)
	if !ok {
		t.Fatalf("MapRefs type = %T, want Attributes", mapped)
	}

	parentGot, ok := AsResolved(attrs["parent"])
	if !ok || parentGot.Identity.ID != "id-homepage" {
		t.Fatalf("parent = (%v, %v), want resolved homepage identity", parentGot, ok)
	}
	meta := attrs["meta"].(map[string]any)
	also := meta["also"].([]any)
	nested, ok := AsResolved(also[0])
	if !ok || nested.Identity.ID != "id-sidebar" || nested.Outputs["path"] != "meta.also[0]" {
		t.Fatalf("nested = (%v, %v)", nested, ok)
	}
	if also[1] != "literal" {
		t.Fatalf("literal = %v, want unchanged", also[1])
	}

	if _, ok := AsResolved(orig["parent"]); ok {
		t.Fatal("MapRefs mutated original parent")
	}
	origMeta := orig["meta"].(map[string]any)
	origAlso := origMeta["also"].([]any)
	if _, ok := AsRef(origAlso[0]); !ok {
		t.Fatal("MapRefs mutated original nested ref")
	}
}

func TestMapRefsErrorIncludesPath(t *testing.T) {
	t.Parallel()

	_, err := MapRefs(Attributes{
		"parent": Ref{Address: mustTestAddress(t, "fake.widget.missing")},
	}, func(path string, ref Ref) (any, error) {
		return nil, fmt.Errorf("%s: missing %s", path, ref.Address)
	})
	if err == nil {
		t.Fatal("MapRefs succeeded, want error")
	}
	if !strings.Contains(err.Error(), "parent") || !strings.Contains(err.Error(), "fake.widget.missing") {
		t.Fatalf("error = %q, want path and address", err)
	}
}

func TestMapRefsNilCallbackClones(t *testing.T) {
	t.Parallel()

	orig := Attributes{"title": "Homepage"}
	mapped, err := MapRefs(orig, nil)
	if err != nil {
		t.Fatalf("MapRefs: %v", err)
	}
	attrs := mapped.(Attributes)
	attrs["title"] = "Changed"
	if orig["title"] != "Homepage" {
		t.Fatal("nil-callback MapRefs mutated original")
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
