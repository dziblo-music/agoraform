package matomo

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

const (
	// AttrIDGoal is the former manifest identity field. It is no longer
	// configurable; Matomo goal identity is stored in local state.
	AttrIDGoal = "idGoal"
)

// validateGoalSafe layers stable-identity and Matomo-exact validation on
// top of the original v0.1 goal schema.
func (p *Provider) validateGoalSafe(res resource.Resource) error {
	if err := rejectManifestIdentity(res); err != nil {
		return err
	}
	if err := p.validateGoal(res); err != nil {
		return err
	}
	if _, bound, err := boundGoalIdentity(res); err != nil {
		return err
	} else if bound {
		// Format is checked by boundGoalIdentity.
	}

	match := stringAttr(res.Attributes, AttrMatchAttribute)
	patternType := stringAttr(res.Attributes, AttrPatternType)
	if patternType == "" && match != matchManually {
		patternType = defaultPatternTypeFor(matchAttributes[match])
	}
	if patternType == "exact" && requiresHTTPExactMatch(match) {
		pattern := stringAttr(res.Attributes, AttrPattern)
		if !hasHTTPPrefix(pattern) {
			return fmt.Errorf("resource %s: attribute %q must start with http:// or https:// when %s is \"exact\" and %s is %q", res.Address, AttrPattern, AttrPatternType, AttrMatchAttribute, match)
		}
	}
	return nil
}

func (p *Provider) readGoalSafe(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateGoalSafe(res); err != nil {
		return resource.RemoteResource{}, err
	}
	id, bound, err := boundGoalIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if !bound {
		return p.readGoal(ctx, res)
	}

	live, err := p.readGoalByID(ctx, res.Address, id)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: persisted identity %q was not found remotely; refusing to plan a replacement resource: %w", res.Address, id, state.ErrStaleIdentity)
		}
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s by identity %q: %w", res.Address, id, err)
	}
	if err := ensureImmutableGoalName(res, live); err != nil {
		return resource.RemoteResource{}, err
	}
	return live, nil
}

func (p *Provider) createGoalSafe(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateGoalSafe(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundGoalIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("matomo: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}
	return p.createGoal(ctx, res)
}

func (p *Provider) updateGoalSafe(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateGoalSafe(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: missing remote identity", desired.Address)
	}
	if id, bound, err := boundGoalIdentity(desired); err != nil {
		return resource.RemoteResource{}, err
	} else if bound && id != actual.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: persisted identity %q does not match planned remote identity %q", desired.Address, id, actual.Identity.ID)
	}

	// Re-read immediately before mutation. Matomo's Goals.updateGoal API writes
	// a complete record and supplies destructive defaults for omitted fields,
	// so the update must carry forward the live values of unmanaged settings.
	current, err := p.readGoalByID(ctx, desired.Address, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: refresh remote goal %q: %w", desired.Address, actual.Identity.ID, err)
	}
	if err := ensureImmutableGoalName(desired, current); err != nil {
		return resource.RemoteResource{}, err
	}

	preserved := client.GoalPreservedFields{
		CaseSensitive:                    computedString(current.Computed, "case_sensitive"),
		Revenue:                          computedString(current.Computed, "revenue"),
		AllowMultipleConversionsPerVisit: computedString(current.Computed, "allow_multiple"),
		Description:                      computedString(current.Computed, "description"),
		UseEventValueAsRevenue:           computedString(current.Computed, "event_value_as_revenue"),
	}
	if truthyMatomoValue(preserved.UseEventValueAsRevenue) && !strings.HasPrefix(stringAttr(desired.Attributes, AttrMatchAttribute), "event_") {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: cannot change %s away from an event attribute while unmanaged useEventValueAsRevenue is enabled", desired.Address, AttrMatchAttribute)
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := c.Analytics().UpdateGoalPreserving(ctx, actual.Identity.ID, goalInput(desired.Attributes), preserved); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: %w", desired.Address, err)
	}

	live, err := p.readGoalByID(ctx, desired.Address, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s succeeded but refreshing goal %q failed: %w", desired.Address, actual.Identity.ID, err)
	}
	return live, nil
}

func (p *Provider) importGoal(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	id = strings.TrimSpace(id)
	if err := validateGoalIdentity(addr, id); err != nil {
		return resource.RemoteResource{}, err
	}
	live, err := p.readGoalByID(ctx, addr, id)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("matomo: import %s: remote goal %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("matomo: import %s: %w", addr, err)
	}
	return live, nil
}

func (p *Provider) normalizeGoalComparableSafe(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	if err := rejectManifestIdentity(desired); err != nil {
		return nil, nil, err
	}
	if live != nil {
		if _, bound, err := boundGoalIdentity(desired); err != nil {
			return nil, nil, err
		} else if bound {
			if live.Identity.IsZero() || live.Identity.ID != desired.Identity.ID {
				return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, desired.Identity.ID, live.Identity.ID)
			}
			if err := ensureImmutableGoalName(desired, *live); err != nil {
				return nil, nil, err
			}
		}
	}
	return p.normalizeGoalComparable(desired, live)
}

func rejectManifestIdentity(res resource.Resource) error {
	if _, ok := res.Attributes[AttrIDGoal]; ok {
		return fmt.Errorf("resource %s: attribute %q is not configurable; persist the Matomo goal identity in local state (%s), not in the manifest", res.Address, AttrIDGoal, state.DefaultFilename)
	}
	return nil
}

func boundGoalIdentity(res resource.Resource) (string, bool, error) {
	if res.Identity.IsZero() {
		return "", false, nil
	}
	id := strings.TrimSpace(res.Identity.ID)
	if err := validateGoalIdentity(res.Address, id); err != nil {
		return "", true, err
	}
	return id, true, nil
}

func validateGoalIdentity(addr resource.Address, id string) error {
	if id == "" {
		return fmt.Errorf("resource %s: persisted identity is empty; a Matomo goal id is required", addr)
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("resource %s: persisted identity %q is not a valid Matomo goal id", addr, id)
	}
	return nil
}

func ensureImmutableGoalName(desired resource.Resource, live resource.RemoteResource) error {
	want := stringAttr(desired.Attributes, AttrName)
	got := stringAttr(live.Attributes, AttrName)
	if want == got {
		return nil
	}
	return fmt.Errorf("resource %s: attribute %q is immutable for bound Matomo goal %q; remote name is %q and configuration requests %q", desired.Address, AttrName, live.Identity.ID, got, want)
}

func requiresHTTPExactMatch(match string) bool {
	switch match {
	case "url", "file", "external_website":
		return true
	default:
		return false
	}
}

func computedString(attrs resource.Attributes, key string) string {
	if attrs == nil {
		return ""
	}
	s, err := coerceString(attrs[key])
	if err != nil {
		return ""
	}
	return s
}

func truthyMatomoValue(v string) bool {
	v = strings.TrimSpace(strings.ToLower(v))
	return v == "1" || v == "true" || v == "yes"
}
