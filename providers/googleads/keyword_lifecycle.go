package googleads

import (
	"context"
	"fmt"

	"github.com/dziblo-music/agoraform/internal/resource"
)

// Negative AdGroupCriterion resources cannot be updated by Google Ads. Keep
// their lifecycle explicit so Agoraform never creates a disabled negative
// keyword that it cannot later enable, or plans an update the API will reject.
func (p *Provider) createKeywordLifecycle(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	negative, _, err := optionalBool(res, AttrNegative)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if !negative {
		return p.createKeyword(ctx, res)
	}

	status, set, err := optionalEnum(res, AttrStatus, keywordStatuses)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if set && status != keywordStatusEnabled {
		return resource.RemoteResource{}, negativeKeywordCreateStatusError(res, status)
	}

	normalized := negativeKeywordDefaultEnabled(res)
	return p.createKeyword(ctx, normalized)
}

func (p *Provider) updateKeywordLifecycle(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	negative, _, err := optionalBool(desired, AttrNegative)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if !negative {
		return p.updateKeyword(ctx, desired, actual)
	}
	if err := p.validateKeyword(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: missing remote identity", desired.Address)
	}
	if id, bound, err := boundKeywordIdentity(desired); err != nil {
		return resource.RemoteResource{}, err
	} else if bound && id != actual.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: persisted identity %q does not match planned remote identity %q", desired.Address, id, actual.Identity.ID)
	}

	live, err := p.readKeywordByID(ctx, desired.Address, actual.Identity.ID, desired.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: refreshing current keyword: %w", desired.Address, err)
	}
	if _, _, err := p.normalizeKeywordComparableLifecycle(desired, &live); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}

	// Negative keywords have no mutable fields in the supported model. If the
	// normalized state is equivalent, return the refreshed live object without
	// sending a mutate request.
	return p.rememberLive(live), nil
}

func (p *Provider) normalizeKeywordComparableLifecycle(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	negative, _, err := optionalBool(desired, AttrNegative)
	if err != nil {
		return nil, nil, err
	}
	if !negative {
		return p.normalizeKeywordComparable(desired, live)
	}

	status, set, err := optionalEnum(desired, AttrStatus, keywordStatuses)
	if err != nil {
		return nil, nil, err
	}
	if live == nil && set && status != keywordStatusEnabled {
		return nil, nil, negativeKeywordCreateStatusError(desired, status)
	}

	normalized := negativeKeywordDefaultEnabled(desired)
	want, got, err := p.normalizeKeywordComparable(normalized, live)
	if err != nil {
		return nil, nil, err
	}
	if live != nil && want[AttrStatus] != got[AttrStatus] {
		return nil, nil, fmt.Errorf("resource %s: status is immutable for negative keywords and cannot be changed from %v to %v; Google Ads does not allow updating negative ad-group criteria", desired.Address, got[AttrStatus], want[AttrStatus])
	}
	return want, got, nil
}

func negativeKeywordDefaultEnabled(res resource.Resource) resource.Resource {
	negative, _, err := optionalBool(res, AttrNegative)
	if err != nil || !negative {
		return res
	}
	if _, ok := res.Attributes[AttrStatus]; ok {
		return res
	}
	clone := res
	clone.Attributes = res.Attributes.Clone()
	clone.Attributes[AttrStatus] = keywordStatusEnabled
	return clone
}

func negativeKeywordCreateStatusError(res resource.Resource, status string) error {
	return fmt.Errorf("resource %s: negative keywords must be created with status ENABLED because Google Ads does not allow updating negative ad-group criteria after creation; omit %q or set it to ENABLED instead of %s", res.Address, AttrStatus, status)
}
