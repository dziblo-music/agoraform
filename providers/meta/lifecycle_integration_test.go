package meta_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/destroy"
	"github.com/dziblo-music/agoraform/internal/graph"
	"github.com/dziblo-music/agoraform/internal/importer"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/meta"
)

func TestLifecycleDeclaresEveryManagedRelationship(t *testing.T) {
	t.Parallel()
	want := map[string]map[string]string{
		meta.TypePixel:            {},
		meta.TypeCustomConversion: {meta.AttrPixel: meta.TypePixel},
		meta.TypeCampaign:         {},
		meta.TypeAdSet: {meta.AttrCampaign: meta.TypeCampaign, meta.AttrPixel: meta.TypePixel,
			meta.AttrCustomConversion: meta.TypeCustomConversion},
		meta.TypeAdCreative: {},
		meta.TypeAd:         {meta.AttrAdSet: meta.TypeAdSet, meta.AttrCreative: meta.TypeAdCreative},
	}
	for typ, relationships := range want {
		spec, ok := meta.Lifecycle(typ)
		if !ok {
			t.Fatalf("missing lifecycle for %s", typ)
		}
		for _, relation := range spec.Prerequisites {
			dependency, exists := relationships[relation.Attribute]
			if !exists || dependency != relation.ResourceType {
				t.Fatalf("%s has unexpected relationship %#v", typ, relation)
			}
			if relation.Output == "" || relation.ExternalAllowed {
				t.Fatalf("%s relationship %#v is not a declared-output-only managed edge", typ, relation)
			}
			if _, ok := provider.FindOutput(meta.New(meta.Config{}).Outputs(relation.ResourceType), relation.Output); !ok {
				t.Fatalf("%s relationship %#v selects an undeclared output", typ, relation)
			}
			delete(relationships, relation.Attribute)
		}
		if len(relationships) != 0 {
			t.Fatalf("%s lifecycle is missing relationships %v", typ, relationships)
		}
	}
	campaign, _ := meta.Lifecycle(meta.TypeCampaign)
	adSet, _ := meta.Lifecycle(meta.TypeAdSet)
	if campaign.BudgetOwnership == "" || adSet.BudgetOwnership == "" {
		t.Fatal("campaign/ad-set budget ownership lifecycle is not declared")
	}
}

func TestMetaImportUsesOutputCatalogForRelationships(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	seedAdDependencies(srv)
	httpSrv := srv.start()
	defer httpSrv.Close()

	p := testProvider(t, httpSrv)
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for addr, id := range map[resource.Address]string{
		campaignAddress(t, "acquisition"):     testCampaignID,
		pixelAddress(t, "website"):            testPixelID,
		conversionAddress(t, "trial_started"): testConvID,
	} {
		if err := st.Bind(addr, resource.Identity{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	var catalog provider.OutputMatcher
	lookup := func(resource.Address) (provider.Provider, error) {
		p.SetOutputMatcher(catalog)
		return p, nil
	}
	catalog = importer.NewOutputCatalog(metaStateBindings{st}, lookup)
	p.SetOutputMatcher(catalog)

	live, err := p.Import(context.Background(), adSetAddress(t, "instagram"), testAdSetID)
	if err != nil {
		t.Fatal(err)
	}
	assertRefAddress(t, live.Attributes[meta.AttrCampaign], campaignAddress(t, "acquisition"))
	assertRefAddress(t, live.Attributes[meta.AttrPixel], pixelAddress(t, "website"))
	assertRefAddress(t, live.Attributes[meta.AttrCustomConversion], conversionAddress(t, "trial_started"))
}

func TestMetaImportMissingOrAmbiguousRelationshipNeverGuesses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		matcher provider.OutputMatcher
		want    string
	}{
		{name: "missing", matcher: fixedOutputMatcher{match: provider.OutputMatchNone}, want: "not uniquely bound"},
		{name: "ambiguous", matcher: fixedOutputMatcher{match: provider.OutputMatchAmbiguous}, want: "ambiguously matches"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newGraphServer(t)
			seedAdDependencies(srv)
			srv.seedAd(testAdID, nil)
			httpSrv := srv.start()
			defer httpSrv.Close()
			p := testProvider(t, httpSrv)
			p.SetOutputMatcher(tc.matcher)
			_, err := p.Import(context.Background(), adAddress(t, "instagram"), testAdID)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Import error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestMetaDestroyGraphAndPartialFailureAreRetrySafe(t *testing.T) {
	t.Parallel()
	resources := standardAdResources(t)
	g, err := graph.Build(resources)
	if err != nil {
		t.Fatal(err)
	}
	positions := addressPositions(g.ReverseOrder())
	assertBefore := func(dependent, prerequisite resource.Address) {
		t.Helper()
		if positions[dependent.String()] >= positions[prerequisite.String()] {
			t.Fatalf("destroy order %s must precede %s: %v", dependent, prerequisite, g.ReverseOrder())
		}
	}
	assertBefore(adAddress(t, "instagram"), adSetAddress(t, "instagram"))
	assertBefore(adAddress(t, "instagram"), creativeAddress(t, "instagram"))
	assertBefore(adSetAddress(t, "instagram"), campaignAddress(t, "acquisition"))
	assertBefore(adSetAddress(t, "instagram"), conversionAddress(t, "trial_started"))
	assertBefore(conversionAddress(t, "trial_started"), pixelAddress(t, "website"))

	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range resources {
		if err := st.Bind(res.Address, resource.Identity{ID: "id-" + res.Address.Name}); err != nil {
			t.Fatal(err)
		}
	}
	p := &scriptedMetaDestroyer{Provider: meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID}), fail: campaignAddress(t, "acquisition").String()}
	lookup := func(resource.Address) (provider.Provider, error) { return p, nil }
	result, err := destroy.Run(context.Background(), resources, lookup, st, &bytes.Buffer{}, nil)
	if err == nil || result.Removed == 0 {
		t.Fatalf("first destroy = (%+v, %v), want partial success then failure", result, err)
	}
	if _, ok, _ := st.Identity(campaignAddress(t, "acquisition")); !ok {
		t.Fatal("failed campaign binding was not preserved")
	}
	for _, addr := range p.succeeded {
		if _, ok, _ := st.Identity(addr); ok {
			t.Fatalf("confirmed terminal resource %s remained bound", addr)
		}
	}

	p.fail = ""
	result, err = destroy.Run(context.Background(), resources, lookup, st, &bytes.Buffer{}, nil)
	var remaining *destroy.RemainingError
	if !errors.As(err, &remaining) || result.Remaining != 1 {
		t.Fatalf("retry = (%+v, %v), want only provider-owned pixel remaining", result, err)
	}
	if id, ok, identityErr := st.Identity(pixelAddress(t, "website")); identityErr != nil || !ok || id.ID == "" {
		t.Fatalf("provider-owned pixel binding = (%v, %v, %v)", id, ok, identityErr)
	}
}

type metaStateBindings struct{ st *state.Store }

func (b metaStateBindings) Bindings(providerName, resourceType string) ([]importer.RemoteBinding, error) {
	items, err := b.st.Bindings(providerName, resourceType)
	if err != nil {
		return nil, err
	}
	out := make([]importer.RemoteBinding, len(items))
	for i, item := range items {
		out[i] = importer.RemoteBinding{Address: item.Address, RemoteID: item.RemoteID}
	}
	return out, nil
}

type fixedOutputMatcher struct{ match provider.OutputMatch }

func (m fixedOutputMatcher) Match(context.Context, provider.OutputMatchQuery) (resource.Ref, provider.OutputMatch, error) {
	return resource.Ref{}, m.match, nil
}

type scriptedMetaDestroyer struct {
	*meta.Provider
	fail      string
	succeeded []resource.Address
}

func (p *scriptedMetaDestroyer) Destroy(_ context.Context, res resource.Resource) (provider.DestroyResult, error) {
	if res.Address.String() == p.fail {
		return provider.DestroyResult{}, errors.New("injected Meta removal failure")
	}
	spec, ok := meta.Lifecycle(res.Address.Type)
	if !ok {
		return provider.DestroyResult{}, errors.New("missing lifecycle")
	}
	p.succeeded = append(p.succeeded, res.Address)
	if spec.Destroy == provider.DestroyDelete {
		return provider.DestroyResult{Status: provider.DestroyStatusDestroyed}, nil
	}
	return provider.DestroyResult{Status: provider.DestroyStatusRemoved}, nil
}

func assertRefAddress(t *testing.T, value any, want resource.Address) {
	t.Helper()
	ref, ok := resource.AsRef(value)
	if !ok || ref.Address != want {
		t.Fatalf("reference = %#v, want %s", value, want)
	}
}
