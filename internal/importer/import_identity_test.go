package importer_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/importer"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestRunRejectsProviderIdentityMismatch(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	addr := mustAddress(t, "fake.widget.homepage")
	seedImported(t, inner, addr, resource.Attributes{fake.AttrTitle: "Imported"}, nil)
	p := &mismatchedImportProvider{
		Provider:   inner,
		identityID: "other-id",
	}
	st := mustStore(t)

	_, err := importer.Run(context.Background(), addr, "widget-imported", lookupProvider(p), st)
	if err == nil {
		t.Fatal("Run succeeded, want provider identity mismatch")
	}
	if !strings.Contains(err.Error(), "other-id") || !strings.Contains(err.Error(), "widget-imported") {
		t.Fatalf("error = %q, want returned and requested identities", err)
	}
	if !strings.Contains(err.Error(), "refusing to bind") {
		t.Fatalf("error = %q, want bind refusal", err)
	}
	if _, ok, stateErr := st.Identity(addr); stateErr != nil || ok {
		t.Fatalf("state binding = (%v,%v), want none", ok, stateErr)
	}

	_, creates, updates, imports := inner.Calls()
	if creates != 0 || updates != 0 || imports != 1 {
		t.Fatalf("provider calls creates=%d updates=%d imports=%d, want 0 0 1", creates, updates, imports)
	}
}

func TestRunRejectsProviderAddressMismatch(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	addr := mustAddress(t, "fake.widget.homepage")
	seedImported(t, inner, addr, resource.Attributes{fake.AttrTitle: "Imported"}, nil)
	p := &mismatchedImportProvider{
		Provider: inner,
		address:  mustAddress(t, "fake.widget.other"),
	}
	st := mustStore(t)

	_, err := importer.Run(context.Background(), addr, "widget-imported", lookupProvider(p), st)
	if err == nil {
		t.Fatal("Run succeeded, want provider address mismatch")
	}
	if !strings.Contains(err.Error(), "fake.widget.other") || !strings.Contains(err.Error(), addr.String()) {
		t.Fatalf("error = %q, want returned and requested addresses", err)
	}
	if !strings.Contains(err.Error(), "refusing to bind") {
		t.Fatalf("error = %q, want bind refusal", err)
	}
	if _, ok, stateErr := st.Identity(addr); stateErr != nil || ok {
		t.Fatalf("state binding = (%v,%v), want none", ok, stateErr)
	}
}

type mismatchedImportProvider struct {
	provider.Provider
	identityID string
	address    resource.Address
}

func (p *mismatchedImportProvider) Import(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	live, err := p.Provider.Import(ctx, addr, id)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if p.identityID != "" {
		live.Identity = resource.Identity{ID: p.identityID}
	}
	if p.address.String() != "" {
		live.Address = p.address
	}
	return live, nil
}
