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
		meta.TypeImage:            provider.DestroyProviderOwned,
		meta.TypePixel:            provider.DestroyProviderOwned,
		meta.TypeCustomConversion: provider.DestroyRemove,
		meta.TypeCampaign:         provider.DestroyRemove,
		meta.TypeAdSet:            provider.DestroyRemove,
		meta.TypeAdCreative:       provider.DestroyDelete,
		meta.TypeAd:               provider.DestroyRemove,
	}
	for _, typ := range p.ResourceTypes() {
		cap, ok := want[typ]
		if !ok {
			t.Fatalf("registered type %s has no expected lifecycle", typ)
		}
		spec, declared := meta.Lifecycle(typ)
		if !declared {
			t.Fatalf("registered type %s has no explicit lifecycle declaration", typ)
		}
		// ImportSupported is intentionally false for meta.image (create-managed only).
		importRequired := spec.Create != meta.CreateExternalImportOnly && typ != meta.TypeImage
		if spec.RemoteIdentity == "" || (importRequired && !spec.ImportSupported) || spec.Create == "" || spec.Update == "" || spec.Destroy == "" || spec.AlreadyTerminal == "" || spec.ServingSafety == "" {
			t.Fatalf("registered type %s has incomplete lifecycle declaration: %#v", typ, spec)
		}
		if spec.Destroy != provider.DestroyProviderOwned && spec.TerminalState == "" {
			t.Fatalf("registered type %s has no terminal state", typ)
		}
		if spec.Update == meta.UpdateSupported && len(spec.MutableFields) == 0 {
			t.Fatalf("registered type %s supports update but declares no mutable fields", typ)
		}
		if spec.Update == meta.UpdateReadOnly && len(spec.MutableFields) != 0 {
			t.Fatalf("registered type %s is read-only but declares mutable fields %v", typ, spec.MutableFields)
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
		if got != spec.Destroy {
			t.Fatalf("%s handler capability = %q, lifecycle declares %q", typ, got, spec.Destroy)
		}
		delete(want, typ)
	}
	if len(want) != 0 {
		t.Fatalf("documented types missing from ResourceTypes: %v", want)
	}
}
