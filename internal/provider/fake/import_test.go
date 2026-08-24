package fake_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestImportIsReadOnlyAndReturnsRequestedLogicalAddress(t *testing.T) {
	t.Parallel()

	p := fake.New()
	source := mustAddress(t, "fake.widget.source")
	target := mustAddress(t, "fake.widget.managed")
	if err := p.Seed(resource.RemoteResource{
		Address:    source,
		Identity:   resource.Identity{ID: "widget-remote"},
		Attributes: resource.Attributes{fake.AttrTitle: "Existing"},
		Computed:   resource.Attributes{fake.AttrSerial: 7},
	}); err != nil {
		t.Fatal(err)
	}

	live, err := p.Import(context.Background(), target, "widget-remote")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if live.Address != target {
		t.Fatalf("import address = %s, want %s", live.Address, target)
	}
	if live.Identity.ID != "widget-remote" {
		t.Fatalf("import identity = %q, want widget-remote", live.Identity.ID)
	}

	original, err := p.Read(context.Background(), resource.Resource{Address: source})
	if err != nil {
		t.Fatalf("read original after import: %v", err)
	}
	if original.Address != source || original.Identity.ID != "widget-remote" {
		t.Fatalf("original after import = address %s identity %q, want %s/widget-remote", original.Address, original.Identity.ID, source)
	}
	if original.Attributes[fake.AttrTitle] != "Existing" {
		t.Fatalf("original title = %v, want Existing", original.Attributes[fake.AttrTitle])
	}

	if _, err := p.Read(context.Background(), resource.Resource{Address: target}); !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("unbound target read error = %v, want ErrNotFound", err)
	}

	bound, err := p.Read(context.Background(), resource.Resource{
		Address:  target,
		Identity: resource.Identity{ID: "widget-remote"},
	})
	if err != nil {
		t.Fatalf("identity-bound target read: %v", err)
	}
	if bound.Address != target {
		t.Fatalf("identity-bound read address = %s, want %s", bound.Address, target)
	}

	_, creates, updates, imports := p.Calls()
	if creates != 0 || updates != 0 || imports != 1 {
		t.Fatalf("calls creates=%d updates=%d imports=%d, want 0 0 1", creates, updates, imports)
	}
}

func mustAddress(t *testing.T, s string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress(s)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}
