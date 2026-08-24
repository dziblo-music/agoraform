package apply_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestRunRejectsCycleWithoutMutation(t *testing.T) {
	t.Parallel()

	p := fake.New()
	a := widget(t, "alpha", resource.Attributes{
		fake.AttrTitle:  "Alpha",
		fake.AttrParent: resource.Ref{Address: mustAddress(t, "fake.widget.beta")},
	})
	b := widget(t, "beta", resource.Attributes{
		fake.AttrTitle:  "Beta",
		fake.AttrParent: resource.Ref{Address: mustAddress(t, "fake.widget.alpha")},
	})
	st := mustStore(t)

	_, err := apply.Run(context.Background(), []resource.Resource{a, b}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Run succeeded, want cycle error")
	}
	if !strings.Contains(err.Error(), "cyclic dependency") {
		t.Fatalf("error = %q, want cyclic dependency", err)
	}

	_, creates, updates, imports := p.Calls()
	if creates != 0 || updates != 0 || imports != 0 {
		t.Fatalf("cycle mutated provider: creates=%d updates=%d imports=%d", creates, updates, imports)
	}
}

func TestRunReferencedResourcesUseLogicalAddresses(t *testing.T) {
	t.Parallel()

	p := fake.New()
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	child := widget(t, "banner", resource.Attributes{
		fake.AttrTitle:  "Banner",
		fake.AttrParent: resource.Ref{Address: parent.Address},
	})
	st := mustStore(t)
	var out bytes.Buffer

	result, err := apply.Run(context.Background(), []resource.Resource{child, parent}, lookupProvider(p), st, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Created != 2 {
		t.Fatalf("result = %+v, want 2 created", result)
	}

	got := out.String()
	if !strings.Contains(got, "fake.widget.banner") || !strings.Contains(got, "fake.widget.homepage") {
		t.Fatalf("apply output missing logical addresses:\n%s", got)
	}
	assertNoSecret(t, got)
}
