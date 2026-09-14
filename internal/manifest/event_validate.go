package manifest

import (
	"fmt"
	"strings"

	"github.com/dziblo-music/agoraform/internal/resource"
)

const (
	knownMatomoTriggerType              = "trigger"
	knownGoogleAdsConversionActionType = "conversion_action"
	knownMetaPixelType                  = "pixel"
)

// CheckApplicationEvents validates only the provider-neutral application event
// contract. It never requires provider credentials or contacts remote APIs.
func CheckApplicationEvents(m *Manifest) error {
	if m == nil {
		return fmt.Errorf("manifest is nil")
	}
	origin := m.Origin
	if origin == "" {
		origin = "manifest"
	}
	return validateApplicationEvents(origin, m.ApplicationEvents, m.Resources)
}

func validateApplicationEvents(origin string, events map[string]ApplicationEvent, resources []resource.Resource) error {
	if len(events) == 0 {
		return nil
	}
	byAddr := make(map[string]resource.Resource, len(resources))
	for _, res := range resources {
		byAddr[res.Address.String()] = res
	}

	for _, name := range ApplicationEventNames(events) {
		evt := events[name]
		path := fmt.Sprintf("%s: applicationEvents.%s", origin, name)
		if err := validateMatomoBinding(path, evt.Matomo, byAddr); err != nil {
			return err
		}
		if err := validateGoogleAdsBinding(path, evt.GoogleAds, byAddr); err != nil {
			return err
		}
		if err := validateMetaBinding(path, evt.Meta, byAddr); err != nil {
			return err
		}
	}
	return nil
}

func validateMatomoBinding(path string, b *MatomoEventBinding, byAddr map[string]resource.Resource) error {
	if b == nil {
		return nil
	}
	p := path + ".matomo"
	res, err := requireResourceRef(p+".trigger", b.Trigger, "matomo", knownMatomoTriggerType, byAddr)
	if err != nil {
		return err
	}
	typ, ok := res.Attributes["type"].(string)
	if !ok || strings.TrimSpace(typ) != "customEvent" {
		return fmt.Errorf("%s.trigger: references %q but the trigger must have type customEvent", p, res.Address)
	}
	event, ok := res.Attributes["event"].(string)
	if !ok || strings.TrimSpace(event) == "" {
		return fmt.Errorf("%s.trigger: references %q but the trigger must declare a non-empty event", p, res.Address)
	}
	b.Event = strings.TrimSpace(event)
	return nil
}

func validateGoogleAdsBinding(path string, b *GoogleAdsEventBinding, byAddr map[string]resource.Resource) error {
	if b == nil {
		return nil
	}
	_, err := requireResourceRef(path+".googleAds.conversion", b.Conversion, "googleads", knownGoogleAdsConversionActionType, byAddr)
	return err
}

func validateMetaBinding(path string, b *MetaEventBinding, byAddr map[string]resource.Resource) error {
	if b == nil {
		return nil
	}
	_, err := requireResourceRef(path+".meta.eventSource", b.EventSource, "meta", knownMetaPixelType, byAddr)
	return err
}

func requireResourceRef(path string, ref resource.Ref, expectedProvider, expectedType string, byAddr map[string]resource.Resource) (resource.Resource, error) {
	if ref.IsZero() {
		return resource.Resource{}, fmt.Errorf("%s: resource reference is required", path)
	}
	addr := ref.Address
	res, ok := byAddr[addr.String()]
	if !ok {
		return resource.Resource{}, fmt.Errorf("%s: references unknown resource %q", path, addr)
	}
	if res.Address.Provider != expectedProvider || res.Address.Type != expectedType {
		return resource.Resource{}, fmt.Errorf("%s: references %q but expected a %s.%s resource", path, addr, expectedProvider, expectedType)
	}
	return res, nil
}
