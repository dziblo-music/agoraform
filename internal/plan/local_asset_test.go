package plan_test

import (
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/asset"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestBuildLocalAssetCreateShowsRelativePathAndDigest(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widgetWithAsset(t, "hero", "hero.jpg", "abc123")
	got := mustBuild(t, []resource.Resource{res}, p)
	if got.Changes[0].Action != plan.ActionCreate {
		t.Fatalf("action = %s, want create", got.Changes[0].Action)
	}
	rendered := plan.Format(got)
	if !strings.Contains(rendered, `source.file: "hero.jpg"`) {
		t.Fatalf("format missing relative path:\n%s", rendered)
	}
	if !strings.Contains(rendered, `source.digest: "sha256:abc123"`) {
		t.Fatalf("format missing digest:\n%s", rendered)
	}
	if strings.Contains(rendered, string([]byte{0xff, 0xd8})) {
		t.Fatalf("format leaked binary data:\n%s", rendered)
	}
	assertNoMutations(t, p, 1)
}

func TestBuildLocalAssetUnchangedWhenDigestMatches(t *testing.T) {
	t.Parallel()

	p := fake.New()
	res := widgetWithAsset(t, "hero", "hero.jpg", "abc123")
	seedAsset(t, p, res, "abc123")
	got := mustBuild(t, []resource.Resource{res}, p)
	if got.HasChanges() {
		t.Fatalf("HasChanges() = true, want false: %+v", got.Changes)
	}
	assertNoMutations(t, p, 1)
}

func TestBuildLocalAssetUpdateWhenBytesChange(t *testing.T) {
	t.Parallel()

	p := fake.New()
	live := widgetWithAsset(t, "hero", "hero.jpg", "abc123")
	seedAsset(t, p, live, "abc123")
	desired := widgetWithAsset(t, "hero", "hero.jpg", "def456")
	got := mustBuild(t, []resource.Resource{desired}, p)
	if got.Changes[0].Action != plan.ActionUpdate {
		t.Fatalf("action = %s, want update", got.Changes[0].Action)
	}
	rendered := plan.Format(got)
	if !strings.Contains(rendered, "sha256:abc123") || !strings.Contains(rendered, "sha256:def456") {
		t.Fatalf("format missing digest change:\n%s", rendered)
	}
	if strings.Contains(rendered, "/tmp") || strings.Contains(strings.ToLower(rendered), `c:\`) {
		t.Fatalf("format leaked host path:\n%s", rendered)
	}
	assertNoMutations(t, p, 1)
}

func widgetWithAsset(t *testing.T, name, file, digest string) resource.Resource {
	t.Helper()
	res := widget(t, name, resource.Attributes{
		fake.AttrTitle: "Hero",
		asset.AttrName: map[string]any{asset.AttrFile: file},
	})
	local := resource.NewLocalAsset(file, digest, 4, "image/jpeg", nil)
	res.LocalAsset = &local
	return res
}

func seedAsset(t *testing.T, p *fake.Provider, res resource.Resource, digest string) {
	t.Helper()
	if err := p.Seed(resource.RemoteResource{
		Address: res.Address,
		Identity: resource.Identity{
			ID:          "id-" + res.Address.Name,
			Fingerprint: digest,
		},
		Attributes: resource.Attributes{
			fake.AttrTitle: res.Attributes[fake.AttrTitle],
			asset.AttrName: map[string]any{asset.AttrFile: res.LocalAsset.Path},
		},
		Computed: resource.Attributes{fake.AttrSerial: 1},
	}); err != nil {
		t.Fatalf("Seed: %v", err)
	}
}
