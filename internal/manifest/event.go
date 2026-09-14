package manifest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dziblo-music/agoraform/internal/resource"
)

// Delivery describes the browser/server delivery mode for a Meta application event.
type Delivery string

const (
	// DeliveryBrowser is a browser-only event delivery via Meta Pixel.
	DeliveryBrowser Delivery = "browser"
	// DeliveryServer is a server-only event delivery via Conversions API.
	DeliveryServer Delivery = "server"
	// DeliveryBoth delivers the event from both browser Pixel and Conversions API.
	DeliveryBoth Delivery = "both"
)

// ApplicationEvent is a provider-neutral application instrumentation contract
// for a single logical event.
//
// Each ApplicationEvent describes what the application must emit so that the
// managed provider-side conversion infrastructure receives it correctly.
// Agoraform owns the contract declaration and the provider-side resources;
// the application owns actual event emission, SDK installation, and transport.
type ApplicationEvent struct {
	// Name is the logical application-facing event key declared in the manifest.
	Name string

	// Matomo is the optional Matomo Data Layer / Tag Manager binding.
	Matomo *MatomoEventBinding

	// GoogleAds is the optional Google Ads conversion binding.
	GoogleAds *GoogleAdsEventBinding

	// Meta is the optional Meta Pixel / Conversions API binding.
	Meta *MetaEventBinding
}

// MatomoEventBinding describes the Matomo Data Layer contract for an event.
type MatomoEventBinding struct {
	// Event is the Data Layer event name consumed by a managed customEvent trigger.
	Event string

	// Fields are optional data-layer field names declared as part of the
	// tracking contract. This is informational; Agoraform does not validate
	// field values emitted by the application.
	Fields []string
}

// GoogleAdsEventBinding describes the Google Ads conversion binding for an
// application event.
type GoogleAdsEventBinding struct {
	// Conversion references the managed googleads.conversion_action resource
	// that records this event on the provider side.
	Conversion resource.Ref
}

// MetaEventBinding describes the Meta Pixel / Conversions API binding for an
// application event.
type MetaEventBinding struct {
	// EventSource references the managed meta.pixel resource that receives
	// this event.
	EventSource resource.Ref

	// EventName is the standard or custom Meta event name the application must
	// emit (for example "StartTrial" or "Purchase").
	EventName string

	// Delivery declares whether the event is expected over browser Pixel,
	// server-side Conversions API, or both.
	Delivery Delivery
}

// ApplicationEventNames returns the sorted logical names of all declared
// application events.
func ApplicationEventNames(events map[string]ApplicationEvent) []string {
	names := make([]string, 0, len(events))
	for name := range events {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// parseApplicationEvents converts a raw YAML map into typed ApplicationEvent
// values with resolved resource references.
func parseApplicationEvents(origin string, raw map[string]any) (map[string]ApplicationEvent, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string]ApplicationEvent, len(raw))
	for name, item := range raw {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("%s: applicationEvents: event name is empty", origin)
		}
		itemMap, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: applicationEvents.%s: must be a mapping", origin, name)
		}
		evt, err := parseApplicationEvent(origin, name, itemMap)
		if err != nil {
			return nil, err
		}
		out[name] = evt
	}
	return out, nil
}

func parseApplicationEvent(origin, name string, item map[string]any) (ApplicationEvent, error) {
	path := fmt.Sprintf("%s: applicationEvents.%s", origin, name)

	evt := ApplicationEvent{Name: name}

	for key := range item {
		switch key {
		case "matomo", "googleAds", "meta":
		default:
			return ApplicationEvent{}, fmt.Errorf("%s: unknown field %q (supported: matomo, googleAds, meta)", path, key)
		}
	}

	if rawMatomo, ok := item["matomo"]; ok && rawMatomo != nil {
		mMap, ok := rawMatomo.(map[string]any)
		if !ok {
			return ApplicationEvent{}, fmt.Errorf("%s.matomo: must be a mapping", path)
		}
		binding, err := parseMatomoBinding(path, mMap)
		if err != nil {
			return ApplicationEvent{}, err
		}
		evt.Matomo = binding
	}

	if rawGoogleAds, ok := item["googleAds"]; ok && rawGoogleAds != nil {
		gMap, ok := rawGoogleAds.(map[string]any)
		if !ok {
			return ApplicationEvent{}, fmt.Errorf("%s.googleAds: must be a mapping", path)
		}
		binding, err := parseGoogleAdsBinding(path, gMap)
		if err != nil {
			return ApplicationEvent{}, err
		}
		evt.GoogleAds = binding
	}

	if rawMeta, ok := item["meta"]; ok && rawMeta != nil {
		mMap, ok := rawMeta.(map[string]any)
		if !ok {
			return ApplicationEvent{}, fmt.Errorf("%s.meta: must be a mapping", path)
		}
		binding, err := parseMetaBinding(path, mMap)
		if err != nil {
			return ApplicationEvent{}, err
		}
		evt.Meta = binding
	}

	if evt.Matomo == nil && evt.GoogleAds == nil && evt.Meta == nil {
		return ApplicationEvent{}, fmt.Errorf("%s: at least one provider binding (matomo, googleAds, meta) is required", path)
	}

	return evt, nil
}

func parseMatomoBinding(path string, raw map[string]any) (*MatomoEventBinding, error) {
	p := path + ".matomo"

	for key := range raw {
		switch key {
		case "event", "fields":
		default:
			return nil, fmt.Errorf("%s: unknown field %q (supported: event, fields)", p, key)
		}
	}

	eventVal, ok := raw["event"]
	if !ok || eventVal == nil {
		return nil, fmt.Errorf("%s: event is required", p)
	}
	event, ok := eventVal.(string)
	if !ok {
		return nil, fmt.Errorf("%s: event must be a string", p)
	}
	event = strings.TrimSpace(event)
	if event == "" {
		return nil, fmt.Errorf("%s: event must not be empty", p)
	}

	var fields []string
	if rawFields, ok := raw["fields"]; ok && rawFields != nil {
		slice, ok := rawFields.([]any)
		if !ok {
			return nil, fmt.Errorf("%s: fields must be a list", p)
		}
		for i, f := range slice {
			s, ok := f.(string)
			if !ok {
				return nil, fmt.Errorf("%s: fields[%d] must be a string", p, i)
			}
			s = strings.TrimSpace(s)
			if s == "" {
				return nil, fmt.Errorf("%s: fields[%d] must not be empty", p, i)
			}
			fields = append(fields, s)
		}
	}

	return &MatomoEventBinding{Event: event, Fields: fields}, nil
}

func parseGoogleAdsBinding(path string, raw map[string]any) (*GoogleAdsEventBinding, error) {
	p := path + ".googleAds"

	for key := range raw {
		switch key {
		case "conversion":
		default:
			return nil, fmt.Errorf("%s: unknown field %q (supported: conversion)", p, key)
		}
	}

	convRaw, ok := raw["conversion"]
	if !ok || convRaw == nil {
		return nil, fmt.Errorf("%s: conversion is required", p)
	}
	convMap, ok := convRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: conversion must be a resource reference ($ref)", p)
	}
	val, err := normalizeValue(convMap)
	if err != nil {
		return nil, fmt.Errorf("%s: conversion: %w", p, err)
	}
	ref, ok := resource.AsRef(val)
	if !ok {
		return nil, fmt.Errorf("%s: conversion must be a resource reference ($ref)", p)
	}
	return &GoogleAdsEventBinding{Conversion: ref}, nil
}

func parseMetaBinding(path string, raw map[string]any) (*MetaEventBinding, error) {
	p := path + ".meta"

	for key := range raw {
		switch key {
		case "eventSource", "eventName", "delivery":
		default:
			return nil, fmt.Errorf("%s: unknown field %q (supported: eventSource, eventName, delivery)", p, key)
		}
	}

	esRaw, ok := raw["eventSource"]
	if !ok || esRaw == nil {
		return nil, fmt.Errorf("%s: eventSource is required", p)
	}
	esMap, ok := esRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: eventSource must be a resource reference ($ref)", p)
	}
	val, err := normalizeValue(esMap)
	if err != nil {
		return nil, fmt.Errorf("%s: eventSource: %w", p, err)
	}
	ref, ok := resource.AsRef(val)
	if !ok {
		return nil, fmt.Errorf("%s: eventSource must be a resource reference ($ref)", p)
	}

	enRaw, ok := raw["eventName"]
	if !ok || enRaw == nil {
		return nil, fmt.Errorf("%s: eventName is required", p)
	}
	eventName, ok := enRaw.(string)
	if !ok {
		return nil, fmt.Errorf("%s: eventName must be a string", p)
	}
	eventName = strings.TrimSpace(eventName)
	if eventName == "" {
		return nil, fmt.Errorf("%s: eventName must not be empty", p)
	}

	delivery := DeliveryBrowser
	if dRaw, ok := raw["delivery"]; ok && dRaw != nil {
		ds, ok := dRaw.(string)
		if !ok {
			return nil, fmt.Errorf("%s: delivery must be a string", p)
		}
		delivery = Delivery(strings.TrimSpace(ds))
		switch delivery {
		case DeliveryBrowser, DeliveryServer, DeliveryBoth:
		default:
			return nil, fmt.Errorf("%s: delivery must be browser, server, or both (got %q)", p, delivery)
		}
	}

	return &MetaEventBinding{
		EventSource: ref,
		EventName:   eventName,
		Delivery:    delivery,
	}, nil
}
