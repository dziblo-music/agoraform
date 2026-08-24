package plan_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestBuildResourceReferenceUsesLogicalAddress(t *testing.T) {
	t.Parallel()

	p := fake.New()
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	child := widget(t, "banner", resource.Attributes{
		fake.AttrTitle:  "Banner",
		fake.AttrParent: resource.Ref{Address: parent.Address},
	})

	got := mustBuild(t, []resource.Resource{child, parent}, p)
	if len(got.Changes) != 2 {
		t.Fatalf("len(changes) = %d, want 2", len(got.Changes))
	}
	for _, change := range got.Changes {
		if change.Action != plan.ActionCreate {
			t.Fatalf("change %s = %+v, want create", change.Address, change)
		}
		if !change.Identity.IsZero() {
			t.Fatalf("create %s leaked identity %+v", change.Address, change.Identity)
		}
	}

	out := plan.Format(got)
	if !strings.Contains(out, "+ fake.widget.banner") || !strings.Contains(out, "+ fake.widget.homepage") {
		t.Fatalf("plan missing logical addresses:\n%s", out)
	}
	if !strings.Contains(out, `parent: "fake.widget.homepage"`) {
		t.Fatalf("plan missing logical parent reference:\n%s", out)
	}
	if strings.Contains(out, "widget-") || strings.Contains(out, "id-") {
		t.Fatalf("plan leaked provider-native identity:\n%s", out)
	}

	again := mustBuild(t, []resource.Resource{parent, child}, p)
	if plan.Format(got) != plan.Format(again) {
		t.Fatalf("referenced plan was not stable\nfirst:\n%s\nsecond:\n%s", plan.Format(got), plan.Format(again))
	}
	assertNoMutations(t, p, 4)
}

func TestBuildRejectsMissingReferenceBeforeRead(t *testing.T) {
	t.Parallel()

	reads := 0
	lookup := func(resource.Address) (provider.Reader, error) {
		reads++
		return failingReader{err: errors.New("should not read")}, nil
	}
	child := widget(t, "banner", resource.Attributes{
		fake.AttrTitle:  "Banner",
		fake.AttrParent: resource.Ref{Address: mustAddress(t, "fake.widget.missing")},
	})

	_, err := plan.Build(context.Background(), []resource.Resource{child}, lookup)
	if err == nil {
		t.Fatal("Build succeeded, want missing reference")
	}
	if !strings.Contains(err.Error(), "unknown resource") {
		t.Fatalf("error %q, want unknown resource", err)
	}
	if reads != 0 {
		t.Fatalf("reads = %d, want 0 before graph validation", reads)
	}
}

func TestBuildRejectsCycleBeforeRead(t *testing.T) {
	t.Parallel()

	reads := 0
	lookup := func(resource.Address) (provider.Reader, error) {
		reads++
		return failingReader{err: errors.New("should not read")}, nil
	}
	a := widget(t, "alpha", resource.Attributes{
		fake.AttrTitle:  "Alpha",
		fake.AttrParent: resource.Ref{Address: mustAddress(t, "fake.widget.beta")},
	})
	b := widget(t, "beta", resource.Attributes{
		fake.AttrTitle:  "Beta",
		fake.AttrParent: resource.Ref{Address: mustAddress(t, "fake.widget.alpha")},
	})

	_, err := plan.Build(context.Background(), []resource.Resource{a, b}, lookup)
	if err == nil {
		t.Fatal("Build succeeded, want cycle")
	}
	if !strings.Contains(err.Error(), "cyclic dependency") {
		t.Fatalf("error %q, want cyclic dependency", err)
	}
	if reads != 0 {
		t.Fatalf("reads = %d, want 0 before graph validation", reads)
	}
}

func TestBuildV01ManifestWithoutReferences(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage banner"})
	got := mustBuild(t, []resource.Resource{res}, p)
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionCreate {
		t.Fatalf("change = %+v, want create", got.Changes)
	}
	assertNoMutations(t, p, 1)
}
