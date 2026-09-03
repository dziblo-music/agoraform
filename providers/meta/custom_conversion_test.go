package meta_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/apply"
	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/meta"
)

func TestValidateCustomConversion(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	if err := p.Validate(context.Background(), conversionResource(t, "trial_started", websiteConversionAttrs(t))); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	invalid := websiteConversionAttrs(t)
	invalid[meta.AttrEventType] = "SIGNUP"
	if err := p.Validate(context.Background(), conversionResource(t, "trial_started", invalid)); err == nil || !strings.Contains(err.Error(), "eventType") {
		t.Fatalf("unsupported eventType: %v", err)
	}

	literal := websiteConversionAttrs(t)
	literal[meta.AttrPixel] = testPixelID
	if err := p.Validate(context.Background(), conversionResource(t, "trial_started", literal)); err == nil || !strings.Contains(err.Error(), "$ref") {
		t.Fatalf("literal pixel id: %v", err)
	}

	currency := websiteConversionAttrs(t)
	currency["currency"] = "USD"
	if err := p.Validate(context.Background(), conversionResource(t, "trial_started", currency)); err == nil || !strings.Contains(err.Error(), "computed") {
		t.Fatalf("currency: %v", err)
	}

	invalidRules := []any{
		[]any{map[string]any{"event": map[string]any{"eq": "StartTrial"}}},
		true,
		1,
		"null",
		`{"event":{"eq":"StartTrial"}} trailing`,
	}
	for _, rule := range invalidRules {
		attrs := websiteConversionAttrs(t)
		attrs[meta.AttrRule] = rule
		if err := p.Validate(context.Background(), conversionResource(t, "trial_started", attrs)); err == nil || !strings.Contains(err.Error(), "rule") {
			t.Fatalf("rule %#v should be rejected: %v", rule, err)
		}
	}
}

func TestCustomConversionOutputs(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	specs := p.Outputs(meta.TypeCustomConversion)
	id, ok := provider.FindOutput(specs, meta.OutputCustomConversionID)
	if !ok || id.Sensitive {
		t.Fatalf("customConversionId = (%v, %v)", id, ok)
	}
}

func TestCreateReadUpdateImportCustomConversion(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedPixel(testPixelID, "Website")
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	rememberPixel(t, p, testPixelID)

	desired := conversionResource(t, "trial_started", websiteConversionAttrs(t))
	desired.Attributes[meta.AttrPixel] = resource.Resolved{
		Address:  pixelAddress(t, "website"),
		Identity: resource.Identity{ID: testPixelID},
		Outputs:  resource.Attributes{meta.OutputPixelID: testPixelID},
	}
	created, err := p.Create(context.Background(), desired)
	if err != nil {
		t.Fatal(err)
	}
	if created.Identity.ID != testConvID {
		t.Fatalf("created id = %q", created.Identity.ID)
	}

	bound := desired
	bound.Identity = created.Identity
	bound.Attributes[meta.AttrPixel] = resource.Ref{Address: pixelAddress(t, "website")}
	live, err := p.Read(context.Background(), bound)
	if err != nil {
		t.Fatal(err)
	}
	if !sameRef(live.Attributes[meta.AttrPixel], pixelAddress(t, "website")) {
		t.Fatalf("pixel attr = %#v", live.Attributes[meta.AttrPixel])
	}

	renamed := bound
	renamed.Attributes = websiteConversionAttrs(t)
	renamed.Attributes[meta.AttrName] = "Trial Started Website"
	updated, err := p.Update(context.Background(), renamed, live)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Attributes[meta.AttrName] != "Trial Started Website" {
		t.Fatalf("name = %v", updated.Attributes[meta.AttrName])
	}

	noop, err := p.Update(context.Background(), renamed, updated)
	if err != nil {
		t.Fatal(err)
	}
	if noop.Identity.ID != testConvID {
		t.Fatalf("no-op id = %q", noop.Identity.ID)
	}
}

func TestUpdateCustomConversionRejectsImmutableFields(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedPixel(testPixelID, "Website")
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	actual := resource.RemoteResource{
		Address:  conversionAddress(t, "trial_started"),
		Identity: resource.Identity{ID: testConvID},
		Attributes: resource.Attributes{
			meta.AttrName:      "Trial Started",
			meta.AttrEventType: "START_TRIAL",
			meta.AttrRule:      websiteRule(),
			meta.AttrPixel:     resource.Ref{Address: pixelAddress(t, "website")},
		},
	}
	desired := conversionResource(t, "trial_started", websiteConversionAttrs(t))
	desired.Attributes[meta.AttrEventType] = "PURCHASE"
	if _, err := p.Update(context.Background(), desired, actual); err == nil || !strings.Contains(err.Error(), "eventType is immutable") {
		t.Fatalf("eventType update = %v", err)
	}

	desired = conversionResource(t, "trial_started", websiteConversionAttrs(t))
	desired.Attributes[meta.AttrRule] = map[string]any{"event": map[string]any{"eq": "Purchase"}}
	if _, err := p.Update(context.Background(), desired, actual); err == nil || !strings.Contains(err.Error(), "rule is immutable") {
		t.Fatalf("rule update = %v", err)
	}
}

func TestImportCustomConversionReconstructsUniquePixel(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedPixel(testPixelID, "Website")
	srv.seedConversion(testConvID, graphObject{
		"name":              "Trial Started",
		"custom_event_type": "START_TRIAL",
		"rule":              `{"and":[{"event":{"eq":"StartTrial"}}]}`,
		"pixel":             graphObject{"id": testPixelID},
		"event_source_type": "pixel",
	})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(staticCatalog{
		"meta/pixel/" + testPixelID: pixelAddress(t, "website"),
	})

	live, err := p.Import(context.Background(), conversionAddress(t, "trial_started"), testConvID)
	if err != nil {
		t.Fatal(err)
	}
	ref, ok := resource.AsRef(live.Attributes[meta.AttrPixel])
	if !ok || ref.Address != pixelAddress(t, "website") || ref.HasOutput() {
		t.Fatalf("pixel = %#v, want address-only $ref", live.Attributes[meta.AttrPixel])
	}
	if err := p.Validate(context.Background(), resource.Resource{Address: live.Address, Attributes: live.Attributes.Clone()}); err != nil {
		t.Fatalf("imported attributes must validate: %v", err)
	}
	posts, deletes := srv.mutationCounts()
	if posts != 0 || deletes != 0 {
		t.Fatalf("import mutated remote state posts=%d deletes=%d", posts, deletes)
	}

	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(pixelAddress(t, "website"), resource.Identity{ID: testPixelID}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(live.Address, live.Identity); err != nil {
		t.Fatal(err)
	}
	desired := []resource.Resource{
		pixelResource(t, "website"),
		{Address: live.Address, Attributes: live.Attributes.Clone()},
	}
	got, err := plan.BuildWithState(context.Background(), desired, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatal(err)
	}
	if got.HasChanges() {
		t.Fatalf("equivalent imported configuration produced changes:\n%s", plan.Format(got))
	}
}

func TestImportCustomConversionMissingPixelRelationship(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedConversion(testConvID, graphObject{
		"name":              "Trial Started",
		"custom_event_type": "START_TRIAL",
		"rule":              `{"and":[{"event":{"eq":"StartTrial"}}]}`,
		"pixel":             graphObject{"id": testPixelID},
		"event_source_type": "pixel",
	})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	_, err := p.Import(context.Background(), conversionAddress(t, "trial_started"), testConvID)
	if err == nil || !strings.Contains(err.Error(), "import that event source first") {
		t.Fatalf("missing pixel relationship = %v", err)
	}
}

func TestImportCustomConversionRejectsNonWebsiteSource(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedConversion(testConvID, graphObject{
		"name":              "App Install",
		"custom_event_type": "OTHER",
		"rule":              `{"event":{"eq":"fb_mobile_activate_app"}}`,
		"pixel":             graphObject{"id": "555"},
		"event_source_type": "app",
	})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	_, err := p.Import(context.Background(), conversionAddress(t, "app"), testConvID)
	if err == nil || !strings.Contains(err.Error(), "website Pixel/Dataset") {
		t.Fatalf("app source = %v", err)
	}
}

func TestEquivalentUnboundCustomConversionPlansCreate(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedPixel(testPixelID, "Website")
	srv.seedConversion(testConvID, graphObject{
		"name":              "Trial Started",
		"custom_event_type": "START_TRIAL",
		"rule":              `{"and":[{"event":{"eq":"StartTrial"}}]}`,
		"pixel":             graphObject{"id": testPixelID},
		"event_source_type": "pixel",
	})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(pixelAddress(t, "website"), resource.Identity{ID: testPixelID}); err != nil {
		t.Fatal(err)
	}
	desired := []resource.Resource{
		pixelResource(t, "website"),
		conversionResource(t, "trial_started", websiteConversionAttrs(t)),
	}
	got, err := plan.BuildWithState(context.Background(), desired, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range got.Changes {
		if change.Address == conversionAddress(t, "trial_started") {
			if change.Action != plan.ActionCreate {
				t.Fatalf("unbound custom conversion action = %q, want create", change.Action)
			}
			return
		}
	}
	t.Fatal("custom conversion change was not planned")
}

func TestDestroyCustomConversionArchives(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedConversion(testConvID, graphObject{
		"name":              "Trial Started",
		"custom_event_type": "START_TRIAL",
		"rule":              `{"and":[{"event":{"eq":"StartTrial"}}]}`,
		"pixel":             graphObject{"id": testPixelID},
		"event_source_type": "pixel",
	})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	p.SetIdentityCatalog(staticCatalog{"meta/pixel/" + testPixelID: pixelAddress(t, "website")})

	res := conversionResource(t, "trial_started", websiteConversionAttrs(t))
	res.Identity = resource.Identity{ID: testConvID}
	got, err := p.Destroy(context.Background(), res)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != provider.DestroyStatusRemoved {
		t.Fatalf("status = %q, want removed/archived", got.Status)
	}
	_, err = p.Read(context.Background(), res)
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("archived read = %v", err)
	}
	again, err := p.Destroy(context.Background(), res)
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != provider.DestroyStatusAlreadyAbsent {
		t.Fatalf("second destroy = %q", again.Status)
	}
}

func TestPixelDestroyIsProviderOwned(t *testing.T) {
	t.Parallel()
	p := meta.New(meta.Config{AccessToken: testToken, AdAccountID: testAccountID})
	res := pixelResource(t, "website")
	cap, err := p.DestroyCapability(res)
	if err != nil || cap != provider.DestroyProviderOwned {
		t.Fatalf("capability = (%q, %v)", cap, err)
	}
	if _, err := p.Destroy(context.Background(), res); err == nil || !strings.Contains(err.Error(), "never deletes") {
		t.Fatalf("pixel destroy = %v", err)
	}
}

func TestApplyCreateThenNoOpPlan(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedPixel(testPixelID, "Website")
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)

	st, err := state.Load(filepath.Join(t.TempDir(), "agoraform.state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(pixelAddress(t, "website"), resource.Identity{ID: testPixelID}); err != nil {
		t.Fatal(err)
	}
	desired := []resource.Resource{
		pixelResource(t, "website"),
		conversionResource(t, "trial_started", websiteConversionAttrs(t)),
	}
	lookup := func(resource.Address) (provider.Provider, error) { return p, nil }
	if _, err := apply.Run(context.Background(), desired, lookup, st, nil); err != nil {
		t.Fatal(err)
	}
	got, err := plan.BuildWithState(context.Background(), desired, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, st)
	if err != nil {
		t.Fatal(err)
	}
	if got.HasChanges() {
		t.Fatalf("post-apply plan changed:\n%s", plan.Format(got))
	}
}

func TestCustomConversionAPIErrorRedactsToken(t *testing.T) {
	t.Parallel()
	httpSrv := newGraphServer(t).start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	_, err := p.Create(context.Background(), conversionResource(t, "trial_started", websiteConversionAttrs(t)))
	if err == nil {
		t.Fatal("expected create without pixel identity to fail")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("token leaked in %q", err)
	}
}

func rememberPixel(t *testing.T, p *meta.Provider, id string) {
	t.Helper()
	live, err := p.Import(context.Background(), pixelAddress(t, "website"), id)
	if err != nil {
		t.Fatal(err)
	}
	if live.Identity.ID != id {
		t.Fatalf("remembered pixel = %q", live.Identity.ID)
	}
}

func sameRef(v any, addr resource.Address) bool {
	ref, ok := resource.AsRef(v)
	return ok && ref.Address == addr
}
