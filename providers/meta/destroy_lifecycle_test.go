package meta_test

import (
	"testing"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/meta"
)

func TestDestroyLifecycleCoversRegisteredTypes(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	want := map[string]provider.DestroyCapability{
		meta.TypePixel:            provider.DestroyProviderOwned,
		meta.TypeCustomConversion: provider.DestroyRemove,
		meta.TypeCampaign:         provider.DestroyRemove,
	}
	for _, typ := range p.ResourceTypes() {
		cap, ok := want[typ]
		if !ok {
			t.Fatalf("registered type %s has no documented destroy lifecycle", typ)
		}
		addr, err := resource.ParseAddress("meta." + typ + ".example")
		if err != nil {
			t.Fatal(err)
		}
		got, err := p.DestroyCapability(resource.Resource{Address: addr})
		if err != nil {
			t.Fatal(err)
		}
		if got != cap {
			t.Fatalf("%s capability = %q, want %q", typ, got, cap)
		}
		delete(want, typ)
	}
	if len(want) != 0 {
		t.Fatalf("documented types missing from ResourceTypes: %v", want)
	}
}
