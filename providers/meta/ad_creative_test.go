package meta_test

import (
	"context"
	"errors"
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

const (
	testPageID      = "123123123123123"
	testInstagramID = "456456456456456"
	testImageHash   = "0123456789abcdef0123456789abcdef"
	testVideoID     = "789789789789789"
)

func TestValidateAdCreativeImageAndVideoModes(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	for _, attrs := range []resource.Attributes{standardImageCreativeAttrs(), standardVideoCreativeAttrs()} {
		res := creativeResource(t, "instagram", attrs)
		if err := p.Validate(context.Background(), res); err != nil {
			t.Fatal(err)
		}
		want, _, err := p.NormalizeComparable(res, nil)
		if err != nil {
			t.Fatal(err)
		}
		if want[meta.AttrCallToAction] != "LEARN_MORE" || want[meta.AttrInstagramUserID] != testInstagramID {
			t.Fatalf("normalized = %#v", want)
		}
	}
}

func TestValidateAdCreativeRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	tests := []struct {
		name, contains string
		mutate         func(resource.Attributes)
	}{
		{"missing page", "pageId", func(a resource.Attributes) { delete(a, meta.AttrPageID) }},
		{"invalid instagram id", "instagramUserId", func(a resource.Attributes) { a[meta.AttrInstagramUserID] = "account-name" }},
		{"invalid URL", "absolute http", func(a resource.Attributes) { a[meta.AttrDestinationURL] = "/trial" }},
		{"missing primary text", "primaryText", func(a resource.Attributes) { delete(a, meta.AttrPrimaryText) }},
		{"missing headline", "headline", func(a resource.Attributes) { delete(a, meta.AttrHeadline) }},
		{"unsupported CTA", "START_TRIAL is a conversion event type", func(a resource.Attributes) { a[meta.AttrCallToAction] = "START_TRIAL" }},
		{"both media modes", "exactly one", func(a resource.Attributes) { a[meta.AttrVideoID] = testVideoID }},
		{"no media mode", "exactly one", func(a resource.Attributes) { delete(a, meta.AttrImageHash) }},
		{"local media path", "without whitespace", func(a resource.Attributes) { a[meta.AttrImageHash] = "assets/hero image.png" }},
		{"bad URL tags", "key=value", func(a resource.Attributes) { a[meta.AttrURLTags] = "utm_source" }},
		{"raw story spec", "unsupported attribute", func(a resource.Attributes) { a["object_story_spec"] = map[string]any{} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			attrs := standardImageCreativeAttrs()
			tc.mutate(attrs)
			err := p.Validate(context.Background(), creativeResource(t, "bad", attrs))
			if err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("error=%v, want %q", err, tc.contains)
			}
		})
	}
}

func TestCreateReadAndRenameImageAdCreative(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	res := creativeResource(t, "image", standardImageCreativeAttrs())
	created, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatal(err)
	}
	if created.Identity.ID != testCreativeID || created.Computed[meta.OutputAdCreativeID] != testCreativeID {
		t.Fatalf("created=%#v", created)
	}
	if created.Attributes[meta.AttrImageHash] != testImageHash || created.Attributes[meta.AttrURLTags] != standardImageCreativeAttrs()[meta.AttrURLTags] {
		t.Fatalf("attributes=%#v", created.Attributes)
	}
	bound := res
	bound.Identity = created.Identity
	bound.Attributes = created.Attributes.Clone()
	bound.Attributes[meta.AttrName] = "Instagram Creative v2"
	updated, err := p.Update(context.Background(), bound, created)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Attributes[meta.AttrName] != "Instagram Creative v2" {
		t.Fatalf("updated=%#v", updated.Attributes)
	}
	posts, _ := srv.mutationCounts()
	if posts != 2 {
		t.Fatalf("posts=%d, want create plus rename", posts)
	}
	if _, err := p.Update(context.Background(), bound, updated); err != nil {
		t.Fatal(err)
	}
	posts, _ = srv.mutationCounts()
	if posts != 2 {
		t.Fatalf("no-op posts=%d", posts)
	}
}

func TestCreateVideoAdCreativeRoundTripsExternalVideo(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	created, err := p.Create(context.Background(), creativeResource(t, "video", standardVideoCreativeAttrs()))
	if err != nil {
		t.Fatal(err)
	}
	if created.Attributes[meta.AttrVideoID] != testVideoID {
		t.Fatalf("external video id was not preserved: %#v", created.Attributes)
	}
	if _, exists := created.Attributes[meta.AttrImageHash]; exists {
		t.Fatalf("video creative unexpectedly contains imageHash: %#v", created.Attributes)
	}
}

func TestAdCreativeImmutableContentFailsPlanningWithoutMutation(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedCreative(testCreativeID, nil)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	addr := creativeAddress(t, "instagram")
	if err := st.Bind(addr, resource.Identity{ID: testCreativeID}); err != nil {
		t.Fatal(err)
	}
	attrs := standardImageCreativeAttrs()
	attrs[meta.AttrHeadline] = "Changed headline"
	_, err = plan.BuildWithState(context.Background(), []resource.Resource{{Address: addr, Attributes: attrs}}, func(resource.Address) (provider.Reader, error) { return p, nil }, st)
	if err == nil || !strings.Contains(err.Error(), "declare a new logical meta.ad_creative") {
		t.Fatalf("error=%v", err)
	}
	posts, deletes := srv.mutationCounts()
	if posts != 0 || deletes != 0 {
		t.Fatalf("plan mutated posts=%d deletes=%d", posts, deletes)
	}
}

func TestAdCreativeEquivalentPlanIsNoOp(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedCreative(testCreativeID, nil)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	addr := creativeAddress(t, "instagram")
	if err := st.Bind(addr, resource.Identity{ID: testCreativeID}); err != nil {
		t.Fatal(err)
	}
	got, err := plan.BuildWithState(context.Background(), []resource.Resource{{Address: addr, Attributes: standardImageCreativeAttrs()}}, func(resource.Address) (provider.Reader, error) { return p, nil }, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionUnchanged {
		t.Fatalf("plan=%#v", got.Changes)
	}
}

func TestImportVideoAdCreativeProducesCanonicalYAML(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedCreative(testCreativeID, graphObject{
		"object_story_spec": graphObject{
			"page_id": testPageID, "instagram_user_id": testInstagramID,
			"video_data": graphObject{
				"video_id": testVideoID, "message": "Start your trial today.", "title": "Try Sync Warehouse",
				"link_description": "Organize your sync catalog.",
				"call_to_action":   graphObject{"type": "LEARN_MORE", "value": graphObject{"link": "https://example.com/trial"}},
			},
		},
	})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Run(context.Background(), creativeAddress(t, "video"), testCreativeID, func(resource.Address) (provider.Provider, error) { return p, nil }, st)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"pageId: \"" + testPageID + "\"", "instagramUserId: \"" + testInstagramID + "\"", "videoId: \"" + testVideoID + "\"", "callToAction: LEARN_MORE", "urlTags: utm_source=meta", "{{campaign.name}}", "{{ad.name}}"} {
		if !strings.Contains(result.YAML, want) {
			t.Fatalf("YAML missing %q:\n%s", want, result.YAML)
		}
	}
	if strings.Contains(result.YAML, "access_token") || strings.Contains(result.YAML, testToken) || strings.Contains(result.YAML, "object_story_spec") {
		t.Fatalf("import leaked unsupported data:\n%s", result.YAML)
	}
}

func TestDestroyAdCreativeIsIdempotent(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedCreative(testCreativeID, nil)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	res := creativeResource(t, "instagram", standardImageCreativeAttrs())
	res.Identity = resource.Identity{ID: testCreativeID}
	capability, err := p.DestroyCapability(res)
	if err != nil || capability != provider.DestroyDelete {
		t.Fatalf("capability=%q err=%v", capability, err)
	}
	got, err := p.Destroy(context.Background(), res)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != provider.DestroyStatusDestroyed {
		t.Fatalf("status=%q", got.Status)
	}
	got, err = p.Destroy(context.Background(), res)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != provider.DestroyStatusAlreadyAbsent {
		t.Fatalf("second status=%q", got.Status)
	}
	if _, err := p.Read(context.Background(), res); !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("read=%v", err)
	}
}

func TestAdCreativeAPIFailureDoesNotLeakToken(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.creativeCreateFailure = true
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	_, err := p.Create(context.Background(), creativeResource(t, "failure", standardImageCreativeAttrs()))
	if err == nil || !strings.Contains(err.Error(), "temporary creative failure") {
		t.Fatalf("error=%v", err)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("token leaked: %v", err)
	}
}

func standardImageCreativeAttrs() resource.Attributes {
	return resource.Attributes{
		meta.AttrName: "Instagram Creative", meta.AttrPageID: testPageID,
		meta.AttrInstagramUserID: testInstagramID, meta.AttrDestinationURL: "https://example.com/trial",
		meta.AttrPrimaryText: "Start your trial today.", meta.AttrHeadline: "Try Sync Warehouse",
		meta.AttrDescription: "Organize your sync catalog.", meta.AttrCallToAction: "learn_more",
		meta.AttrImageHash: testImageHash,
		meta.AttrURLTags:   "utm_source=meta&utm_campaign={{campaign.name}}&utm_content={{ad.name}}",
	}
}

func standardVideoCreativeAttrs() resource.Attributes {
	attrs := standardImageCreativeAttrs()
	delete(attrs, meta.AttrImageHash)
	attrs[meta.AttrVideoID] = testVideoID
	return attrs
}
