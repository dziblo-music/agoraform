package apply_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/graph"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
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

func TestExecuteRejectsCycleWithoutMutation(t *testing.T) {
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
	planned := &plan.Plan{
		Changes: []plan.Change{
			{Address: a.Address, Action: plan.ActionCreate, After: a.Attributes},
			{Address: b.Address, Action: plan.ActionCreate, After: b.Attributes},
		},
	}

	_, err := apply.Execute(context.Background(), planned, []resource.Resource{a, b}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Execute succeeded, want cycle error")
	}
	if !strings.Contains(err.Error(), "cyclic dependency") {
		t.Fatalf("error = %q, want cyclic dependency", err)
	}
	_, creates, updates, imports := p.Calls()
	if creates != 0 || updates != 0 || imports != 0 {
		t.Fatalf("cycle mutated provider: creates=%d updates=%d imports=%d", creates, updates, imports)
	}
}

func TestExecuteRejectsMissingReferenceWithoutMutation(t *testing.T) {
	t.Parallel()

	p := fake.New()
	child := widget(t, "banner", resource.Attributes{
		fake.AttrTitle:  "Banner",
		fake.AttrParent: resource.Ref{Address: mustAddress(t, "fake.widget.missing")},
	})
	st := mustStore(t)
	planned := &plan.Plan{
		Changes: []plan.Change{
			{Address: child.Address, Action: plan.ActionCreate, After: child.Attributes},
		},
	}

	_, err := apply.Execute(context.Background(), planned, []resource.Resource{child}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Execute succeeded, want missing reference")
	}
	if !strings.Contains(err.Error(), "unknown resource") {
		t.Fatalf("error = %q, want unknown resource", err)
	}
	_, creates, _, _ := p.Calls()
	if creates != 0 {
		t.Fatalf("missing reference mutated provider: creates=%d", creates)
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
	if strings.Contains(got, "widget-") {
		t.Fatalf("apply output leaked provider-native identity:\n%s", got)
	}
	assertNoSecret(t, got)
}

func TestRunDirectDependencyOrder(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	rec := &recordingProvider{Provider: inner}
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	child := widget(t, "banner", resource.Attributes{
		fake.AttrTitle:  "Banner",
		fake.AttrParent: resource.Ref{Address: parent.Address},
	})
	st := mustStore(t)

	if _, err := apply.Run(context.Background(), []resource.Resource{child, parent}, lookupProvider(rec), st, ioDiscard()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []string{"create:fake.widget.homepage", "create:fake.widget.banner"}
	assertOps(t, rec.ops, want)
}

func TestRunTransitiveDependencyOrder(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	rec := &recordingProvider{Provider: inner}
	root := widget(t, "root", resource.Attributes{fake.AttrTitle: "Root"})
	mid := widget(t, "mid", resource.Attributes{
		fake.AttrTitle:  "Mid",
		fake.AttrParent: resource.Ref{Address: root.Address},
	})
	leaf := widget(t, "leaf", resource.Attributes{
		fake.AttrTitle:  "Leaf",
		fake.AttrParent: resource.Ref{Address: mid.Address},
	})
	st := mustStore(t)

	if _, err := apply.Run(context.Background(), []resource.Resource{leaf, root, mid}, lookupProvider(rec), st, ioDiscard()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	assertOps(t, rec.ops, []string{
		"create:fake.widget.root",
		"create:fake.widget.mid",
		"create:fake.widget.leaf",
	})
}

func TestRunBranchingDependencyOrder(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	cap := &capturingProvider{Provider: inner}
	left := widget(t, "left", resource.Attributes{fake.AttrTitle: "Left"})
	right := widget(t, "right", resource.Attributes{fake.AttrTitle: "Right"})
	child := widget(t, "child", resource.Attributes{
		fake.AttrTitle:  "Child",
		fake.AttrParent: resource.Ref{Address: left.Address},
		fake.AttrAlso:   resource.Ref{Address: right.Address},
	})
	desired := []resource.Resource{child, right, left}
	st := mustStore(t)

	if _, err := apply.Run(context.Background(), desired, lookupProvider(cap), st, ioDiscard()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	g, err := graph.Build(desired)
	if err != nil {
		t.Fatal(err)
	}
	got := addressesOf(cap.created)
	want := addresses(g.Order())
	if len(got) != len(want) {
		t.Fatalf("create order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("create order = %v, want %v", got, want)
		}
	}

	createdChild := cap.created[len(cap.created)-1]
	parent, ok := resource.AsResolved(createdChild.Attributes[fake.AttrParent])
	if !ok || parent.Address != left.Address || parent.Identity.IsZero() {
		t.Fatalf("child parent = (%v, %v), want resolved left identity", parent, ok)
	}
	also, ok := resource.AsResolved(createdChild.Attributes[fake.AttrAlso])
	if !ok || also.Address != right.Address || also.Identity.IsZero() {
		t.Fatalf("child also = (%v, %v), want resolved right identity", also, ok)
	}
}

func TestRunResolvesIdentityForDependentCreate(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	cap := &capturingProvider{Provider: inner}
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	child := widget(t, "banner", resource.Attributes{
		fake.AttrTitle:  "Banner",
		fake.AttrParent: resource.Ref{Address: parent.Address},
	})
	st := mustStore(t)

	if _, err := apply.Run(context.Background(), []resource.Resource{child, parent}, lookupProvider(cap), st, ioDiscard()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(cap.created) != 2 {
		t.Fatalf("created = %d, want 2", len(cap.created))
	}
	parentLive := cap.created[0]
	if parentLive.Address.String() != "fake.widget.homepage" {
		t.Fatalf("first create = %s, want homepage", parentLive.Address)
	}
	if _, ok := resource.AsRef(parentLive.Attributes[fake.AttrParent]); ok {
		t.Fatal("parent create should not carry a parent ref")
	}

	childLive := cap.created[1]
	if childLive.Address.String() != "fake.widget.banner" {
		t.Fatalf("second create = %s, want banner", childLive.Address)
	}
	if _, ok := resource.AsRef(childLive.Attributes[fake.AttrParent]); ok {
		t.Fatal("dependent create still received an unresolved Ref")
	}
	resolved, ok := resource.AsResolved(childLive.Attributes[fake.AttrParent])
	if !ok {
		t.Fatalf("parent attr type = %T, want resource.Resolved", childLive.Attributes[fake.AttrParent])
	}
	if resolved.Address != parent.Address {
		t.Fatalf("resolved address = %s, want %s", resolved.Address, parent.Address)
	}
	parentID, ok, err := st.Identity(parent.Address)
	if err != nil || !ok {
		t.Fatalf("parent identity = (%v,%v,%v)", parentID, ok, err)
	}
	if resolved.Identity != parentID {
		t.Fatalf("resolved identity = %q, want persisted %q", resolved.Identity.ID, parentID.ID)
	}
	if _, ok := resolved.Outputs[fake.AttrSerial]; !ok {
		t.Fatalf("resolved outputs = %+v, want computed serial", resolved.Outputs)
	}
	if _, ok := resource.AsRef(child.Attributes[fake.AttrParent]); !ok {
		t.Fatal("user-authored child attributes lost their Ref")
	}
}

func TestRunUnchangedPrerequisiteStillResolves(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	cap := &capturingProvider{Provider: inner}
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, inner, parent, resource.Attributes{fake.AttrSerial: 4})
	st := mustStore(t)
	if err := st.Bind(parent.Address, resource.Identity{ID: "id-homepage"}); err != nil {
		t.Fatal(err)
	}

	child := widget(t, "banner", resource.Attributes{
		fake.AttrTitle:  "Banner",
		fake.AttrParent: resource.Ref{Address: parent.Address},
	})
	if _, err := apply.Run(context.Background(), []resource.Resource{child, parent}, lookupProvider(cap), st, ioDiscard()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(cap.created) != 1 || cap.created[0].Address.String() != "fake.widget.banner" {
		t.Fatalf("creates = %v, want banner only", addressesOf(cap.created))
	}
	_, creates, updates, _ := inner.Calls()
	if creates != 1 || updates != 0 {
		t.Fatalf("inner creates=%d updates=%d, want 1 0", creates, updates)
	}
	resolved, ok := resource.AsResolved(cap.created[0].Attributes[fake.AttrParent])
	if !ok || resolved.Address != parent.Address || resolved.Identity.ID != "id-homepage" {
		t.Fatalf("resolved parent = (%v, %v), want homepage id-homepage", resolved, ok)
	}
	if got := resolved.Outputs[fake.AttrSerial]; got != 4 {
		t.Fatalf("resolved outputs = %+v, want serial 4", resolved.Outputs)
	}
}

func TestRunResolvesIdentityForDependentUpdate(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	cap := &capturingProvider{Provider: inner}
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	child := widget(t, "banner", resource.Attributes{
		fake.AttrTitle:  "Banner",
		fake.AttrParent: resource.Ref{Address: parent.Address},
	})
	seed(t, inner, parent, resource.Attributes{fake.AttrSerial: 1})
	seed(t, inner, child, resource.Attributes{fake.AttrSerial: 2})
	st := mustStore(t)
	if err := st.Bind(parent.Address, resource.Identity{ID: "id-homepage"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(child.Address, resource.Identity{ID: "id-banner"}); err != nil {
		t.Fatal(err)
	}

	desiredChild := widget(t, "banner", resource.Attributes{
		fake.AttrTitle:  "Banner",
		fake.AttrColor:  "red",
		fake.AttrParent: resource.Ref{Address: parent.Address},
	})
	if _, err := apply.Run(context.Background(), []resource.Resource{desiredChild, parent}, lookupProvider(cap), st, ioDiscard()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(cap.updated) != 1 {
		t.Fatalf("updates = %d, want 1", len(cap.updated))
	}
	resolved, ok := resource.AsResolved(cap.updated[0].Attributes[fake.AttrParent])
	if !ok || resolved.Identity.ID != "id-homepage" {
		t.Fatalf("updated parent attr = (%v, %v), want resolved id-homepage", resolved, ok)
	}
	if got := resolved.Outputs[fake.AttrSerial]; got != 1 {
		t.Fatalf("updated parent outputs = %+v, want serial 1 from unchanged prerequisite", resolved.Outputs)
	}
}

func TestExecuteMissingRuntimeOutputDoesNotMutateDependent(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	p := &requiringOutputProvider{Provider: inner, attr: fake.AttrParent, output: "etag"}
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	child := widget(t, "banner", resource.Attributes{
		fake.AttrTitle:  "Banner",
		fake.AttrParent: resource.Ref{Address: parent.Address},
	})
	st := mustStore(t)
	planned := &plan.Plan{
		Changes: []plan.Change{
			{
				Address:  parent.Address,
				Action:   plan.ActionUnchanged,
				Identity: resource.Identity{ID: "id-homepage"},
				After:    parent.Attributes,
				Computed: resource.Attributes{fake.AttrSerial: 4},
			},
			{Address: child.Address, Action: plan.ActionCreate, After: child.Attributes},
		},
	}

	_, err := apply.Execute(context.Background(), planned, []resource.Resource{child, parent}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Execute succeeded, want missing runtime output")
	}
	msg := err.Error()
	for _, want := range []string{"fake.widget.banner", "create", "parent", "fake.widget.homepage", "etag"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error = %q, want %q", msg, want)
		}
	}
	_, creates, updates, _ := inner.Calls()
	if creates != 0 || updates != 0 {
		t.Fatalf("missing output mutated provider: creates=%d updates=%d", creates, updates)
	}
	if _, ok, _ := st.Identity(child.Address); ok {
		t.Fatal("dependent identity was persisted after missing output")
	}
}

func TestExecuteMissingRuntimeIdentityDoesNotMutateDependent(t *testing.T) {
	t.Parallel()

	p := fake.New()
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	child := widget(t, "banner", resource.Attributes{
		fake.AttrTitle:  "Banner",
		fake.AttrParent: resource.Ref{Address: parent.Address},
	})
	st := mustStore(t)
	planned := &plan.Plan{
		Changes: []plan.Change{
			{Address: parent.Address, Action: plan.ActionUnchanged, After: parent.Attributes},
			{Address: child.Address, Action: plan.ActionCreate, After: child.Attributes},
		},
	}

	_, err := apply.Execute(context.Background(), planned, []resource.Resource{child, parent}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Execute succeeded, want missing runtime identity")
	}
	msg := err.Error()
	for _, want := range []string{"fake.widget.banner", "create", "parent", "fake.widget.homepage", "no provider-native identity"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error = %q, want %q", msg, want)
		}
	}
	_, creates, updates, _ := p.Calls()
	if creates != 0 || updates != 0 {
		t.Fatalf("missing identity mutated provider: creates=%d updates=%d", creates, updates)
	}
}

func TestRunUpdatePrerequisiteBeforeDependentCreate(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	rec := &recordingProvider{Provider: inner}
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, inner, parent, resource.Attributes{fake.AttrSerial: 1})
	st := mustStore(t)
	if err := st.Bind(parent.Address, resource.Identity{ID: "id-homepage"}); err != nil {
		t.Fatal(err)
	}

	desiredParent := widget(t, "homepage", resource.Attributes{
		fake.AttrTitle: "Homepage",
		fake.AttrColor: "blue",
	})
	child := widget(t, "banner", resource.Attributes{
		fake.AttrTitle:  "Banner",
		fake.AttrParent: resource.Ref{Address: parent.Address},
	})
	if _, err := apply.Run(context.Background(), []resource.Resource{child, desiredParent}, lookupProvider(rec), st, ioDiscard()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertOps(t, rec.ops, []string{
		"update:fake.widget.homepage",
		"create:fake.widget.banner",
	})
}

func TestRunFailedPrerequisiteDoesNotExecuteDependent(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	parentAddr := mustAddress(t, "fake.widget.homepage")
	p := &failingAddressProvider{
		Provider: inner,
		failAddr: parentAddr,
		failErr:  errors.New("remote create rejected"),
	}
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	child := widget(t, "banner", resource.Attributes{
		fake.AttrTitle:  "Banner",
		fake.AttrParent: resource.Ref{Address: parent.Address},
	})
	st := mustStore(t)

	_, err := apply.Run(context.Background(), []resource.Resource{child, parent}, lookupProvider(p), st, ioDiscard())
	if err == nil {
		t.Fatal("Run succeeded, want parent create failure")
	}
	if !strings.Contains(err.Error(), "fake.widget.homepage") || !strings.Contains(err.Error(), "remote create rejected") {
		t.Fatalf("error = %q, want homepage create failure", err)
	}

	_, creates, updates, _ := inner.Calls()
	if creates != 0 || updates != 0 {
		t.Fatalf("failed prerequisite still mutated dependents: creates=%d updates=%d", creates, updates)
	}
	if _, ok, _ := st.Identity(child.Address); ok {
		t.Fatal("dependent identity was persisted after prerequisite failure")
	}
}

func TestRunThenPlanReferencedIsUnchanged(t *testing.T) {
	t.Parallel()

	p := fake.New()
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	child := widget(t, "banner", resource.Attributes{
		fake.AttrTitle:  "Banner",
		fake.AttrParent: resource.Ref{Address: parent.Address},
	})
	st := mustStore(t)
	desired := []resource.Resource{child, parent}

	if _, err := apply.Run(context.Background(), desired, lookupProvider(p), st, ioDiscard()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, err := plan.BuildWithState(context.Background(), desired, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("plan after apply: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("plan after referenced apply has changes: %+v", got.Changes)
	}
}

func assertOps(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("ops = %v, want %v", got, want)
	}
	for i, op := range got {
		if op != want[i] {
			t.Fatalf("ops = %v, want %v", got, want)
		}
	}
}

func addressesOf(resources []resource.Resource) []string {
	out := make([]string, len(resources))
	for i, res := range resources {
		out[i] = res.Address.String()
	}
	return out
}

func addresses(addrs []resource.Address) []string {
	out := make([]string, len(addrs))
	for i, addr := range addrs {
		out[i] = addr.String()
	}
	return out
}

type capturingProvider struct {
	provider.Provider
	mu      sync.Mutex
	created []resource.Resource
	updated []resource.Resource
}

func (p *capturingProvider) Outputs(resourceType string) []provider.OutputSpec {
	return provider.OutputsOf(p.Provider, resourceType)
}

func (p *capturingProvider) Create(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	p.mu.Lock()
	p.created = append(p.created, cloneCaptured(res))
	p.mu.Unlock()
	return p.Provider.Create(ctx, res)
}

func (p *capturingProvider) Update(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	p.mu.Lock()
	p.updated = append(p.updated, cloneCaptured(desired))
	p.mu.Unlock()
	return p.Provider.Update(ctx, desired, actual)
}

func cloneCaptured(res resource.Resource) resource.Resource {
	return resource.Resource{
		Address:    res.Address,
		Identity:   res.Identity,
		Attributes: res.Attributes.Clone(),
	}
}

type failingAddressProvider struct {
	provider.Provider
	failAddr resource.Address
	failErr  error
}

func (p *failingAddressProvider) Create(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if res.Address == p.failAddr {
		return resource.RemoteResource{}, p.failErr
	}
	return p.Provider.Create(ctx, res)
}

type requiringOutputProvider struct {
	provider.Provider
	attr   string
	output string
}

func (p *requiringOutputProvider) Create(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := requireResolvedOutput(res, p.attr, p.output); err != nil {
		return resource.RemoteResource{}, err
	}
	return p.Provider.Create(ctx, res)
}

func (p *requiringOutputProvider) Update(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := requireResolvedOutput(desired, p.attr, p.output); err != nil {
		return resource.RemoteResource{}, err
	}
	return p.Provider.Update(ctx, desired, actual)
}

func requireResolvedOutput(res resource.Resource, attr, output string) error {
	v, ok := res.Attributes[attr]
	if !ok {
		return nil
	}
	resolved, ok := resource.AsResolved(v)
	if !ok {
		return fmt.Errorf("attribute %q: dependency is not resolved", attr)
	}
	if _, ok := resolved.Outputs[output]; !ok {
		return fmt.Errorf("attribute %q: dependency %s has no output %q", attr, resolved.Address, output)
	}
	return nil
}
