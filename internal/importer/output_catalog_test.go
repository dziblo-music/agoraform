package importer_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/importer"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestOutputCatalogUniqueMatch(t *testing.T) {
	t.Parallel()

	widgets := fake.New()
	parent := widgetAddr(t, "homepage")
	seedImported(t, widgets, parent, resource.Attributes{fake.AttrTitle: "Homepage"}, resource.Attributes{
		fake.AttrSerial:  1,
		fake.OutputToken: "tok-homepage",
	})

	cat := importer.NewOutputCatalog(staticBindings{
		{Address: parent, RemoteID: "widget-imported"},
	}, lookupProvider(widgets))

	ref, result, err := cat.Match(context.Background(), provider.OutputMatchQuery{
		Provider:     fake.Name,
		ResourceType: fake.TypeWidget,
		Output:       fake.OutputToken,
		Value:        "tok-homepage",
	})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if result != provider.OutputMatchUnique || ref.Address != parent || ref.Output != fake.OutputToken {
		t.Fatalf("Match = (%+v, %v), want unique %s output %s", ref, result, parent, fake.OutputToken)
	}
}

func TestOutputCatalogNoneAndAmbiguous(t *testing.T) {
	t.Parallel()

	widgets := fake.New()
	home := widgetAddr(t, "homepage")
	side := widgetAddr(t, "sidebar")
	seedImported(t, widgets, home, resource.Attributes{fake.AttrTitle: "Home"}, resource.Attributes{fake.OutputToken: "shared"})
	if err := widgets.Seed(resource.RemoteResource{
		Address:    side,
		Identity:   resource.Identity{ID: "widget-side"},
		Attributes: resource.Attributes{fake.AttrTitle: "Side"},
		Computed:   resource.Attributes{fake.AttrSerial: 2, fake.OutputToken: "shared"},
	}); err != nil {
		t.Fatal(err)
	}

	cat := importer.NewOutputCatalog(staticBindings{
		{Address: home, RemoteID: "widget-imported"},
		{Address: side, RemoteID: "widget-side"},
	}, lookupProvider(widgets))

	ref, result, err := cat.Match(context.Background(), provider.OutputMatchQuery{
		Provider:     fake.Name,
		ResourceType: fake.TypeWidget,
		Output:       fake.OutputToken,
		Value:        "missing",
	})
	if err != nil {
		t.Fatalf("missing Match: %v", err)
	}
	if result != provider.OutputMatchNone || !ref.IsZero() {
		t.Fatalf("missing result = (%+v, %v), want none", ref, result)
	}

	ref, result, err = cat.Match(context.Background(), provider.OutputMatchQuery{
		Provider:     fake.Name,
		ResourceType: fake.TypeWidget,
		Output:       fake.OutputToken,
		Value:        "shared",
	})
	if err != nil {
		t.Fatalf("ambiguous Match: %v", err)
	}
	if result != provider.OutputMatchAmbiguous || !ref.IsZero() {
		t.Fatalf("shared result = (%+v, %v), want ambiguous", ref, result)
	}
}

func TestOutputCatalogMultipleFieldMatch(t *testing.T) {
	t.Parallel()

	widgets := fake.New()
	home := widgetAddr(t, "homepage")
	side := widgetAddr(t, "sidebar")
	seedImported(t, widgets, home, resource.Attributes{fake.AttrTitle: "Home"}, resource.Attributes{
		fake.AttrSerial:  1,
		fake.OutputToken: "shared",
	})
	if err := widgets.Seed(resource.RemoteResource{
		Address:    side,
		Identity:   resource.Identity{ID: "widget-side"},
		Attributes: resource.Attributes{fake.AttrTitle: "Side"},
		Computed:   resource.Attributes{fake.AttrSerial: 2, fake.OutputToken: "shared"},
	}); err != nil {
		t.Fatal(err)
	}

	cat := importer.NewOutputCatalog(staticBindings{
		{Address: side, RemoteID: "widget-side"},
		{Address: home, RemoteID: "widget-imported"},
	}, lookupProvider(widgets))

	ref, result, err := cat.Match(context.Background(), provider.OutputMatchQuery{
		Provider:     fake.Name,
		ResourceType: fake.TypeWidget,
		Output:       fake.OutputToken,
		Equals: map[string]string{
			fake.OutputToken: "shared",
			fake.AttrSerial:  "1",
		},
	})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if result != provider.OutputMatchUnique || ref.Address != home || ref.Output != fake.OutputToken {
		t.Fatalf("Match = (%+v, %v), want unique %s", ref, result, home)
	}

	ref, result, err = cat.Match(context.Background(), provider.OutputMatchQuery{
		Provider:     fake.Name,
		ResourceType: fake.TypeWidget,
		Output:       fake.OutputToken,
		Equals: map[string]string{
			fake.OutputToken: "shared",
			fake.AttrSerial:  "99",
		},
	})
	if err != nil {
		t.Fatalf("missing multi-field Match: %v", err)
	}
	if result != provider.OutputMatchNone || !ref.IsZero() {
		t.Fatalf("missing multi-field = (%+v, %v), want none", ref, result)
	}
}

func TestOutputCatalogMatchesCreatedAndImportedPrerequisites(t *testing.T) {
	t.Parallel()

	widgets := fake.New()
	created := widgetAddr(t, "created")
	live, err := widgets.Create(context.Background(), resource.Resource{
		Address:    created,
		Attributes: resource.Attributes{fake.AttrTitle: "Created"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	token, _ := live.Computed[fake.OutputToken].(string)
	if token == "" {
		t.Fatal("created widget missing token")
	}

	imported := widgetAddr(t, "imported")
	if err := widgets.Seed(resource.RemoteResource{
		Address:    imported,
		Identity:   resource.Identity{ID: "widget-imported"},
		Attributes: resource.Attributes{fake.AttrTitle: "Imported"},
		Computed:   resource.Attributes{fake.AttrSerial: 9, fake.OutputToken: "tok-imported"},
	}); err != nil {
		t.Fatal(err)
	}

	cat := importer.NewOutputCatalog(staticBindings{
		{Address: created, RemoteID: live.Identity.ID},
		{Address: imported, RemoteID: "widget-imported"},
	}, lookupProvider(widgets))

	ref, result, err := cat.Match(context.Background(), provider.OutputMatchQuery{
		Provider:     fake.Name,
		ResourceType: fake.TypeWidget,
		Output:       fake.OutputToken,
		Value:        token,
	})
	if err != nil {
		t.Fatalf("created Match: %v", err)
	}
	if result != provider.OutputMatchUnique || ref.Address != created {
		t.Fatalf("created Match = (%+v, %v), want unique %s", ref, result, created)
	}

	ref, result, err = cat.Match(context.Background(), provider.OutputMatchQuery{
		Provider:     fake.Name,
		ResourceType: fake.TypeWidget,
		Output:       fake.OutputToken,
		Value:        "tok-imported",
	})
	if err != nil {
		t.Fatalf("imported Match: %v", err)
	}
	if result != provider.OutputMatchUnique || ref.Address != imported || ref.Output != fake.OutputToken {
		t.Fatalf("imported Match = (%+v, %v), want unique %s", ref, result, imported)
	}
}

func TestOutputCatalogMissingDeclaredOutput(t *testing.T) {
	t.Parallel()

	widgets := fake.New()
	parent := widgetAddr(t, "homepage")
	seedImported(t, widgets, parent, resource.Attributes{fake.AttrTitle: "Homepage"}, resource.Attributes{
		fake.AttrSerial: 1,
	})
	cat := importer.NewOutputCatalog(staticBindings{
		{Address: parent, RemoteID: "widget-imported"},
	}, lookupProvider(widgets))

	ref, result, err := cat.Match(context.Background(), provider.OutputMatchQuery{
		Provider:     fake.Name,
		ResourceType: fake.TypeWidget,
		Output:       fake.OutputToken,
		Value:        "tok-homepage",
	})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if result != provider.OutputMatchNone || !ref.IsZero() {
		t.Fatalf("missing output = (%+v, %v), want none", ref, result)
	}
}

func TestOutputCatalogExcludesSensitiveAndUndeclared(t *testing.T) {
	t.Parallel()

	widgets := fake.New()
	parent := widgetAddr(t, "homepage")
	seedImported(t, widgets, parent, resource.Attributes{fake.AttrTitle: "Homepage"}, resource.Attributes{
		fake.OutputToken:  "tok-homepage",
		fake.OutputSecret: "s3cret",
		"etag":            "abc",
	})
	cat := importer.NewOutputCatalog(staticBindings{
		{Address: parent, RemoteID: "widget-imported"},
	}, lookupProvider(widgets))

	ref, result, err := cat.Match(context.Background(), provider.OutputMatchQuery{
		Provider:     fake.Name,
		ResourceType: fake.TypeWidget,
		Output:       fake.OutputSecret,
		Value:        "s3cret",
	})
	if err != nil {
		t.Fatalf("sensitive Match: %v", err)
	}
	if result != provider.OutputMatchNone || !ref.IsZero() {
		t.Fatalf("sensitive result = (%+v, %v), want none", ref, result)
	}

	ref, result, err = cat.Match(context.Background(), provider.OutputMatchQuery{
		Provider:     fake.Name,
		ResourceType: fake.TypeWidget,
		Output:       "etag",
		Value:        "abc",
	})
	if err != nil {
		t.Fatalf("undeclared Match: %v", err)
	}
	if result != provider.OutputMatchNone || !ref.IsZero() {
		t.Fatalf("undeclared result = (%+v, %v), want none", ref, result)
	}
}

func TestOutputCatalogReadFailure(t *testing.T) {
	t.Parallel()

	parent := widgetAddr(t, "homepage")
	cat := importer.NewOutputCatalog(staticBindings{
		{Address: parent, RemoteID: "missing"},
	}, lookupProvider(fake.New()))

	_, _, err := cat.Match(context.Background(), provider.OutputMatchQuery{
		Provider:     fake.Name,
		ResourceType: fake.TypeWidget,
		Output:       fake.OutputToken,
		Value:        "tok",
	})
	if err == nil {
		t.Fatal("expected read failure")
	}
	if !strings.Contains(err.Error(), parent.String()) {
		t.Fatalf("error = %q, want address", err)
	}
	if strings.Contains(err.Error(), "s3cret") || strings.Contains(err.Error(), "tok-homepage") {
		t.Fatalf("error leaked an output value: %q", err)
	}
}

func TestOutputCatalogDoesNotMutate(t *testing.T) {
	t.Parallel()

	widgets := fake.New()
	parent := widgetAddr(t, "homepage")
	seedImported(t, widgets, parent, resource.Attributes{fake.AttrTitle: "Homepage"}, resource.Attributes{
		fake.OutputToken: "tok-homepage",
	})
	_, creates, updates, imports := widgets.Calls()
	cat := importer.NewOutputCatalog(staticBindings{
		{Address: parent, RemoteID: "widget-imported"},
	}, lookupProvider(widgets))
	if _, _, err := cat.Match(context.Background(), provider.OutputMatchQuery{
		Provider:     fake.Name,
		ResourceType: fake.TypeWidget,
		Output:       fake.OutputToken,
		Value:        "tok-homepage",
	}); err != nil {
		t.Fatalf("Match: %v", err)
	}
	_, createsAfter, updatesAfter, importsAfter := widgets.Calls()
	if createsAfter != creates || updatesAfter != updates {
		t.Fatalf("catalog mutated provider: creates %d->%d updates %d->%d", creates, createsAfter, updates, updatesAfter)
	}
	if importsAfter != imports+1 {
		t.Fatalf("imports = %d, want %d", importsAfter, imports+1)
	}
}

func TestOutputCatalogDeterministicOrdering(t *testing.T) {
	t.Parallel()

	widgets := fake.New()
	zeta := widgetAddr(t, "zeta")
	alpha := widgetAddr(t, "alpha")
	if err := widgets.Seed(resource.RemoteResource{
		Address:    zeta,
		Identity:   resource.Identity{ID: "widget-zeta"},
		Attributes: resource.Attributes{fake.AttrTitle: "Zeta"},
		Computed:   resource.Attributes{fake.OutputToken: "tok-unique"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := widgets.Seed(resource.RemoteResource{
		Address:    alpha,
		Identity:   resource.Identity{ID: "widget-alpha"},
		Attributes: resource.Attributes{fake.AttrTitle: "Alpha"},
		Computed:   resource.Attributes{fake.OutputToken: "other"},
	}); err != nil {
		t.Fatal(err)
	}

	first := importer.NewOutputCatalog(staticBindings{
		{Address: zeta, RemoteID: "widget-zeta"},
		{Address: alpha, RemoteID: "widget-alpha"},
	}, lookupProvider(widgets))
	second := importer.NewOutputCatalog(staticBindings{
		{Address: alpha, RemoteID: "widget-alpha"},
		{Address: zeta, RemoteID: "widget-zeta"},
	}, lookupProvider(widgets))

	query := provider.OutputMatchQuery{
		Provider:     fake.Name,
		ResourceType: fake.TypeWidget,
		Output:       fake.OutputToken,
		Value:        "tok-unique",
	}
	a, resultA, err := first.Match(context.Background(), query)
	if err != nil {
		t.Fatalf("first Match: %v", err)
	}
	b, resultB, err := second.Match(context.Background(), query)
	if err != nil {
		t.Fatalf("second Match: %v", err)
	}
	if resultA != provider.OutputMatchUnique || resultB != provider.OutputMatchUnique {
		t.Fatalf("results = %v / %v, want unique", resultA, resultB)
	}
	if a != b || a.Address != zeta || a.Output != fake.OutputToken {
		t.Fatalf("Match order-dependent: %+v vs %+v", a, b)
	}
}

func widgetAddr(t *testing.T, name string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress("fake.widget." + name)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

type staticBindings []importer.RemoteBinding

func (s staticBindings) Bindings(providerName, resourceType string) ([]importer.RemoteBinding, error) {
	out := make([]importer.RemoteBinding, 0, len(s))
	for _, b := range s {
		if b.Address.Provider == providerName && b.Address.Type == resourceType {
			out = append(out, b)
		}
	}
	return out, nil
}
