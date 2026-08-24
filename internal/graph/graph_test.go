package graph_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/graph"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestBuildIndependentResourcesAddressOrder(t *testing.T) {
	t.Parallel()

	// v0.1-style manifests with no references stay in address order.
	resources := []resource.Resource{
		widget(t, "zeta", nil),
		widget(t, "alpha", nil),
		widget(t, "mu", nil),
	}

	first := mustBuild(t, resources)
	second := mustBuild(t, []resource.Resource{resources[2], resources[0], resources[1]})

	want := []string{"fake.widget.alpha", "fake.widget.mu", "fake.widget.zeta"}
	assertOrder(t, first, want)
	assertOrder(t, second, want)
}

func TestBuildSingleDependency(t *testing.T) {
	t.Parallel()

	parent := widget(t, "homepage", nil)
	child := widget(t, "banner", resource.Attributes{
		"parent": resource.Ref{Address: parent.Address},
	})

	g := mustBuild(t, []resource.Resource{child, parent})
	assertOrder(t, g, []string{"fake.widget.homepage", "fake.widget.banner"})

	deps := g.Dependencies(child.Address)
	if len(deps) != 1 || deps[0] != parent.Address {
		t.Fatalf("Dependencies(banner) = %v, want [%s]", deps, parent.Address)
	}
	if deps := g.Dependencies(parent.Address); deps != nil {
		t.Fatalf("Dependencies(homepage) = %v, want nil", deps)
	}
}

func TestBuildMultipleDependencies(t *testing.T) {
	t.Parallel()

	left := widget(t, "left", nil)
	right := widget(t, "right", nil)
	child := widget(t, "child", resource.Attributes{
		"left":  resource.Ref{Address: left.Address},
		"right": "fake.widget.right",
	})

	g := mustBuild(t, []resource.Resource{child, right, left})
	assertOrder(t, g, []string{"fake.widget.left", "fake.widget.right", "fake.widget.child"})

	got := addresses(g.Dependencies(child.Address))
	want := []string{"fake.widget.left", "fake.widget.right"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Dependencies(child) = %v, want %v", got, want)
	}
}

func TestBuildTransitiveDependencies(t *testing.T) {
	t.Parallel()

	root := widget(t, "root", nil)
	mid := widget(t, "mid", resource.Attributes{
		"parent": resource.Ref{Address: root.Address},
	})
	leaf := widget(t, "leaf", resource.Attributes{
		"parent": resource.Ref{Address: mid.Address},
	})

	g := mustBuild(t, []resource.Resource{leaf, root, mid})
	assertOrder(t, g, []string{"fake.widget.root", "fake.widget.mid", "fake.widget.leaf"})
	if got := addresses(g.Dependencies(leaf.Address)); !reflect.DeepEqual(got, []string{"fake.widget.mid"}) {
		t.Fatalf("leaf dependencies = %v, want direct mid only", got)
	}
}

func TestBuildMissingReference(t *testing.T) {
	t.Parallel()

	child := widget(t, "banner", resource.Attributes{
		"parent": "fake.widget.missing",
	})
	_, err := graph.Build([]resource.Resource{child})
	if err == nil {
		t.Fatal("Build succeeded, want missing reference error")
	}
	msg := err.Error()
	for _, want := range []string{"fake.widget.banner", "parent", "unknown resource", "fake.widget.missing"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q, want substring %q", msg, want)
		}
	}
}

func TestBuildSelfReference(t *testing.T) {
	t.Parallel()

	res := widget(t, "loop", resource.Attributes{
		"parent": "fake.widget.loop",
	})
	_, err := graph.Build([]resource.Resource{res})
	if err == nil {
		t.Fatal("Build succeeded, want self-reference error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "fake.widget.loop") || !strings.Contains(msg, "itself") {
		t.Fatalf("error %q, want self-reference", msg)
	}
}

func TestBuildDependencyCycle(t *testing.T) {
	t.Parallel()

	a := widget(t, "alpha", resource.Attributes{"parent": "fake.widget.beta"})
	b := widget(t, "beta", resource.Attributes{"parent": "fake.widget.alpha"})

	_, err := graph.Build([]resource.Resource{a, b})
	if err == nil {
		t.Fatal("Build succeeded, want cycle error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "cyclic dependency") {
		t.Fatalf("error %q, want cyclic dependency", msg)
	}
	want := "fake.widget.alpha -> fake.widget.beta -> fake.widget.alpha"
	if !strings.Contains(msg, want) {
		t.Fatalf("error %q, want canonical cycle %q", msg, want)
	}
}

func TestBuildDeterministicOrdering(t *testing.T) {
	t.Parallel()

	// Ready-set tie-break is address order: mu and zeta start ready,
	// mu is smaller, then alpha becomes ready and precedes zeta.
	zeta := widget(t, "zeta", nil)
	mu := widget(t, "mu", nil)
	alpha := widget(t, "alpha", resource.Attributes{
		"parent": resource.Ref{Address: mu.Address},
	})

	resources := []resource.Resource{zeta, alpha, mu}
	first := mustBuild(t, resources)
	second := mustBuild(t, []resource.Resource{alpha, zeta, mu})
	want := []string{"fake.widget.mu", "fake.widget.alpha", "fake.widget.zeta"}
	assertOrder(t, first, want)
	assertOrder(t, second, want)
}

func TestBuildDuplicateReferenceCollapsed(t *testing.T) {
	t.Parallel()

	parent := widget(t, "homepage", nil)
	child := widget(t, "banner", resource.Attributes{
		"parent": "fake.widget.homepage",
		"also":   resource.Ref{Address: parent.Address},
	})

	g := mustBuild(t, []resource.Resource{parent, child})
	if got := g.Dependencies(child.Address); len(got) != 1 || got[0] != parent.Address {
		t.Fatalf("Dependencies = %v, want a single homepage edge", got)
	}
}

func TestBuildNestedReferences(t *testing.T) {
	t.Parallel()

	a := widget(t, "a", nil)
	b := widget(t, "b", nil)
	child := widget(t, "child", resource.Attributes{
		"meta": map[string]any{
			"primary": "fake.widget.a",
			"list": []any{
				"plain",
				"fake.widget.b",
			},
		},
	})

	g := mustBuild(t, []resource.Resource{child, a, b})
	got := addresses(g.Dependencies(child.Address))
	want := []string{"fake.widget.a", "fake.widget.b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Dependencies = %v, want %v", got, want)
	}
}

func TestBuildDuplicateAddress(t *testing.T) {
	t.Parallel()

	res := widget(t, "homepage", nil)
	_, err := graph.Build([]resource.Resource{res, res})
	if err == nil {
		t.Fatal("Build succeeded, want duplicate address error")
	}
	if !strings.Contains(err.Error(), "duplicate resource address") {
		t.Fatalf("error %q", err.Error())
	}
}

func TestBuildEmpty(t *testing.T) {
	t.Parallel()

	g, err := graph.Build(nil)
	if err != nil {
		t.Fatalf("Build(nil): %v", err)
	}
	if g.Order() != nil {
		t.Fatalf("Order() = %v, want nil", g.Order())
	}
}

func TestNilGraphAccessors(t *testing.T) {
	t.Parallel()

	var g *graph.Graph
	if g.Order() != nil {
		t.Fatalf("nil Order = %v", g.Order())
	}
	if g.Dependencies(mustAddress(t, "fake.widget.homepage")) != nil {
		t.Fatal("nil Dependencies should be nil")
	}
}

func widget(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	if attrs == nil {
		attrs = resource.Attributes{"title": name}
	} else if _, ok := attrs["title"]; !ok {
		attrs["title"] = name
	}
	return resource.Resource{
		Address:    mustAddress(t, "fake.widget."+name),
		Attributes: attrs,
	}
}

func mustBuild(t *testing.T, resources []resource.Resource) *graph.Graph {
	t.Helper()
	g, err := graph.Build(resources)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return g
}

func assertOrder(t *testing.T, g *graph.Graph, want []string) {
	t.Helper()
	got := addresses(g.Order())
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Order() = %v, want %v", got, want)
	}
}

func addresses(addrs []resource.Address) []string {
	out := make([]string, len(addrs))
	for i, addr := range addrs {
		out[i] = addr.String()
	}
	return out
}

func mustAddress(t *testing.T, s string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress(s)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}
