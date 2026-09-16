package googleads_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestSitelinkDescriptionRemovalReconciles(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	item := sampleSitelinkAsset("91", "Features", []any{"https://example.com/features"})
	item["sitelinkAsset"] = map[string]any{
		"linkText":     "Features",
		"description1": "See product features",
		"description2": "Built for growing teams",
	}
	fake.seedAsset(item)
	p := testAssetProvider(t, fake)

	desired := sitelinkAssetResource(t, "features", "Features", []any{"https://example.com/features"})
	desired.Identity = resource.Identity{ID: "91"}

	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(desired.Address, desired.Identity); err != nil {
		t.Fatal(err)
	}

	before, err := plan.BuildWithState(context.Background(), []resource.Resource{desired}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("plan before removal: %v", err)
	}
	if !before.HasChanges() {
		t.Fatal("expected plan to remove existing sitelink descriptions")
	}

	actual, err := p.Read(context.Background(), desired)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if actual.Attributes[googleads.AttrDescription1] != "See product features" || actual.Attributes[googleads.AttrDescription2] != "Built for growing teams" {
		t.Fatalf("read descriptions = %q / %q", actual.Attributes[googleads.AttrDescription1], actual.Attributes[googleads.AttrDescription2])
	}

	updated, err := p.Update(context.Background(), desired, actual)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got, ok := updated.Attributes[googleads.AttrDescription1]; ok && got != "" {
		t.Fatalf("updated description1 = %v, want empty/omitted", got)
	}
	if got, ok := updated.Attributes[googleads.AttrDescription2]; ok && got != "" {
		t.Fatalf("updated description2 = %v, want empty/omitted", got)
	}

	body := fake.lastMutateBody()
	for _, want := range []string{"sitelinkAsset.description1", "sitelinkAsset.description2"} {
		if !strings.Contains(body, want) {
			t.Fatalf("update missing %s in update mask: %s", want, body)
		}
	}

	after, err := plan.BuildWithState(context.Background(), []resource.Resource{desired}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("plan after removal: %v", err)
	}
	if after.HasChanges() {
		t.Fatalf("description removal did not converge: %+v", after.Changes)
	}
}
