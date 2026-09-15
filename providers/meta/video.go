package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/dziblo-music/agoraform/internal/asset"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/meta/client"
)

const (
	videoDestroyGuidance = "DELETE /{video_id} only; Meta may reject deletion while a creative still references the video. Agoraform never activates ads as a side effect of media teardown."

	maxVideoBytes  = 4 * 1024 * 1024 * 1024 // 4 GB documented Marketing API video limit
	videoSizeLabel = "4 GB"

	defaultVideoReadyTimeout = 5 * time.Minute
	defaultVideoPollInterval = 2 * time.Second

	videoStatusReady      = "ready"
	videoStatusProcessing = "processing"
	videoStatusError      = "error"
)

var (
	supportedVideoAttrs = map[string]struct{}{
		asset.AttrName: {},
	}
	computedVideoAttrs = map[string]struct{}{
		AttrVideoID: {}, "id": {}, "title": {}, "length": {}, "picture": {},
		"status": {}, "videoStatus": {}, asset.AttrDigest: {},
	}
	supportedVideoMediaTypes = map[string]struct{}{
		"video/mp4": {}, "video/quicktime": {}, "video/x-m4v": {},
	}
	supportedVideoExts = map[string]struct{}{
		".mp4": {}, ".mov": {}, ".m4v": {},
	}
)

type adVideo struct {
	ID     string      `json:"id"`
	Title  string      `json:"title"`
	Length json.Number `json:"length"`
	Status videoStatus `json:"status"`
}

type videoStatus struct {
	VideoStatus        string     `json:"video_status"`
	ProcessingProgress int        `json:"processing_progress"`
	UploadingPhase     videoPhase `json:"uploading_phase"`
	ProcessingPhase    videoPhase `json:"processing_phase"`
	PublishingPhase    videoPhase `json:"publishing_phase"`
}

type videoPhase struct {
	Status string `json:"status"`
}

func (p *Provider) validateVideo(res resource.Resource) error {
	if err := p.requireAdAccount(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}
	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedVideoAttrs[key]; ok {
			continue
		}
		if _, computed := computedVideoAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; meta.video supports %s", res.Address, key, joinSorted(keys(supportedVideoAttrs)))
	}
	_, present, err := declaredSourceFile(res)
	if err != nil {
		return err
	}
	if present {
		if err := validateVideoLocalAsset(res); err != nil {
			return err
		}
	} else if res.Identity.IsZero() {
		return fmt.Errorf("resource %s: requires %s.%s naming a local file", res.Address, asset.AttrName, asset.AttrFile)
	} else if res.LocalAsset != nil {
		return fmt.Errorf("resource %s: requires %s.%s naming a local file", res.Address, asset.AttrName, asset.AttrFile)
	}
	if _, _, _, err := boundVideoIdentity(res); err != nil {
		return err
	}
	return nil
}

func validateVideoLocalAsset(res resource.Resource) error {
	if err := requireLocalAsset(res); err != nil {
		return err
	}
	local := res.LocalAsset
	if local.Size <= 0 {
		return fmt.Errorf("resource %s: local file %q is empty", res.Address, local.Path)
	}
	if local.Size > maxVideoBytes {
		return fmt.Errorf("resource %s: local file %q exceeds the %s size limit", res.Address, local.Path, videoSizeLabel)
	}
	media := strings.ToLower(strings.TrimSpace(local.MediaType))
	ext := strings.ToLower(path.Ext(local.Path))
	_, mediaOK := supportedVideoMediaTypes[media]
	_, extOK := supportedVideoExts[ext]
	if media == "application/octet-stream" {
		mediaOK = extOK
	}
	if !mediaOK && !extOK {
		return fmt.Errorf("resource %s: local file %q has unsupported type %q; Meta ad videos accept MP4 or MOV", res.Address, local.Path, local.MediaType)
	}
	if !extOK {
		return fmt.Errorf("resource %s: local file %q has unsupported file extension %q; supported: .mp4, .mov, .m4v", res.Address, local.Path, ext)
	}
	return nil
}

func boundVideoIdentity(res resource.Resource) (id, digest string, bound bool, err error) {
	if res.Identity.IsZero() {
		return "", "", false, nil
	}
	id, err = normalizeObjectID(res.Identity.ID)
	if err != nil {
		return "", "", true, fmt.Errorf("resource %s: persisted identity is invalid: %w", res.Address, err)
	}
	digest = strings.TrimSpace(res.Identity.Fingerprint)
	if err := validateContentDigest(res.Address, digest); err != nil {
		return "", "", true, err
	}
	return id, digest, true, nil
}

func (p *Provider) readVideo(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateVideo(res); err != nil {
		return resource.RemoteResource{}, err
	}
	id, digest, bound, err := boundVideoIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if !bound {
		return resource.RemoteResource{}, fmt.Errorf("meta: read %s: %w", res.Address, provider.ErrNotFound)
	}
	item, err := p.readAdVideo(ctx, id)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: read %s: %w", res.Address, err)
	}
	if err := requireVideoReady(res.Address, item, false); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: read %s: %w", res.Address, err)
	}
	return p.rememberVideoLive(remoteVideo(res.Address, item, digest, true)), nil
}

func (p *Provider) createVideo(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateVideo(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, _, bound, err := boundVideoIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}
	if err := requireLocalAsset(res); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: %w", res.Address, err)
	}
	local := res.LocalAsset
	filename := path.Base(local.Path)
	if filename == "" || filename == "." || filename == "/" {
		filename = "upload.mp4"
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	rawID, uploadErr := c.UploadVideoResumable(ctx, c.AdAccountID()+"/advideos", filename, local.Size, local.Open)
	if uploadErr != nil {
		if strings.TrimSpace(rawID) == "" {
			return resource.RemoteResource{}, fmt.Errorf("meta: create %s: upload failed before Meta returned a video id: %w", res.Address, uploadErr)
		}
		id, normalizeErr := normalizeObjectID(rawID)
		if normalizeErr != nil {
			id = strings.TrimSpace(rawID)
			uploadErr = fmt.Errorf("%v; Meta returned invalid recovery video id %q: %w", uploadErr, rawID, normalizeErr)
		}
		live := pendingVideoRemote(res.Address, id, localContentFingerprint(res))
		return live, fmt.Errorf("meta: create %s: Meta accepted video %s but the resumable upload did not converge: %w", res.Address, id, uploadErr)
	}
	id, err := normalizeObjectID(rawID)
	if err != nil {
		live := pendingVideoRemote(res.Address, strings.TrimSpace(rawID), localContentFingerprint(res))
		return live, fmt.Errorf("meta: create %s: Meta accepted the upload but returned an invalid video id %q: %w", res.Address, rawID, err)
	}

	item, err := p.waitForVideoReady(ctx, id)
	if err != nil {
		if strings.TrimSpace(item.ID) == "" {
			item.ID = id
		}
		live := remoteVideo(res.Address, item, localContentFingerprint(res), false)
		return live, fmt.Errorf("meta: create %s: video %s was accepted by Meta but is not ready: %w", res.Address, id, err)
	}
	return p.rememberVideoLive(remoteVideo(res.Address, item, localContentFingerprint(res), true)), nil
}

func pendingVideoRemote(addr resource.Address, id, digest string) resource.RemoteResource {
	return resource.RemoteResource{
		Address:  addr,
		Identity: resource.Identity{ID: strings.TrimSpace(id), Fingerprint: strings.TrimSpace(digest)},
		Computed: resource.Attributes{},
	}
}

func (p *Provider) updateVideo(_ context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateVideo(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if err := rejectChangedLocalContent(desired, &actual, videoReplaceGuidance); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: %w", desired.Address, err)
	}
	return resource.RemoteResource{}, fmt.Errorf("meta: update %s: %s", desired.Address, videoReplaceGuidance)
}

func (p *Provider) importVideo(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	id, err := p.canonicalCustomConversionImportID(addr, rawID)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireConfig(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: import %s: %w", addr, err)
	}
	item, err := p.readAdVideo(ctx, id)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("meta: import %s: remote video %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("meta: import %s: %w", addr, err)
	}
	if err := requireVideoReady(addr, item, true); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: import %s: %w", addr, err)
	}
	ready := videoIsReady(item)
	return p.rememberVideoLive(remoteVideo(addr, item, "", ready)), nil
}

func (p *Provider) normalizeVideoComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	if err := p.validateVideo(desired); err != nil {
		return nil, nil, err
	}
	if err := rejectChangedLocalContent(desired, live, videoReplaceGuidance); err != nil {
		return nil, nil, err
	}
	if live == nil {
		return resource.Attributes{}, nil, nil
	}
	if id, _, bound, err := boundVideoIdentity(desired); err != nil {
		return nil, nil, err
	} else if bound && (live.Identity.IsZero() || live.Identity.ID != id) {
		return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, id, live.Identity.ID)
	}
	return resource.Attributes{}, resource.Attributes{}, nil
}

func (p *Provider) readAdVideo(ctx context.Context, id string) (adVideo, error) {
	c, err := p.Client()
	if err != nil {
		return adVideo{}, err
	}
	var item adVideo
	if err := c.Get(ctx, id, url.Values{"fields": {"id,title,length,status"}}, &item); err != nil {
		if client.IsNotFound(err) {
			return adVideo{}, provider.ErrNotFound
		}
		return adVideo{}, err
	}
	got, err := normalizeObjectID(item.ID)
	if err != nil {
		return adVideo{}, fmt.Errorf("remote video id is invalid: %w", err)
	}
	if got != id {
		return adVideo{}, fmt.Errorf("remote video id %s does not match requested %s", got, id)
	}
	item.ID = got
	return item, nil
}

func (p *Provider) waitForVideoReady(ctx context.Context, id string) (adVideo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := p.videoReadyTimeout
	if timeout <= 0 {
		timeout = defaultVideoReadyTimeout
	}
	interval := p.videoPollInterval
	if interval <= 0 {
		interval = defaultVideoPollInterval
	}
	sleep := p.sleep
	if sleep == nil {
		sleep = sleepContext
	}

	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	last := adVideo{ID: id}

	for {
		item, err := p.readAdVideo(ctx, id)
		if err != nil {
			return last, err
		}
		last = item
		if videoIsReady(item) {
			return item, nil
		}
		if videoHasError(item) {
			return item, fmt.Errorf("Meta reported video processing failed")
		}
		if !time.Now().Before(deadline) {
			return item, fmt.Errorf("still processing after %s", timeout)
		}
		if err := sleep(ctx, interval); err != nil {
			return item, err
		}
	}
}

func (p *Provider) destroyVideo(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	id, _, bound, err := boundVideoIdentity(res)
	if err != nil {
		return provider.DestroyResult{}, err
	}
	if !bound {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: missing persisted identity", res.Address)
	}
	if _, err := p.readAdVideo(ctx, id); errors.Is(err, provider.ErrNotFound) {
		return provider.DestroyResult{Status: provider.DestroyStatusAlreadyAbsent}, nil
	} else if err != nil {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: %w", res.Address, err)
	}
	c, err := p.Client()
	if err != nil {
		return provider.DestroyResult{}, err
	}
	var result map[string]any
	if err := c.Delete(ctx, id, nil, &result); err != nil {
		if client.IsNotFound(err) {
			return provider.DestroyResult{Status: provider.DestroyStatusAlreadyAbsent}, nil
		}
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: %w", res.Address, err)
	}
	if success, ok := result["success"].(bool); ok && !success {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: API did not report success", res.Address)
	}
	if _, err := p.readAdVideo(ctx, id); errors.Is(err, provider.ErrNotFound) {
		return provider.DestroyResult{Status: provider.DestroyStatusDestroyed}, nil
	} else if err != nil {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: DELETE succeeded but confirming terminal state failed: %w", res.Address, err)
	}
	return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: video %s is still present after DELETE", res.Address, id)
}

func remoteVideo(addr resource.Address, item adVideo, digest string, ready bool) resource.RemoteResource {
	computed := resource.Attributes{}
	if ready {
		computed[OutputVideoID] = item.ID
	}
	if title := strings.TrimSpace(item.Title); title != "" {
		computed["title"] = title
	}
	if length := strings.TrimSpace(item.Length.String()); length != "" {
		computed["length"] = length
	}
	if status := strings.TrimSpace(item.Status.VideoStatus); status != "" {
		computed["videoStatus"] = status
	}
	return resource.RemoteResource{
		Address: addr,
		Identity: resource.Identity{
			ID:          item.ID,
			Fingerprint: digest,
		},
		Attributes: resource.Attributes{},
		Computed:   computed,
	}
}

func requireVideoReady(addr resource.Address, item adVideo, allowProcessing bool) error {
	if videoHasError(item) {
		return fmt.Errorf("remote video %s processing failed; Meta reported status %q", item.ID, videoStatusValue(item))
	}
	if videoIsReady(item) {
		return nil
	}
	if allowProcessing && videoIsProcessing(item) {
		return nil
	}
	return fmt.Errorf("uploaded video %s is still processing (status %q); retry after Meta finishes encoding", item.ID, videoStatusValue(item))
}

func videoIsReady(item adVideo) bool {
	return strings.EqualFold(strings.TrimSpace(item.Status.VideoStatus), videoStatusReady)
}

func videoIsProcessing(item adVideo) bool {
	status := strings.ToLower(strings.TrimSpace(item.Status.VideoStatus))
	return status == "" || status == videoStatusProcessing
}

func videoHasError(item adVideo) bool {
	return strings.EqualFold(strings.TrimSpace(item.Status.VideoStatus), videoStatusError)
}

func videoStatusValue(item adVideo) string {
	if s := strings.TrimSpace(item.Status.VideoStatus); s != "" {
		return s
	}
	if s := strings.TrimSpace(item.Status.ProcessingPhase.Status); s != "" {
		return s
	}
	return "unknown"
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
