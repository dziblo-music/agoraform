package matomo

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

const (
	// AttrIDGoal binds a logical Agoraform resource to Matomo's provider-native
	// goal identity. It is identity metadata, not a mutable Matomo Goal field,
	// and is excluded from comparable attributes and API mutation payloads.
	AttrIDGoal = "idGoal"
)

// validateGoalSafe layers stable-identity and Matomo-exact validation on top
// of the original v0.1 goal schema.
func (p *Provider) validateGoalSafe(res resource.Resource) error {
	base, idGoal, bound, err := goalWithoutIdentity(res)
	if err != nil {
		return err
	}
	if err := p.validateGoal(base); err != nil {
		return err
	}

	if bound {
		if err := validateGoalIdentity(res.Address, idGoal); err != nil {
			return err
		}
	}

	match := stringAttr(base.Attributes, AttrMatchAttribute)
	patternType := stringAttr(base.Attributes, AttrPatternType)
	if patternType == "" && match != matchManually {
		patternType = defaultPatternTypeFor(matchAttributes[match])
	}
	if patternType == "exact" && requiresHTTPExactMatch(match) {
		pattern := stringAttr(base.Attributes, AttrPattern)
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
	base, idGoal, bound, _ := goalWithoutIdentity(res)
	if !bound {
		return p.readGoal(ctx, base)
	}

	live, err := p.readGoalByID(ctx, res.Address, idGoal)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: bound %s %q was not found; refusing to plan a replacement resource", res.Address, AttrIDGoal, idGoal)
		}
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s by %s %q: %w", res.Address, AttrIDGoal, idGoal, err)
	}
	if err := ensureImmutableGoalName(base, live); err != nil {
		return resource.RemoteResource{}, err
	}
	return live, nil
}

func (p *Provider) createGoalSafe(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateGoalSafe(res); err != nil {
		return resource.RemoteResource{}, err
	}
	base, idGoal, bound, _ := goalWithoutIdentity(res)
	if bound {
		return resource.RemoteResource{}, fmt.Errorf("matomo: create %s: %s %q already binds this resource to an existing remote goal", res.Address, AttrIDGoal, idGoal)
	}
	return p.createGoal(ctx, base)
}

func (p *Provider) updateGoalSafe(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateGoalSafe(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: missing remote identity", desired.Address)
	}

	base, idGoal, bound, _ := goalWithoutIdentity(desired)
	if bound && idGoal != actual.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: configured %s %q does not match planned remote identity %q", desired.Address, AttrIDGoal, idGoal, actual.Identity.ID)
	}

	// Re-read immediately before mutation. Matomo's Goals.updateGoal API writes
	// a complete record and supplies destructive defaults for omitted fields,
	// so the update must carry forward the live values of unmanaged settings.
	current, err := p.readGoalByID(ctx, desired.Address, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: refresh remote goal %q: %w", desired.Address, actual.Identity.ID, err)
	}
	if err := ensureImmutableGoalName(base, current); err != nil {
		return resource.RemoteResource{}, err
	}

	preserved := client.GoalPreservedFields{
		CaseSensitive:                    computedString(current.Computed, "case_sensitive"),
		Revenue:                          computedString(current.Computed, "revenue"),
		AllowMultipleConversionsPerVisit: computedString(current.Computed, "allow_multiple"),
		Description:                      computedString(current.Computed, "description"),
		UseEventValueAsRevenue:           computedString(current.Computed, "event_value_as_revenue"),
	}
	if truthyMatomoValue(preserved.UseEventValueAsRevenue) && !strings.HasPrefix(stringAttr(base.Attributes, AttrMatchAttribute), "event_") {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: cannot change %s away from an event attribute while unmanaged useEventValueAsRevenue is enabled", desired.Address, AttrMatchAttribute)
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := c.Analytics().UpdateGoalPreserving(ctx, actual.Identity.ID, goalInput(base.Attributes), preserved); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: %w", desired.Address, err)
	}

	live, err := p.readGoalByID(ctx, desired.Address, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s succeeded but refreshing goal %q failed: %w", desired.Address, actual.Identity.ID, err)
	}
	return live, nil
}

func (p *Provider) normalizeGoalComparableSafe(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	base, idGoal, bound, err := goalWithoutIdentity(desired)
	if err != nil {
		return nil, nil, err
	}
	if live != nil && bound {
		if live.Identity.IsZero() || live.Identity.ID != idGoal {
			return nil, nil, fmt.Errorf("resource %s: configured %s %q does not match remote identity %q", desired.Address, AttrIDGoal, idGoal, live.Identity.ID)
		}
		if err := ensureImmutableGoalName(base, *live); err != nil {
			return nil, nil, err
		}
	}
	return p.normalizeGoalComparable(base, live)
}

func goalWithoutIdentity(res resource.Resource) (resource.Resource, string, bool, error) {
	attrs := res.Attributes.Clone()
	v, ok := attrs[AttrIDGoal]
	if !ok {
		res.Attributes = attrs
		return res, "", false, nil
	}
	delete(attrs, AttrIDGoal)
	id, err := coerceString(v)
	if err != nil {
		return resource.Resource{}, "", true, fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrIDGoal, err)
	}
	id = strings.TrimSpace(id)
	res.Attributes = attrs
	return res, id, true, nil
}

func validateGoalIdentity(addr resource.Address, id string) error {
	if id == "" {
		return fmt.Errorf("resource %s: attribute %q must be a non-empty Matomo goal id", addr, AttrIDGoal)
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("resource %s: attribute %q must be a positive integer Matomo goal id", addr, AttrIDGoal)
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
