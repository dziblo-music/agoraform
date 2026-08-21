package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestRegistryRegisterAndLookup(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	p := fake.New()
	if err := reg.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if reg.Len() != 1 {
		t.Fatalf("Len = %d, want 1", reg.Len())
	}

	got, ok := reg.Lookup("fake")
	if !ok {
		t.Fatal("Lookup(fake) missed registered provider")
	}
	if got.Name() != "fake" {
		t.Fatalf("Lookup name = %q, want fake", got.Name())
	}

	addr, err := resource.ParseAddress("fake.widget.homepage")
	if err != nil {
		t.Fatal(err)
	}
	got, err = reg.LookupFor(addr)
	if err != nil {
		t.Fatalf("LookupFor: %v", err)
	}
	if got.Name() != "fake" {
		t.Fatalf("LookupFor name = %q, want fake", got.Name())
	}
}

func TestRegistryDuplicateAndInvalid(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := reg.Register(fake.New()); err == nil {
		t.Fatal("duplicate Register succeeded")
	}
	if err := reg.Register(nil); err == nil {
		t.Fatal("nil Register succeeded")
	}
	if err := reg.Register(namedProvider{name: "Fake"}); err == nil {
		t.Fatal("invalid provider name Register succeeded")
	}
}

func TestRegistryUnknownProviderAndType(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatalf("Register: %v", err)
	}

	unknownProvider, err := resource.ParseAddress("matomo.goal.trial_started")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.LookupFor(unknownProvider); err == nil {
		t.Fatal("LookupFor unknown provider succeeded")
	}

	unknownType, err := resource.ParseAddress("fake.goal.trial_started")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.LookupFor(unknownType); err == nil {
		t.Fatal("LookupFor unknown type succeeded")
	}

	if _, ok := reg.Lookup("matomo"); ok {
		t.Fatal("Lookup of unregistered provider succeeded")
	}
}

func TestSupports(t *testing.T) {
	t.Parallel()

	p := fake.New()
	if !provider.Supports(p, fake.TypeWidget) {
		t.Fatal("expected fake provider to support widget")
	}
	if provider.Supports(p, "goal") {
		t.Fatal("fake provider should not support goal")
	}
	if provider.Supports(nil, fake.TypeWidget) {
		t.Fatal("nil provider should not support any type")
	}
}

func TestErrNotFoundSentinel(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := resource.Resource{
		Address:    mustAddress(t, "fake.widget.missing"),
		Attributes: resource.Attributes{"title": "Missing"},
	}
	_, err := p.Read(context.Background(), res)
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read missing = %v, want ErrNotFound", err)
	}
}

type namedProvider struct {
	name string
	provider.Provider
}

func (p namedProvider) Name() string { return p.name }

func mustAddress(t *testing.T, s string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress(s)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}
