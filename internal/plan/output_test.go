package plan_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
)

func TestBuildOutputReferenceRendersLogicalSelector(t *testing.T) {
	t.Parallel()

	widgets := fake.New()
	notes := fake.NewAlt()
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	child := noteResource(t, "banner", resource.Ref{Address: parent.Address, Output: fake.OutputToken})

	got := mustBuildOutputs(t, []resource.Resource{child, parent}, widgets, notes)
	if len(got.Changes) != 2 {
		t.Fatalf("len(changes) = %d, want 2", len(got.Changes))
	}
	for _, change := range got.Changes {
		if change.Action != plan.ActionCreate {
			t.Fatalf("change %s = %+v, want create", change.Address, change)
		}
	}

	out := plan.Format(got)
	if !strings.Contains(out, `{$ref: "fake.widget.homepage", output: "token"}`) {
		t.Fatalf("plan missing logical output reference:\n%s", out)
	}
	if strings.Contains(out, "tok-") || strings.Contains(out, "widget-") || strings.Contains(out, "note-") {
		t.Fatalf("plan leaked provider-native value:\n%s", out)
	}

	again := mustBuildOutputs(t, []resource.Resource{parent, child}, widgets, notes)
	if plan.Format(got) != plan.Format(again) {
		t.Fatalf("output plan was not stable\nfirst:\n%s\nsecond:\n%s", plan.Format(got), plan.Format(again))
	}
}

func TestBuildUnknownOutputFailsBeforeRead(t *testing.T) {
	t.Parallel()

	widgets := &countingReader{Reader: fake.New()}
	notes := &countingReader{Reader: fake.NewAlt()}
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	child := noteResource(t, "banner", resource.Ref{Address: parent.Address, Output: "etag"})

	_, err := plan.Build(context.Background(), []resource.Resource{child, parent}, lookupOutputs(widgets, notes))
	if err == nil {
		t.Fatal("Build succeeded, want unknown output")
	}
	if !strings.Contains(err.Error(), "no declared output") {
		t.Fatalf("error = %q, want no declared output", err)
	}
	if widgets.reads != 0 || notes.reads != 0 {
		t.Fatalf("reads = widget %d note %d, want 0 before output validation", widgets.reads, notes.reads)
	}
}

func TestBuildSensitiveOutputFailsBeforeRead(t *testing.T) {
	t.Parallel()

	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	child := noteResource(t, "banner", resource.Ref{Address: parent.Address, Output: fake.OutputSecret})
	_, err := plan.Build(context.Background(), []resource.Resource{child, parent}, lookupOutputs(fake.New(), fake.NewAlt()))
	if err == nil {
		t.Fatal("Build succeeded, want sensitive output")
	}
	if !strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("error = %q, want sensitive", err)
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("error leaked a secret: %q", err)
	}
}

func TestBuildKnownOutputDoesNotPerpetualDiff(t *testing.T) {
	t.Parallel()

	widgets := fake.New()
	notes := fake.NewAlt()
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	seed(t, widgets, parent, resource.Attributes{
		fake.AttrSerial:  4,
		fake.OutputToken: "tok-homepage",
	})
	child := noteResource(t, "banner", resource.Ref{Address: parent.Address, Output: fake.OutputToken})
	if err := notes.Seed(resource.RemoteResource{
		Address:    child.Address,
		Identity:   resource.Identity{ID: "id-banner"},
		Attributes: resource.Attributes{fake.AttrText: "tok-homepage"},
		Computed:   resource.Attributes{},
	}); err != nil {
		t.Fatal(err)
	}

	st := mustOutputStore(t)
	if err := st.Bind(parent.Address, resource.Identity{ID: "id-homepage"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(child.Address, resource.Identity{ID: "id-banner"}); err != nil {
		t.Fatal(err)
	}

	got, err := plan.BuildWithState(context.Background(), []resource.Resource{child, parent}, lookupOutputs(widgets, notes), st)
	if err != nil {
		t.Fatalf("BuildWithState: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("plan has changes after matching output:\n%s", plan.Format(got))
	}
}

func TestBuildUnknownUntilApplyKeepsLogicalOutput(t *testing.T) {
	t.Parallel()

	widgets := fake.New()
	notes := fake.NewAlt()
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	child := noteResource(t, "banner", resource.Ref{Address: parent.Address, Output: fake.OutputToken})

	got := mustBuildOutputs(t, []resource.Resource{child, parent}, widgets, notes)
	out := plan.Format(got)
	if !strings.Contains(out, `{$ref: "fake.widget.homepage", output: "token"}`) {
		t.Fatalf("create plan should keep unknown output logical:\n%s", out)
	}
	if strings.Contains(out, "tok-homepage") {
		t.Fatalf("plan fabricated an output value:\n%s", out)
	}
}

func noteResource(t *testing.T, name string, text any) resource.Resource {
	t.Helper()
	return resource.Resource{
		Address:    mustAddress(t, "alt.note."+name),
		Attributes: resource.Attributes{fake.AttrText: text},
	}
}

func lookupOutputs(providers ...provider.Reader) plan.Lookup {
	byName := make(map[string]provider.Reader, len(providers))
	for _, p := range providers {
		byName[p.Name()] = p
	}
	return func(addr resource.Address) (provider.Reader, error) {
		p, ok := byName[addr.Provider]
		if !ok {
			return nil, errUnknownOutputProvider(addr.Provider)
		}
		return p, nil
	}
}

func mustBuildOutputs(t *testing.T, desired []resource.Resource, providers ...provider.Reader) *plan.Plan {
	t.Helper()
	got, err := plan.Build(context.Background(), desired, lookupOutputs(providers...))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return got
}

func mustOutputStore(t *testing.T) *state.Store {
	t.Helper()
	st, err := state.New(t.TempDir() + "/agoraform.state.json")
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func errUnknownOutputProvider(name string) error {
	return &outputProviderError{name: name}
}

type outputProviderError struct {
	name string
}

func (e *outputProviderError) Error() string {
	return "unknown provider " + e.name
}

type countingReader struct {
	provider.Reader
	reads int
}

func (r *countingReader) Read(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	r.reads++
	return r.Reader.Read(ctx, res)
}

func (r *countingReader) Outputs(resourceType string) []provider.OutputSpec {
	return provider.OutputsOf(r.Reader, resourceType)
}
