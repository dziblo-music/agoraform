package meta_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/importer"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/meta"
)

// TestEveryResourceTypeRejectsUnknownAttributes proves that no Meta resource
// type silently drops an attribute it does not support. Every registered type
// needs a valid fixture here, so adding a type without an unknown-attribute
// guard fails this test.
func TestEveryResourceTypeRejectsUnknownAttributes(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	valid := map[string]resource.Resource{
		meta.TypeImage:            imageResource(t, "trial_ad"),
		meta.TypePixel:            pixelResource(t, "website"),
		meta.TypeCustomConversion: conversionResource(t, "trial_started", websiteConversionAttrs(t)),
		meta.TypeCampaign:         campaignResource(t, "acquisition", standardCampaignAttrs()),
		meta.TypeAdSet:            adSetResource(t, "instagram", standardAdSetAttrs(t)),
		meta.TypeAdCreative:       creativeResource(t, "instagram", standardImageCreativeAttrs()),
		meta.TypeAd:               adResource(t, "instagram", standardAdAttrs(t)),
	}
	types := p.ResourceTypes()
	if len(valid) != len(types) {
		t.Fatalf("fixtures = %d, registered types = %d", len(valid), len(types))
	}
	for _, typ := range types {
		res, ok := valid[typ]
		if !ok {
			t.Fatalf("meta.%s has no unknown-attribute fixture", typ)
		}
		t.Run(typ, func(t *testing.T) {
			if err := p.Validate(context.Background(), res); err != nil {
				t.Fatalf("fixture is not valid: %v", err)
			}
			attrs := res.Attributes.Clone()
			attrs["unsupportedField"] = "value"
			err := p.Validate(context.Background(), resource.Resource{Address: res.Address, Attributes: attrs})
			if err == nil || !strings.Contains(err.Error(), "unsupported attribute") {
				t.Fatalf("error = %v, want an unsupported attribute rejection", err)
			}
			if !strings.Contains(err.Error(), "unsupportedField") {
				t.Fatalf("error does not name the rejected attribute: %v", err)
			}
		})
	}
}

// TestPlanNeverMutatesMeta covers the create, no-op, and update plan paths for
// the complete serving graph. The existing per-resource mutation assertions
// only cover plans that fail on an immutable field, which never reach the
// provider write paths in the first place.
func TestPlanNeverMutatesMeta(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedPixel(testPixelID, "Website")
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	reader := func(resource.Address) (provider.Reader, error) { return p, nil }

	created, err := plan.Build(context.Background(), standardAdResources(t), reader)
	if err != nil {
		t.Fatal(err)
	}
	if !created.HasChanges() {
		t.Fatal("create plan reported no changes; the mutation assertion below would be vacuous")
	}
	assertNoMutations(t, srv, "create plan")

	seedAdDependencies(srv)
	srv.seedAd(testAdID, nil)
	st := boundGraphState(t)

	if _, err := plan.BuildWithState(context.Background(), standardAdResources(t), reader, st); err != nil {
		t.Fatal(err)
	}
	assertNoMutations(t, srv, "converged plan")

	serving := standardAdResources(t)
	for i := range serving {
		if serving[i].Address.Type == meta.TypeCampaign {
			serving[i].Attributes[meta.AttrStatus] = "ACTIVE"
		}
	}
	activated, err := plan.BuildWithState(context.Background(), serving, reader, st)
	if err != nil {
		t.Fatal(err)
	}
	if !activated.HasChanges() {
		t.Fatal("activating the campaign produced no planned change")
	}
	assertNoMutations(t, srv, "update plan")
}

// TestAccessTokenNeverAppearsInUserVisibleOutput covers the reviewable
// artifacts a user reads or commits. Token redaction in returned errors and
// diagnostics is asserted per resource elsewhere; the Meta provider writes no
// logs of its own, so plan output, import YAML, and the state file are the
// remaining surfaces.
func TestAccessTokenNeverAppearsInUserVisibleOutput(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	seedAdDependencies(srv)
	srv.seedAd(testAdID, nil)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	dir := t.TempDir()
	st, err := state.Load(filepath.Join(dir, "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for addr, id := range graphBindings(t) {
		if err := st.Bind(addr, resource.Identity{ID: id}); err != nil {
			t.Fatal(err)
		}
	}

	// Plan an update so the rendered output carries real attribute values
	// rather than a bare "No changes." line.
	serving := standardAdResources(t)
	for i := range serving {
		if serving[i].Address.Type == meta.TypeCampaign {
			serving[i].Attributes[meta.AttrStatus] = "ACTIVE"
		}
	}
	got, err := plan.BuildWithState(context.Background(), serving,
		func(resource.Address) (provider.Reader, error) { return p, nil }, st)
	if err != nil {
		t.Fatal(err)
	}
	rendered := plan.Format(got)
	if !strings.Contains(rendered, "meta.campaign.acquisition") {
		t.Fatalf("plan output is not substantive:\n%s", rendered)
	}
	requireNoToken(t, "plan output", rendered)

	// Import needs an unbound address, so it runs against its own store.
	adopting, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	imported, err := importer.Run(context.Background(), campaignAddress(t, "acquisition"), testCampaignID,
		func(resource.Address) (provider.Provider, error) { return p, nil }, adopting)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(imported.YAML, "meta.campaign.acquisition") {
		t.Fatalf("import YAML is not substantive:\n%s", imported.YAML)
	}
	requireNoToken(t, "import YAML", imported.YAML)

	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(st.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), testAdID) {
		t.Fatalf("state file is not substantive:\n%s", data)
	}
	requireNoToken(t, "state file", string(data))
}

func assertNoMutations(t *testing.T, srv *graphServer, stage string) {
	t.Helper()
	posts, deletes := srv.mutationCounts()
	if posts != 0 || deletes != 0 {
		t.Fatalf("%s mutated Meta: posts=%d deletes=%d", stage, posts, deletes)
	}
}

func requireNoToken(t *testing.T, label, value string) {
	t.Helper()
	if strings.Contains(value, testToken) {
		t.Fatalf("access token leaked in %s:\n%s", label, value)
	}
}

func graphBindings(t *testing.T) map[resource.Address]string {
	t.Helper()
	return map[resource.Address]string{
		pixelAddress(t, "website"):            testPixelID,
		conversionAddress(t, "trial_started"): testConvID,
		campaignAddress(t, "acquisition"):     testCampaignID,
		adSetAddress(t, "instagram"):          testAdSetID,
		creativeAddress(t, "instagram"):       testCreativeID,
		adAddress(t, "instagram"):             testAdID,
	}
}

func boundGraphState(t *testing.T) *state.Store {
	t.Helper()
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for addr, id := range graphBindings(t) {
		if err := st.Bind(addr, resource.Identity{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	return st
}
