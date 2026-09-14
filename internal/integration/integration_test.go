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

// stubIdentities is an in-memory implementation of integration.Identities.
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

// stubReader is a minimal provider.Reader for tests.
type stubReader struct {
	name      string
	types     []string
	readFn    func(ctx context.Context, res resource.Resource) (resource.RemoteResource, error)
}

func (r *stubReader) Name() string            { return r.name }
func (r *stubReader) ResourceTypes() []string { return r.types }
func (r *stubReader) Validate(_ context.Context, _ resource.Resource) error { return nil }
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

// TestResolve_MatomoOnly verifies that a Matomo-only event is resolved
// entirely from the manifest without calling providers.
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
		context.Background(),
		events,
		nil,
		&stubIdentities{},
		func(addr resource.Address) (provider.Reader, error) {
			t.Fatalf("provider lookup should not be called for matomo-only event")
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
	if evt.Name != "trial_started" {
		t.Errorf("Name = %q, want trial_started", evt.Name)
	}
	if evt.Matomo == nil {
		t.Fatal("Matomo is nil")
	}
	if evt.Matomo.Event != "trialStarted" {
		t.Errorf("Matomo.Event = %q, want trialStarted", evt.Matomo.Event)
	}
	if len(evt.Matomo.Fields) != 1 || evt.Matomo.Fields[0] != "userId" {
		t.Errorf("Matomo.Fields = %v, want [userId]", evt.Matomo.Fields)
	}
	if evt.GoogleAds != nil {
		t.Error("GoogleAds should be nil")
	}
	if evt.Meta != nil {
		t.Error("Meta should be nil")
	}
}

// TestResolve_GoogleAds_NotApplied verifies that when the conversion action
// has no state identity, provider identifiers are absent.
func TestResolve_GoogleAds_NotApplied(t *testing.T) {
	t.Parallel()

	addr := "googleads.conversion_action.trial_started"
	events := map[string]manifest.ApplicationEvent{
		"trial_started": {
			Name: "trial_started",
			GoogleAds: &manifest.GoogleAdsEventBinding{
				Conversion: makeRef(addr),
			},
		},
	}

	resolved, err := integration.Resolve(
		context.Background(),
		events,
		makeResources(addr),
		&stubIdentities{}, // empty: no identity
		func(a resource.Address) (provider.Reader, error) {
			t.Fatalf("provider lookup should not be called when identity absent")
			return nil, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ga := resolved[0].GoogleAds
	if ga == nil {
		t.Fatal("GoogleAds is nil")
	}
	if ga.Applied() {
		t.Error("Applied() should be false when not yet applied")
	}
	if ga.ConversionID != "" {
		t.Errorf("ConversionID = %q, want empty", ga.ConversionID)
	}
}

// TestResolve_GoogleAds_Applied verifies that when the conversion action
// has a state identity, conversion ID and label are read from the provider.
func TestResolve_GoogleAds_Applied(t *testing.T) {
	t.Parallel()

	addr := "googleads.conversion_action.trial_started"
	events := map[string]manifest.ApplicationEvent{
		"trial_started": {
			Name: "trial_started",
			GoogleAds: &manifest.GoogleAdsEventBinding{
				Conversion: makeRef(addr),
			},
		},
	}
	ids := &stubIdentities{
		ids: map[string]resource.Identity{
			addr: {ID: "customers/123/conversionActions/456"},
		},
	}
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
		context.Background(),
		events,
		makeResources(addr),
		ids,
		func(a resource.Address) (provider.Reader, error) {
			return reader, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ga := resolved[0].GoogleAds
	if ga == nil {
		t.Fatal("GoogleAds is nil")
	}
	if !ga.Applied() {
		t.Error("Applied() should be true")
	}
	if ga.ConversionID != "123456789" {
		t.Errorf("ConversionID = %q, want 123456789", ga.ConversionID)
	}
	if ga.ConversionLabel != "AbCdEfGh" {
		t.Errorf("ConversionLabel = %q, want AbCdEfGh", ga.ConversionLabel)
	}
}

// TestResolve_Meta_Applied verifies that Pixel ID is read from the provider.
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
	ids := &stubIdentities{
		ids: map[string]resource.Identity{
			addr: {ID: "987654321"},
		},
	}
	reader := &stubReader{
		name:  "meta",
		types: []string{"pixel"},
		readFn: func(_ context.Context, res resource.Resource) (resource.RemoteResource, error) {
			return resource.RemoteResource{
				Address:  res.Address,
				Identity: res.Identity,
				Computed: resource.Attributes{
					"pixelId": "987654321",
				},
			}, nil
		},
	}

	resolved, err := integration.Resolve(
		context.Background(),
		events,
		makeResources(addr),
		ids,
		func(a resource.Address) (provider.Reader, error) {
			return reader, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	meta := resolved[0].Meta
	if meta == nil {
		t.Fatal("Meta is nil")
	}
	if !meta.Applied() {
		t.Error("Applied() should be true")
	}
	if meta.PixelID != "987654321" {
		t.Errorf("PixelID = %q, want 987654321", meta.PixelID)
	}
	if meta.EventName != "Purchase" {
		t.Errorf("EventName = %q, want Purchase", meta.EventName)
	}
	if meta.Delivery != manifest.DeliveryBoth {
		t.Errorf("Delivery = %q, want both", meta.Delivery)
	}
}

// TestResolve_Meta_NotApplied verifies that when pixel has no identity,
// PixelID is empty and Applied() is false.
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
		context.Background(),
		events,
		makeResources(addr),
		&stubIdentities{},
		func(a resource.Address) (provider.Reader, error) {
			t.Fatalf("provider should not be called when not applied")
			return nil, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	meta := resolved[0].Meta
	if meta == nil {
		t.Fatal("Meta is nil")
	}
	if meta.Applied() {
		t.Error("Applied() should be false")
	}
}

// TestResolve_Empty verifies that no events produce an empty result.
func TestResolve_Empty(t *testing.T) {
	t.Parallel()

	resolved, err := integration.Resolve(
		context.Background(),
		nil,
		nil,
		&stubIdentities{},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 0 {
		t.Errorf("len = %d, want 0", len(resolved))
	}
}

// TestResolve_Deterministic verifies that events are returned in sorted order.
func TestResolve_Deterministic(t *testing.T) {
	t.Parallel()

	events := map[string]manifest.ApplicationEvent{
		"z_event": {Name: "z_event", Matomo: &manifest.MatomoEventBinding{Event: "zEv"}},
		"a_event": {Name: "a_event", Matomo: &manifest.MatomoEventBinding{Event: "aEv"}},
		"m_event": {Name: "m_event", Matomo: &manifest.MatomoEventBinding{Event: "mEv"}},
	}

	resolved, err := integration.Resolve(
		context.Background(),
		events,
		nil,
		&stubIdentities{},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	names := make([]string, len(resolved))
	for i, evt := range resolved {
		names[i] = evt.Name
	}
	want := []string{"a_event", "m_event", "z_event"}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("resolved[%d].Name = %q, want %q", i, n, want[i])
		}
	}
}

// TestFormat_NoEvents verifies the empty-events message.
func TestFormat_NoEvents(t *testing.T) {
	t.Parallel()

	output := integration.Format(nil)
	if !strings.Contains(output, "No application events") {
		t.Errorf("output = %q, want 'No application events'", output)
	}
}

// TestFormat_ContainsExpectedSections verifies that Format produces expected
// sections for a fully-resolved event.
func TestFormat_ContainsExpectedSections(t *testing.T) {
	t.Parallel()

	events := []integration.ResolvedEvent{
		{
			Name: "trial_started",
			Matomo: &integration.MatomoIntegration{
				Event:  "trialStarted",
				Fields: []string{"userId"},
			},
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
		},
	}

	output := integration.Format(events)

	wantSubstrings := []string{
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
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, output)
		}
	}
}

// TestFormat_NotAppliedPlaceholders verifies that unapplied resources show
// the not-yet-applied placeholder rather than blank values.
func TestFormat_NotAppliedPlaceholders(t *testing.T) {
	t.Parallel()

	events := []integration.ResolvedEvent{
		{
			Name: "trial_started",
			GoogleAds: &integration.GoogleAdsIntegration{
				ConversionAddress: makeAddr("googleads.conversion_action.trial_started"),
			},
			Meta: &integration.MetaIntegration{
				EventSourceAddress: makeAddr("meta.pixel.main"),
				EventName:          "StartTrial",
				Delivery:           manifest.DeliveryBrowser,
			},
		},
	}

	output := integration.Format(events)
	if !strings.Contains(output, "not yet applied") {
		t.Errorf("output missing 'not yet applied' placeholder\nfull output:\n%s", output)
	}
}

// TestResolve_SecretRedaction ensures that sensitive fields do not appear in
// integration output. The stub reader returns a secret field; Resolve must
// not include it.
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
	ids := &stubIdentities{
		ids: map[string]resource.Identity{addr: {ID: "111"}},
	}
	reader := &stubReader{
		name:  "meta",
		types: []string{"pixel"},
		readFn: func(_ context.Context, res resource.Resource) (resource.RemoteResource, error) {
			return resource.RemoteResource{
				Address:  res.Address,
				Identity: res.Identity,
				Computed: resource.Attributes{
					"pixelId":        "111",
					"accessToken":    "SECRET_SHOULD_NOT_APPEAR",
					"api_secret":     "ALSO_SECRET",
				},
			}, nil
		},
	}

	resolved, err := integration.Resolve(
		context.Background(),
		events,
		makeResources(addr),
		ids,
		func(a resource.Address) (provider.Reader, error) { return reader, nil },
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := integration.Format(resolved)

	// Only pixelId must appear; secret fields must not.
	if strings.Contains(output, "SECRET_SHOULD_NOT_APPEAR") {
		t.Error("output must not contain access token secret")
	}
	if strings.Contains(output, "ALSO_SECRET") {
		t.Error("output must not contain api_secret value")
	}
	if !strings.Contains(output, "111") {
		t.Errorf("output should contain pixel ID 111\noutput:\n%s", output)
	}
}
