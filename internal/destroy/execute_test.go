package destroy_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/destroy"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
)

const plantedSecret = "super-secret-token-value"

func TestRunReverseDependencyOrder(t *testing.T) {
	t.Parallel()

	p := fake.New()
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	child := widget(t, "banner", resource.Attributes{
		fake.AttrTitle:  "Banner",
		fake.AttrParent: resource.Ref{Address: parent.Address},
	})
	seed(t, p, parent)
	seed(t, p, child)

	st := mustStore(t)
	bind(t, st, parent, "id-homepage")
	bind(t, st, child, "id-banner")

	rec := &recordingDestroyer{Provider: p}
	var out bytes.Buffer
	result, err := destroy.Run(context.Background(), []resource.Resource{parent, child}, lookupProvider(rec), st, &out, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Destroyed != 2 || result.Remaining != 0 {
		t.Fatalf("result = %+v, want 2 destroyed", result)
	}
	if got := rec.destroyed; !equalStrings(got, []string{"fake.widget.banner", "fake.widget.homepage"}) {
		t.Fatalf("destroy order = %v, want banner then homepage", got)
	}
	assertMissing(t, st, parent.Address, child.Address)
	if !strings.Contains(out.String(), "Agoraform will destroy the following resources:") {
		t.Fatalf("plan missing from output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Destroy complete! 2 destroyed.") {
		t.Fatalf("summary missing:\n%s", out.String())
	}
}

func TestRunUnrelatedResourcesReverseAddressOrder(t *testing.T) {
	t.Parallel()

	p := fake.New()
	alpha := widget(t, "alpha", resource.Attributes{fake.AttrTitle: "A"})
	mu := widget(t, "mu", resource.Attributes{fake.AttrTitle: "M"})
	zeta := widget(t, "zeta", resource.Attributes{fake.AttrTitle: "Z"})
	for _, res := range []resource.Resource{zeta, alpha, mu} {
		seed(t, p, res)
	}

	st := mustStore(t)
	bind(t, st, alpha, "id-alpha")
	bind(t, st, mu, "id-mu")
	bind(t, st, zeta, "id-zeta")

	rec := &recordingDestroyer{Provider: p}
	_, err := destroy.Run(context.Background(), []resource.Resource{zeta, alpha, mu}, lookupProvider(rec), st, nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := rec.destroyed; !equalStrings(got, []string{"fake.widget.zeta", "fake.widget.mu", "fake.widget.alpha"}) {
		t.Fatalf("destroy order = %v, want reverse address order", got)
	}
}

func TestBuildInvalidGraphIsZeroMutation(t *testing.T) {
	t.Parallel()

	p := fake.New()
	child := widget(t, "banner", resource.Attributes{
		fake.AttrTitle:  "Banner",
		fake.AttrParent: resource.Ref{Address: mustAddress(t, "fake.widget.missing")},
	})
	st := mustStore(t)
	bind(t, st, child, "id-banner")

	_, err := destroy.Build(context.Background(), []resource.Resource{child}, lookupProvider(p), st)
	if err == nil {
		t.Fatal("Build succeeded, want invalid graph")
	}
	if p.Destroys() != 0 {
		t.Fatalf("Destroys() = %d, want 0", p.Destroys())
	}
}

func TestRunAlreadyAbsentRemovesState(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	st := mustStore(t)
	bind(t, st, res, "id-homepage")

	var out bytes.Buffer
	result, err := destroy.Run(context.Background(), []resource.Resource{res}, lookupProvider(p), st, &out, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Destroyed != 0 || result.AlreadyAbsent != 1 {
		t.Fatalf("result = %+v, want 1 already absent", result)
	}
	if !strings.Contains(out.String(), "already absent") {
		t.Fatalf("output missing already absent:\n%s", out.String())
	}
	assertMissing(t, st, res.Address)
}

func TestRunUnsupportedDoesNotBlockSupported(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	supported := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	blocked := widget(t, "platform", resource.Attributes{fake.AttrTitle: "Platform"})
	seed(t, inner, supported)
	seed(t, inner, blocked)

	st := mustStore(t)
	bind(t, st, supported, "id-homepage")
	bind(t, st, blocked, "id-platform")

	p := &capabilityProvider{Provider: inner, unsupported: map[string]provider.DestroyCapability{
		blocked.Address.String(): provider.DestroyUnsupported,
	}}
	var out bytes.Buffer
	result, err := destroy.Run(context.Background(), []resource.Resource{supported, blocked}, lookupProvider(p), st, &out, nil)
	if err == nil {
		t.Fatal("Run succeeded, want remaining error")
	}
	var remaining *destroy.RemainingError
	if !errors.As(err, &remaining) {
		t.Fatalf("error = %v, want RemainingError", err)
	}
	if result.Destroyed != 1 || result.Remaining != 1 {
		t.Fatalf("result = %+v, want 1 destroyed and 1 remaining", result)
	}
	assertMissing(t, st, supported.Address)
	id, ok, identErr := st.Identity(blocked.Address)
	if identErr != nil || !ok || id.ID != "id-platform" {
		t.Fatalf("unsupported identity = (%v, %v, %v)", id, ok, identErr)
	}
	if !strings.Contains(out.String(), "cannot be destroyed") {
		t.Fatalf("plan missing unsupported section:\n%s", out.String())
	}
}

func TestRunPreservesStateOnlyIdentities(t *testing.T) {
	t.Parallel()

	p := fake.New()
	managed := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	orphan := widget(t, "orphan", resource.Attributes{fake.AttrTitle: "Orphan"})
	seed(t, p, managed)
	seed(t, p, orphan)

	st := mustStore(t)
	bind(t, st, managed, "id-homepage")
	bind(t, st, orphan, "id-orphan")

	var out bytes.Buffer
	_, err := destroy.Run(context.Background(), []resource.Resource{managed}, lookupProvider(p), st, &out, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertMissing(t, st, managed.Address)
	id, ok, identErr := st.Identity(orphan.Address)
	if identErr != nil || !ok || id.ID != "id-orphan" {
		t.Fatalf("orphan identity = (%v, %v, %v)", id, ok, identErr)
	}
	if !strings.Contains(out.String(), "fake.widget.orphan") || !strings.Contains(out.String(), "not in the manifest") {
		t.Fatalf("plan missing preserved state identity:\n%s", out.String())
	}
}

func TestRunNotManagedSkipsRemote(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, p, res)
	st := mustStore(t)

	var out bytes.Buffer
	result, err := destroy.Run(context.Background(), []resource.Resource{res}, lookupProvider(p), st, &out, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Destroyed != 0 || p.Destroys() != 0 {
		t.Fatalf("result = %+v destroys=%d, want no mutations", result, p.Destroys())
	}
	if !strings.Contains(out.String(), "not managed in local state") {
		t.Fatalf("output missing not-managed note:\n%s", out.String())
	}
}

func TestRunProviderFailurePreservesRemainingState(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	first := widget(t, "banner", resource.Attributes{fake.AttrTitle: "Banner"})
	second := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, inner, first)
	seed(t, inner, second)

	st := mustStore(t)
	bind(t, st, first, "id-banner")
	bind(t, st, second, "id-homepage")

	p := &scriptedDestroyer{Provider: inner, failAddr: first.Address.String(), err: errors.New("remote destroy rejected")}
	result, err := destroy.Run(context.Background(), []resource.Resource{first, second}, lookupProvider(p), st, nil, nil)
	if err == nil {
		t.Fatal("Run succeeded, want provider failure")
	}
	if strings.Contains(err.Error(), plantedSecret) {
		t.Fatalf("secret leaked in error: %v", err)
	}
	if result.Destroyed != 1 {
		t.Fatalf("result = %+v, want first reverse-order resource destroyed", result)
	}
	assertMissing(t, st, second.Address)
	id, ok, identErr := st.Identity(first.Address)
	if identErr != nil || !ok || id.ID != "id-banner" {
		t.Fatalf("failed identity = (%v, %v, %v)", id, ok, identErr)
	}
}

func TestRunRetryAfterPartialFailure(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	first := widget(t, "banner", resource.Attributes{fake.AttrTitle: "Banner"})
	second := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, inner, first)
	seed(t, inner, second)

	st := mustStore(t)
	bind(t, st, first, "id-banner")
	bind(t, st, second, "id-homepage")

	p := &scriptedDestroyer{Provider: inner, failAddr: first.Address.String(), err: errors.New("remote destroy rejected")}
	if _, err := destroy.Run(context.Background(), []resource.Resource{first, second}, lookupProvider(p), st, nil, nil); err == nil {
		t.Fatal("first Run succeeded, want failure")
	}

	p.failAddr = ""
	result, err := destroy.Run(context.Background(), []resource.Resource{first, second}, lookupProvider(p), st, nil, nil)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if result.Destroyed != 1 {
		t.Fatalf("retry result = %+v, want remaining resource destroyed", result)
	}
	assertMissing(t, st, first.Address, second.Address)
}

func TestRunStateWriteFailureAfterDestroyPreservesBinding(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, p, res)

	inner := mustStore(t)
	bind(t, inner, res, "id-homepage")
	st := &failingRemoveStore{Store: inner, err: errors.New("disk full")}

	_, err := destroy.Run(context.Background(), []resource.Resource{res}, lookupProvider(p), st, nil, nil)
	if err == nil {
		t.Fatal("Run succeeded, want persist failure")
	}
	if !destroy.IsPartial(err) {
		t.Fatalf("error = %v, want PartialError", err)
	}
	if !strings.Contains(err.Error(), "local state write failed") {
		t.Fatalf("error = %q, want persist guidance", err.Error())
	}
	id, ok, identErr := inner.Identity(res.Address)
	if identErr != nil || !ok || id.ID != "id-homepage" {
		t.Fatalf("binding after persist failure = (%v, %v, %v)", id, ok, identErr)
	}
	if _, err := p.Read(context.Background(), resource.Resource{Address: res.Address, Identity: resource.Identity{ID: "id-homepage"}}); !errors.Is(err, provider.ErrNotFound) {
		t.Fatal("remote object should already be gone")
	}
}

func TestRunDeclinedApprovalIsZeroMutation(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, p, res)
	st := mustStore(t)
	bind(t, st, res, "id-homepage")

	_, err := destroy.Run(context.Background(), []resource.Resource{res}, lookupProvider(p), st, nil, func(*destroy.Plan) error {
		return errCancelled
	})
	if !errors.Is(err, errCancelled) {
		t.Fatalf("error = %v, want cancelled", err)
	}
	if p.Destroys() != 0 {
		t.Fatalf("Destroys() = %d, want 0", p.Destroys())
	}
	id, ok, identErr := st.Identity(res.Address)
	if identErr != nil || !ok || id.ID != "id-homepage" {
		t.Fatalf("identity after cancel = (%v, %v, %v)", id, ok, identErr)
	}
}

func TestRunFinalizationAfterDestroy(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, inner, res)
	st := mustStore(t)
	bind(t, st, res, "id-homepage")

	p := &finalizingDestroyer{
		Provider: inner,
		plan: &provider.FinalizationPlan{
			Address:     res.Address,
			Action:      "publish",
			Target:      "live",
			Conditional: true,
		},
	}
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	result, err := destroy.Run(context.Background(), []resource.Resource{res}, nil, st, &out, nil, reg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Destroyed != 1 || result.Finalized != 1 {
		t.Fatalf("result = %+v, want destroy + finalize", result)
	}
	if p.destroyedAfterPlan {
		t.Fatal("finalization planned after mutation")
	}
	if !p.finalizeCalled {
		t.Fatal("Finalize was not called")
	}
	if !strings.Contains(out.String(), "> fake.widget.homepage: publish -> live [conditional]") {
		t.Fatalf("plan missing finalization:\n%s", out.String())
	}
}

func TestRunFailedMutationSkipsFinalization(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, inner, res)
	st := mustStore(t)
	bind(t, st, res, "id-homepage")

	p := &finalizingDestroyer{
		Provider: &scriptedDestroyer{Provider: inner, failAddr: res.Address.String(), err: errors.New("boom")},
		plan: &provider.FinalizationPlan{
			Address: res.Address,
			Action:  "publish",
			Target:  "live",
		},
	}
	_, err := destroy.Run(context.Background(), []resource.Resource{res}, lookupProvider(p), st, nil, nil)
	if err == nil {
		t.Fatal("Run succeeded, want mutation failure")
	}
	if p.finalizeCalled {
		t.Fatal("Finalize ran after failed mutation")
	}
}

func TestFormatResult(t *testing.T) {
	t.Parallel()

	got := destroy.FormatResult(destroy.Result{Destroyed: 1, AlreadyAbsent: 1, Remaining: 1})
	want := "Destroy complete! 1 destroyed, 1 already absent, 1 unsupported remaining.\n"
	if got != want {
		t.Fatalf("FormatResult = %q, want %q", got, want)
	}
}

var errCancelled = errors.New("destroy cancelled")

type recordingDestroyer struct {
	provider.Provider
	destroyed []string
}

func (p *recordingDestroyer) DestroyCapability(res resource.Resource) (provider.DestroyCapability, error) {
	return provider.DestroySupported, nil
}

func (p *recordingDestroyer) Destroy(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	p.destroyed = append(p.destroyed, res.Address.String())
	return p.Provider.(provider.Destroyer).Destroy(ctx, res)
}

type capabilityProvider struct {
	provider.Provider
	unsupported map[string]provider.DestroyCapability
}

func (p *capabilityProvider) DestroyCapability(res resource.Resource) (provider.DestroyCapability, error) {
	if cap, ok := p.unsupported[res.Address.String()]; ok {
		return cap, nil
	}
	return provider.DestroySupported, nil
}

func (p *capabilityProvider) Destroy(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	return p.Provider.(provider.Destroyer).Destroy(ctx, res)
}

type scriptedDestroyer struct {
	provider.Provider
	failAddr string
	err      error
}

func (p *scriptedDestroyer) DestroyCapability(resource.Resource) (provider.DestroyCapability, error) {
	return provider.DestroySupported, nil
}

func (p *scriptedDestroyer) Destroy(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	if p.failAddr != "" && res.Address.String() == p.failAddr {
		return provider.DestroyResult{}, p.err
	}
	return p.Provider.(provider.Destroyer).Destroy(ctx, res)
}

type finalizingDestroyer struct {
	provider.Provider
	plan               *provider.FinalizationPlan
	destroyedAfterPlan bool
	finalizeCalled     bool
	sawPending         bool
}

func (p *finalizingDestroyer) Name() string {
	if named, ok := p.Provider.(interface{ Name() string }); ok {
		return named.Name()
	}
	return fake.Name
}

func (p *finalizingDestroyer) DestroyCapability(resource.Resource) (provider.DestroyCapability, error) {
	return provider.DestroySupported, nil
}

func (p *finalizingDestroyer) Destroy(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	if !p.sawPending {
		p.destroyedAfterPlan = true
	}
	return p.Provider.(provider.Destroyer).Destroy(ctx, res)
}

func (p *finalizingDestroyer) PlanFinalization(_ context.Context, pending []provider.PendingChange) (*provider.FinalizationPlan, error) {
	p.sawPending = true
	if len(pending) == 0 {
		return nil, nil
	}
	return p.plan, nil
}

func (p *finalizingDestroyer) Finalize(context.Context, provider.FinalizationPlan) (provider.FinalizationResult, error) {
	p.finalizeCalled = true
	return provider.FinalizationResult{
		Address: p.plan.Address,
		Details: []string{"published to live"},
		Changed: true,
	}, nil
}

type failingRemoveStore struct {
	*state.Store
	err error
}

func (s *failingRemoveStore) Remove(resource.Address) error {
	return s.err
}

func widget(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{
		Address:    mustAddress(t, "fake.widget."+name),
		Attributes: attrs,
	}
}

func seed(t *testing.T, p *fake.Provider, res resource.Resource) {
	t.Helper()
	id := "id-" + res.Address.Name
	if err := p.Seed(resource.RemoteResource{
		Address:    res.Address,
		Identity:   resource.Identity{ID: id},
		Attributes: res.Attributes.Clone(),
		Computed:   resource.Attributes{fake.AttrSerial: 1},
	}); err != nil {
		t.Fatalf("Seed: %v", err)
	}
}

func bind(t *testing.T, st *state.Store, res resource.Resource, id string) {
	t.Helper()
	if err := st.Bind(res.Address, resource.Identity{ID: id}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
}

func lookupProvider(p provider.Provider) destroy.Lookup {
	return func(resource.Address) (provider.Provider, error) {
		return p, nil
	}
}

func mustStore(t *testing.T) *state.Store {
	t.Helper()
	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func mustAddress(t *testing.T, s string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress(s)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func assertMissing(t *testing.T, st *state.Store, addrs ...resource.Address) {
	t.Helper()
	for _, addr := range addrs {
		if _, ok, err := st.Identity(addr); err != nil || ok {
			t.Fatalf("Identity(%s) still present: ok=%v err=%v", addr, ok, err)
		}
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestNoSecretInDestroyErrors(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: plantedSecret})
	seed(t, inner, res)
	st := mustStore(t)
	bind(t, st, res, "id-homepage")

	p := &scriptedDestroyer{Provider: inner, failAddr: res.Address.String(), err: fmt.Errorf("provider rejected destroy")}
	_, err := destroy.Run(context.Background(), []resource.Resource{res}, lookupProvider(p), st, nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), plantedSecret) {
		t.Fatalf("secret leaked in error: %v", err)
	}
}
