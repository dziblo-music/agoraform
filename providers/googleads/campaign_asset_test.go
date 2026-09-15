package googleads_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestValidateCampaignAssetValid(t *testing.T) {
	t.Parallel()

	p := testAssetProvider(t, nil)
	if err := p.Validate(context.Background(), campaignAssetResource(t, "product_image", defaultCampaignAssetAttrs(t))); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateCampaignAssetErrors(t *testing.T) {
	t.Parallel()

	p := testAssetProvider(t, nil)
	addr := mustCampaignAssetAddress(t, "product_image")
	campaign := campaignRef(t, "brand")
	asset := assetRef(t, "product_image")

	cases := []struct {
		name  string
		attrs resource.Attributes
		want  string
	}{
		{
			name:  "missing campaign",
			attrs: resource.Attributes{googleads.AttrAsset: asset, googleads.AttrFieldType: "AD_IMAGE"},
			want:  "missing required attribute \"campaign\"",
		},
		{
			name:  "missing asset",
			attrs: resource.Attributes{googleads.AttrCampaign: campaign, googleads.AttrFieldType: "AD_IMAGE"},
			want:  "missing required attribute \"asset\"",
		},
		{
			name:  "missing field type",
			attrs: resource.Attributes{googleads.AttrCampaign: campaign, googleads.AttrAsset: asset},
			want:  "missing required attribute \"fieldType\"",
		},
		{
			name:  "image alias rejected",
			attrs: resource.Attributes{googleads.AttrCampaign: campaign, googleads.AttrAsset: asset, googleads.AttrFieldType: "IMAGE"},
			want:  "AD_IMAGE",
		},
		{
			name:  "campaign not a ref",
			attrs: resource.Attributes{googleads.AttrCampaign: "customers/" + testCustomerID + "/campaigns/21", googleads.AttrAsset: asset, googleads.AttrFieldType: "AD_IMAGE"},
			want:  "$ref",
		},
		{
			name:  "asset wrong type",
			attrs: resource.Attributes{googleads.AttrCampaign: campaign, googleads.AttrAsset: campaignRef(t, "other"), googleads.AttrFieldType: "AD_IMAGE"},
			want:  "googleads.asset",
		},
		{
			name:  "computed resourceName",
			attrs: resource.Attributes{googleads.AttrCampaign: campaign, googleads.AttrAsset: asset, googleads.AttrFieldType: "AD_IMAGE", "resourceName": "x"},
			want:  "computed",
		},
		{
			name:  "removed status",
			attrs: resource.Attributes{googleads.AttrCampaign: campaign, googleads.AttrAsset: asset, googleads.AttrFieldType: "AD_IMAGE", googleads.AttrStatus: "REMOVED"},
			want:  "must be one of",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := p.Validate(context.Background(), resource.Resource{Address: addr, Attributes: tc.attrs})
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
			assertNoProviderSecret(t, err.Error())
		})
	}
}

func TestValidateResourceSetCampaignAssetTypeMustMatch(t *testing.T) {
	t.Parallel()

	p := googleads.New(googleads.Config{})
	image := textAssetResource(t, "product_image", "Acme Inc")
	link := campaignAssetResource(t, "product_image", defaultCampaignAssetAttrs(t))
	err := p.ValidateResourceSet(context.Background(), []resource.Resource{image, link})
	if err == nil || !strings.Contains(err.Error(), "AD_IMAGE") || !strings.Contains(err.Error(), "IMAGE") {
		t.Fatalf("ValidateResourceSet = %v, want field type / asset type mismatch", err)
	}
}

func TestValidateResourceSetCampaignAssetDuplicate(t *testing.T) {
	t.Parallel()

	p := googleads.New(googleads.Config{})
	img := imageAssetResource(t, "product_image", localPNG(t, "pic.png", 128, 128))
	first := campaignAssetResource(t, "one", defaultCampaignAssetAttrs(t))
	second := campaignAssetResource(t, "two", defaultCampaignAssetAttrs(t))
	err := p.ValidateResourceSet(context.Background(), []resource.Resource{img, first, second})
	if err == nil || !strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("ValidateResourceSet = %v, want duplicate attachment", err)
	}
}

func TestCreateAndReadCampaignAsset(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedAsset(sampleImageAsset("81", "product_image"))
	p := testAssetProvider(t, fake)
	bindCampaignIdentity(t, p, "21")
	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustAssetAddress(t, "product_image"), resource.Identity{ID: "81"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	res := campaignAssetResource(t, "product_image", resource.Attributes{
		googleads.AttrCampaign:  resolvedCampaign(t, "brand", "21"),
		googleads.AttrAsset:     resolvedAsset(t, "product_image", "81"),
		googleads.AttrFieldType: "AD_IMAGE",
	})
	created, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Identity.ID != "21~81~AD_IMAGE" {
		t.Fatalf("identity = %q", created.Identity.ID)
	}
	ref, ok := resource.AsRef(created.Attributes[googleads.AttrCampaign])
	if !ok || ref.Address != mustCampaignAddress(t, "brand") {
		t.Fatalf("campaign = %#v, want logical $ref", created.Attributes[googleads.AttrCampaign])
	}
	asset, ok := resource.AsRef(created.Attributes[googleads.AttrAsset])
	if !ok || asset.Address != mustAssetAddress(t, "product_image") {
		t.Fatalf("asset = %#v, want logical $ref", created.Attributes[googleads.AttrAsset])
	}
	if _, ok := created.Attributes["resourceName"]; ok {
		t.Fatal("resourceName must stay computed")
	}

	local := localPNG(t, "google/product-ui.png", 128, 128)
	imageRes := imageAssetResource(t, "product_image", local)
	imageRes.Identity = resource.Identity{ID: "81", Fingerprint: local.Digest}
	if err := st.Bind(imageRes.Address, imageRes.Identity); err != nil {
		t.Fatal(err)
	}
	res.Identity = created.Identity
	if err := st.Bind(res.Address, created.Identity); err != nil {
		t.Fatal(err)
	}
	got, err := plan.BuildWithState(context.Background(), []resource.Resource{imageRes, res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("unchanged campaign asset produced changes: %+v", got.Changes)
	}
}

func TestCreateBusinessLogoAndNameAttachments(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedAsset(sampleImageAsset("81", "logo"))
	fake.seedAsset(sampleTextAsset("82", "Acme Inc"))
	p := testAssetProvider(t, fake)
	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustAssetAddress(t, "logo"), resource.Identity{ID: "81"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustAssetAddress(t, "business_name"), resource.Identity{ID: "82"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	logo := campaignAssetResource(t, "logo", resource.Attributes{
		googleads.AttrCampaign:  resolvedCampaign(t, "brand", "21"),
		googleads.AttrAsset:     resolvedAsset(t, "logo", "81"),
		googleads.AttrFieldType: "BUSINESS_LOGO",
	})
	created, err := p.Create(context.Background(), logo)
	if err != nil {
		t.Fatalf("logo Create: %v", err)
	}
	if created.Identity.ID != "21~81~BUSINESS_LOGO" {
		t.Fatalf("logo identity = %q", created.Identity.ID)
	}

	name := campaignAssetResource(t, "business_name", resource.Attributes{
		googleads.AttrCampaign:  resolvedCampaign(t, "brand", "21"),
		googleads.AttrAsset:     resolvedAsset(t, "business_name", "82"),
		googleads.AttrFieldType: "BUSINESS_NAME",
	})
	createdName, err := p.Create(context.Background(), name)
	if err != nil {
		t.Fatalf("business name Create: %v", err)
	}
	if createdName.Identity.ID != "21~82~BUSINESS_NAME" {
		t.Fatalf("name identity = %q", createdName.Identity.ID)
	}
}

func TestCreateCampaignAssetSurfacesEligibilityError(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.mutateStatus = 400
	p := testAssetProvider(t, fake)
	_, err := p.Create(context.Background(), campaignAssetResource(t, "product_image", resource.Attributes{
		googleads.AttrCampaign:  resolvedCampaign(t, "brand", "21"),
		googleads.AttrAsset:     resolvedAsset(t, "product_image", "81"),
		googleads.AttrFieldType: "BUSINESS_LOGO",
	}))
	if err == nil {
		t.Fatal("expected provider error")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestPlanCampaignAssetCreateWhenMissing(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedAsset(sampleImageAsset("81", "product_image"))
	p := testAssetProvider(t, fake)
	local := localPNG(t, "pic.png", 128, 128)
	imageRes := imageAssetResource(t, "product_image", local)
	imageRes.Identity = resource.Identity{ID: "81", Fingerprint: local.Digest}
	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(imageRes.Address, imageRes.Identity); err != nil {
		t.Fatal(err)
	}
	res := campaignAssetResource(t, "product_image", resource.Attributes{
		googleads.AttrCampaign:  resolvedCampaign(t, "brand", "21"),
		googleads.AttrAsset:     resolvedAsset(t, "product_image", "81"),
		googleads.AttrFieldType: "AD_IMAGE",
	})
	got, err := plan.BuildWithState(context.Background(), []resource.Resource{imageRes, res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	byAddr := map[string]plan.Action{}
	for _, change := range got.Changes {
		byAddr[change.Address.String()] = change.Action
	}
	if byAddr["googleads.campaign_asset.product_image"] != plan.ActionCreate {
		t.Fatalf("changes = %+v, want campaign asset create", got.Changes)
	}
}

func TestPlanCampaignAssetImmutableAssetIsVisible(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedCampaignAsset(map[string]any{
		"campaign":  "customers/" + testCustomerID + "/campaigns/21",
		"asset":     "customers/" + testCustomerID + "/assets/81",
		"fieldType": "AD_IMAGE",
		"status":    "ENABLED",
	})
	fake.seedAsset(sampleImageAsset("81", "product_image"))
	fake.seedAsset(sampleImageAsset("99", "other"))
	p := testAssetProvider(t, fake)
	local := localPNG(t, "pic.png", 128, 128)
	other := imageAssetResource(t, "other", local)
	other.Identity = resource.Identity{ID: "99", Fingerprint: local.Digest}
	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustAssetAddress(t, "product_image"), resource.Identity{ID: "81"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(other.Address, other.Identity); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustCampaignAssetAddress(t, "product_image"), resource.Identity{ID: "21~81~AD_IMAGE"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	res := campaignAssetResource(t, "product_image", resource.Attributes{
		googleads.AttrCampaign:  resolvedCampaign(t, "brand", "21"),
		googleads.AttrAsset:     resolvedAsset(t, "other", "99"),
		googleads.AttrFieldType: "AD_IMAGE",
	})
	res.Identity = resource.Identity{ID: "21~81~AD_IMAGE"}
	_, err := plan.BuildWithState(context.Background(), []resource.Resource{other, res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err == nil || (!strings.Contains(err.Error(), "immutable") && !strings.Contains(err.Error(), "does not match")) {
		t.Fatalf("plan = %v, want immutable asset guidance", err)
	}
}

func TestImportCampaignAssetReconstructsRefs(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedCampaignAsset(map[string]any{
		"campaign":  "customers/" + testCustomerID + "/campaigns/21",
		"asset":     "customers/" + testCustomerID + "/assets/81",
		"fieldType": "AD_IMAGE",
		"status":    "ENABLED",
	})
	p := testAssetProvider(t, fake)
	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustAssetAddress(t, "product_image"), resource.Identity{ID: "81"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	live, err := p.Import(context.Background(), mustCampaignAssetAddress(t, "product_image"), "21~81~AD_IMAGE")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	ref, ok := resource.AsRef(live.Attributes[googleads.AttrCampaign])
	if !ok || ref.Address != mustCampaignAddress(t, "brand") {
		t.Fatalf("campaign = %#v", live.Attributes[googleads.AttrCampaign])
	}
	asset, ok := resource.AsRef(live.Attributes[googleads.AttrAsset])
	if !ok || asset.Address != mustAssetAddress(t, "product_image") {
		t.Fatalf("asset = %#v", live.Attributes[googleads.AttrAsset])
	}
}

func TestImportCampaignAssetRequiresBoundParents(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedCampaignAsset(map[string]any{
		"campaign":  "customers/" + testCustomerID + "/campaigns/21",
		"asset":     "customers/" + testCustomerID + "/assets/81",
		"fieldType": "AD_IMAGE",
		"status":    "ENABLED",
	})
	p := testAssetProvider(t, fake)
	_, err := p.Import(context.Background(), mustCampaignAssetAddress(t, "product_image"), "customers/"+testCustomerID+"/campaignAssets/21~81~AD_IMAGE")
	if err == nil || !strings.Contains(err.Error(), "campaign is not bound") {
		t.Fatalf("Import = %v, want unbound campaign guidance", err)
	}
}

func TestImportCampaignAssetRejectsUnsupportedFieldType(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedCampaignAsset(map[string]any{
		"campaign":  "customers/" + testCustomerID + "/campaigns/21",
		"asset":     "customers/" + testCustomerID + "/assets/81",
		"fieldType": "SITELINK",
		"status":    "ENABLED",
	})
	p := testAssetProvider(t, fake)
	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustAssetAddress(t, "product_image"), resource.Identity{ID: "81"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)
	_, err := p.Import(context.Background(), mustCampaignAssetAddress(t, "product_image"), "21~81~SITELINK")
	if err == nil {
		t.Fatal("expected unsupported field type")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported field type must not look like not found")
	}
}

func TestNormalizeCampaignAssetImportID(t *testing.T) {
	t.Parallel()

	p := testAssetProvider(t, nil)
	got, err := p.NormalizeImportID(mustCampaignAssetAddress(t, "product_image"), "customers/"+testCustomerID+"/campaignAssets/21~81~AD_IMAGE")
	if err != nil {
		t.Fatalf("NormalizeImportID: %v", err)
	}
	if got != "21~81~AD_IMAGE" {
		t.Fatalf("id = %q", got)
	}
}

func TestUpdateCampaignAssetStatus(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedCampaignAsset(map[string]any{
		"campaign":  "customers/" + testCustomerID + "/campaigns/21",
		"asset":     "customers/" + testCustomerID + "/assets/81",
		"fieldType": "AD_IMAGE",
		"status":    "ENABLED",
	})
	p := testAssetProvider(t, fake)
	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustAssetAddress(t, "product_image"), resource.Identity{ID: "81"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	desired := campaignAssetResource(t, "product_image", resource.Attributes{
		googleads.AttrCampaign:  resolvedCampaign(t, "brand", "21"),
		googleads.AttrAsset:     resolvedAsset(t, "product_image", "81"),
		googleads.AttrFieldType: "AD_IMAGE",
		googleads.AttrStatus:    "PAUSED",
	})
	desired.Identity = resource.Identity{ID: "21~81~AD_IMAGE"}
	live, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: desired.Identity,
		Attributes: resource.Attributes{
			googleads.AttrCampaign:  campaignRef(t, "brand"),
			googleads.AttrAsset:     assetRef(t, "product_image"),
			googleads.AttrFieldType: "AD_IMAGE",
			googleads.AttrStatus:    "ENABLED",
		},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if live.Attributes[googleads.AttrStatus] != "PAUSED" {
		t.Fatalf("status = %v, want PAUSED", live.Attributes[googleads.AttrStatus])
	}
}

func TestDestroyCampaignAssetAndPreserveAsset(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedAsset(sampleImageAsset("81", "product_image"))
	fake.seedCampaignAsset(map[string]any{
		"campaign":  "customers/" + testCustomerID + "/campaigns/21",
		"asset":     "customers/" + testCustomerID + "/assets/81",
		"fieldType": "AD_IMAGE",
		"status":    "ENABLED",
	})
	p := testAssetProvider(t, fake)

	assetRes := imageAssetResource(t, "product_image", localPNG(t, "pic.png", 128, 128))
	assetRes.Identity = resource.Identity{ID: "81", Fingerprint: "abc"}
	capability, err := p.DestroyCapability(assetRes)
	if err != nil {
		t.Fatalf("DestroyCapability asset: %v", err)
	}
	if capability != provider.DestroyUnsupported {
		t.Fatalf("asset capability = %q, want unsupported", capability)
	}

	link := campaignAssetResource(t, "product_image", resource.Attributes{
		googleads.AttrCampaign:  resolvedCampaign(t, "brand", "21"),
		googleads.AttrAsset:     resolvedAsset(t, "product_image", "81"),
		googleads.AttrFieldType: "AD_IMAGE",
	})
	link.Identity = resource.Identity{ID: "21~81~AD_IMAGE"}
	result, err := p.Destroy(context.Background(), link)
	if err != nil {
		t.Fatalf("Destroy campaign asset: %v", err)
	}
	if result.Status != provider.DestroyStatusRemoved {
		t.Fatalf("status = %q", result.Status)
	}

	if _, err := p.Destroy(context.Background(), assetRes); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("Destroy asset after detach = %v, want unsupported refusal", err)
	}
	ops := fake.operations()
	if len(ops) != 1 || ops[0].collection != "campaignAssets" || ops[0].kind != "remove" {
		t.Fatalf("operations = %+v, want only campaignAssets remove", ops)
	}
}
