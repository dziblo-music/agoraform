package meta_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/meta"
)

func TestPixelOutputs(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	specs := p.Outputs(meta.TypePixel)
	id, ok := provider.FindOutput(specs, meta.OutputPixelID)
	if !ok || id.Sensitive || id.Kind != provider.OutputKindString {
		t.Fatalf("pixelId spec = (%v, %v)", id, ok)
	}
	if _, ok := provider.FindOutput(specs, "code"); ok {
		t.Fatal("pixel code must not be a selectable output")
	}
	if _, ok := provider.FindOutput(specs, "id"); ok {
		t.Fatal("raw id must not be a selectable output")
	}
}

func TestValidatePixel(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	if err := p.Validate(context.Background(), pixelResource(t, "website")); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	res := pixelResource(t, "website")
	res.Attributes["code"] = "fbq('init')"
	if err := p.Validate(context.Background(), res); err == nil || !strings.Contains(err.Error(), "computed") {
		t.Fatalf("code should be rejected: %v", err)
	}
	res = pixelResource(t, "website")
	delete(res.Attributes, meta.AttrName)
	if err := p.Validate(context.Background(), res); err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("missing name: %v", err)
	}
}

func TestReadPixelBoundAndUnbound(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedPixel(testPixelID, "Website")
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	unbound := pixelResource(t, "website")
	_, err := p.Read(context.Background(), unbound)
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("unbound Read = %v, want ErrNotFound", err)
	}

	bound := unbound
	bound.Identity = resource.Identity{ID: testPixelID}
	live, err := p.Read(context.Background(), bound)
	if err != nil {
		t.Fatal(err)
	}
	if live.Identity.ID != testPixelID || live.Computed[meta.OutputPixelID] != testPixelID {
		t.Fatalf("live = %+v", live)
	}
	if _, ok := live.Attributes["code"]; ok || live.Computed["code"] != nil {
		t.Fatal("pixel snippet leaked into attributes or computed")
	}
	posts, deletes := srv.mutationCounts()
	if posts != 0 || deletes != 0 {
		t.Fatalf("read mutated remote state posts=%d deletes=%d", posts, deletes)
	}
}

func TestAdoptPixelUniqueName(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedPixel(testPixelID, "Website")
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	mode, err := p.PlanMissingResource(pixelResource(t, "website"))
	if err != nil || mode != provider.MissingResourceAdopt {
		t.Fatalf("PlanMissingResource = (%q, %v)", mode, err)
	}
	live, err := p.Create(context.Background(), pixelResource(t, "website"))
	if err != nil {
		t.Fatal(err)
	}
	if live.Identity.ID != testPixelID {
		t.Fatalf("adopted id = %q", live.Identity.ID)
	}
	posts, deletes := srv.mutationCounts()
	if posts != 0 || deletes != 0 {
		t.Fatalf("adopt mutated remote pixel posts=%d deletes=%d", posts, deletes)
	}
}

func TestAdoptPixelMissingAndAmbiguous(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedPixel("1", "Website")
	srv.seedPixel("2", "Website")
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	_, err := p.Create(context.Background(), pixelResource(t, "website"))
	if err == nil || !strings.Contains(err.Error(), "refusing to guess") {
		t.Fatalf("ambiguous adopt = %v", err)
	}

	missing := pixelResource(t, "website")
	missing.Attributes[meta.AttrName] = "Other"
	_, err = p.Create(context.Background(), missing)
	if err == nil || !strings.Contains(err.Error(), "does not create") {
		t.Fatalf("missing adopt = %v", err)
	}
}

func TestImportPixelAndNoOpPlan(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedPixel(testPixelID, "Website")
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	live, err := p.Import(context.Background(), pixelAddress(t, "website"), testPixelID)
	if err != nil {
		t.Fatal(err)
	}
	if live.Identity.ID != testPixelID {
		t.Fatalf("imported id = %q", live.Identity.ID)
	}

	st, err := state.Load(t.TempDir() + "/agoraform.state.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(live.Address, live.Identity); err != nil {
		t.Fatal(err)
	}
	got, err := plan.BuildWithState(context.Background(), []resource.Resource{pixelResource(t, "website")}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatal(err)
	}
	if got.HasChanges() {
		t.Fatalf("imported pixel produced changes:\n%s", plan.Format(got))
	}
}

func TestImportPixelMissing(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	_, err := p.Import(context.Background(), pixelAddress(t, "website"), testPixelID)
	if err == nil || !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("missing import = %v", err)
	}
}

func TestImportPixelRejectsObjectOutsideConfiguredAccount(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedForeignPixel(testPixelID, "Other Account Website")
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	_, err := p.Import(context.Background(), pixelAddress(t, "website"), testPixelID)
	if err == nil || !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("foreign-account pixel import = %v, want ErrNotFound", err)
	}
}

func TestUpdatePixelNameIsUnsupported(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedPixel(testPixelID, "Website")
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	desired := pixelResource(t, "website")
	desired.Attributes[meta.AttrName] = "Renamed"
	_, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:    desired.Address,
		Identity:   resource.Identity{ID: testPixelID},
		Attributes: resource.Attributes{meta.AttrName: "Website"},
	})
	if err == nil || !strings.Contains(err.Error(), "not updated") {
		t.Fatalf("rename = %v", err)
	}
}

func TestPixelAPIErrorRedactsToken(t *testing.T) {
	t.Parallel()
	httpSrv := newGraphServer(t).start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	bound := pixelResource(t, "website")
	bound.Identity = resource.Identity{ID: "404404404"}
	err := error(nil)
	_, err = p.Read(context.Background(), bound)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("token leaked in %q", err)
	}
}
