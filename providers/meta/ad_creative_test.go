package meta_test

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/apply"
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
	srv.mu.Lock()
	story := srv.creatives[testCreativeID]["object_story_spec"].(graphObject)
	videoData := story["video_data"].(map[string]any)
	gotThumbnail := videoData["image_url"]
	srv.mu.Unlock()
	if gotThumbnail != "https://example.com/video-thumbnail.jpg" {
		t.Fatalf("video creative image_url = %v, want generated thumbnail URL", gotThumbnail)
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

func TestAdCreativeNameDriftComparison(t *testing.T) {
	t.Parallel()
	const configured = "IG_V1_Spreadsheet"
	const generated = "2026-09-29-8c94bf54cfdbc6682f9d8db4647fe9b5"
	tests := []struct {
		name       string
		remoteName string
		wantUpdate bool
	}{
		{name: "exact", remoteName: configured},
		{name: "meta suffix", remoteName: configured + " " + generated},
		{name: "uppercase hex suffix", remoteName: configured + " 2026-09-29-8C94BF54CFDBC6682F9D8DB4647FE9B5"},
		{name: "renamed base", remoteName: configured + "_NEW", wantUpdate: true},
		{name: "different base with meta suffix", remoteName: "IG_V1_Spreadsheet_NEW " + generated, wantUpdate: true},
		{name: "arbitrary suffix", remoteName: configured + " extra", wantUpdate: true},
		{name: "short hex", remoteName: configured + " 2026-09-29-8c94bf54", wantUpdate: true},
		{name: "invalid calendar date", remoteName: configured + " 2026-02-31-" + generated[len("2026-09-29-"):], wantUpdate: true},
		{name: "non-hex", remoteName: configured + " 2026-09-29-zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", wantUpdate: true},
		{name: "trailing junk", remoteName: configured + " " + generated + " extra", wantUpdate: true},
		{name: "missing space", remoteName: configured + generated, wantUpdate: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := newGraphServer(t)
			srv.seedCreative(testCreativeID, graphObject{"name": tc.remoteName})
			httpSrv := srv.start()
			defer httpSrv.Close()
			p := testProvider(t, httpSrv)
			st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
			if err != nil {
				t.Fatal(err)
			}
			addr := creativeAddress(t, "ig_v1_spreadsheet")
			if err := st.Bind(addr, resource.Identity{ID: testCreativeID}); err != nil {
				t.Fatal(err)
			}
			attrs := standardImageCreativeAttrs()
			attrs[meta.AttrName] = configured
			got, err := plan.BuildWithState(context.Background(), []resource.Resource{{Address: addr, Attributes: attrs}}, func(resource.Address) (provider.Reader, error) { return p, nil }, st)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Changes) != 1 {
				t.Fatalf("changes=%d", len(got.Changes))
			}
			change := got.Changes[0]
			if tc.wantUpdate {
				if change.Action != plan.ActionUpdate || len(change.Diffs) != 1 || change.Diffs[0].Path != meta.AttrName || change.Diffs[0].Before != tc.remoteName || change.Diffs[0].After != configured {
					t.Fatalf("change=%#v", change)
				}
				return
			}
			if change.Action != plan.ActionUnchanged {
				t.Fatalf("action=%s diffs=%#v", change.Action, change.Diffs)
			}
			formatted := plan.Format(got)
			if !strings.Contains(formatted, "Plan: 0 to create, 0 to update, 0 to destroy.") {
				t.Fatalf("plan:\n%s", formatted)
			}
		})
	}
}

func TestApplyThenPlanIgnoresMetaCreativeNameSuffix(t *testing.T) {
	t.Parallel()
	const configured = "IG_V1_Spreadsheet"
	const remoteName = "IG_V1_Spreadsheet 2026-09-29-8c94bf54cfdbc6682f9d8db4647fe9b5"
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	attrs := standardImageCreativeAttrs()
	attrs[meta.AttrName] = configured
	res := creativeResource(t, "ig_v1_spreadsheet", attrs)
	lookup := func(resource.Address) (provider.Provider, error) { return p, nil }
	if _, err := apply.Run(context.Background(), []resource.Resource{res}, lookup, st, io.Discard); err != nil {
		t.Fatal(err)
	}
	posts, deletes := srv.mutationCounts()
	if posts != 1 || deletes != 0 {
		t.Fatalf("after apply posts=%d deletes=%d", posts, deletes)
	}
	srv.mu.Lock()
	srv.creatives[testCreativeID]["name"] = remoteName
	srv.mu.Unlock()

	got, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) { return p, nil }, st)
	if err != nil {
		t.Fatal(err)
	}
	formatted := plan.Format(got)
	if !strings.Contains(formatted, "Plan: 0 to create, 0 to update, 0 to destroy.") {
		t.Fatalf("plan:\n%s", formatted)
	}
	if _, err := apply.Run(context.Background(), []resource.Resource{res}, lookup, st, io.Discard); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	posts, deletes = srv.mutationCounts()
	if posts != 1 || deletes != 0 {
		t.Fatalf("second apply posts=%d deletes=%d", posts, deletes)
	}
	srv.mu.Lock()
	stored, _ := srv.creatives[testCreativeID]["name"].(string)
	srv.mu.Unlock()
	if stored != remoteName {
		t.Fatalf("remote name=%q, want Meta suffix preserved", stored)
	}
}

func TestReadAndImportAdCreativeRetainMetaGeneratedName(t *testing.T) {
	t.Parallel()
	const remoteName = "IG_V1_Spreadsheet 2026-09-29-8c94bf54cfdbc6682f9d8db4647fe9b5"
	srv := newGraphServer(t)
	srv.seedCreative(testCreativeID, graphObject{"name": remoteName})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	res := creativeResource(t, "ig_v1_spreadsheet", standardImageCreativeAttrs())
	res.Identity = resource.Identity{ID: testCreativeID}
	live, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatal(err)
	}
	if live.Attributes[meta.AttrName] != remoteName {
		t.Fatalf("read name=%v, want remote name", live.Attributes[meta.AttrName])
	}
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Run(context.Background(), res.Address, testCreativeID, func(resource.Address) (provider.Provider, error) { return p, nil }, st)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.YAML, remoteName) {
		t.Fatalf("import dropped Meta name:\n%s", result.YAML)
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
