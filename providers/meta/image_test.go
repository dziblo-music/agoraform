package meta_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/meta"
)

// writeTestImage creates a small JPEG file with the given content in dir.
func writeTestImage(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func sha256HexOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestValidateImageAcceptsValidFile(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	dir := t.TempDir()
	path := writeTestImage(t, dir, "hero.jpg", []byte("fake jpeg content"))
	res := resource.Resource{
		Address:    imageAddress(t, "hero"),
		Attributes: resource.Attributes{meta.AttrFile: path},
	}
	if err := p.Validate(context.Background(), res); err != nil {
		t.Fatalf("Validate = %v, want nil", err)
	}
}

func TestValidateImageRejectsUnsupportedExtension(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	dir := t.TempDir()
	path := writeTestImage(t, dir, "hero.bmp", []byte("fake bmp"))
	err := p.Validate(context.Background(), resource.Resource{
		Address:    imageAddress(t, "hero"),
		Attributes: resource.Attributes{meta.AttrFile: path},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported file extension") {
		t.Fatalf("error = %v, want unsupported extension error", err)
	}
}

func TestValidateImageRejectsMissingFile(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	err := p.Validate(context.Background(), resource.Resource{
		Address:    imageAddress(t, "hero"),
		Attributes: resource.Attributes{meta.AttrFile: "does_not_exist.jpg"},
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v, want file not found error", err)
	}
}

func TestValidateImageRejectsUnknownAttributes(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	dir := t.TempDir()
	path := writeTestImage(t, dir, "hero.jpg", []byte("fake jpeg"))
	err := p.Validate(context.Background(), resource.Resource{
		Address: imageAddress(t, "hero"),
		Attributes: resource.Attributes{
			meta.AttrFile:  path,
			"unknownField": "value",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported attribute") {
		t.Fatalf("error = %v, want unsupported attribute error", err)
	}
}

func TestValidateImageRejectsComputedAttributes(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	dir := t.TempDir()
	path := writeTestImage(t, dir, "hero.jpg", []byte("fake jpeg"))
	err := p.Validate(context.Background(), resource.Resource{
		Address: imageAddress(t, "hero"),
		Attributes: resource.Attributes{
			meta.AttrFile:      path,
			meta.AttrImageHash: "shouldnotbeset",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "computed") {
		t.Fatalf("error = %v, want computed field error", err)
	}
}

func TestCreateImageUploadsFileAndPersistsIdentity(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	dir := t.TempDir()
	content := []byte("fake jpeg image bytes")
	path := writeTestImage(t, dir, "trial.jpg", content)
	expectedSHA256 := sha256HexOf(content)

	res := resource.Resource{
		Address:    imageAddress(t, "trial_ad"),
		Attributes: standardImageAttrs(path),
	}
	created, err := p.Create(context.Background(), res)
	if err != nil {
		t.Fatal(err)
	}

	// State identity should be the sha256.
	if created.Identity.ID != expectedSHA256 {
		t.Errorf("Identity.ID = %q, want sha256 %q", created.Identity.ID, expectedSHA256)
	}
	// Fingerprint should be the Meta image hash.
	if created.Identity.Fingerprint != testImageHash {
		t.Errorf("Identity.Fingerprint = %q, want Meta image hash %q", created.Identity.Fingerprint, testImageHash)
	}
	// Computed should expose imageHash for creative resolution.
	if got := created.Computed[meta.OutputImageHash]; got != testImageHash {
		t.Errorf("Computed[imageHash] = %v, want %q", got, testImageHash)
	}
	// Attributes["file"] should be the sha256 for plan comparison.
	if got := created.Attributes[meta.AttrFile]; got != expectedSHA256 {
		t.Errorf("Attributes[file] = %v, want sha256 %q", got, expectedSHA256)
	}

	posts, _ := srv.mutationCounts()
	if posts != 1 {
		t.Errorf("posts = %d, want 1 (the upload)", posts)
	}
}

func TestReadImageReturnsSHA256AndMetaHash(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	dir := t.TempDir()
	content := []byte("stable image content")
	path := writeTestImage(t, dir, "hero.png", content)
	sha256hex := sha256HexOf(content)

	res := resource.Resource{
		Address:    imageAddress(t, "hero"),
		Attributes: standardImageAttrs(path),
		Identity: resource.Identity{
			ID:          sha256hex,
			Fingerprint: testImageHash,
		},
	}
	live, err := p.Read(context.Background(), res)
	if err != nil {
		t.Fatal(err)
	}
	if live.Identity.ID != sha256hex {
		t.Errorf("live.Identity.ID = %q, want %q", live.Identity.ID, sha256hex)
	}
	if live.Computed[meta.OutputImageHash] != testImageHash {
		t.Errorf("live.Computed[imageHash] = %v, want %q", live.Computed[meta.OutputImageHash], testImageHash)
	}
	// live.Attributes["file"] should equal the stored sha256 for plan comparison.
	if live.Attributes[meta.AttrFile] != sha256hex {
		t.Errorf("live.Attributes[file] = %v, want sha256 %q", live.Attributes[meta.AttrFile], sha256hex)
	}

	// Read must not mutate Meta.
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

	dir := t.TempDir()
	path := writeTestImage(t, dir, "hero.jpg", []byte("content"))

	res := resource.Resource{
		Address:    imageAddress(t, "hero"),
		Attributes: standardImageAttrs(path),
		// No Identity: unbound
	}
	_, err := p.Read(context.Background(), res)
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

	dir := t.TempDir()
	path := writeTestImage(t, dir, "trial.jpg", []byte("image bytes"))

	resources := []resource.Resource{{
		Address:    imageAddress(t, "trial_ad"),
		Attributes: standardImageAttrs(path),
	}}
	got, err := plan.Build(context.Background(), resources, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionCreate {
		t.Fatalf("changes = %#v, want one create", got.Changes)
	}
	// Plan must not upload.
	posts, _ := srv.mutationCounts()
	if posts != 0 {
		t.Fatalf("plan uploaded to Meta: posts=%d", posts)
	}
}

func TestPlanImageUnchangedWhenContentMatches(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	dir := t.TempDir()
	content := []byte("stable image bytes")
	path := writeTestImage(t, dir, "stable.jpg", content)
	sha256hex := sha256HexOf(content)

	st, err := state.Load(filepath.Join(dir, "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	addr := imageAddress(t, "stable")
	if err := st.Bind(addr, resource.Identity{ID: sha256hex, Fingerprint: testImageHash}); err != nil {
		t.Fatal(err)
	}

	resources := []resource.Resource{{Address: addr, Attributes: standardImageAttrs(path)}}
	got, err := plan.BuildWithState(context.Background(), resources, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionUnchanged {
		t.Fatalf("changes = %#v, want unchanged", got.Changes)
	}
	// Plan must not upload.
	posts, _ := srv.mutationCounts()
	if posts != 0 {
		t.Fatalf("plan uploaded to Meta: posts=%d", posts)
	}
}

func TestPlanImageDetectsContentChange(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	dir := t.TempDir()

	// Write the OLD content whose sha256 is stored in state.
	oldContent := []byte("old image bytes")
	oldSHA256 := sha256HexOf(oldContent)

	// The local file now has NEW content.
	newContent := []byte("new image bytes — completely different")
	path := writeTestImage(t, dir, "changed.png", newContent)

	st, err := state.Load(filepath.Join(dir, "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	addr := imageAddress(t, "changed")
	// State holds the sha256 of the OLD content.
	if err := st.Bind(addr, resource.Identity{ID: oldSHA256, Fingerprint: testImageHash}); err != nil {
		t.Fatal(err)
	}

	resources := []resource.Resource{{Address: addr, Attributes: standardImageAttrs(path)}}
	got, err := plan.BuildWithState(context.Background(), resources, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatal(err)
	}
	// Content changed → plan must show an update.
	if len(got.Changes) != 1 || got.Changes[0].Action != plan.ActionUpdate {
		t.Fatalf("changes = %#v, want update for content change", got.Changes)
	}
	// Plan must not upload.
	posts, _ := srv.mutationCounts()
	if posts != 0 {
		t.Fatalf("plan uploaded to Meta: posts=%d", posts)
	}
}

func TestDestroyImageRemovesStateBindingOnly(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	dir := t.TempDir()
	content := []byte("image to remove")
	path := writeTestImage(t, dir, "bye.jpg", content)
	sha256hex := sha256HexOf(content)

	res := resource.Resource{
		Address:    imageAddress(t, "bye"),
		Attributes: standardImageAttrs(path),
		Identity:   resource.Identity{ID: sha256hex, Fingerprint: testImageHash},
	}
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
	// Destroy must not contact Meta.
	posts, deletes := srv.mutationCounts()
	if posts != 0 || deletes != 0 {
		t.Fatalf("Destroy mutated Meta: posts=%d deletes=%d", posts, deletes)
	}
}

func TestImportImageIsUnsupported(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	_, err := p.Import(context.Background(), imageAddress(t, "external"), "someHash")
	if err == nil || !strings.Contains(err.Error(), "create-managed") {
		t.Fatalf("import error = %v, want create-managed error", err)
	}
}

func TestImageUploadFailureDoesNotLeakToken(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.imageUploadFailure = true
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	dir := t.TempDir()
	path := writeTestImage(t, dir, "fail.jpg", []byte("fail content"))

	_, err := p.Create(context.Background(), resource.Resource{
		Address:    imageAddress(t, "fail"),
		Attributes: standardImageAttrs(path),
	})
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
	// imageHash is set by standardImageCreativeAttrs; also set image ref.
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
	// Point image ref to a campaign instead of an image.
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
	// At apply time the image ref is resolved with outputs.
	attrs[meta.AttrImageRef] = resource.Resolved{
		Address:  addr,
		Identity: resource.Identity{ID: "abc123sha256", Fingerprint: testImageHash},
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
