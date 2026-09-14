package integration

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

// Identities looks up the persisted remote identity for a logical resource
// address.
type Identities interface {
	Identity(addr resource.Address) (resource.Identity, bool, error)
}

// ResolvedEvent is the computed integration contract for a single logical
// application event.
type ResolvedEvent struct {
	// Name is the logical application event name from the manifest.
	Name string

	// Matomo is the resolved Matomo data-layer contract, or nil.
	Matomo *MatomoIntegration

	// GoogleAds is the resolved Google Ads conversion contract, or nil.
	GoogleAds *GoogleAdsIntegration

	// Meta is the resolved Meta Pixel/CAPI contract, or nil.
	Meta *MetaIntegration
}

// MatomoIntegration holds the Matomo data-layer contract for an event.
type MatomoIntegration struct {
	// Event is the Data Layer event name the application must push.
	Event string

	// Fields are the optional data-layer field names declared in the contract.
	Fields []string
}

// GoogleAdsIntegration holds the Google Ads conversion identifiers needed by
// external application instrumentation (for example gtag.js).
type GoogleAdsIntegration struct {
	// ConversionAddress is the logical resource address of the managed
	// conversion action.
	ConversionAddress resource.Address

	// ConversionID is the numeric Google Ads tag ID (AW-XXXXXXXX format prefix)
	// required by gtag.js. Empty when the resource has not been applied.
	ConversionID string

	// ConversionLabel is the per-conversion label required by gtag.js. Empty
	// when the resource has not been applied.
	ConversionLabel string
}

// Applied reports whether the conversion action has been applied and its
// provider identifiers are available.
func (g *GoogleAdsIntegration) Applied() bool {
	return g != nil && g.ConversionID != ""
}

// MetaIntegration holds the Meta Pixel/CAPI identifiers needed by external
// application instrumentation.
type MetaIntegration struct {
	// EventSourceAddress is the logical resource address of the managed pixel.
	EventSourceAddress resource.Address

	// PixelID is the Pixel/Dataset numeric identifier required by fbq() and
	// the Conversions API. Empty when the resource has not been applied.
	PixelID string

	// EventName is the standard or custom Meta event name.
	EventName string

	// Delivery declares whether the event is expected browser-only,
	// server-only, or both.
	Delivery manifest.Delivery
}

// Applied reports whether the pixel resource has been applied and its
// identifier is available.
func (m *MetaIntegration) Applied() bool {
	return m != nil && m.PixelID != ""
}

// Resolve computes the integration output for every declared application event.
// It reads computed provider outputs (conversion ID, pixel ID) from live
// provider state for resources that have already been applied. Resources
// without a persisted identity produce a partial result indicating that
// agoraform apply is required first.
//
// resources is the full set of desired resources from the manifest; it is
// used to locate the resource referenced by each binding. ids is the state
// store. lookup is used to call provider Read for bound resources.
func Resolve(
	ctx context.Context,
	events map[string]manifest.ApplicationEvent,
	resources []resource.Resource,
	ids Identities,
	lookup func(resource.Address) (provider.Reader, error),
) ([]ResolvedEvent, error) {
	if len(events) == 0 {
		return nil, nil
	}

	byAddr := make(map[string]resource.Resource, len(resources))
	for _, res := range resources {
		byAddr[res.Address.String()] = res
	}

	names := manifest.ApplicationEventNames(events)
	sort.Strings(names)

	out := make([]ResolvedEvent, 0, len(names))
	for _, name := range names {
		evt := events[name]
		resolved := ResolvedEvent{Name: name}

		if evt.Matomo != nil {
			resolved.Matomo = &MatomoIntegration{
				Event:  evt.Matomo.Event,
				Fields: append([]string(nil), evt.Matomo.Fields...),
			}
		}

		if evt.GoogleAds != nil {
			ga, err := resolveGoogleAds(ctx, evt.GoogleAds, byAddr, ids, lookup)
			if err != nil {
				return nil, fmt.Errorf("applicationEvents.%s.googleAds: %w", name, err)
			}
			resolved.GoogleAds = ga
		}

		if evt.Meta != nil {
			meta, err := resolveMeta(ctx, evt.Meta, byAddr, ids, lookup)
			if err != nil {
				return nil, fmt.Errorf("applicationEvents.%s.meta: %w", name, err)
			}
			resolved.Meta = meta
		}

		out = append(out, resolved)
	}
	return out, nil
}

func resolveGoogleAds(
	ctx context.Context,
	b *manifest.GoogleAdsEventBinding,
	byAddr map[string]resource.Resource,
	ids Identities,
	lookup func(resource.Address) (provider.Reader, error),
) (*GoogleAdsIntegration, error) {
	addr := b.Conversion.Address
	res, ok := byAddr[addr.String()]
	if !ok {
		return nil, fmt.Errorf("references unknown resource %q", addr)
	}

	result := &GoogleAdsIntegration{ConversionAddress: addr}

	identity, bound, err := ids.Identity(addr)
	if err != nil {
		return nil, fmt.Errorf("state lookup for %q: %w", addr, err)
	}
	if !bound {
		return result, nil
	}

	res.Identity = identity
	reader, err := lookup(addr)
	if err != nil {
		return nil, fmt.Errorf("provider lookup for %q: %w", addr, err)
	}

	live, err := reader.Read(ctx, res)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", addr, err)
	}

	if v, ok := live.Computed["conversionId"]; ok {
		result.ConversionID = fmt.Sprint(v)
	}
	if v, ok := live.Computed["conversionLabel"]; ok {
		result.ConversionLabel = fmt.Sprint(v)
	}
	return result, nil
}

func resolveMeta(
	ctx context.Context,
	b *manifest.MetaEventBinding,
	byAddr map[string]resource.Resource,
	ids Identities,
	lookup func(resource.Address) (provider.Reader, error),
) (*MetaIntegration, error) {
	addr := b.EventSource.Address
	res, ok := byAddr[addr.String()]
	if !ok {
		return nil, fmt.Errorf("references unknown resource %q", addr)
	}

	result := &MetaIntegration{
		EventSourceAddress: addr,
		EventName:          b.EventName,
		Delivery:           b.Delivery,
	}

	identity, bound, err := ids.Identity(addr)
	if err != nil {
		return nil, fmt.Errorf("state lookup for %q: %w", addr, err)
	}
	if !bound {
		return result, nil
	}

	res.Identity = identity
	reader, err := lookup(addr)
	if err != nil {
		return nil, fmt.Errorf("provider lookup for %q: %w", addr, err)
	}

	live, err := reader.Read(ctx, res)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", addr, err)
	}

	if v, ok := live.Computed["pixelId"]; ok {
		result.PixelID = fmt.Sprint(v)
	}
	return result, nil
}

const notApplied = "(not yet applied)"

// Format renders the resolved integration output as a human-readable string.
func Format(events []ResolvedEvent) string {
	if len(events) == 0 {
		return "No application events declared.\n"
	}

	var sb strings.Builder
	for i, evt := range events {
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "Application event: %s\n", evt.Name)

		if evt.Matomo != nil {
			sb.WriteString("\nMatomo\n")
			fmt.Fprintf(&sb, "  Data Layer event: %s\n", evt.Matomo.Event)
			if len(evt.Matomo.Fields) > 0 {
				fmt.Fprintf(&sb, "  Fields: %s\n", strings.Join(evt.Matomo.Fields, ", "))
			}
		}

		if evt.GoogleAds != nil {
			sb.WriteString("\nGoogle Ads\n")
			fmt.Fprintf(&sb, "  Conversion action: %s\n", evt.GoogleAds.ConversionAddress)
			if evt.GoogleAds.Applied() {
				fmt.Fprintf(&sb, "  Conversion ID: AW-%s\n", evt.GoogleAds.ConversionID)
				fmt.Fprintf(&sb, "  Conversion label: %s\n", evt.GoogleAds.ConversionLabel)
			} else {
				fmt.Fprintf(&sb, "  Conversion ID: %s\n", notApplied)
				fmt.Fprintf(&sb, "  Conversion label: %s\n", notApplied)
			}
		}

		if evt.Meta != nil {
			sb.WriteString("\nMeta\n")
			fmt.Fprintf(&sb, "  Event source: %s\n", evt.Meta.EventSourceAddress)
			if evt.Meta.Applied() {
				fmt.Fprintf(&sb, "  Pixel ID: %s\n", evt.Meta.PixelID)
			} else {
				fmt.Fprintf(&sb, "  Pixel ID: %s\n", notApplied)
			}
			fmt.Fprintf(&sb, "  Event: %s\n", evt.Meta.EventName)
			fmt.Fprintf(&sb, "  Delivery: %s\n", evt.Meta.Delivery)
		}
	}
	return sb.String()
}
