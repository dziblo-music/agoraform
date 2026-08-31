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

	addr, result, err := cat.Match(context.Background(), provider.OutputMatchQuery{
		Provider:     fake.Name,
		ResourceType: fake.TypeWidget,
		Output:       fake.OutputToken,
		Value:        "tok-homepage",
	})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if result != provider.OutputMatchUnique || addr != parent {
		t.Fatalf("Match = (%s, %v), want unique %s", addr, result, parent)
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

	_, result, err := cat.Match(context.Background(), provider.OutputMatchQuery{
		Provider:     fake.Name,
		ResourceType: fake.TypeWidget,
		Output:       fake.OutputToken,
		Value:        "missing",
	})
	if err != nil {
		t.Fatalf("missing Match: %v", err)
	}
	if result != provider.OutputMatchNone {
		t.Fatalf("missing result = %v, want none", result)
	}

	_, result, err = cat.Match(context.Background(), provider.OutputMatchQuery{
		Provider:     fake.Name,
		ResourceType: fake.TypeWidget,
		Output:       fake.OutputToken,
		Value:        "shared",
	})
	if err != nil {
		t.Fatalf("ambiguous Match: %v", err)
	}
	if result != provider.OutputMatchAmbiguous {
		t.Fatalf("shared result = %v, want ambiguous", result)
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

	_, result, err := cat.Match(context.Background(), provider.OutputMatchQuery{
		Provider:     fake.Name,
		ResourceType: fake.TypeWidget,
		Output:       fake.OutputSecret,
		Value:        "s3cret",
	})
	if err != nil {
		t.Fatalf("sensitive Match: %v", err)
	}
	if result != provider.OutputMatchNone {
		t.Fatalf("sensitive result = %v, want none", result)
	}

	_, result, err = cat.Match(context.Background(), provider.OutputMatchQuery{
		Provider:     fake.Name,
		ResourceType: fake.TypeWidget,
		Output:       "etag",
		Value:        "abc",
	})
	if err != nil {
		t.Fatalf("undeclared Match: %v", err)
	}
	if result != provider.OutputMatchNone {
		t.Fatalf("undeclared result = %v, want none", result)
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
