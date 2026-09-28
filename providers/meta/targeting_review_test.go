package meta_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/providers/meta"
)

func TestValidateAdSetRejectsNumericStringTargetingEntity(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	attrs := standardAdSetAttrs(t)
	attrs[meta.AttrTargeting].(map[string]any)["interests"] = []any{testInterestID}

	err := p.Validate(context.Background(), adSetResource(t, "string_interest", attrs))
	if err == nil || !strings.Contains(err.Error(), "must be an object with a numeric id") {
		t.Fatalf("error=%v", err)
	}
}

func TestCreateAdSetCustomAudienceReadUsesSupportedV26Fields(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	seedTargetingReferenceDependencies(srv)
	srv.seedAudience(testAudienceIncludeID, graphObject{
		"subtype":           "CUSTOM",
	})
	httpSrv := srv.start()
	defer httpSrv.Close()

	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adSetCatalog(t))
	rememberAdSetDependencies(t, p)
	attrs := standardAdSetAttrs(t)
	attrs[meta.AttrTargeting].(map[string]any)["customAudiences"] = []any{map[string]any{"id": testAudienceIncludeID}}

	if _, err := p.Create(context.Background(), adSetResource(t, "supported_audience_fields", attrs)); err != nil {
		t.Fatal(err)
	}
}

func TestCreateAdSetAllowsSharedCustomAudience(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	seedTargetingReferenceDependencies(srv)
	srv.seedAudience(testAudienceIncludeID, graphObject{
		"account_id":        "999888777666555",
		"subtype":           "CUSTOM",
	})
	httpSrv := srv.start()
	defer httpSrv.Close()

	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(adSetCatalog(t))
	rememberAdSetDependencies(t, p)
	attrs := standardAdSetAttrs(t)
	attrs[meta.AttrTargeting].(map[string]any)["customAudiences"] = []any{map[string]any{"id": testAudienceIncludeID}}

	created, err := p.Create(context.Background(), adSetResource(t, "shared_audience", attrs))
	if err != nil {
		t.Fatal(err)
	}
	if created.Identity.ID != testAdSetID {
		t.Fatalf("created id=%q", created.Identity.ID)
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
