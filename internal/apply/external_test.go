package apply_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
)

func TestApplyExternalReferenceDoesNotMutateRemote(t *testing.T) {
	t.Parallel()

	widgets := fake.New()
	notes := fake.NewAlt()
	parent := resource.Resource{
		Address:    mustAddress(t, "fake.widget.homepage"),
		Ownership:  resource.OwnershipExternal,
		ExternalID: "widget-1",
	}
	if err := widgets.Seed(resource.RemoteResource{
		Address:    parent.Address,
		Identity:   resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage"},
		Computed:   resource.Attributes{fake.OutputToken: "tok-homepage", fake.AttrSerial: 3},
	}); err != nil {
		t.Fatal(err)
	}
	child := resource.Resource{
		Address: mustAddress(t, "alt.note.banner"),
		Attributes: resource.Attributes{
			fake.AttrText: resource.Ref{Address: parent.Address, Output: fake.OutputToken},
		},
	}
	st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}

	result, err := apply.Run(context.Background(), []resource.Resource{child, parent}, func(addr resource.Address) (provider.Provider, error) {
		switch addr.Provider {
		case fake.Name:
			return widgets, nil
		case fake.AltName:
			return notes, nil
		default:
			return nil, errUnknown(addr)
		}
	}, st, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Created != 1 || result.Updated != 0 || result.External != 1 {
		t.Fatalf("result = %+v", result)
	}
	_, creates, updates, _ := widgets.Calls()
	if creates != 0 || updates != 0 || widgets.Destroys() != 0 {
		t.Fatalf("external widget mutated: creates=%d updates=%d destroys=%d", creates, updates, widgets.Destroys())
	}
	ownership, ok, err := st.Ownership(parent.Address)
	if err != nil || !ok || ownership != resource.OwnershipExternal {
		t.Fatalf("ownership = (%s, %v, %v)", ownership, ok, err)
	}
	note, err := notes.Read(context.Background(), child)
	if err != nil {
		t.Fatal(err)
	}
	if note.Attributes[fake.AttrText] != "tok-homepage" {
		t.Fatalf("note text = %v, want resolved external output", note.Attributes[fake.AttrText])
	}
}

func errUnknown(addr resource.Address) error {
	return &unknownProviderError{addr: addr}
}

type unknownProviderError struct{ addr resource.Address }

func (e *unknownProviderError) Error() string {
	return "unknown provider for " + e.addr.String()
}
