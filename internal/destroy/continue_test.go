package destroy_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/destroy"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestRunContinuesPastFailureAndBlocksPrerequisites(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	child := widget(t, "banner", resource.Attributes{
		fake.AttrTitle:  "Banner",
		fake.AttrParent: resource.Ref{Address: parent.Address},
	})
	unrelated := widget(t, "aaa_footer", resource.Attributes{fake.AttrTitle: "Footer"})
	for _, res := range []resource.Resource{parent, child, unrelated} {
		seed(t, inner, res)
	}
	st := mustStore(t)
	bind(t, st, parent, "id-homepage")
	bind(t, st, child, "id-banner")
	bind(t, st, unrelated, "id-aaa_footer")

	p := &scriptedDestroyer{Provider: inner, failAddr: child.Address.String(), err: errors.New("still in use")}
	desired := []resource.Resource{parent, child, unrelated}
	var out bytes.Buffer
	result, err := destroy.Run(context.Background(), desired, lookupProvider(p), st, &out, nil)

	var incomplete *destroy.IncompleteError
	if !errors.As(err, &incomplete) {
		t.Fatalf("error = %v, want IncompleteError", err)
	}
	if len(incomplete.Failures) != 1 || incomplete.Failures[0].Address != child.Address {
		t.Fatalf("failures = %+v, want only %s", incomplete.Failures, child.Address)
	}
	if len(incomplete.Skipped) != 1 || incomplete.Skipped[0].Address != parent.Address || incomplete.Skipped[0].Cause != child.Address {
		t.Fatalf("skipped = %+v, want %s blocked by %s", incomplete.Skipped, parent.Address, child.Address)
	}
	if !strings.Contains(err.Error(), "still in use") || !strings.Contains(err.Error(), "fake.widget.homepage (blocked by fake.widget.banner)") {
		t.Fatalf("error does not name the failure and the block:\n%s", err.Error())
	}
	if result.Destroyed != 1 || result.Failed != 1 || result.Skipped != 1 {
		t.Fatalf("result = %+v, want 1 destroyed, 1 failed, 1 skipped", result)
	}
	if inner.Destroys() != 1 {
		t.Fatalf("Destroys() = %d, want only the unrelated widget", inner.Destroys())
	}
	assertMissing(t, st, unrelated.Address)
	for _, addr := range []resource.Address{parent.Address, child.Address} {
		if _, ok, _ := st.Identity(addr); !ok {
			t.Fatalf("%s binding was removed", addr)
		}
	}
	for _, want := range []string{
		"fake.widget.banner: failed:",
		"fake.widget.homepage: skipped; fake.widget.banner depends on it and was not destroyed",
		"Destroy incomplete! 1 destroyed, 1 failed, 1 skipped.",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}

	p.failAddr = ""
	result, err = destroy.Run(context.Background(), desired, lookupProvider(p), st, nil, nil)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if result.Destroyed != 2 {
		t.Fatalf("retry result = %+v, want only the failed and blocked widgets", result)
	}
	assertMissing(t, st, parent.Address, child.Address)
}

func TestRunFinalizationSkippedOnlyForIncompleteProvider(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	ok := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	bad := widget(t, "banner", resource.Attributes{fake.AttrTitle: "Banner"})
	seed(t, inner, ok)
	seed(t, inner, bad)
	note := resource.Resource{Address: mustAddress(t, "alt.note.welcome")}

	st := mustStore(t)
	bind(t, st, ok, "id-homepage")
	bind(t, st, bad, "id-banner")
	bind(t, st, note, "id-welcome")

	fin := &finalizingDestroyer{
		Provider: &scriptedDestroyer{Provider: inner, failAddr: bad.Address.String(), err: errors.New("boom")},
		plan:     &provider.FinalizationPlan{Address: ok.Address, Action: "publish", Target: "live"},
	}
	alt := &altDestroyer{Alt: fake.NewAlt()}
	lookup := func(addr resource.Address) (provider.Provider, error) {
		if addr.Provider == fake.AltName {
			return alt, nil
		}
		return fin, nil
	}

	var out bytes.Buffer
	result, err := destroy.Run(context.Background(), []resource.Resource{ok, bad, note}, lookup, st, &out, nil)
	var incomplete *destroy.IncompleteError
	if !errors.As(err, &incomplete) {
		t.Fatalf("error = %v, want IncompleteError", err)
	}
	if fin.finalizeCalled {
		t.Fatal("publication ran although a fake resource failed")
	}
	if len(incomplete.SkippedFinalizations) != 1 || !strings.Contains(err.Error(), "fake.widget.homepage: publish") {
		t.Fatalf("skipped finalizations = %+v\n%s", incomplete.SkippedFinalizations, err)
	}
	if result.Destroyed != 2 || result.Failed != 1 {
		t.Fatalf("result = %+v, want 2 destroyed and 1 failed", result)
	}
	assertMissing(t, st, note.Address)
	if _, bound, _ := st.Identity(ok.Address); !bound {
		t.Fatal("destroyed fake widget lost its binding before its provider finalization ran")
	}
	if !strings.Contains(out.String(), "publish skipped") {
		t.Fatalf("output does not report the skipped publication:\n%s", out.String())
	}
}

func TestRunBindsPlannedIdentitiesForFinalization(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	child := widget(t, "banner", resource.Attributes{fake.AttrTitle: "Banner"})
	seed(t, inner, child)
	external := resource.Resource{
		Address:    mustAddress(t, "fake.widget.site"),
		Ownership:  resource.OwnershipExternal,
		ExternalID: "id-site",
	}
	st := mustStore(t)
	bind(t, st, child, "id-banner")

	p := &bindingDestroyer{Provider: inner, bound: map[string]string{}}
	if _, err := destroy.Run(context.Background(), []resource.Resource{child, external}, lookupProvider(p), st, nil, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := p.bound["fake.widget.site"]; got != "id-site" {
		t.Fatalf("external identity bound = %q, want lifecycle.id from the manifest", got)
	}
	if got := p.bound["fake.widget.banner"]; got != "id-banner" {
		t.Fatalf("managed identity bound = %q, want state identity", got)
	}
}

type altDestroyer struct {
	*fake.Alt
}

func (p *altDestroyer) DestroyCapability(resource.Resource) (provider.DestroyCapability, error) {
	return provider.DestroyDelete, nil
}

func (p *altDestroyer) Destroy(context.Context, resource.Resource) (provider.DestroyResult, error) {
	return provider.DestroyResult{Status: provider.DestroyStatusDestroyed}, nil
}

type bindingDestroyer struct {
	provider.Provider
	bound map[string]string
}

func (p *bindingDestroyer) BindIdentity(addr resource.Address, id resource.Identity) {
	p.bound[addr.String()] = id.ID
}

func (p *bindingDestroyer) DestroyCapability(resource.Resource) (provider.DestroyCapability, error) {
	return provider.DestroyDelete, nil
}

func (p *bindingDestroyer) Destroy(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	return p.Provider.(provider.Destroyer).Destroy(ctx, res)
}

func TestRunCancellationKeepsRecordedFailures(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	st := mustStore(t)
	var desired []resource.Resource
	for _, name := range []string{"one", "two", "three"} {
		res := widget(t, name, resource.Attributes{fake.AttrTitle: name})
		seed(t, inner, res)
		bind(t, st, res, "id-"+name)
		desired = append(desired, res)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &cancellingDestroyer{Provider: inner, cancel: cancel}
	_, err := destroy.Run(ctx, desired, lookupProvider(p), st, nil, nil)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	var incomplete *destroy.IncompleteError
	if !errors.As(err, &incomplete) || len(incomplete.Failures) != 1 {
		t.Fatalf("error = %v, want the failure recorded before cancellation", err)
	}
	if p.calls != 1 {
		t.Fatalf("Destroy calls = %d, want the run to stop after cancellation", p.calls)
	}
}

// cancellingDestroyer fails its first destroy and cancels the run, as an
// interrupt during a provider call would.
type cancellingDestroyer struct {
	provider.Provider
	cancel context.CancelFunc
	calls  int
}

func (p *cancellingDestroyer) DestroyCapability(resource.Resource) (provider.DestroyCapability, error) {
	return provider.DestroyDelete, nil
}

func (p *cancellingDestroyer) Destroy(context.Context, resource.Resource) (provider.DestroyResult, error) {
	p.calls++
	p.cancel()
	return provider.DestroyResult{}, errors.New("interrupted")
}
