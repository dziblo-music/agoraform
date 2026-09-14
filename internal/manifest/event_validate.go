package manifest

import (
	"fmt"
	"sort"

	"github.com/dziblo-music/agoraform/internal/resource"
)

// knownGoogleAdsConversionActionType is the expected resource type for a
// Google Ads conversion binding reference.
const knownGoogleAdsConversionActionType = "conversion_action"

// knownMetaPixelType is the expected resource type for a Meta event-source
// binding reference.
const knownMetaPixelType = "pixel"

// validateApplicationEvents checks the declared instrumentation contracts
// against the resource set. It reports invalid references, wrong resource
// types, and missing required contract fields without contacting providers or
// mutating any remote state.
func validateApplicationEvents(origin string, events map[string]ApplicationEvent, resources []resource.Resource) error {
	if len(events) == 0 {
		return nil
	}

	byAddr := make(map[string]resource.Resource, len(resources))
	for _, res := range resources {
		byAddr[res.Address.String()] = res
	}

	names := ApplicationEventNames(events)
	sort.Strings(names)

	for _, name := range names {
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

func validateMatomoBinding(path string, b *MatomoEventBinding, _ map[string]resource.Resource) error {
	if b == nil {
		return nil
	}
	// Event name is already validated during parsing; no resource references to
	// resolve here because Matomo bindings reference the data-layer contract,
	// not a specific managed resource.
	_ = b.Event
	return nil
}

func validateGoogleAdsBinding(path string, b *GoogleAdsEventBinding, byAddr map[string]resource.Resource) error {
	if b == nil {
		return nil
	}
	p := path + ".googleAds"
	if err := requireResourceRef(p+".conversion", b.Conversion, "googleads", knownGoogleAdsConversionActionType, byAddr); err != nil {
		return err
	}
	return nil
}

func validateMetaBinding(path string, b *MetaEventBinding, byAddr map[string]resource.Resource) error {
	if b == nil {
		return nil
	}
	p := path + ".meta"
	if err := requireResourceRef(p+".eventSource", b.EventSource, "meta", knownMetaPixelType, byAddr); err != nil {
		return err
	}
	return nil
}

// requireResourceRef validates that ref resolves to a known resource of the
// expected provider and type.
func requireResourceRef(path string, ref resource.Ref, expectedProvider, expectedType string, byAddr map[string]resource.Resource) error {
	if ref.IsZero() {
		return fmt.Errorf("%s: resource reference is required", path)
	}
	addr := ref.Address
	res, ok := byAddr[addr.String()]
	if !ok {
		return fmt.Errorf("%s: references unknown resource %q", path, addr)
	}
	if res.Address.Provider != expectedProvider {
		return fmt.Errorf("%s: references %q but expected a %s.%s resource", path, addr, expectedProvider, expectedType)
	}
	if res.Address.Type != expectedType {
		return fmt.Errorf("%s: references %q but expected a %s.%s resource", path, addr, expectedProvider, expectedType)
	}
	return nil
}
