package apply_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestRunCrossProviderOutputResolution(t *testing.T) {
	t.Parallel()

	widgets := fake.New()
	notes := fake.NewAlt()
	cap := &capturingProvider{Provider: notes}
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	child := noteResource(t, "banner", resource.Ref{Address: parent.Address, Output: fake.OutputToken})
	st := mustStore(t)
	var out bytes.Buffer

	result, err := apply.Run(context.Background(), []resource.Resource{child, parent}, lookupOutputs(widgets, cap), st, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Created != 2 {
		t.Fatalf("result = %+v, want 2 created", result)
	}

	got := out.String()
	if strings.Index(got, "fake.widget.homepage: created") > strings.Index(got, "alt.note.banner: created") {
		t.Fatalf("dependent created before prerequisite:\n%s", got)
	}
	if strings.Contains(got, "tok-") || strings.Contains(got, "widget-") {
		t.Fatalf("apply leaked native output or identity:\n%s", got)
	}

	if len(cap.created) != 1 {
		t.Fatalf("note creates = %d, want 1", len(cap.created))
	}
	text := cap.created[0].Attributes[fake.AttrText]
	if text != "tok-homepage" {
		t.Fatalf("resolved text = %#v, want tok-homepage", text)
	}
	if _, ok := resource.AsRef(text); ok {
		t.Fatal("output reference was not substituted")
	}
	if _, ok := resource.AsResolved(text); ok {
		t.Fatal("output reference leaked a full Resolved binding")
	}

	planned, err := plan.BuildWithState(context.Background(), []resource.Resource{child, parent}, func(addr resource.Address) (provider.Reader, error) {
		return lookupOutputs(widgets, notes)(addr)
	}, st)
	if err != nil {
		t.Fatalf("plan after apply: %v", err)
	}
	if planned.HasChanges() {
		t.Fatalf("plan after output apply has changes:\n%s", plan.Format(planned))
	}
}

func TestRunSameProviderAddressRefUnchanged(t *testing.T) {
	t.Parallel()

	inner := fake.New()
	cap := &capturingProvider{Provider: inner}
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	child := widget(t, "banner", resource.Attributes{
		fake.AttrTitle:  "Banner",
		fake.AttrParent: resource.Ref{Address: parent.Address},
		fake.AttrLabel:  resource.Ref{Address: parent.Address, Output: fake.OutputToken},
	})
	st := mustStore(t)

	if _, err := apply.Run(context.Background(), []resource.Resource{child, parent}, lookupProvider(cap), st, ioDiscard()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(cap.created) != 2 {
		t.Fatalf("created = %d, want 2", len(cap.created))
	}
	banner := cap.created[1]
	resolved, ok := resource.AsResolved(banner.Attributes[fake.AttrParent])
	if !ok || resolved.Identity.IsZero() {
		t.Fatalf("parent = (%v, %v), want full Resolved binding", resolved, ok)
	}
	if banner.Attributes[fake.AttrLabel] != "tok-homepage" {
		t.Fatalf("label = %#v, want tok-homepage", banner.Attributes[fake.AttrLabel])
	}
}

func TestExecuteMissingOutputDoesNotMutateDependent(t *testing.T) {
	t.Parallel()

	widgets := fake.New()
	notes := fake.NewAlt()
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	child := noteResource(t, "banner", resource.Ref{Address: parent.Address, Output: fake.OutputToken})
	later := widget(t, "sidebar", resource.Attributes{fake.AttrTitle: "Sidebar"})
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
			{Address: later.Address, Action: plan.ActionCreate, After: later.Attributes},
		},
	}

	_, err := apply.Execute(context.Background(), planned, []resource.Resource{child, parent, later}, lookupOutputs(widgets, notes), st, ioDiscard())
	if err == nil {
		t.Fatal("Execute succeeded, want missing output")
	}
	msg := err.Error()
	for _, want := range []string{"alt.note.banner", "create", "text", "fake.widget.homepage", fake.OutputToken} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error = %q, want %q", msg, want)
		}
	}
	_, creates, updates, _ := widgets.Calls()
	noteReads, noteCreates, noteUpdates, _ := notes.Calls()
	if creates != 0 || updates != 0 || noteCreates != 0 || noteUpdates != 0 {
		t.Fatalf("missing output mutated providers: widget creates=%d updates=%d note creates=%d updates=%d reads=%d", creates, updates, noteCreates, noteUpdates, noteReads)
	}
	if _, ok, _ := st.Identity(child.Address); ok {
		t.Fatal("dependent identity was persisted after missing output")
	}
	if _, ok, _ := st.Identity(later.Address); ok {
		t.Fatal("later resource was persisted after missing output")
	}
}

func TestExecuteWrongKindOutputDoesNotMutateDependent(t *testing.T) {
	t.Parallel()

	widgets := fake.New()
	notes := fake.NewAlt()
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	child := noteResource(t, "banner", resource.Ref{Address: parent.Address, Output: fake.OutputToken})
	st := mustStore(t)
	planned := &plan.Plan{
		Changes: []plan.Change{
			{
				Address:  parent.Address,
				Action:   plan.ActionUnchanged,
				Identity: resource.Identity{ID: "id-homepage"},
				After:    parent.Attributes,
				Computed: resource.Attributes{fake.OutputToken: 12},
			},
			{Address: child.Address, Action: plan.ActionCreate, After: child.Attributes},
		},
	}

	_, err := apply.Execute(context.Background(), planned, []resource.Resource{child, parent}, lookupOutputs(widgets, notes), st, ioDiscard())
	if err == nil {
		t.Fatal("Execute succeeded, want wrong-kind output")
	}
	msg := err.Error()
	for _, want := range []string{"kind", "number", "string", fake.OutputToken} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error = %q, want %q", msg, want)
		}
	}
	_, noteCreates, _, _ := notes.Calls()
	if noteCreates != 0 {
		t.Fatalf("wrong-kind mutated note provider: creates=%d", noteCreates)
	}
}

func TestExecuteUnknownOutputDoesNotMutate(t *testing.T) {
	t.Parallel()

	widgets := fake.New()
	notes := fake.NewAlt()
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	child := noteResource(t, "banner", resource.Ref{Address: parent.Address, Output: "etag"})
	st := mustStore(t)
	planned := &plan.Plan{
		Changes: []plan.Change{
			{Address: parent.Address, Action: plan.ActionCreate, After: parent.Attributes},
			{Address: child.Address, Action: plan.ActionCreate, After: child.Attributes},
		},
	}

	_, err := apply.Execute(context.Background(), planned, []resource.Resource{child, parent}, lookupOutputs(widgets, notes), st, ioDiscard())
	if err == nil {
		t.Fatal("Execute succeeded, want unknown output")
	}
	if !strings.Contains(err.Error(), "no declared output") {
		t.Fatalf("error = %q, want no declared output", err)
	}
	_, creates, _, _ := widgets.Calls()
	_, noteCreates, _, _ := notes.Calls()
	if creates != 0 || noteCreates != 0 {
		t.Fatalf("unknown output mutated providers: widget creates=%d note creates=%d", creates, noteCreates)
	}
}

func TestRunImportedResourceOutputsResolve(t *testing.T) {
	t.Parallel()

	widgets := fake.New()
	notes := fake.NewAlt()
	cap := &capturingProvider{Provider: notes}
	parent := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Imported"})
	seed(t, widgets, parent, resource.Attributes{
		fake.AttrSerial:  9,
		fake.OutputToken: "tok-homepage",
	})
	st := mustStore(t)
	if err := st.Bind(parent.Address, resource.Identity{ID: "id-homepage"}); err != nil {
		t.Fatal(err)
	}

	child := noteResource(t, "banner", resource.Ref{Address: parent.Address, Output: fake.OutputToken})
	if _, err := apply.Run(context.Background(), []resource.Resource{child, parent}, lookupOutputs(widgets, cap), st, ioDiscard()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(cap.created) != 1 {
		t.Fatalf("creates = %d, want 1", len(cap.created))
	}
	if cap.created[0].Attributes[fake.AttrText] != "tok-homepage" {
		t.Fatalf("imported output = %#v, want tok-homepage", cap.created[0].Attributes[fake.AttrText])
	}
}

func noteResource(t *testing.T, name string, text any) resource.Resource {
	t.Helper()
	return resource.Resource{
		Address:    mustAddress(t, "alt.note."+name),
		Attributes: resource.Attributes{fake.AttrText: text},
	}
}

func lookupOutputs(providers ...provider.Provider) apply.Lookup {
	byName := make(map[string]provider.Provider, len(providers))
	for _, p := range providers {
		byName[p.Name()] = p
	}
	return func(addr resource.Address) (provider.Provider, error) {
		p, ok := byName[addr.Provider]
		if !ok {
			return nil, errUnknownApplyProvider(addr.Provider)
		}
		return p, nil
	}
}

func errUnknownApplyProvider(name string) error {
	return &applyProviderError{name: name}
}

type applyProviderError struct {
	name string
}

func (e *applyProviderError) Error() string {
	return "unknown provider " + e.name
}
