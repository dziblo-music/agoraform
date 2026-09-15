package meta_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/meta"
)

func TestValidateAdSetRejectsNumericStringTargetingEntity(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	attrs := standardAdSetAttrs(t)
	attrs[meta.AttrTargeting].(map[string]any)["interests"] = []any{testInterestID}

	err := p.Validate(context.Background(), adSetResource(t, "string-interest", attrs))
	if err == nil || !strings.Contains(err.Error(), "must be an object with a numeric id") {
		t.Fatalf("error=%v", err)
	}
}

func TestCreateAdSetAllowsSharedCustomAudience(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	seedTargetingReferenceDependencies(srv)
	srv.seedAudience(testAudienceIncludeID, graphObject{
		"account_id":        "999888777666555",
		"subtype":           "CUSTOM",
		"usage_restriction": "NONE",
		"use_in_campaigns":  true,
	})
	httpSrv := srv.start()
	defer httpSrv.Close()

	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adSetCatalog(t))
	rememberAdSetDependencies(t, p)
	attrs := standardAdSetAttrs(t)
	attrs[meta.AttrTargeting].(map[string]any)["customAudiences"] = []any{map[string]any{"id": testAudienceIncludeID}}

	created, err := p.Create(context.Background(), adSetResource(t, "shared-audience", attrs))
	if err != nil {
		t.Fatal(err)
	}
	if created.Identity.ID != testAdSetID {
		t.Fatalf("created id=%q", created.Identity.ID)
	}
}

func TestCreateAdSetRejectsExclusionOnlyAudienceForInclusion(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	seedTargetingReferenceDependencies(srv)
	srv.seedAudience(testAudienceIncludeID, graphObject{
		"subtype":           "CUSTOM",
		"usage_restriction": "EXCLUSION_ONLY",
		"use_in_campaigns":  true,
	})
	httpSrv := srv.start()
	defer httpSrv.Close()

	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adSetCatalog(t))
	rememberAdSetDependencies(t, p)
	attrs := standardAdSetAttrs(t)
	attrs[meta.AttrTargeting].(map[string]any)["customAudiences"] = []any{map[string]any{"id": testAudienceIncludeID}}

	_, err := p.Create(context.Background(), adSetResource(t, "exclusion-only-include", attrs))
	if err == nil || !strings.Contains(err.Error(), "restricted to exclusions") {
		t.Fatalf("error=%v", err)
	}
	posts, deletes := srv.mutationCounts()
	if posts != 0 || deletes != 0 {
		t.Fatalf("mutated posts=%d deletes=%d", posts, deletes)
	}
}

func TestCreateAdSetAllowsExclusionOnlyAudienceForExclusion(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	seedTargetingReferenceDependencies(srv)
	srv.seedAudience(testAudienceExcludeID, graphObject{
		"subtype":           "CUSTOM",
		"usage_restriction": "EXCLUSION_ONLY",
		"use_in_campaigns":  true,
	})
	httpSrv := srv.start()
	defer httpSrv.Close()

	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adSetCatalog(t))
	rememberAdSetDependencies(t, p)
	attrs := standardAdSetAttrs(t)
	attrs[meta.AttrTargeting].(map[string]any)["excludedCustomAudiences"] = []any{map[string]any{"id": testAudienceExcludeID}}

	created, err := p.Create(context.Background(), adSetResource(t, "exclusion-only-exclude", attrs))
	if err != nil {
		t.Fatal(err)
	}
	if created.Identity.ID != testAdSetID {
		t.Fatalf("created id=%q", created.Identity.ID)
	}
}

func TestCreateAdSetRejectsAudienceDisabledForCampaigns(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	seedTargetingReferenceDependencies(srv)
	srv.seedAudience(testAudienceIncludeID, graphObject{
		"subtype":           "CUSTOM",
		"usage_restriction": "NONE",
		"use_in_campaigns":  false,
	})
	httpSrv := srv.start()
	defer httpSrv.Close()

	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adSetCatalog(t))
	rememberAdSetDependencies(t, p)
	attrs := standardAdSetAttrs(t)
	attrs[meta.AttrTargeting].(map[string]any)["customAudiences"] = []any{map[string]any{"id": testAudienceIncludeID}}

	_, err := p.Create(context.Background(), adSetResource(t, "disabled-audience", attrs))
	if err == nil || !strings.Contains(err.Error(), "cannot be used in campaigns") {
		t.Fatalf("error=%v", err)
	}
	posts, deletes := srv.mutationCounts()
	if posts != 0 || deletes != 0 {
		t.Fatalf("mutated posts=%d deletes=%d", posts, deletes)
	}
}

func seedTargetingReferenceDependencies(srv *graphServer) {
	srv.seedCampaign(testCampaignID, graphObject{"name": "Acquisition", "objective": "OUTCOME_SALES"})
	srv.seedPixel(testPixelID, "Website")
	srv.seedConversion(testConvID, graphObject{
		"name":              "Trial Started",
		"custom_event_type": "START_TRIAL",
		"rule":              `{"and":[{"event":{"eq":"StartTrial"}}]}`,
		"pixel":             graphObject{"id": testPixelID},
		"event_source_type": "pixel",
	})
}

var _ = resource.Attributes{}
