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

func TestBuildUnchangedIdenticalState(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, p, res, resource.Attributes{fake.AttrSerial: 7})

	got := mustBuild(t, []resource.Resource{res}, p)
	if got.HasChanges() {
		t.Fatalf("HasChanges() = true, want false: %+v", got.Changes)
	}
	create, update, destroy := got.Counts()
	if create != 0 || update != 0 || destroy != 0 {
		t.Fatalf("Counts() = %d %d %d, want 0 0 0", create, update, destroy)
	}
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionUnchanged {
		t.Fatalf("change = %+v, want unchanged", got.Changes)
	}
	assertNoMutations(t, p, 1)
}

func TestBuildCreateWhenRemoteMissing(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})

	got := mustBuild(t, []resource.Resource{res}, p)
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionCreate {
		t.Fatalf("change = %+v, want create", got.Changes)
	}
	if !got.HasChanges() {
		t.Fatal("HasChanges() = false, want true")
	}
	create, update, destroy := got.Counts()
	if create != 1 || update != 0 || destroy != 0 {
		t.Fatalf("Counts() = %d %d %d, want 1 0 0", create, update, destroy)
	}
	if got.Changes[0].Identity != (resource.Identity{}) {
		t.Fatalf("create identity = %+v, want zero", got.Changes[0].Identity)
	}
	if len(got.Changes[0].Diffs) != 1 || got.Changes[0].Diffs[0].Path != fake.AttrTitle {
		t.Fatalf("create diffs = %+v, want title", got.Changes[0].Diffs)
	}
	assertNoMutations(t, p, 1)
}

func TestBuildUpdateConfigurableField(t *testing.T) {
	t.Parallel()

	p := fake.New()
	live := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, p, live, resource.Attributes{fake.AttrSerial: 3})

	desired := widget(t, "homepage", resource.Attributes{
		fake.AttrTitle: "Homepage",
		fake.AttrColor: "blue",
	})
	got := mustBuild(t, []resource.Resource{desired}, p)
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want update", got.Changes)
	}
	if got.Changes[0].Identity.ID == "" {
		t.Fatal("update missing live identity")
	}

	var color *plan.AttributeDiff
	for i := range got.Changes[0].Diffs {
		if got.Changes[0].Diffs[i].Path == fake.AttrColor {
			color = &got.Changes[0].Diffs[i]
		}
	}
	if color == nil {
		t.Fatalf("missing color diff: %+v", got.Changes[0].Diffs)
	}
	if color.Before != nil || color.After != "blue" {
		t.Fatalf("color diff = %+v, want nil -> blue", color)
	}
	assertNoMutations(t, p, 1)
}

func TestBuildIgnoresComputedFieldDrift(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, p, res, resource.Attributes{fake.AttrSerial: 1})

	first := mustBuild(t, []resource.Resource{res}, p)
	if first.HasChanges() {
		t.Fatalf("first plan has changes: %+v", first.Changes)
	}

	// Refresh computed serial without touching configurable attributes.
	seed(t, p, res, resource.Attributes{fake.AttrSerial: 99})
	second := mustBuild(t, []resource.Resource{res}, p)
	if second.HasChanges() {
		t.Fatalf("computed serial drift produced a change: %+v", second.Changes)
	}
	assertNoMutations(t, p, 2)
}

func TestBuildReadFailure(t *testing.T) {
	t.Parallel()

	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	lookup := func(resource.Address) (provider.Reader, error) {
		return failingReader{err: errors.New("connection refused")}, nil
	}

	_, err := plan.Build(context.Background(), []resource.Resource{res}, lookup)
	if err == nil {
		t.Fatal("Build succeeded, want read error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "fake.widget.homepage") {
		t.Fatalf("error %q missing resource address", msg)
	}
	if !strings.Contains(msg, "connection refused") {
		t.Fatalf("error %q missing underlying cause", msg)
	}
}

func TestBuildDeterministicOrdering(t *testing.T) {
	t.Parallel()

	p := fake.New()
	zeta := widget(t, "zeta", resource.Attributes{fake.AttrTitle: "Zeta"})
	alpha := widget(t, "alpha", resource.Attributes{fake.AttrTitle: "Alpha"})
	mu := widget(t, "mu", resource.Attributes{fake.AttrTitle: "Mu"})

	first := mustBuild(t, []resource.Resource{zeta, alpha, mu}, p)
	second := mustBuild(t, []resource.Resource{mu, zeta, alpha}, p)

	if len(first.Changes) != 3 {
		t.Fatalf("len(changes) = %d, want 3", len(first.Changes))
	}
	wantOrder := []string{"fake.widget.alpha", "fake.widget.mu", "fake.widget.zeta"}
	for i, addr := range wantOrder {
		if first.Changes[i].Address.String() != addr {
			t.Fatalf("first[%d] = %s, want %s", i, first.Changes[i].Address, addr)
		}
		if second.Changes[i].Address.String() != addr {
			t.Fatalf("second[%d] = %s, want %s", i, second.Changes[i].Address, addr)
		}
	}

	out1 := plan.Format(first)
	out2 := plan.Format(second)
	if out1 != out2 {
		t.Fatalf("Format output differed:\n%s\n---\n%s", out1, out2)
	}
	assertNoMutations(t, p, 6)
}

func TestBuildUsesReaderOnly(t *testing.T) {
	t.Parallel()

	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	reader := staticReader{
		live: resource.RemoteResource{
			Address:    res.Address,
			Attributes: resource.Attributes{fake.AttrTitle: "Homepage"},
			Computed:   resource.Attributes{fake.AttrSerial: 4},
		},
	}

	got, err := plan.Build(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return reader, nil
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("reader-only plan has changes: %+v", got.Changes)
	}
}

func TestBuildNormalizerOmitsDefaultValues(t *testing.T) {
	t.Parallel()

	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	reader := normalizingReader{
		staticReader: staticReader{
			live: resource.RemoteResource{
				Address: res.Address,
				Attributes: resource.Attributes{
					fake.AttrTitle: "Homepage",
					fake.AttrColor: "",
				},
			},
		},
	}

	got, err := plan.Build(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return reader, nil
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("default color should not diff: %+v", got.Changes)
	}
}

func TestBuildUnknownProvider(t *testing.T) {
	t.Parallel()

	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	_, err := plan.Build(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return nil, errors.New("unknown provider \"fake\"")
	})
	if err == nil {
		t.Fatal("Build succeeded, want lookup error")
	}
	if !strings.Contains(err.Error(), "fake.widget.homepage") {
		t.Fatalf("error %q missing address", err)
	}
}

func TestBuildEmptyDesired(t *testing.T) {
	t.Parallel()

	got, err := plan.Build(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Build empty: %v", err)
	}
	if got.HasChanges() || len(got.Changes) != 0 {
		t.Fatalf("empty plan = %+v", got)
	}
}

func TestBuildNestedAttributeDiff(t *testing.T) {
	t.Parallel()

	addr := mustAddress(t, "fake.widget.homepage")
	desired := resource.Resource{
		Address: addr,
		Attributes: resource.Attributes{
			fake.AttrTitle: "Homepage",
			"meta": map[string]any{
				"theme": "dark",
			},
			"tags": []any{"a", "c"},
		},
	}
	reader := staticReader{
		live: resource.RemoteResource{
			Address: addr,
			Attributes: resource.Attributes{
				fake.AttrTitle: "Homepage",
				"meta": map[string]any{
					"theme": "light",
				},
				"tags": []any{"a", "b"},
			},
		},
	}

	got, err := plan.Build(context.Background(), []resource.Resource{desired}, func(resource.Address) (provider.Reader, error) {
		return reader, nil
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionUpdate {
		t.Fatalf("change = %+v, want update", got.Changes)
	}

	paths := map[string]plan.AttributeDiff{}
	for _, d := range got.Changes[0].Diffs {
		paths[d.Path] = d
	}
	if d, ok := paths["meta.theme"]; !ok || d.Before != "light" || d.After != "dark" {
		t.Fatalf("meta.theme diff = %+v", got.Changes[0].Diffs)
	}
	if d, ok := paths["tags[1]"]; !ok || d.Before != "b" || d.After != "c" {
		t.Fatalf("tags[1] diff = %+v", got.Changes[0].Diffs)
	}
}

func TestFormatCreateUpdateAndZeroChange(t *testing.T) {
	t.Parallel()

	p := fake.New()
	createRes := widget(t, "trial_started", resource.Attributes{
		fake.AttrTitle: "Trial Started",
	})
	updateLive := widget(t, "user_id", resource.Attributes{
		fake.AttrTitle: "User",
		fake.AttrColor: "uid",
	})
	seed(t, p, updateLive, resource.Attributes{fake.AttrSerial: 2})
	unchanged := widget(t, "stable", resource.Attributes{fake.AttrTitle: "Stable"})
	seed(t, p, unchanged, resource.Attributes{fake.AttrSerial: 1})

	desired := []resource.Resource{
		createRes,
		widget(t, "user_id", resource.Attributes{
			fake.AttrTitle: "User",
			fake.AttrColor: "userId",
		}),
		unchanged,
	}

	got := mustBuild(t, desired, p)
	out := plan.Format(got)
	want := strings.Join([]string{
		"Agoraform will perform the following actions:",
		"",
		"+ fake.widget.trial_started",
		"",
		`    title: "Trial Started"`,
		"",
		"~ fake.widget.user_id",
		"",
		"    color:",
		`      "uid" -> "userId"`,
		"",
		"Plan: 1 to create, 1 to update, 0 to destroy.",
		"",
	}, "\n")
	if out != want {
		t.Fatalf("Format() mismatch\ngot:\n%s\nwant:\n%s", out, want)
	}

	zero := mustBuild(t, []resource.Resource{unchanged}, p)
	zeroOut := plan.Format(zero)
	if !strings.Contains(zeroOut, "No changes.") {
		t.Fatalf("zero-change output missing No changes:\n%s", zeroOut)
	}
	if !strings.HasSuffix(zeroOut, "Plan: 0 to create, 0 to update, 0 to destroy.\n") {
		t.Fatalf("zero-change summary:\n%s", zeroOut)
	}

	again := plan.Format(got)
	if again != out {
		t.Fatal("Format is not deterministic")
	}
}

func TestFormatNilPlan(t *testing.T) {
	t.Parallel()

	out := plan.Format(nil)
	if !strings.Contains(out, "No changes.") {
		t.Fatalf("nil plan output:\n%s", out)
	}
}

type staticReader struct {
	live resource.RemoteResource
	err  error
}

func (s staticReader) Name() string            { return fake.Name }
func (s staticReader) ResourceTypes() []string { return []string{fake.TypeWidget} }
func (s staticReader) Validate(context.Context, resource.Resource) error {
	return nil
}
func (s staticReader) Read(context.Context, resource.Resource) (resource.RemoteResource, error) {
	if s.err != nil {
		return resource.RemoteResource{}, s.err
	}
	return s.live, nil
}

type failingReader struct {
	err error
}

func (f failingReader) Name() string            { return fake.Name }
func (f failingReader) ResourceTypes() []string { return []string{fake.TypeWidget} }
func (f failingReader) Validate(context.Context, resource.Resource) error {
	return nil
}
func (f failingReader) Read(context.Context, resource.Resource) (resource.RemoteResource, error) {
	return resource.RemoteResource{}, f.err
}

type normalizingReader struct {
	staticReader
}

func (n normalizingReader) NormalizeComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	want := dropEmptyColor(desired.Attributes.Clone())
	if live == nil {
		return want, nil, nil
	}
	return want, dropEmptyColor(live.Attributes.Clone()), nil
}

func dropEmptyColor(attrs resource.Attributes) resource.Attributes {
	if color, ok := attrs[fake.AttrColor]; ok {
		s, isString := color.(string)
		if isString && s == "" {
			delete(attrs, fake.AttrColor)
		}
	}
	return attrs
}

func widget(t *testing.T, name string, attrs resource.Attributes) resource.Resource {
	t.Helper()
	return resource.Resource{
		Address:    mustAddress(t, "fake.widget."+name),
		Attributes: attrs,
	}
}

func seed(t *testing.T, p *fake.Provider, res resource.Resource, computed resource.Attributes) {
	t.Helper()
	if err := p.Seed(resource.RemoteResource{
		Address:    res.Address,
		Identity:   resource.Identity{ID: "id-" + res.Address.Name},
		Attributes: res.Attributes.Clone(),
		Computed:   computed.Clone(),
	}); err != nil {
		t.Fatalf("Seed: %v", err)
	}
}

func mustBuild(t *testing.T, desired []resource.Resource, p *fake.Provider) *plan.Plan {
	t.Helper()
	got, err := plan.Build(context.Background(), desired, lookupProvider(p))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return got
}

func lookupProvider(p provider.Reader) plan.Lookup {
	return func(resource.Address) (provider.Reader, error) {
		return p, nil
	}
}

func assertNoMutations(t *testing.T, p *fake.Provider, wantReads int) {
	t.Helper()
	reads, creates, updates, imports := p.Calls()
	if creates != 0 || updates != 0 || imports != 0 {
		t.Fatalf("mutations during plan: creates=%d updates=%d imports=%d", creates, updates, imports)
	}
	if reads != wantReads {
		t.Fatalf("reads = %d, want %d", reads, wantReads)
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
