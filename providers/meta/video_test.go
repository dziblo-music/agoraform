package meta_test

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dziblo-music/agoraform/internal/asset"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/meta"
)

func TestValidateVideoAcceptsMP4(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	if err := p.Validate(context.Background(), videoResourceFrom(t, "demo", localMP4(t, "product-demo.mp4"))); err != nil {
		t.Fatalf("Validate = %v, want nil", err)
	}
}

func TestValidateVideoRejectsUnsupportedType(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	local := resource.NewLocalAsset("notes.txt", "abc", 8, "text/plain", func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("not video")), nil
	})
	err := p.Validate(context.Background(), videoResourceFrom(t, "demo", local))
	if err == nil || !strings.Contains(err.Error(), "MP4 or MOV") {
		t.Fatalf("error = %v, want unsupported type error", err)
	}
}

func TestValidateVideoRejectsMissingSource(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	err := p.Validate(context.Background(), resource.Resource{Address: videoAddress(t, "demo")})
	if err == nil || !strings.Contains(err.Error(), "source.file") {
		t.Fatalf("error = %v, want source.file error", err)
	}
}

func TestCreateVideoUploadsAndWaitsUntilReady(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.videoProcessingPolls = 1
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	meta.SetVideoPollingForTest(p, time.Second, time.Millisecond, nil)

	local := localMP4(t, "product-demo.mp4")
	created, err := p.Create(context.Background(), videoResourceFrom(t, "demo", local))
	if err != nil {
		t.Fatal(err)
	}
	if created.Identity.ID != testVideoID {
		t.Errorf("Identity.ID = %q, want %q", created.Identity.ID, testVideoID)
	}
	if created.Identity.Fingerprint != local.Digest {
		t.Errorf("Identity.Fingerprint = %q, want digest %q", created.Identity.Fingerprint, local.Digest)
	}
	if created.Computed[meta.OutputVideoID] != testVideoID {
		t.Errorf("Computed[videoId] = %v, want %q", created.Computed[meta.OutputVideoID], testVideoID)
	}
	if _, ok := created.Attributes[asset.AttrName]; ok {
		t.Fatal("source path must not be stored in comparable attributes")
	}
	posts, _ := srv.mutationCounts()
	if posts != 1 {
		t.Errorf("posts = %d, want 1 (the upload)", posts)
	}
}

func TestCreateVideoTimeoutDoesNotTreatUploadAsReady(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.videoProcessingPolls = 1000
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	meta.SetVideoPollingForTest(p, 15*time.Millisecond, time.Millisecond, nil)

	_, err := p.Create(context.Background(), videoResourceFrom(t, "demo", localMP4(t, "product-demo.mp4")))
	if err == nil || !strings.Contains(err.Error(), "still processing") {
		t.Fatalf("error = %v, want processing timeout", err)
	}
	if !strings.Contains(err.Error(), testVideoID) {
		t.Fatalf("timeout error should include uploaded video id: %v", err)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("token leaked: %v", err)
	}
}

func TestCreateVideoProcessingFailure(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.videoStatusError = true
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	meta.SetVideoPollingForTest(p, time.Second, time.Millisecond, nil)

	_, err := p.Create(context.Background(), videoResourceFrom(t, "demo", localMP4(t, "product-demo.mp4")))
	if err == nil || !strings.Contains(err.Error(), "processing failed") {
		t.Fatalf("error = %v, want processing failure", err)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("token leaked: %v", err)
	}
}

func TestReadVideoNotReadyIsExplicit(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedVideo(testVideoID, graphObject{
		"status": graphObject{"video_status": "processing", "processing_progress": 20},
	})
	srv.videoProcessingPolls = 0
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	local := localMP4(t, "product-demo.mp4")
	res := videoResourceFrom(t, "demo", local)
	res.Identity = resource.Identity{ID: testVideoID, Fingerprint: local.Digest}
	_, err := p.Read(context.Background(), res)
	if err == nil || !strings.Contains(err.Error(), "still processing") {
		t.Fatalf("Read = %v, want still processing", err)
	}
}

func TestReadVideoReturnsNotFoundWhenUnbound(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	_, err := p.Read(context.Background(), videoResourceFrom(t, "demo", localMP4(t, "product-demo.mp4")))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
}

func TestPlanVideoUnchangedWhenContentMatches(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedVideo(testVideoID, nil)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	local := localMP4(t, "product-demo.mp4")
	res := videoResourceFrom(t, "demo", local)
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(res.Address, resource.Identity{ID: testVideoID, Fingerprint: local.Digest}); err != nil {
		t.Fatal(err)
	}
	got, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionUnchanged {
		t.Fatalf("changes = %#v, want unchanged", got.Changes)
	}
	posts, _ := srv.mutationCounts()
	if posts != 0 {
		t.Fatalf("plan uploaded: posts=%d", posts)
	}
}

func TestPlanVideoChangedContentFails(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedVideo(testVideoID, nil)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	local := localMP4(t, "product-demo.mp4")
	res := videoResourceFrom(t, "demo", local)
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(res.Address, resource.Identity{ID: testVideoID, Fingerprint: strings.Repeat("ab", 32)}); err != nil {
		t.Fatal(err)
	}
	_, err = plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("plan = %v, want immutable content guidance", err)
	}
}

func TestImportVideoDoesNotFabricateSource(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedVideo(testVideoID, nil)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	live, err := p.Import(context.Background(), videoAddress(t, "external"), testVideoID)
	if err != nil {
		t.Fatal(err)
	}
	if live.Identity.ID != testVideoID {
		t.Fatalf("id = %q", live.Identity.ID)
	}
	if live.Identity.Fingerprint != "" {
		t.Fatalf("import fabricated digest %q", live.Identity.Fingerprint)
	}
	if _, ok := live.Attributes[asset.AttrName]; ok {
		t.Fatal("import fabricated a local source")
	}
	if live.Computed[meta.OutputVideoID] != testVideoID {
		t.Fatalf("Computed[videoId] = %v", live.Computed[meta.OutputVideoID])
	}
}

func TestDestroyVideoDeletesRemoteObject(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedVideo(testVideoID, nil)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	local := localMP4(t, "product-demo.mp4")
	res := videoResourceFrom(t, "demo", local)
	res.Identity = resource.Identity{ID: testVideoID, Fingerprint: local.Digest}
	capability, err := p.DestroyCapability(res)
	if err != nil {
		t.Fatal(err)
	}
	if capability != provider.DestroyDelete {
		t.Fatalf("capability = %q, want DestroyDelete", capability)
	}
	result, err := p.Destroy(context.Background(), res)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != provider.DestroyStatusDestroyed {
		t.Fatalf("status = %q, want Destroyed", result.Status)
	}
	_, deletes := srv.mutationCounts()
	if deletes != 1 {
		t.Fatalf("deletes = %d, want 1", deletes)
	}
}

func TestVideoUploadFailureDoesNotLeakToken(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.videoUploadFailure = true
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	_, err := p.Create(context.Background(), videoResourceFrom(t, "fail", localMP4(t, "fail.mp4")))
	if err == nil || !strings.Contains(err.Error(), "temporary video upload failure") {
		t.Fatalf("error = %v, want upload failure", err)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("token leaked: %v", err)
	}
}

func TestAdCreativeManagedVideoCreatesWithResolvedID(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	attrs := standardVideoCreativeAttrs()
	delete(attrs, meta.AttrVideoID)
	attrs[meta.AttrVideoRef] = resource.Resolved{
		Address:  videoAddress(t, "demo"),
		Identity: resource.Identity{ID: testVideoID},
		Outputs:  resource.Attributes{meta.OutputVideoID: testVideoID},
	}
	created, err := p.Create(context.Background(), creativeResource(t, "instagram", attrs))
	if err != nil {
		t.Fatal(err)
	}
	if created.Attributes[meta.AttrVideoID] != testVideoID {
		t.Errorf("created videoId = %v, want %q", created.Attributes[meta.AttrVideoID], testVideoID)
	}
}

func TestAdCreativeManagedVideoBlockedUntilReady(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	attrs := standardVideoCreativeAttrs()
	delete(attrs, meta.AttrVideoID)
	attrs[meta.AttrVideoRef] = resource.Resolved{
		Address:  videoAddress(t, "demo"),
		Identity: resource.Identity{ID: testVideoID},
		Outputs:  resource.Attributes{},
	}
	_, err := p.Create(context.Background(), creativeResource(t, "blocked", attrs))
	if err == nil || !strings.Contains(err.Error(), "finished processing") {
		t.Fatalf("error = %v, want video not ready error", err)
	}
}

func TestAdCreativeVideoRefMustTargetMetaVideo(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	attrs := standardVideoCreativeAttrs()
	delete(attrs, meta.AttrVideoID)
	attrs[meta.AttrVideoRef] = resource.Ref{Address: imageAddress(t, "hero")}
	err := p.Validate(context.Background(), creativeResource(t, "bad", attrs))
	if err == nil || !strings.Contains(err.Error(), "meta.video") {
		t.Fatalf("error = %v, want meta.video reference error", err)
	}
}

func TestAdCreativeVideoAndVideoIDAreMutuallyExclusive(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	attrs := standardVideoCreativeAttrs()
	attrs[meta.AttrVideoRef] = resource.Ref{Address: videoAddress(t, "demo")}
	err := p.Validate(context.Background(), creativeResource(t, "bad", attrs))
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("error = %v, want mutually exclusive error", err)
	}
}
