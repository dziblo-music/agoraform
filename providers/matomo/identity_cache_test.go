package matomo

import (
	"testing"

	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestBindIdentityKeepsKnownNameForSameIdentity(t *testing.T) {
	t.Parallel()

	addr, err := resource.ParseAddress("matomo.tag.pageview")
	if err != nil {
		t.Fatal(err)
	}
	p := &Provider{}
	p.rememberBinding(addr, "7", "Pageview")

	p.BindIdentity(addr, resource.Identity{ID: "7"})
	if got := p.lookupName(addr); got != "Pageview" {
		t.Fatalf("name = %q, want the name from the earlier read", got)
	}

	p.BindIdentity(addr, resource.Identity{ID: "8"})
	if got, name := p.lookupID(addr), p.lookupName(addr); got != "8" || name != "" {
		t.Fatalf("binding = %q/%q, want the new identity without the stale name", got, name)
	}
}
