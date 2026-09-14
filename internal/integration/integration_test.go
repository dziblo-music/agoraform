package integration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/integration"
	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

type stubIdentities struct {
	ids map[string]resource.Identity
}

func (s *stubIdentities) Identity(addr resource.Address) (resource.Identity, bool, error) {
	if s == nil || s.ids == nil {
		return resource.Identity{}, false, nil
	}
	id, ok := s.ids[addr.String()]
	return id, ok, nil
}

type stubReader struct {
	name   string
	types  []string
	readFn func(context.Context, resource.Resource) (resource.RemoteResource, error)
}

func (r *stubReader) Name() string            { return r.name }
func (r *stubReader) ResourceTypes() []string { return r.types }
func (r *stubReader) Validate(context.Context, resource.Resource) error {
	return nil
}
func (r *stubReader) Read(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if r.readFn != nil {
		return r.readFn(ctx, res)
	}
	return resource.RemoteResource{Address: res.Address}, nil
}

func makeAddr(s string) resource.Address {
	addr, err := resource.ParseAddress(s)
	if err != nil {
		panic(err)
	}
	return addr
}

func makeRef(s string) resource.Ref {
	return resource.Ref{Address: makeAddr(s)}
}

func makeResources(addrs ...string) []resource.Resource {
	out := make([]resource.Resource, len(addrs))
	for i, s := range addrs {
		out[i] = resource.Resource{Address: makeAddr(s)}
	}
	return out
}

func TestResolve_MatomoOnly(t *testing.T) {
	t.Parallel()
	events := map[string]manifest.ApplicationEvent{
		"trial_started": {
			Name: "trial_started",
			Matomo: &manifest.MatomoEventBinding{
				Event:  "trialStarted",
				Fields: []string{"userId"},
			},
		},
	}

	resolved, err := integration.Resolve(
		context.Background(), events, nil, &stubIdentities{},
		func(resource.Address) (provider.Reader, error) {
			t.Fatal("provider lookup should not be called for Matomo-only event")
			return nil, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("len = %d, want 1", len(resolved))
	}
	evt := resolved[0]
	if evt.Name != "trial_started" || evt.Matomo == nil {
		t.Fatalf("unexpected resolved event: %#v", evt)
	}
	if evt.Matomo.Event != "trialStarted" {
		t.Errorf("Matomo.Event = %q, want trialStarted", evt.Matomo.Event)
	}
	if len(evt.Matomo.Fields) != 1 || evt.Matomo.Fields[0] != "userId" {
		t.Errorf("Matomo.Fields = %v, want [userId]", evt.Matomo.Fields)
	}
	if evt.GoogleAds != nil || evt.Meta != nil {
		t.Errorf("unexpected additional bindings: %#v", evt)
	}
}

func TestResolve_GoogleAds_NotApplied(t *testing.T) {
	t.Parallel()
	addr := "googleads.conversion_action.trial_started"
	events := map[string]manifest.ApplicationEvent{
		"trial_started": {
			Name:      "trial_started",
			GoogleAds: &manifest.GoogleAdsEventBinding{Conversion: makeRef(addr)},
		},
	}

	resolved, err := integration.Resolve(
		context.Background(), events, makeResources(addr), &stubIdentities{},
		func(resource.Address) (provider.Reader, error) {
			t.Fatal("provider lookup should not be called when identity is absent")
			return nil, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ga := resolved[0].GoogleAds
	if ga == nil || ga.Applied() || ga.ConversionID != "" || ga.ConversionLabel != "" {
		t.Fatalf("unexpected unapplied Google Ads integration: %#v", ga)
	}
}

func TestResolve_GoogleAds_Applied(t *testing.T) {
	t.Parallel()
	addr := "googleads.conversion_action.trial_started"
	events := map[string]manifest.ApplicationEvent{
		"trial_started": {
			Name:      "trial_started",
			GoogleAds: &manifest.GoogleAdsEventBinding{Conversion: makeRef(addr)},
		},
	}
	ids := &stubIdentities{ids: map[string]resource.Identity{
		addr: {ID: "customers/123/conversionActions/456"},
	}}
	reader := &stubReader{
		name:  "googleads",
		types: []string{"conversion_action"},
		readFn: func(_ context.Context, res resource.Resource) (resource.RemoteResource, error) {
			return resource.RemoteResource{
				Address:  res.Address,
				Identity: res.Identity,
				Computed: resource.Attributes{
					"conversionId":    "123456789",
					"conversionLabel": "AbCdEfGh",
				},
			}, nil
		},
	}

	resolved, err := integration.Resolve(
		context.Background(), events, makeResources(addr), ids,
		func(resource.Address) (provider.Reader, error) { return reader, nil },
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ga := resolved[0].GoogleAds
	if ga == nil || !ga.Applied() {
		t.Fatalf("expected applied Google Ads integration: %#v", ga)
	}
	if ga.ConversionID != "123456789" || ga.ConversionLabel != "AbCdEfGh" {
		t.Errorf("unexpected Google Ads outputs: %#v", ga)
	}
}

func TestResolve_Meta_Applied(t *testing.T) {
	t.Parallel()
	addr := "meta.pixel.main"
	events := map[string]manifest.ApplicationEvent{
		"purchase": {
			Name: "purchase",
			Meta: &manifest.MetaEventBinding{
				EventSource: makeRef(addr),
				EventName:   "Purchase",
				Delivery:    manifest.DeliveryBoth,
			},
		},
	}
	ids := &stubIdentities{ids: map[string]resource.Identity{addr: {ID: "987654321"}}}
	reader := &stubReader{
		name:  "meta",
		types: []string{"pixel"},
		readFn: func(_ context.Context, res resource.Resource) (resource.RemoteResource, error) {
			return resource.RemoteResource{
				Address:  res.Address,
				Identity: res.Identity,
				Computed: resource.Attributes{"pixelId": "987654321"},
			}, nil
		},
	}

	resolved, err := integration.Resolve(
		context.Background(), events, makeResources(addr), ids,
		func(resource.Address) (provider.Reader, error) { return reader, nil },
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	meta := resolved[0].Meta
	if meta == nil || !meta.Applied() {
		t.Fatalf("expected applied Meta integration: %#v", meta)
	}
	if meta.PixelID != "987654321" || meta.EventName != "Purchase" || meta.Delivery != manifest.DeliveryBoth {
		t.Errorf("unexpected Meta integration: %#v", meta)
	}
}

func TestResolve_Meta_NotApplied(t *testing.T) {
	t.Parallel()
	addr := "meta.pixel.main"
	events := map[string]manifest.ApplicationEvent{
		"purchase": {
			Name: "purchase",
			Meta: &manifest.MetaEventBinding{
				EventSource: makeRef(addr),
				EventName:   "Purchase",
				Delivery:    manifest.DeliveryServer,
			},
		},
	}

	resolved, err := integration.Resolve(
		context.Background(), events, makeResources(addr), &stubIdentities{},
		func(resource.Address) (provider.Reader, error) {
			t.Fatal("provider lookup should not be called when identity is absent")
			return nil, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	meta := resolved[0].Meta
	if meta == nil || meta.Applied() || meta.PixelID != "" {
		t.Fatalf("unexpected unapplied Meta integration: %#v", meta)
	}
}

func TestResolve_Empty(t *testing.T) {
	t.Parallel()
	resolved, err := integration.Resolve(context.Background(), nil, nil, &stubIdentities{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 0 {
		t.Errorf("len = %d, want 0", len(resolved))
	}
}

func TestResolve_Deterministic(t *testing.T) {
	t.Parallel()
	events := map[string]manifest.ApplicationEvent{
		"z_event": {Name: "z_event", Matomo: &manifest.MatomoEventBinding{Event: "zEv"}},
		"a_event": {Name: "a_event", Matomo: &manifest.MatomoEventBinding{Event: "aEv"}},
		"m_event": {Name: "m_event", Matomo: &manifest.MatomoEventBinding{Event: "mEv"}},
	}
	resolved, err := integration.Resolve(context.Background(), events, nil, &stubIdentities{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a_event", "m_event", "z_event"}
	if len(resolved) != len(want) {
		t.Fatalf("len = %d, want %d", len(resolved), len(want))
	}
	for i, name := range want {
		if resolved[i].Name != name {
			t.Errorf("resolved[%d].Name = %q, want %q", i, resolved[i].Name, name)
		}
	}
}

func TestFormat_NoEvents(t *testing.T) {
	t.Parallel()
	if output := integration.Format(nil); !strings.Contains(output, "No application events") {
		t.Errorf("output = %q, want no-events message", output)
	}
}

func TestFormat_ContainsExpectedSections(t *testing.T) {
	t.Parallel()
	events := []integration.ResolvedEvent{{
		Name:   "trial_started",
		Matomo: &integration.MatomoIntegration{Event: "trialStarted", Fields: []string{"userId"}},
		GoogleAds: &integration.GoogleAdsIntegration{
			ConversionAddress: makeAddr("googleads.conversion_action.trial_started"),
			ConversionID:      "123456789",
			ConversionLabel:   "AbCdEfGh",
		},
		Meta: &integration.MetaIntegration{
			EventSourceAddress: makeAddr("meta.pixel.main"),
			PixelID:            "987654321",
			EventName:          "StartTrial",
			Delivery:           manifest.DeliveryBoth,
		},
	}}
	output := integration.Format(events)
	for _, want := range []string{
		"Application event: trial_started",
		"Matomo",
		"Data Layer event: trialStarted",
		"Fields: userId",
		"Google Ads",
		"AW-123456789",
		"AbCdEfGh",
		"Meta",
		"987654321",
		"StartTrial",
		"both",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, output)
		}
	}
}

func TestFormat_NotAppliedPlaceholders(t *testing.T) {
	t.Parallel()
	events := []integration.ResolvedEvent{{
		Name: "trial_started",
		GoogleAds: &integration.GoogleAdsIntegration{
			ConversionAddress: makeAddr("googleads.conversion_action.trial_started"),
		},
		Meta: &integration.MetaIntegration{
			EventSourceAddress: makeAddr("meta.pixel.main"),
			EventName:          "StartTrial",
			Delivery:           manifest.DeliveryBrowser,
		},
	}}
	output := integration.Format(events)
	if !strings.Contains(output, "not yet applied") {
		t.Errorf("output missing not-applied placeholder:\n%s", output)
	}
}

func TestResolve_SecretRedaction(t *testing.T) {
	t.Parallel()
	addr := "meta.pixel.main"
	events := map[string]manifest.ApplicationEvent{
		"purchase": {
			Name: "purchase",
			Meta: &manifest.MetaEventBinding{
				EventSource: makeRef(addr),
				EventName:   "Purchase",
				Delivery:    manifest.DeliveryBrowser,
			},
		},
	}
	ids := &stubIdentities{ids: map[string]resource.Identity{addr: {ID: "111"}}}
	reader := &stubReader{
		name:  "meta",
		types: []string{"pixel"},
		readFn: func(_ context.Context, res resource.Resource) (resource.RemoteResource, error) {
			return resource.RemoteResource{
				Address:  res.Address,
				Identity: res.Identity,
				Computed: resource.Attributes{
					"pixelId":     "111",
					"accessToken": "SECRET_SHOULD_NOT_APPEAR",
					"api_secret":  "ALSO_SECRET",
				},
			}, nil
		},
	}
	resolved, err := integration.Resolve(
		context.Background(), events, makeResources(addr), ids,
		func(resource.Address) (provider.Reader, error) { return reader, nil },
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := integration.Format(resolved)
	if strings.Contains(output, "SECRET_SHOULD_NOT_APPEAR") || strings.Contains(output, "ALSO_SECRET") {
		t.Fatalf("integration output leaked secret field:\n%s", output)
	}
	if !strings.Contains(output, "111") {
		t.Errorf("output should contain pixel ID 111:\n%s", output)
	}
}
