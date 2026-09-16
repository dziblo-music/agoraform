package googleads_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/asset"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestValidateImageAssetValid(t *testing.T) {
	t.Parallel()

	p := testAssetProvider(t, nil)
	res := imageAssetResource(t, "product_image", localPNG(t, "google/product-ui.png", 128, 128))
	if err := p.Validate(context.Background(), res); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateImageAssetErrors(t *testing.T) {
	t.Parallel()

	p := testAssetProvider(t, nil)
	addr := mustAssetAddress(t, "product_image")
	ok := localPNG(t, "google/product-ui.png", 128, 128)

	cases := []struct {
		name string
		res  resource.Resource
		want string
	}{
		{
			name: "missing type",
			res:  resource.Resource{Address: addr, Attributes: resource.Attributes{asset.AttrName: map[string]any{asset.AttrFile: "pic.png"}}, LocalAsset: &ok},
			want: "missing required attribute \"type\"",
		},
		{
			name: "unsupported type",
			res:  resource.Resource{Address: addr, Attributes: resource.Attributes{googleads.AttrType: "YOUTUBE_VIDEO"}},
			want: "must be one of",
		},
		{
			name: "image missing source",
			res:  resource.Resource{Address: addr, Attributes: resource.Attributes{googleads.AttrType: "IMAGE"}},
			want: "source.file",
		},
		{
			name: "image too small",
			res: func() resource.Resource {
				small := localPNG(t, "tiny.png", 16, 16)
				return resource.Resource{
					Address:    addr,
					Attributes: resource.Attributes{googleads.AttrType: "IMAGE", asset.AttrName: map[string]any{asset.AttrFile: small.Path}},
					LocalAsset: &small,
				}
			}(),
			want: "at least 128x128",
		},
		{
			name: "image unsupported media",
			res: func() resource.Resource {
				local := resource.NewLocalAsset("notes.txt", "abc", 4, "text/plain", func() (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader("note")), nil
				})
				return resource.Resource{
					Address:    addr,
					Attributes: resource.Attributes{googleads.AttrType: "IMAGE", asset.AttrName: map[string]any{asset.AttrFile: local.Path}},
					LocalAsset: &local,
				}
			}(),
			want: "JPEG, PNG, or GIF",
		},
		{
			name: "image too large",
			res: func() resource.Resource {
				local := resource.NewLocalAsset("large.png", "sha256:large", 5_120_001, "image/png", func() (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader("")), nil
				})
				return resource.Resource{
					Address:    addr,
					Attributes: resource.Attributes{googleads.AttrType: "IMAGE", asset.AttrName: map[string]any{asset.AttrFile: local.Path}},
					LocalAsset: &local,
				}
			}(),
			want: "5,120 KB",
		},
		{
			name: "computed id",
			res: resource.Resource{
				Address:    addr,
				Attributes: resource.Attributes{googleads.AttrType: "IMAGE", "id": "81", asset.AttrName: map[string]any{asset.AttrFile: ok.Path}},
				LocalAsset: &ok,
			},
			want: "computed",
		},
		{
			name: "text on image",
			res: resource.Resource{
				Address: addr,
				Attributes: resource.Attributes{
					googleads.AttrType: "IMAGE",
					googleads.AttrText: "Acme",
					asset.AttrName:     map[string]any{asset.AttrFile: ok.Path},
				},
				LocalAsset: &ok,
			},
			want: "only valid for TEXT",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := p.Validate(context.Background(), tc.res)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
			assertNoProviderSecret(t, err.Error())
			if strings.Contains(err.Error(), string([]byte{0x89, 0x50, 0x4e, 0x47})) {
				t.Fatalf("error leaked PNG bytes: %v", err)
			}
		})
	}
}

func TestValidateTextAsset(t *testing.T) {
	t.Parallel()

	p := testAssetProvider(t, nil)
	if err := p.Validate(context.Background(), textAssetResource(t, "brand", "Acme Inc")); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	tooLong := strings.Repeat("A", 26)
	err := p.Validate(context.Background(), textAssetResource(t, "brand", tooLong))
	if err == nil || !strings.Contains(err.Error(), "at most 25") {
		t.Fatalf("Validate = %v, want 25-character limit", err)
	}

	local := localPNG(t, "logo.png", 128, 128)
	err = p.Validate(context.Background(), resource.Resource{
		Address: mustAssetAddress(t, "brand"),
		Attributes: resource.Attributes{
			googleads.AttrType: "TEXT",
			googleads.AttrText: "Acme Inc",
			asset.AttrName:     map[string]any{asset.AttrFile: local.Path},
		},
		LocalAsset: &local,
	})
	if err == nil || !strings.Contains(err.Error(), "source") {
		t.Fatalf("Validate = %v, want source rejection", err)
	}
}

func TestCreateImageAssetUploadsOnce(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	p := testAssetProvider(t, fake)
	local := localPNG(t, "google/product-ui.png", 128, 128)
	res := imageAssetResource(t, "product_image", local)

	created, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Identity.ID == "" {
		t.Fatal("missing asset id")
	}
	if created.Identity.Fingerprint != local.Digest {
		t.Fatalf("fingerprint = %q, want digest %q", created.Identity.Fingerprint, local.Digest)
	}
	if _, ok := created.Attributes[asset.AttrName]; ok {
		t.Fatal("source path must not be stored in comparable attributes")
	}
	if strings.Contains(fake.lastMutateBody(), local.Path) && strings.Contains(strings.ToLower(fake.lastMutateBody()), `c:\`) {
		t.Fatal("mutate leaked host path")
	}
	if fake.uploadCount() != 1 {
		t.Fatalf("uploads = %d, want 1", fake.uploadCount())
	}

	res.Identity = created.Identity
	live, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Identity.Fingerprint != local.Digest {
		t.Fatalf("read fingerprint = %q", live.Identity.Fingerprint)
	}

	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(res.Address, created.Identity); err != nil {
		t.Fatal(err)
	}
	got, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("unchanged image produced plan changes: %+v", got.Changes)
	}
	if fake.uploadCount() != 1 {
		t.Fatalf("unchanged apply re-uploaded: uploads = %d", fake.uploadCount())
	}
	rendered := plan.Format(got)
	if strings.Contains(rendered, string([]byte{0x89, 0x50, 0x4e, 0x47})) {
		t.Fatalf("plan leaked PNG bytes:\n%s", rendered)
	}
}

func TestPlanImageAssetCreateShowsPathAndDigest(t *testing.T) {
	t.Parallel()

	p := testAssetProvider(t, newAssetFake())
	local := localPNG(t, "google/product-ui.png", 128, 128)
	res := imageAssetResource(t, "product_image", local)
	got, err := plan.Build(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionCreate {
		t.Fatalf("changes = %+v, want create", got.Changes)
	}
	rendered := plan.Format(got)
	if !strings.Contains(rendered, `source.file: "google/product-ui.png"`) {
		t.Fatalf("plan missing relative path:\n%s", rendered)
	}
	if !strings.Contains(rendered, "source.digest:") || !strings.Contains(rendered, "sha256:") {
		t.Fatalf("plan missing digest:\n%s", rendered)
	}
	if strings.Contains(rendered, string([]byte{0x89, 0x50})) {
		t.Fatalf("plan leaked binary:\n%s", rendered)
	}
}

func TestPlanImageAssetChangedContentFails(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedAsset(sampleImageAsset("81", "product_image"))
	p := testAssetProvider(t, fake)
	local := localPNG(t, "google/product-ui.png", 128, 128)
	res := imageAssetResource(t, "product_image", local)
	res.Identity = resource.Identity{ID: "81", Fingerprint: "olddigest"}

	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(res.Address, res.Identity); err != nil {
		t.Fatal(err)
	}
	_, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("plan = %v, want immutable content guidance", err)
	}
	if fake.uploadCount() != 0 {
		t.Fatalf("changed content uploaded: %d", fake.uploadCount())
	}
}

func TestUpdateImageAssetRefusesContentChange(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedAsset(sampleImageAsset("81", "product_image"))
	p := testAssetProvider(t, fake)
	local := localPNG(t, "google/product-ui.png", 128, 128)
	desired := imageAssetResource(t, "product_image", local)
	desired.Identity = resource.Identity{ID: "81", Fingerprint: "olddigest"}
	_, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: desired.Identity,
		Attributes: resource.Attributes{
			googleads.AttrType: "IMAGE",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("Update = %v, want immutable guidance", err)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestPlanImageAssetTreatsNameAsCreateTimeOnly(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedAsset(sampleImageAsset("81", "Existing Google Name"))
	p := testAssetProvider(t, fake)
	local := localPNG(t, "google/product-ui.png", 128, 128)
	res := imageAssetResource(t, "product_image", local)
	res.Attributes[googleads.AttrName] = "Requested Create Name"
	res.Identity = resource.Identity{ID: "81", Fingerprint: local.Digest}

	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(res.Address, res.Identity); err != nil {
		t.Fatal(err)
	}
	got, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("create-time asset name produced drift: %+v", got.Changes)
	}
}

func TestCreateAndReadTextAsset(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	p := testAssetProvider(t, fake)
	res := textAssetResource(t, "business_name", "Acme Inc")
	created, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Attributes[googleads.AttrText] != "Acme Inc" {
		t.Fatalf("text = %v", created.Attributes[googleads.AttrText])
	}
	if created.Attributes[googleads.AttrType] != "TEXT" {
		t.Fatalf("type = %v", created.Attributes[googleads.AttrType])
	}

	res.Identity = created.Identity
	live, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Attributes[googleads.AttrText] != "Acme Inc" {
		t.Fatalf("read text = %v", live.Attributes[googleads.AttrText])
	}

	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(res.Address, created.Identity); err != nil {
		t.Fatal(err)
	}
	got, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("unchanged text produced changes: %+v", got.Changes)
	}
}

func TestUpdateTextAssetRefusesContentChange(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedAsset(sampleTextAsset("82", "Acme Inc"))
	p := testAssetProvider(t, fake)
	desired := textAssetResource(t, "business_name", "New Name")
	desired.Identity = resource.Identity{ID: "82"}
	_, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:    desired.Address,
		Identity:   desired.Identity,
		Attributes: resource.Attributes{googleads.AttrType: "TEXT", googleads.AttrText: "Acme Inc"},
	})
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("Update = %v, want immutable text guidance", err)
	}
}

func TestImportImageAssetDoesNotFabricateSource(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedAsset(sampleImageAsset("81", "Hero"))
	p := testAssetProvider(t, fake)
	live, err := p.Import(context.Background(), mustAssetAddress(t, "product_image"), "81")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if live.Identity.ID != "81" {
		t.Fatalf("id = %q", live.Identity.ID)
	}
	if live.Attributes[googleads.AttrType] != "IMAGE" {
		t.Fatalf("type = %v", live.Attributes[googleads.AttrType])
	}
	if live.Attributes[googleads.AttrName] != "Hero" {
		t.Fatalf("name = %v", live.Attributes[googleads.AttrName])
	}
	if _, ok := live.Attributes[asset.AttrName]; ok {
		t.Fatal("import fabricated a local source path")
	}
}

func TestNormalizeAssetImportID(t *testing.T) {
	t.Parallel()

	p := testAssetProvider(t, nil)
	got, err := p.NormalizeImportID(mustAssetAddress(t, "product_image"), "customers/"+testCustomerID+"/assets/81")
	if err != nil {
		t.Fatalf("NormalizeImportID: %v", err)
	}
	if got != "81" {
		t.Fatalf("id = %q, want 81", got)
	}
}

func TestImportAssetRejectsUnsupportedType(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedAsset(map[string]any{"id": "99", "type": "YOUTUBE_VIDEO", "name": "Demo"})
	p := testAssetProvider(t, fake)
	_, err := p.Import(context.Background(), mustAssetAddress(t, "features"), "99")
	if err == nil {
		t.Fatal("expected unsupported type")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported type must not look like not found")
	}
	if !strings.Contains(err.Error(), "IMAGE") || !strings.Contains(err.Error(), "TEXT") {
		t.Fatalf("error = %q, want IMAGE/TEXT guidance", err)
	}
}

func TestImportAssetAuthIsNotNotFound(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedAsset(sampleImageAsset("81", "Hero"))
	fake.searchStatus = http.StatusForbidden
	p := testAssetProvider(t, fake)
	_, err := p.Import(context.Background(), mustAssetAddress(t, "product_image"), "81")
	if err == nil {
		t.Fatal("expected auth error")
	}
	assertNoProviderSecret(t, err.Error())
	if errors.Is(err, provider.ErrNotFound) || strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Fatalf("auth treated as not found: %v", err)
	}
}

func TestCreateImageAssetMutateErrorRedactsSecrets(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.mutateStatus = http.StatusBadRequest
	p := testAssetProvider(t, fake)
	_, err := p.Create(context.Background(), imageAssetResource(t, "product_image", localPNG(t, "pic.png", 128, 128)))
	if err == nil {
		t.Fatal("expected mutate error")
	}
	assertNoProviderSecret(t, err.Error())
	if strings.Contains(err.Error(), string([]byte{0x89, 0x50, 0x4e, 0x47})) {
		t.Fatalf("error leaked PNG bytes: %v", err)
	}
}

func TestPlanImageAssetIgnoresPolicyComputedFields(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	item := sampleImageAsset("81", "product_image")
	item["policySummary"] = map[string]any{"approvalStatus": "APPROVED", "reviewStatus": "REVIEWED"}
	item["source"] = "ADVERTISER"
	fake.seedAsset(item)
	p := testAssetProvider(t, fake)
	local := localPNG(t, "google/product-ui.png", 128, 128)
	res := imageAssetResource(t, "product_image", local)
	res.Identity = resource.Identity{ID: "81", Fingerprint: local.Digest}

	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(res.Address, res.Identity); err != nil {
		t.Fatal(err)
	}
	got, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("policy metadata produced drift: %+v", got.Changes)
	}
}

func TestAutomaticallyCreatedAssetRefusesUpdate(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	item := sampleImageAsset("81", "Auto")
	item["source"] = "AUTOMATICALLY_CREATED"
	fake.seedAsset(item)
	p := testAssetProvider(t, fake)
	local := localPNG(t, "pic.png", 128, 128)
	desired := imageAssetResource(t, "product_image", local)
	desired.Identity = resource.Identity{ID: "81", Fingerprint: local.Digest}
	_, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:    desired.Address,
		Identity:   desired.Identity,
		Attributes: resource.Attributes{googleads.AttrType: "IMAGE"},
		Computed:   resource.Attributes{"source": "AUTOMATICALLY_CREATED"},
	})
	if err == nil || !strings.Contains(err.Error(), "AUTOMATICALLY_CREATED") {
		t.Fatalf("Update = %v, want provider-generated refusal", err)
	}
}
