package googleads_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestValidateSitelinkAndCalloutAssets(t *testing.T) {
	t.Parallel()

	p := testAssetProvider(t, nil)
	if err := p.Validate(context.Background(), sitelinkAssetResource(t, "features", "Features", []any{"https://example.com/features"})); err != nil {
		t.Fatalf("Validate sitelink: %v", err)
	}
	withDescriptions := sitelinkAssetResource(t, "features", "Features", []any{"https://example.com/features"})
	withDescriptions.Attributes[googleads.AttrDescription1] = "See product features"
	withDescriptions.Attributes[googleads.AttrDescription2] = "Built for growing teams"
	if err := p.Validate(context.Background(), withDescriptions); err != nil {
		t.Fatalf("Validate sitelink descriptions: %v", err)
	}
	if err := p.Validate(context.Background(), calloutAssetResource(t, "free_trial", "Free trial")); err != nil {
		t.Fatalf("Validate callout: %v", err)
	}
}

func TestValidateSitelinkAndCalloutErrors(t *testing.T) {
	t.Parallel()

	p := testAssetProvider(t, nil)
	addr := mustAssetAddress(t, "features")
	cases := []struct {
		name string
		res  resource.Resource
		want string
	}{
		{
			name: "sitelink missing link text",
			res: resource.Resource{
				Address:    addr,
				Attributes: resource.Attributes{googleads.AttrType: "SITELINK", googleads.AttrFinalUrls: []any{"https://example.com/features"}},
			},
			want: "missing required attribute \"linkText\"",
		},
		{
			name: "sitelink missing final urls",
			res: resource.Resource{
				Address:    addr,
				Attributes: resource.Attributes{googleads.AttrType: "SITELINK", googleads.AttrLinkText: "Features"},
			},
			want: "missing required attribute \"finalUrls\"",
		},
		{
			name: "sitelink unpaired description",
			res: resource.Resource{
				Address: addr,
				Attributes: resource.Attributes{
					googleads.AttrType:         "SITELINK",
					googleads.AttrLinkText:     "Features",
					googleads.AttrFinalUrls:    []any{"https://example.com/features"},
					googleads.AttrDescription1: "See product features",
				},
			},
			want: "both be set or both omitted",
		},
		{
			name: "sitelink link text too long",
			res: resource.Resource{
				Address: addr,
				Attributes: resource.Attributes{
					googleads.AttrType:      "SITELINK",
					googleads.AttrLinkText:  strings.Repeat("A", 26),
					googleads.AttrFinalUrls: []any{"https://example.com/features"},
				},
			},
			want: "at most 25",
		},
		{
			name: "callout missing text",
			res: resource.Resource{
				Address:    addr,
				Attributes: resource.Attributes{googleads.AttrType: "CALLOUT"},
			},
			want: "missing required attribute \"calloutText\"",
		},
		{
			name: "callout too long",
			res:  calloutAssetResource(t, "features", strings.Repeat("A", 26)),
			want: "at most 25",
		},
		{
			name: "sitelink rejects callout text",
			res: resource.Resource{
				Address: addr,
				Attributes: resource.Attributes{
					googleads.AttrType:        "SITELINK",
					googleads.AttrLinkText:    "Features",
					googleads.AttrFinalUrls:   []any{"https://example.com/features"},
					googleads.AttrCalloutText: "Free shipping",
				},
			},
			want: "only valid for CALLOUT",
		},
		{
			name: "callout rejects final urls",
			res: resource.Resource{
				Address: addr,
				Attributes: resource.Attributes{
					googleads.AttrType:        "CALLOUT",
					googleads.AttrCalloutText: "Free shipping",
					googleads.AttrFinalUrls:   []any{"https://example.com/"},
				},
			},
			want: "only valid for SITELINK",
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
		})
	}
}

func TestCreateAndReadSitelinkAsset(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	p := testAssetProvider(t, fake)
	res := sitelinkAssetResource(t, "features", "Features", []any{"https://example.com/features"})
	res.Attributes[googleads.AttrDescription1] = "See product features"
	res.Attributes[googleads.AttrDescription2] = "Built for growing teams"

	created, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Attributes[googleads.AttrType] != "SITELINK" {
		t.Fatalf("type = %v", created.Attributes[googleads.AttrType])
	}
	if created.Attributes[googleads.AttrLinkText] != "Features" {
		t.Fatalf("linkText = %v", created.Attributes[googleads.AttrLinkText])
	}
	if created.Attributes[googleads.AttrDescription1] != "See product features" {
		t.Fatalf("description1 = %v", created.Attributes[googleads.AttrDescription1])
	}
	if _, ok := created.Attributes["sitelinkAsset"]; ok {
		t.Fatal("nested sitelinkAsset must stay computed")
	}
	if !strings.Contains(fake.lastMutateBody(), `"sitelinkAsset"`) || !strings.Contains(fake.lastMutateBody(), `"finalUrls"`) {
		t.Fatalf("create missing sitelink payload: %s", fake.lastMutateBody())
	}

	res.Identity = created.Identity
	live, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if live.Attributes[googleads.AttrLinkText] != "Features" {
		t.Fatalf("read linkText = %v", live.Attributes[googleads.AttrLinkText])
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
		t.Fatalf("unchanged sitelink produced changes: %+v", got.Changes)
	}
}

func TestCreateAndReadCalloutAsset(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	p := testAssetProvider(t, fake)
	res := calloutAssetResource(t, "free_trial", "Free trial")

	created, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Attributes[googleads.AttrCalloutText] != "Free trial" {
		t.Fatalf("calloutText = %v", created.Attributes[googleads.AttrCalloutText])
	}
	if !strings.Contains(fake.lastMutateBody(), `"calloutAsset"`) {
		t.Fatalf("create missing callout payload: %s", fake.lastMutateBody())
	}

	res.Identity = created.Identity
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
		t.Fatalf("unchanged callout produced changes: %+v", got.Changes)
	}
}

func TestUpdateSitelinkAndCalloutCopy(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedAsset(sampleSitelinkAsset("91", "Features", []any{"https://example.com/features"}))
	fake.seedAsset(sampleCalloutAsset("92", "Free trial"))
	p := testAssetProvider(t, fake)

	sitelink := sitelinkAssetResource(t, "features", "Pricing", []any{"https://example.com/pricing"})
	sitelink.Identity = resource.Identity{ID: "91"}
	updated, err := p.Update(context.Background(), sitelink, resource.RemoteResource{
		Address:  sitelink.Address,
		Identity: sitelink.Identity,
		Attributes: resource.Attributes{
			googleads.AttrType:      "SITELINK",
			googleads.AttrLinkText:  "Features",
			googleads.AttrFinalUrls: []any{"https://example.com/features"},
		},
	})
	if err != nil {
		t.Fatalf("Update sitelink: %v", err)
	}
	if updated.Attributes[googleads.AttrLinkText] != "Pricing" {
		t.Fatalf("updated linkText = %v", updated.Attributes[googleads.AttrLinkText])
	}
	if !strings.Contains(fake.lastMutateBody(), "updateMask") || !strings.Contains(fake.lastMutateBody(), "sitelinkAsset.linkText") {
		t.Fatalf("sitelink update missing field mask: %s", fake.lastMutateBody())
	}

	callout := calloutAssetResource(t, "free_trial", "No credit card")
	callout.Identity = resource.Identity{ID: "92"}
	updatedCallout, err := p.Update(context.Background(), callout, resource.RemoteResource{
		Address:    callout.Address,
		Identity:   callout.Identity,
		Attributes: resource.Attributes{googleads.AttrType: "CALLOUT", googleads.AttrCalloutText: "Free trial"},
	})
	if err != nil {
		t.Fatalf("Update callout: %v", err)
	}
	if updatedCallout.Attributes[googleads.AttrCalloutText] != "No credit card" {
		t.Fatalf("updated calloutText = %v", updatedCallout.Attributes[googleads.AttrCalloutText])
	}
}

func TestPlanSitelinkTypeChangeIsImmutable(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedAsset(sampleSitelinkAsset("91", "Features", []any{"https://example.com/features"}))
	p := testAssetProvider(t, fake)
	res := calloutAssetResource(t, "features", "Free trial")
	res.Identity = resource.Identity{ID: "91"}

	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(res.Address, res.Identity); err != nil {
		t.Fatal(err)
	}
	_, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("plan = %v, want immutable type guidance", err)
	}
}

func TestCreateMultipleSitelinkAndCalloutAttachments(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedAsset(sampleSitelinkAsset("91", "Features", []any{"https://example.com/features"}))
	fake.seedAsset(sampleSitelinkAsset("92", "Pricing", []any{"https://example.com/pricing"}))
	fake.seedAsset(sampleCalloutAsset("93", "Free trial"))
	fake.seedAsset(sampleCalloutAsset("94", "No credit card"))
	p := testAssetProvider(t, fake)
	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		name, id string
	}{{"features", "91"}, {"pricing", "92"}, {"free_trial", "93"}, {"no_card", "94"}} {
		if err := st.Bind(mustAssetAddress(t, item.name), resource.Identity{ID: item.id}); err != nil {
			t.Fatal(err)
		}
	}
	p.SetIdentityCatalog(st)

	attachments := []resource.Resource{
		campaignAssetResource(t, "features", resource.Attributes{
			googleads.AttrCampaign:  resolvedCampaign(t, "brand", "21"),
			googleads.AttrAsset:     resolvedAsset(t, "features", "91"),
			googleads.AttrFieldType: "SITELINK",
		}),
		campaignAssetResource(t, "pricing", resource.Attributes{
			googleads.AttrCampaign:  resolvedCampaign(t, "brand", "21"),
			googleads.AttrAsset:     resolvedAsset(t, "pricing", "92"),
			googleads.AttrFieldType: "SITELINK",
		}),
		campaignAssetResource(t, "free_trial", resource.Attributes{
			googleads.AttrCampaign:  resolvedCampaign(t, "brand", "21"),
			googleads.AttrAsset:     resolvedAsset(t, "free_trial", "93"),
			googleads.AttrFieldType: "CALLOUT",
		}),
		campaignAssetResource(t, "no_card", resource.Attributes{
			googleads.AttrCampaign:  resolvedCampaign(t, "brand", "21"),
			googleads.AttrAsset:     resolvedAsset(t, "no_card", "94"),
			googleads.AttrFieldType: "CALLOUT",
		}),
	}
	wantIDs := []string{"21~91~SITELINK", "21~92~SITELINK", "21~93~CALLOUT", "21~94~CALLOUT"}
	for i, res := range attachments {
		created, err := p.Create(context.Background(), res)
		if err != nil {
			t.Fatalf("Create %s: %v", res.Address, err)
		}
		if created.Identity.ID != wantIDs[i] {
			t.Fatalf("%s identity = %q, want %q", res.Address, created.Identity.ID, wantIDs[i])
		}
		res.Identity = created.Identity
		if err := st.Bind(res.Address, created.Identity); err != nil {
			t.Fatal(err)
		}
		attachments[i] = res
	}

	features := sitelinkAssetResource(t, "features", "Features", []any{"https://example.com/features"})
	features.Identity = resource.Identity{ID: "91"}
	pricing := sitelinkAssetResource(t, "pricing", "Pricing", []any{"https://example.com/pricing"})
	pricing.Identity = resource.Identity{ID: "92"}
	freeTrial := calloutAssetResource(t, "free_trial", "Free trial")
	freeTrial.Identity = resource.Identity{ID: "93"}
	noCard := calloutAssetResource(t, "no_card", "No credit card")
	noCard.Identity = resource.Identity{ID: "94"}
	resources := append([]resource.Resource{features, pricing, freeTrial, noCard}, attachments...)
	got, err := plan.BuildWithState(context.Background(), resources, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	if got.HasChanges() {
		t.Fatalf("equivalent sitelink/callout attachments produced changes: %+v", got.Changes)
	}
}

func TestPlanSitelinkAttachmentCreatesAssetAndLink(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	p := testAssetProvider(t, fake)
	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	assetRes := sitelinkAssetResource(t, "features", "Features", []any{"https://example.com/features"})
	link := campaignAssetResource(t, "features", resource.Attributes{
		googleads.AttrCampaign:  resolvedCampaign(t, "brand", "21"),
		googleads.AttrAsset:     assetRef(t, "features"),
		googleads.AttrFieldType: "SITELINK",
	})
	got, err := plan.BuildWithState(context.Background(), []resource.Resource{assetRes, link}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatalf("plan.Build: %v", err)
	}
	byAddr := map[string]plan.Action{}
	for _, change := range got.Changes {
		byAddr[change.Address.String()] = change.Action
	}
	if byAddr["googleads.asset.features"] != plan.ActionCreate {
		t.Fatalf("asset action = %v, want create", byAddr["googleads.asset.features"])
	}
	if byAddr["googleads.campaign_asset.features"] != plan.ActionCreate {
		t.Fatalf("campaign asset action = %v, want create", byAddr["googleads.campaign_asset.features"])
	}
}

func TestCreateCampaignAssetRequiresResolvedAssetIdentity(t *testing.T) {
	t.Parallel()

	p := testAssetProvider(t, newAssetFake())
	_, err := p.Create(context.Background(), campaignAssetResource(t, "features", resource.Attributes{
		googleads.AttrCampaign:  resolvedCampaign(t, "brand", "21"),
		googleads.AttrAsset:     assetRef(t, "features"),
		googleads.AttrFieldType: "SITELINK",
	}))
	if err == nil || !strings.Contains(err.Error(), "asset reference has no provider-native identity") {
		t.Fatalf("Create = %v, want unresolved asset identity", err)
	}
}

func TestImportSitelinkAndCalloutAssets(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	item := sampleSitelinkAsset("91", "Features", []any{"https://example.com/features"})
	item["sitelinkAsset"] = map[string]any{
		"linkText":     "Features",
		"description1": "See product features",
		"description2": "Built for growing teams",
	}
	fake.seedAsset(item)
	fake.seedAsset(sampleCalloutAsset("92", "Free trial"))
	p := testAssetProvider(t, fake)

	sitelink, err := p.Import(context.Background(), mustAssetAddress(t, "features"), "91")
	if err != nil {
		t.Fatalf("Import sitelink: %v", err)
	}
	if sitelink.Attributes[googleads.AttrLinkText] != "Features" {
		t.Fatalf("imported linkText = %v", sitelink.Attributes[googleads.AttrLinkText])
	}
	if sitelink.Attributes[googleads.AttrDescription1] != "See product features" {
		t.Fatalf("imported description1 = %v", sitelink.Attributes[googleads.AttrDescription1])
	}

	callout, err := p.Import(context.Background(), mustAssetAddress(t, "free_trial"), "customers/"+testCustomerID+"/assets/92")
	if err != nil {
		t.Fatalf("Import callout: %v", err)
	}
	if callout.Identity.ID != "92" {
		t.Fatalf("imported callout id = %q", callout.Identity.ID)
	}
	if callout.Attributes[googleads.AttrCalloutText] != "Free trial" {
		t.Fatalf("imported calloutText = %v", callout.Attributes[googleads.AttrCalloutText])
	}
}

func TestImportSitelinkCampaignAssetReconstructsRefs(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedCampaignAsset(map[string]any{
		"campaign":  "customers/" + testCustomerID + "/campaigns/21",
		"asset":     "customers/" + testCustomerID + "/assets/91",
		"fieldType": "SITELINK",
		"status":    "ENABLED",
	})
	p := testAssetProvider(t, fake)
	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustAssetAddress(t, "features"), resource.Identity{ID: "91"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	live, err := p.Import(context.Background(), mustCampaignAssetAddress(t, "features"), "21~91~SITELINK")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	ref, ok := resource.AsRef(live.Attributes[googleads.AttrCampaign])
	if !ok || ref.Address != mustCampaignAddress(t, "brand") {
		t.Fatalf("campaign = %#v", live.Attributes[googleads.AttrCampaign])
	}
	asset, ok := resource.AsRef(live.Attributes[googleads.AttrAsset])
	if !ok || asset.Address != mustAssetAddress(t, "features") {
		t.Fatalf("asset = %#v", live.Attributes[googleads.AttrAsset])
	}
}

func TestDestroySharedSitelinkKeepsOtherAttachment(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.seedAsset(sampleSitelinkAsset("91", "Features", []any{"https://example.com/features"}))
	fake.seedCampaignAsset(map[string]any{
		"campaign":  "customers/" + testCustomerID + "/campaigns/21",
		"asset":     "customers/" + testCustomerID + "/assets/91",
		"fieldType": "SITELINK",
		"status":    "ENABLED",
	})
	fake.seedCampaignAsset(map[string]any{
		"campaign":  "customers/" + testCustomerID + "/campaigns/22",
		"asset":     "customers/" + testCustomerID + "/assets/91",
		"fieldType": "SITELINK",
		"status":    "ENABLED",
	})
	p := testAssetProvider(t, fake)
	st := mustGoogleAdsImportStore(t)
	if err := st.Bind(mustCampaignAddress(t, "brand"), resource.Identity{ID: "21"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustCampaignAddress(t, "other"), resource.Identity{ID: "22"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustAssetAddress(t, "features"), resource.Identity{ID: "91"}); err != nil {
		t.Fatal(err)
	}
	p.SetIdentityCatalog(st)

	first := campaignAssetResource(t, "features", resource.Attributes{
		googleads.AttrCampaign:  resolvedCampaign(t, "brand", "21"),
		googleads.AttrAsset:     resolvedAsset(t, "features", "91"),
		googleads.AttrFieldType: "SITELINK",
	})
	first.Identity = resource.Identity{ID: "21~91~SITELINK"}
	result, err := p.Destroy(context.Background(), first)
	if err != nil {
		t.Fatalf("Destroy first attachment: %v", err)
	}
	if result.Status != provider.DestroyStatusRemoved {
		t.Fatalf("status = %q", result.Status)
	}

	second := campaignAssetResource(t, "features_other", resource.Attributes{
		googleads.AttrCampaign:  resolvedCampaign(t, "other", "22"),
		googleads.AttrAsset:     resolvedAsset(t, "features", "91"),
		googleads.AttrFieldType: "SITELINK",
	})
	second.Identity = resource.Identity{ID: "22~91~SITELINK"}
	live, err := p.Read(context.Background(), second)
	if err != nil {
		t.Fatalf("Read remaining attachment: %v", err)
	}
	if live.Identity.ID != "22~91~SITELINK" {
		t.Fatalf("remaining identity = %q", live.Identity.ID)
	}

	assetRes := sitelinkAssetResource(t, "features", "Features", []any{"https://example.com/features"})
	assetRes.Identity = resource.Identity{ID: "91"}
	if _, err := p.Destroy(context.Background(), assetRes); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("Destroy shared asset = %v, want unsupported refusal", err)
	}
	ops := fake.operations()
	if len(ops) != 1 || ops[0].collection != "campaignAssets" || ops[0].kind != "remove" {
		t.Fatalf("operations = %+v, want only campaignAssets remove", ops)
	}
}

func TestReadSitelinkMalformedResponse(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.searchBody = `{"results":[{"asset":"oops ` + testAccessToken + `"}]}`
	p := testAssetProvider(t, fake)
	res := sitelinkAssetResource(t, "features", "Features", []any{"https://example.com/features"})
	res.Identity = resource.Identity{ID: "91"}
	_, err := p.Read(context.Background(), res)
	if err == nil {
		t.Fatal("expected malformed response error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("malformed response must not be ErrNotFound")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestCreateSitelinkMutateErrorRedactsSecrets(t *testing.T) {
	t.Parallel()

	fake := newAssetFake()
	fake.mutateStatus = http.StatusBadRequest
	p := testAssetProvider(t, fake)
	_, err := p.Create(context.Background(), sitelinkAssetResource(t, "features", "Features", []any{"https://example.com/features"}))
	if err == nil {
		t.Fatal("expected mutate error")
	}
	assertNoProviderSecret(t, err.Error())
}

func TestValidateResourceSetSitelinkAndCalloutTypeMustMatch(t *testing.T) {
	t.Parallel()

	p := googleads.New(googleads.Config{})
	callout := calloutAssetResource(t, "features", "Free trial")
	link := campaignAssetResource(t, "features", resource.Attributes{
		googleads.AttrCampaign:  campaignRef(t, "brand"),
		googleads.AttrAsset:     assetRef(t, "features"),
		googleads.AttrFieldType: "SITELINK",
	})
	err := p.ValidateResourceSet(context.Background(), []resource.Resource{callout, link})
	if err == nil || !strings.Contains(err.Error(), "SITELINK") || !strings.Contains(err.Error(), "CALLOUT") {
		t.Fatalf("ValidateResourceSet = %v, want sitelink/callout type mismatch", err)
	}
}
