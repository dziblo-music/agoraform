package destroy_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/destroy"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestDestroyExternalSkipsProviderAndDropsLocalBinding(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := externalWidget(t, "homepage", "widget-1")
	seedExternal(t, p, res, "widget-1")
	st := mustStore(t)
	if err := st.RecordExternal(res.Address, resource.Identity{ID: "widget-1"}); err != nil {
		t.Fatal(err)
	}

	result, err := destroy.Run(context.Background(), []resource.Resource{res}, lookupProvider(p), st, nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Destroyed != 0 || result.ExternalReleased != 1 {
		t.Fatalf("result = %+v", result)
	}
	if p.Destroys() != 0 {
		t.Fatalf("destroys = %d, want 0", p.Destroys())
	}
	if _, err := p.Read(context.Background(), resource.Resource{Address: res.Address, Identity: resource.Identity{ID: "widget-1"}}); err != nil {
		t.Fatalf("remote external resource missing after destroy: %v", err)
	}
	assertMissing(t, st, res.Address)
}

func TestDestroyRemovalFromManifestDoesNotDeleteExternal(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := externalWidget(t, "homepage", "widget-1")
	seedExternal(t, p, res, "widget-1")
	st := mustStore(t)
	if err := st.RecordExternal(res.Address, resource.Identity{ID: "widget-1"}); err != nil {
		t.Fatal(err)
	}

	result, err := destroy.Run(context.Background(), nil, lookupProvider(p), st, nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Destroyed != 0 || result.ExternalReleased != 0 {
		t.Fatalf("result = %+v", result)
	}
	if p.Destroys() != 0 {
		t.Fatalf("destroys = %d, want 0", p.Destroys())
	}
	id, ok, err := st.Identity(res.Address)
	if err != nil || !ok || id.ID != "widget-1" {
		t.Fatalf("preserved binding = (%v, %v, %v)", id, ok, err)
	}
	ownership, ok, err := st.Ownership(res.Address)
	if err != nil || !ok || ownership != resource.OwnershipExternal {
		t.Fatalf("preserved ownership = (%s, %v, %v)", ownership, ok, err)
	}
}

func TestDestroyExternalPartialFailureKeepsProtection(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	parent := externalWidget(t, "homepage", "widget-1")
	child := widget(t, "banner", resource.Attributes{
		fake.AttrTitle:  "Banner",
		fake.AttrParent: resource.Ref{Address: parent.Address},
	})
	seedExternal(t, inner, parent, "widget-1")
	seedExternal(t, inner, child, "widget-2")

	st := mustStore(t)
	if err := st.RecordExternal(parent.Address, resource.Identity{ID: "widget-1"}); err != nil {
		t.Fatal(err)
	}
	bind(t, st, child, "widget-2")

	p := &failingOnceDestroyer{Provider: inner}
	_, err := destroy.Run(context.Background(), []resource.Resource{parent, child}, lookupProvider(p), st, nil, nil)
	if err == nil {
		t.Fatal("Run succeeded, want child destroy failure")
	}
	if inner.Destroys() != 0 {
		t.Fatalf("failed attempt destroyed %d resources", inner.Destroys())
	}
	ownership, ok, ownErr := st.Ownership(parent.Address)
	if ownErr != nil || !ok || ownership != resource.OwnershipExternal {
		t.Fatalf("external ownership after partial failure = (%s, %v, %v)", ownership, ok, ownErr)
	}

	result, err := destroy.Run(context.Background(), []resource.Resource{parent, child}, lookupProvider(p), st, nil, nil)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if result.Destroyed != 1 || result.ExternalReleased != 1 {
		t.Fatalf("retry result = %+v", result)
	}
	if inner.Destroys() != 1 {
		t.Fatalf("destroys = %d, want only the managed child", inner.Destroys())
	}
	if _, err := inner.Read(context.Background(), resource.Resource{Address: parent.Address, Identity: resource.Identity{ID: "widget-1"}}); err != nil {
		t.Fatalf("external remote object was deleted: %v", err)
	}
	assertMissing(t, st, parent.Address, child.Address)
}

func TestDestroyRefusesManifestEditThatDropsExternalOwnership(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustDestroyAddress(t, "fake.widget.homepage")
	seedExternal(t, p, resource.Resource{Address: addr, Attributes: resource.Attributes{fake.AttrTitle: "Homepage"}}, "widget-1")
	st := mustStore(t)
	if err := st.RecordExternal(addr, resource.Identity{ID: "widget-1"}); err != nil {
		t.Fatal(err)
	}
	managed := resource.Resource{Address: addr, Attributes: resource.Attributes{fake.AttrTitle: "Homepage"}}
	_, err := destroy.Run(context.Background(), []resource.Resource{managed}, lookupProvider(p), st, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "import --adopt") {
		t.Fatalf("error = %v, want adoption safeguard", err)
	}
	if p.Destroys() != 0 {
		t.Fatalf("destroys = %d, want 0", p.Destroys())
	}
	if _, ok, _ := st.Ownership(addr); !ok {
		t.Fatal("external binding was dropped")
	}
}

func seedExternal(t *testing.T, p *fake.Provider, res resource.Resource, id string) {
	t.Helper()
	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{fake.AttrTitle: res.Address.Name}
	}
	if err := p.Seed(resource.RemoteResource{
		Address:    res.Address,
		Identity:   resource.Identity{ID: id},
		Attributes: attrs,
		Computed:   resource.Attributes{fake.AttrSerial: 1},
	}); err != nil {
		t.Fatal(err)
	}
}

func externalWidget(t *testing.T, name, id string) resource.Resource {
	t.Helper()
	res := widget(t, name, nil)
	res.Ownership = resource.OwnershipExternal
	res.ExternalID = id
	res.Attributes = nil
	return res
}

func mustDestroyAddress(t *testing.T, raw string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress(raw)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

type failingOnceDestroyer struct {
	provider.Provider
	failed bool
}

func (p *failingOnceDestroyer) Destroy(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	if !p.failed {
		p.failed = true
		return provider.DestroyResult{}, errors.New("destroy failed")
	}
	return p.Provider.(provider.Destroyer).Destroy(ctx, res)
}

func (p *failingOnceDestroyer) DestroyCapability(res resource.Resource) (provider.DestroyCapability, error) {
	return p.Provider.(provider.Destroyer).DestroyCapability(res)
}
