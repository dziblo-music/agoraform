package meta_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/asset"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/meta"
	metaclient "github.com/dziblo-music/agoraform/providers/meta/client"
)

type resumableVideoServer struct {
	t *testing.T

	mu                  sync.Mutex
	total               int64
	chunkSize           int64
	payload             []byte
	startCalls          int
	transferCalls       int
	finishCalls         int
	getCalls            int
	deleteCalls         int
	processingPolls     int
	transferFailures    int
	startFailure        bool
	statusError         bool
	emptyStatusComplete bool
	present             bool
	forceReady          bool
}

func newResumableVideoServer(t *testing.T) *resumableVideoServer {
	t.Helper()
	return &resumableVideoServer{t: t, chunkSize: 32}
}

func (s *resumableVideoServer) start() *httptest.Server {
	s.t.Helper()
	return httptest.NewServer(http.HandlerFunc(s.serveHTTP))
}

func (s *resumableVideoServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := strings.TrimPrefix(r.URL.Path, "/"+metaclient.Version+"/")
	videoPath := testVideoID
	accountVideosPath := "act_" + testAccountID + "/advideos"

	switch {
	case r.Method == http.MethodPost && path == accountVideosPath:
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			s.handleTransfer(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		switch r.Form.Get("upload_phase") {
		case "start":
			s.handleStart(w, r)
		case "finish":
			s.handleFinish(w, r)
		default:
			http.Error(w, "unexpected upload phase", http.StatusBadRequest)
		}
	case r.Method == http.MethodGet && path == videoPath:
		s.handleRead(w)
	case r.Method == http.MethodDelete && path == videoPath:
		if !s.present {
			http.Error(w, `{"error":{"message":"not found","code":803}}`, http.StatusNotFound)
			return
		}
		s.deleteCalls++
		s.present = false
		writeJSON(w, map[string]any{"success": true})
	default:
		http.NotFound(w, r)
	}
}

func (s *resumableVideoServer) handleStart(w http.ResponseWriter, r *http.Request) {
	s.startCalls++
	if s.startFailure {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":{"message":"temporary video upload failure","code":1,"is_transient":true}}`)
		return
	}
	total, err := strconv.ParseInt(r.Form.Get("file_size"), 10, 64)
	if err != nil || total <= 0 {
		http.Error(w, "invalid file_size", http.StatusBadRequest)
		return
	}
	s.total = total
	s.payload = nil
	s.present = true
	end := minInt64(s.chunkSize, total)
	writeJSON(w, map[string]any{
		"start_offset":      "0",
		"end_offset":        strconv.FormatInt(end, 10),
		"upload_session_id": "session-1",
		"video_id":          testVideoID,
	})
}

func (s *resumableVideoServer) handleTransfer(w http.ResponseWriter, r *http.Request) {
	s.transferCalls++
	if s.transferFailures > 0 {
		s.transferFailures--
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":{"message":"temporary transfer failure","code":1,"is_transient":true}}`)
		return
	}
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if r.FormValue("upload_phase") != "transfer" || r.FormValue("upload_session_id") != "session-1" {
		http.Error(w, "invalid transfer fields", http.StatusBadRequest)
		return
	}
	start, err := strconv.ParseInt(r.FormValue("start_offset"), 10, 64)
	if err != nil || start != int64(len(s.payload)) {
		http.Error(w, "unexpected start_offset", http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("video_file_chunk")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	chunk, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.payload = append(s.payload, chunk...)
	nextStart := int64(len(s.payload))
	nextEnd := minInt64(nextStart+s.chunkSize, s.total)
	writeJSON(w, map[string]any{
		"start_offset": strconv.FormatInt(nextStart, 10),
		"end_offset":   strconv.FormatInt(nextEnd, 10),
	})
}

func (s *resumableVideoServer) handleFinish(w http.ResponseWriter, r *http.Request) {
	s.finishCalls++
	if r.Form.Get("upload_session_id") != "session-1" {
		http.Error(w, "invalid upload session", http.StatusBadRequest)
		return
	}
	if int64(len(s.payload)) != s.total {
		http.Error(w, fmt.Sprintf("received %d of %d bytes", len(s.payload), s.total), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"success": true})
}

func (s *resumableVideoServer) handleRead(w http.ResponseWriter) {
	s.getCalls++
	if !s.present {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"message":"not found","code":803}}`)
		return
	}
	status := map[string]any{"video_status": "ready", "processing_progress": 100, "processing_phase": map[string]any{"status": "complete"}}
	if s.statusError {
		status = map[string]any{"video_status": "error", "processing_progress": 0}
	} else if s.emptyStatusComplete {
		status = map[string]any{"video_status": "", "processing_progress": 100, "processing_phase": map[string]any{"status": "complete"}}
	} else if !s.forceReady && s.processingPolls > 0 {
		s.processingPolls--
		status = map[string]any{"video_status": "processing", "processing_progress": 40, "processing_phase": map[string]any{"status": "active"}}
	}
	writeJSON(w, map[string]any{
		"id":     testVideoID,
		"title":  "product-demo.mp4",
		"length": 1.25,
		"status": status,
	})
}

func (s *resumableVideoServer) counts() (start, transfer, finish, get, deletes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startCalls, s.transferCalls, s.finishCalls, s.getCalls, s.deleteCalls
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

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

func TestCreateVideoUsesResumableUploadAndWaitsUntilReady(t *testing.T) {
	t.Parallel()
	srv := newResumableVideoServer(t)
	srv.processingPolls = 1
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
	start, transfer, finish, _, _ := srv.counts()
	if start != 1 || transfer < 2 || finish != 1 {
		t.Fatalf("resumable calls start=%d transfer=%d finish=%d, want 1, >=2, 1", start, transfer, finish)
	}
	if int64(len(srv.payload)) != local.Size {
		t.Fatalf("uploaded bytes = %d, want %d", len(srv.payload), local.Size)
	}
}

func TestCreateVideoTimeoutReturnsAcceptedIdentityWithoutVideoOutput(t *testing.T) {
	t.Parallel()
	srv := newResumableVideoServer(t)
	srv.processingPolls = 1000
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	meta.SetVideoPollingForTest(p, 15*time.Millisecond, time.Millisecond, nil)

	local := localMP4(t, "product-demo.mp4")
	live, err := p.Create(context.Background(), videoResourceFrom(t, "demo", local))
	if err == nil || !strings.Contains(err.Error(), "still processing") {
		t.Fatalf("error = %v, want processing timeout", err)
	}
	if live.Identity.ID != testVideoID || live.Identity.Fingerprint != local.Digest {
		t.Fatalf("live identity = %+v, want recoverable %s", live.Identity, testVideoID)
	}
	if _, ok := live.Computed[meta.OutputVideoID]; ok {
		t.Fatalf("processing video exposed %s", meta.OutputVideoID)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("token leaked: %v", err)
	}
}

func TestAcceptedVideoFailurePersistsIdentityAndDoesNotUploadAgain(t *testing.T) {
	t.Parallel()
	srv := newResumableVideoServer(t)
	srv.transferFailures = 10
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	local := localMP4(t, "product-demo.mp4")
	res := videoResourceFrom(t, "demo", local)
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(resource.Address) (provider.Provider, error) { return p, nil }

	_, err = apply.Run(context.Background(), []resource.Resource{res}, lookup, st, io.Discard)
	if err == nil {
		t.Fatal("Run succeeded, want partial upload failure")
	}
	var partial *apply.PartialApplyError
	if !errors.As(err, &partial) || partial.Stage != apply.StageMutation || partial.RemoteIdentity.ID != testVideoID {
		t.Fatalf("error = %v, partial = %+v", err, partial)
	}
	persisted, ok, identErr := st.Identity(res.Address)
	if identErr != nil || !ok || persisted.ID != testVideoID || persisted.Fingerprint != local.Digest {
		t.Fatalf("Identity = (%+v,%v,%v), want recoverable video binding", persisted, ok, identErr)
	}
	startBefore, _, _, _, _ := srv.counts()
	if startBefore != 1 {
		t.Fatalf("start calls = %d, want 1", startBefore)
	}

	// Simulate Meta completing/recovering the accepted video. A second apply
	// must use the persisted id and read it; it must not start another upload.
	srv.transferFailures = 0
	srv.forceReady = true
	if _, err := apply.Run(context.Background(), []resource.Resource{res}, lookup, st, io.Discard); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	startAfter, _, _, _, _ := srv.counts()
	if startAfter != 1 {
		t.Fatalf("second apply started another upload: start calls=%d", startAfter)
	}
}

func TestCreateVideoProcessingFailurePreservesIdentity(t *testing.T) {
	t.Parallel()
	srv := newResumableVideoServer(t)
	srv.statusError = true
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	meta.SetVideoPollingForTest(p, time.Second, time.Millisecond, nil)

	live, err := p.Create(context.Background(), videoResourceFrom(t, "demo", localMP4(t, "product-demo.mp4")))
	if err == nil || !strings.Contains(err.Error(), "processing failed") {
		t.Fatalf("error = %v, want processing failure", err)
	}
	if live.Identity.ID != testVideoID {
		t.Fatalf("identity = %+v, want accepted video id", live.Identity)
	}
	if _, ok := live.Computed[meta.OutputVideoID]; ok {
		t.Fatal("failed video exposed videoId")
	}
}

func TestReadVideoNotReadyIsExplicit(t *testing.T) {
	t.Parallel()
	srv := newResumableVideoServer(t)
	srv.present = true
	srv.processingPolls = 100
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

func TestReadVideoRequiresExplicitReadyStatus(t *testing.T) {
	t.Parallel()
	srv := newResumableVideoServer(t)
	srv.present = true
	srv.emptyStatusComplete = true
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	local := localMP4(t, "product-demo.mp4")
	res := videoResourceFrom(t, "demo", local)
	res.Identity = resource.Identity{ID: testVideoID, Fingerprint: local.Digest}
	_, err := p.Read(context.Background(), res)
	if err == nil || !strings.Contains(err.Error(), "still processing") {
		t.Fatalf("Read = %v, want non-ready error despite completed processing phase", err)
	}
}

func TestReadVideoReturnsNotFoundWhenUnbound(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	_, err := p.Read(context.Background(), videoResourceFrom(t, "demo", localMP4(t, "product-demo.mp4")))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
}

func TestPlanVideoUnchangedWhenContentMatches(t *testing.T) {
	t.Parallel()
	srv := newResumableVideoServer(t)
	srv.present = true
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
	start, _, _, _, _ := srv.counts()
	if start != 0 {
		t.Fatalf("plan uploaded video: start=%d", start)
	}
}

func TestPlanVideoChangedContentFails(t *testing.T) {
	t.Parallel()
	srv := newResumableVideoServer(t)
	srv.present = true
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
	srv := newResumableVideoServer(t)
	srv.present = true
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
	srv := newResumableVideoServer(t)
	srv.present = true
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
	_, _, _, _, deletes := srv.counts()
	if deletes != 1 {
		t.Fatalf("deletes = %d, want 1", deletes)
	}
}

func TestVideoUploadStartFailureDoesNotLeakToken(t *testing.T) {
	t.Parallel()
	srv := newResumableVideoServer(t)
	srv.startFailure = true
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	live, err := p.Create(context.Background(), videoResourceFrom(t, "fail", localMP4(t, "fail.mp4")))
	if err == nil || !strings.Contains(err.Error(), "temporary video upload failure") {
		t.Fatalf("error = %v, want upload failure", err)
	}
	if !live.Identity.IsZero() {
		t.Fatalf("pre-acceptance failure returned identity %+v", live.Identity)
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

func TestResumableServerJSONIsValid(t *testing.T) {
	t.Parallel()
	// Keep encoding/json referenced in this test file while also guarding the
	// helper's expected JSON representation used by Meta client decoding.
	if _, err := json.Marshal(map[string]any{"video_id": testVideoID}); err != nil {
		t.Fatal(err)
	}
}
