package graph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dziblo-music/agoraform/internal/resource"
)

// Graph is a directed dependency graph of logical resource addresses.
//
// An edge from A to B means A references B, so B is a prerequisite of A.
// Order is a stable topological sort: prerequisites first, with address
// order as the tie-breaker among ready resources.
type Graph struct {
	order []resource.Address
	deps  map[string][]resource.Address
}

// Build constructs a dependency graph from desired resources.
//
// It rejects duplicate addresses, references to missing resources,
// self-references, and cycles. Equivalent resource sets produce the same
// Order regardless of input slice order.
func Build(resources []resource.Resource) (*Graph, error) {
	nodes := make([]resource.Address, 0, len(resources))
	byKey := make(map[string]resource.Address, len(resources))
	for _, res := range resources {
		if err := res.Address.Validate(); err != nil {
			return nil, err
		}
		key := res.Address.String()
		if _, dup := byKey[key]; dup {
			return nil, fmt.Errorf("duplicate resource address %q", key)
		}
		byKey[key] = res.Address
		nodes = append(nodes, res.Address)
	}

	deps := make(map[string][]resource.Address, len(resources))
	for _, res := range resources {
		direct, err := directDependencies(res, byKey)
		if err != nil {
			return nil, err
		}
		deps[res.Address.String()] = direct
	}

	order, err := topoSort(nodes, deps)
	if err != nil {
		return nil, err
	}

	return &Graph{order: order, deps: deps}, nil
}

// Order returns resources in deterministic prerequisite-first order.
func (g *Graph) Order() []resource.Address {
	if g == nil || len(g.order) == 0 {
		return nil
	}
	out := make([]resource.Address, len(g.order))
	copy(out, g.order)
	return out
}

// ReverseOrder returns resources in deterministic dependent-first order,
// the reverse of apply execution. Destroy uses this so dependents are
// removed before prerequisites.
func (g *Graph) ReverseOrder() []resource.Address {
	order := g.Order()
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	return order
}

// Dependencies returns the direct prerequisites of addr in address order.
func (g *Graph) Dependencies(addr resource.Address) []resource.Address {
	if g == nil {
		return nil
	}
	deps := g.deps[addr.String()]
	if len(deps) == 0 {
		return nil
	}
	out := make([]resource.Address, len(deps))
	copy(out, deps)
	return out
}

func directDependencies(res resource.Resource, known map[string]resource.Address) ([]resource.Address, error) {
	seen := make(map[string]struct{})
	var deps []resource.Address
	var first error
	resource.WalkRefs(res.Attributes, func(path string, target resource.Address) {
		if first != nil {
			return
		}
		if err := target.Validate(); err != nil {
			first = fmt.Errorf("resource %q attribute %q: %w", res.Address, displayPath(path), err)
			return
		}
		tkey := target.String()
		attr := displayPath(path)
		if tkey == res.Address.String() {
			first = fmt.Errorf("resource %q attribute %q references itself", res.Address, attr)
			return
		}
		if _, ok := known[tkey]; !ok {
			first = fmt.Errorf("resource %q attribute %q references unknown resource %q", res.Address, attr, target)
			return
		}
		if _, dup := seen[tkey]; dup {
			return
		}
		seen[tkey] = struct{}{}
		deps = append(deps, target)
	})
	if first != nil {
		return nil, first
	}
	sort.Slice(deps, func(i, j int) bool {
		return deps[i].String() < deps[j].String()
	})
	return deps, nil
}

func displayPath(path string) string {
	if path == "" {
		return "(attributes)"
	}
	return path
}

func topoSort(nodes []resource.Address, deps map[string][]resource.Address) ([]resource.Address, error) {
	incoming := make(map[string]int, len(nodes))
	dependents := make(map[string][]string, len(nodes))
	byKey := make(map[string]resource.Address, len(nodes))
	for _, n := range nodes {
		key := n.String()
		incoming[key] = 0
		byKey[key] = n
	}
	for _, n := range nodes {
		key := n.String()
		for _, dep := range deps[key] {
			incoming[key]++
			dkey := dep.String()
			dependents[dkey] = append(dependents[dkey], key)
		}
	}

	ready := make([]string, 0, len(nodes))
	for _, n := range nodes {
		key := n.String()
		if incoming[key] == 0 {
			ready = append(ready, key)
		}
	}
	sort.Strings(ready)

	order := make([]resource.Address, 0, len(nodes))
	for len(ready) > 0 {
		key := ready[0]
		ready = ready[1:]
		order = append(order, byKey[key])
		next := dependents[key]
		sort.Strings(next)
		for _, dep := range next {
			incoming[dep]--
			if incoming[dep] == 0 {
				ready = append(ready, dep)
			}
		}
		sort.Strings(ready)
	}

	if len(order) != len(nodes) {
		remaining := make(map[string]struct{}, len(nodes)-len(order))
		for _, n := range nodes {
			if incoming[n.String()] > 0 {
				remaining[n.String()] = struct{}{}
			}
		}
		return nil, fmt.Errorf("cyclic dependency: %s", formatCycle(findCycle(remaining, deps)))
	}
	return order, nil
}

func findCycle(remaining map[string]struct{}, deps map[string][]resource.Address) []string {
	start := ""
	for key := range remaining {
		if start == "" || key < start {
			start = key
		}
	}
	if start == "" {
		return nil
	}

	var path []string
	index := make(map[string]int)
	var dfs func(string) []string
	dfs = func(node string) []string {
		if idx, ok := index[node]; ok {
			cycle := append(append([]string{}, path[idx:]...), node)
			return canonicalCycle(cycle)
		}
		if _, ok := remaining[node]; !ok {
			return nil
		}
		index[node] = len(path)
		path = append(path, node)
		for _, dep := range deps[node] {
			if cycle := dfs(dep.String()); cycle != nil {
				return cycle
			}
		}
		path = path[:len(path)-1]
		delete(index, node)
		return nil
	}

	if cycle := dfs(start); len(cycle) > 0 {
		return cycle
	}

	keys := make([]string, 0, len(remaining))
	for key := range remaining {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return append(keys, keys[0])
}

func canonicalCycle(cycle []string) []string {
	if len(cycle) < 2 {
		return cycle
	}
	body := cycle[:len(cycle)-1]
	min := 0
	for i := 1; i < len(body); i++ {
		if body[i] < body[min] {
			min = i
		}
	}
	rotated := append(append([]string{}, body[min:]...), body[:min]...)
	return append(rotated, rotated[0])
}

func formatCycle(cycle []string) string {
	return strings.Join(cycle, " -> ")
}
