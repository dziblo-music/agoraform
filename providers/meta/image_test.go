package meta_test

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/asset"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/meta"
)

func TestValidateImageAcceptsValidFile(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	if err := p.Validate(context.Background(), imageResource(t, "hero")); err != nil {
		t.Fatalf("Validate = %v, want nil", err)
	}
}

func TestValidateImageRejectsUnsupportedType(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	local := resource.NewLocalAsset("hero.bmp", "abc", 12, "image/bmp", func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("not-an-image")), nil
	})
	err := p.Validate(context.Background(), imageResourceFrom(t, "hero", local))
	if err == nil || !strings.Contains(err.Error(), "JPEG, PNG, or GIF") {
		t.Fatalf("error = %v, want unsupported type error", err)
	}
}

func TestValidateImageRejectsMissingSource(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	err := p.Validate(context.Background(), resource.Resource{Address: imageAddress(t, "hero")})
	if err == nil || !strings.Contains(err.Error(), "source.file") {
		t.Fatalf("error = %v, want source.file error", err)
	}
}

func TestValidateImageRejectsUnknownAttributes(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	res := imageResource(t, "hero")
	res.Attributes["unknownField"] = "value"
	err := p.Validate(context.Background(), res)
	if err == nil || !strings.Contains(err.Error(), "unsupported attribute") {
		t.Fatalf("error = %v, want unsupported attribute error", err)
	}
}

func TestValidateImageRejectsComputedAttributes(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	res := imageResource(t, "hero")
	res.Attributes[meta.AttrImageHash] = "shouldnotbeset"
	err := p.Validate(context.Background(), res)
	if err == nil || !strings.Contains(err.Error(), "computed") {
		t.Fatalf("error = %v, want computed field error", err)
	}
}

func TestValidateImageRejectsOversizedFile(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	local := resource.NewLocalAsset("huge.jpg", "abc", 31*1024*1024, "image/jpeg", func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("")), nil
	})
	err := p.Validate(context.Background(), imageResourceFrom(t, "hero", local))
	if err == nil || !strings.Contains(err.Error(), "30 MB") {
		t.Fatalf("error = %v, want size limit error", err)
	}
}

func TestCreateImageUploadsFileAndPersistsIdentity(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	local := localJPEG(t, "trial.jpg", 64, 64)
	res := imageResourceFrom(t, "trial_ad", local)
	created, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatal(err)
	}
	if created.Identity.ID != testImageHash {
		t.Errorf("Identity.ID = %q, want Meta image hash %q", created.Identity.ID, testImageHash)
	}
	if created.Identity.Fingerprint != local.Digest {
		t.Errorf("Identity.Fingerprint = %q, want digest %q", created.Identity.Fingerprint, local.Digest)
	}
	if got := created.Computed[meta.OutputImageHash]; got != testImageHash {
		t.Errorf("Computed[imageHash] = %v, want %q", got, testImageHash)
	}
	if _, ok := created.Attributes[asset.AttrName]; ok {
		t.Fatal("source path must not be stored in comparable attributes")
	}
	posts, _ := srv.mutationCounts()
	if posts != 1 {
		t.Errorf("posts = %d, want 1 (the upload)", posts)
	}
}

func TestReadImageReturnsHashAndDigest(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedImage(testImageHash, nil)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	local := localJPEG(t, "hero.jpg", 64, 64)
	res := imageResourceFrom(t, "hero", local)
	res.Identity = resource.Identity{ID: testImageHash, Fingerprint: local.Digest}
	live, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatal(err)
	}
	if live.Identity.ID != testImageHash {
		t.Errorf("live.Identity.ID = %q, want %q", live.Identity.ID, testImageHash)
	}
	if live.Identity.Fingerprint != local.Digest {
		t.Errorf("live.Identity.Fingerprint = %q, want digest %q", live.Identity.Fingerprint, local.Digest)
	}
	if live.Computed[meta.OutputImageHash] != testImageHash {
		t.Errorf("live.Computed[imageHash] = %v, want %q", live.Computed[meta.OutputImageHash], testImageHash)
	}
	posts, deletes := srv.mutationCounts()
	if posts != 0 || deletes != 0 {
		t.Errorf("Read mutated Meta: posts=%d deletes=%d", posts, deletes)
	}
}

func TestReadImageReturnsNotFoundWhenUnbound(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	_, err := p.Read(context.Background(), imageResource(t, "hero"))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
}

func TestPlanDetectsNewImageAsCreate(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	res := imageResource(t, "trial_ad")
	got, err := plan.Build(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionCreate {
		t.Fatalf("changes = %#v, want one create", got.Changes)
	}
	rendered := plan.Format(got)
	if !strings.Contains(rendered, `source.file: "trial_ad.jpg"`) {
		t.Fatalf("plan missing relative path:\n%s", rendered)
	}
	if !strings.Contains(rendered, "source.digest:") || !strings.Contains(rendered, "sha256:") {
		t.Fatalf("plan missing digest:\n%s", rendered)
	}
	posts, _ := srv.mutationCounts()
	if posts != 0 {
		t.Fatalf("plan uploaded to Meta: posts=%d", posts)
	}
}

func TestPlanImageUnchangedWhenContentMatches(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedImage(testImageHash, nil)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	local := localJPEG(t, "stable.jpg", 64, 64)
	res := imageResourceFrom(t, "stable", local)
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(res.Address, resource.Identity{ID: testImageHash, Fingerprint: local.Digest}); err != nil {
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
		t.Fatalf("plan uploaded to Meta: posts=%d", posts)
	}
}

func TestPlanImageChangedContentFails(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedImage(testImageHash, nil)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	local := localJPEG(t, "changed.jpg", 64, 64)
	res := imageResourceFrom(t, "changed", local)
	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(res.Address, resource.Identity{ID: testImageHash, Fingerprint: strings.Repeat("ab", 32)}); err != nil {
		t.Fatal(err)
	}

	_, err = plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("plan = %v, want immutable content guidance", err)
	}
	posts, _ := srv.mutationCounts()
	if posts != 0 {
		t.Fatalf("changed content uploaded: posts=%d", posts)
	}
}

func TestDestroyImageRemovesStateBindingOnly(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	local := localJPEG(t, "bye.jpg", 64, 64)
	res := imageResourceFrom(t, "bye", local)
	res.Identity = resource.Identity{ID: testImageHash, Fingerprint: local.Digest}
	capability, err := p.DestroyCapability(res)
	if err != nil {
		t.Fatal(err)
	}
	if capability != provider.DestroyProviderOwned {
		t.Fatalf("capability = %q, want DestroyProviderOwned", capability)
	}
	result, err := p.Destroy(context.Background(), res)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != provider.DestroyStatusAlreadyAbsent {
		t.Fatalf("status = %q, want AlreadyAbsent (provider-owned)", result.Status)
	}
	posts, deletes := srv.mutationCounts()
	if posts != 0 || deletes != 0 {
		t.Fatalf("Destroy mutated Meta: posts=%d deletes=%d", posts, deletes)
	}
}

func TestImportImageBindsHashWithoutFabricatingSource(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedImage(testImageHash, nil)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	live, err := p.Import(context.Background(), imageAddress(t, "external"), testImageHash)
	if err != nil {
		t.Fatal(err)
	}
	if live.Identity.ID != testImageHash {
		t.Fatalf("id = %q", live.Identity.ID)
	}
	if live.Identity.Fingerprint != "" {
		t.Fatalf("import fabricated content digest %q", live.Identity.Fingerprint)
	}
	if _, ok := live.Attributes[asset.AttrName]; ok {
		t.Fatal("import fabricated a local source")
	}
	if live.Computed[meta.OutputImageHash] != testImageHash {
		t.Fatalf("Computed[imageHash] = %v", live.Computed[meta.OutputImageHash])
	}
}

func TestImageUploadFailureDoesNotLeakToken(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.imageUploadFailure = true
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	_, err := p.Create(context.Background(), imageResource(t, "fail"))
	if err == nil || !strings.Contains(err.Error(), "temporary image upload failure") {
		t.Fatalf("error = %v, want upload failure", err)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("access token leaked in error: %v", err)
	}
}

func TestAdCreativeManagedImageRefValidates(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	attrs := standardImageCreativeAttrs()
	delete(attrs, meta.AttrImageHash)
	addr, err := resource.ParseAddress("meta.image.trial_ad")
	if err != nil {
		t.Fatal(err)
	}
	attrs[meta.AttrImageRef] = resource.Ref{Address: addr}
	err = p.Validate(context.Background(), creativeResource(t, "instagram", attrs))
	if err != nil {
		t.Fatalf("Validate = %v, want nil", err)
	}
}

func TestAdCreativeImageAndImageHashAreMutuallyExclusive(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	attrs := standardImageCreativeAttrs()
	addr, _ := resource.ParseAddress("meta.image.trial_ad")
	attrs[meta.AttrImageRef] = resource.Ref{Address: addr}
	err := p.Validate(context.Background(), creativeResource(t, "bad", attrs))
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("error = %v, want mutually exclusive error", err)
	}
}

func TestAdCreativeImageRefMustTargetMetaImage(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	attrs := standardImageCreativeAttrs()
	delete(attrs, meta.AttrImageHash)
	campaignAddr, _ := resource.ParseAddress("meta.campaign.some_campaign")
	attrs[meta.AttrImageRef] = resource.Ref{Address: campaignAddr}
	err := p.Validate(context.Background(), creativeResource(t, "bad", attrs))
	if err == nil || !strings.Contains(err.Error(), "meta.image") {
		t.Fatalf("error = %v, want meta.image reference error", err)
	}
}

func TestAdCreativeManagedImageCreatesWithResolvedHash(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	addr, _ := resource.ParseAddress("meta.image.trial_ad")
	attrs := standardImageCreativeAttrs()
	delete(attrs, meta.AttrImageHash)
	attrs[meta.AttrImageRef] = resource.Resolved{
		Address:  addr,
		Identity: resource.Identity{ID: testImageHash, Fingerprint: "digest"},
		Outputs:  resource.Attributes{meta.OutputImageHash: testImageHash},
	}
	created, err := p.Create(context.Background(), creativeResource(t, "instagram", attrs))
	if err != nil {
		t.Fatal(err)
	}
	if created.Attributes[meta.AttrImageHash] != testImageHash {
		t.Errorf("created imageHash = %v, want %q", created.Attributes[meta.AttrImageHash], testImageHash)
	}
}
