package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/meta/client"
)

const (
	adFields         = "id,account_id,adset_id,name,status,configured_status,effective_status,creative"
	adStatusActive   = "ACTIVE"
	adStatusPaused   = "PAUSED"
	adStatusDeleted  = "DELETED"
	adStatusArchived = "ARCHIVED"
)

var (
	supportedAdAttrs = map[string]struct{}{
		AttrName: {}, AttrAdSet: {}, AttrCreative: {}, AttrStatus: {},
	}
	computedAdAttrs = map[string]struct{}{
		"id": {}, "adId": {}, "account_id": {}, "accountId": {}, "adset_id": {},
		"configured_status": {}, "configuredStatus": {}, "effective_status": {}, "effectiveStatus": {},
		"campaign_id": {}, "campaignId": {}, "created_time": {}, "createdTime": {},
		"updated_time": {}, "updatedTime": {}, "issues_info": {}, "issuesInfo": {},
		"tracking_specs": {}, "trackingSpecs": {},
	}
	adStatuses = map[string]struct{}{adStatusActive: {}, adStatusPaused: {}}
)

type ad struct {
	ID               string          `json:"id"`
	AccountID        string          `json:"account_id"`
	AdSetID          string          `json:"adset_id"`
	Name             string          `json:"name"`
	Status           string          `json:"status"`
	ConfiguredStatus string          `json:"configured_status"`
	EffectiveStatus  string          `json:"effective_status"`
	Creative         json.RawMessage `json:"creative"`
}

type normalizedAd struct {
	Name     string
	AdSet    resource.Ref
	Creative resource.Ref
	Status   string
}

func (p *Provider) validateAd(res resource.Resource) error {
	if err := p.requireAdAccount(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}
	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedAdAttrs[key]; ok {
			continue
		}
		if _, computed := computedAdAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; meta.ad supports %s", res.Address, key, joinSorted(keys(supportedAdAttrs)))
	}
	if _, err := normalizeAd(res); err != nil {
		return err
	}
	if _, _, err := boundIdentity(res); err != nil {
		return err
	}
	return nil
}

func (p *Provider) readAd(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateAd(res); err != nil {
		return resource.RemoteResource{}, err
	}
	id, bound, err := boundIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if !bound {
		return resource.RemoteResource{}, fmt.Errorf("meta: read %s: %w", res.Address, provider.ErrNotFound)
	}
	live, err := p.readAdByID(ctx, res, id)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: read %s: %w", res.Address, err)
	}
	return live, nil
}

func (p *Provider) createAd(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateAd(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}
	want, _ := normalizeAd(res)
	form, err := p.adForm(want)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: %w", res.Address, err)
	}
	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := c.Post(ctx, c.AdAccountID()+"/ads", form, &created); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: %w", res.Address, err)
	}
	id, err := normalizeObjectID(created.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: API returned an invalid id: %w", res.Address, err)
	}
	// Once Meta returns an id, preserve it even if the immediate refresh is
	// temporarily unavailable. All create inputs are represented canonically,
	// so this confirmed result can be written to state and a retry cannot create
	// a duplicate live ad.
	live, err := p.readAdByID(ctx, res, id)
	if err == nil {
		return live, nil
	}
	fallback := resource.RemoteResource{
		Address: res.Address, Identity: resource.Identity{ID: id},
		Attributes: adAttributes(want), Computed: resource.Attributes{OutputAdID: id},
	}
	return p.rememberLive(fallback), nil
}

func (p *Provider) updateAd(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateAd(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: missing remote identity", desired.Address)
	}
	if id, bound, err := boundIdentity(desired); err != nil {
		return resource.RemoteResource{}, err
	} else if bound && id != actual.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: persisted identity %q does not match planned remote identity %q", desired.Address, id, actual.Identity.ID)
	}
	want, err := normalizeAd(desired)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	got, err := normalizeAd(resource.Resource{Address: desired.Address, Attributes: actual.Attributes})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: live ad is invalid: %w", desired.Address, err)
	}
	if err := validateAdTransition(desired.Address, want, got); err != nil {
		return resource.RemoteResource{}, err
	}
	form := url.Values{}
	if want.Name != got.Name {
		form.Set("name", want.Name)
	}
	if want.Status != got.Status {
		form.Set("status", want.Status)
	}
	if want.Creative.Address != got.Creative.Address {
		creativeID, err := p.refID(want.Creative, OutputAdCreativeID)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("meta: update %s: creative %w", desired.Address, err)
		}
		raw, _ := json.Marshal(map[string]string{"creative_id": creativeID})
		form.Set("creative", string(raw))
	}
	if len(form) == 0 {
		return p.readAdByID(ctx, desired, actual.Identity.ID)
	}
	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	var result map[string]any
	if err := c.Post(ctx, actual.Identity.ID, form, &result); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: %w", desired.Address, err)
	}
	if success, ok := result["success"].(bool); ok && !success {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: API did not report success", desired.Address)
	}
	live, err := p.readAdByID(ctx, desired, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s succeeded but refreshing ad %q failed: %w", desired.Address, actual.Identity.ID, err)
	}
	return live, nil
}

func (p *Provider) importAd(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	id, err := p.canonicalCustomConversionImportID(addr, rawID)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireConfig(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: import %s: %w", addr, err)
	}
	live, err := p.readAdByID(ctx, resource.Resource{Address: addr}, id)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("meta: import %s: remote ad %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("meta: import %s: %w", addr, err)
	}
	return live, nil
}

func (p *Provider) normalizeAdComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	want, err := normalizeAd(desired)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	wantAttrs := adAttributes(want)
	if live == nil {
		return wantAttrs, nil, nil
	}
	if id, bound, err := boundIdentity(desired); err != nil {
		return nil, nil, err
	} else if bound && (live.Identity.IsZero() || live.Identity.ID != id) {
		return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, id, live.Identity.ID)
	}
	got, err := normalizeAd(resource.Resource{Address: desired.Address, Attributes: live.Attributes})
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: live ad is invalid: %w", desired.Address, err)
	}
	if err := validateAdTransition(desired.Address, want, got); err != nil {
		return nil, nil, err
	}
	return wantAttrs, adAttributes(got), nil
}

func (p *Provider) readAdByID(ctx context.Context, desired resource.Resource, id string) (resource.RemoteResource, error) {
	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	var item ad
	if err := c.Get(ctx, id, url.Values{"fields": {adFields}}, &item); err != nil {
		if client.IsNotFound(err) {
			return resource.RemoteResource{}, provider.ErrNotFound
		}
		return resource.RemoteResource{}, err
	}
	status := strings.ToUpper(strings.TrimSpace(item.ConfiguredStatus))
	if status == "" {
		status = strings.ToUpper(strings.TrimSpace(item.Status))
	}
	if status == adStatusDeleted || status == adStatusArchived {
		return resource.RemoteResource{}, provider.ErrNotFound
	}
	live, err := p.remoteAd(ctx, desired, item)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	return p.rememberLive(live), nil
}

func (p *Provider) remoteAd(ctx context.Context, desired resource.Resource, item ad) (resource.RemoteResource, error) {
	id, err := normalizeObjectID(item.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote ad id is invalid: %w", err)
	}
	if err := p.ensureAdAccount(item.AccountID); err != nil {
		return resource.RemoteResource{}, err
	}
	adSetID, err := normalizeObjectID(item.AdSetID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote ad %s has invalid adset_id: %w", id, err)
	}
	adSet, err := p.managedRefAttr(ctx, TypeAdSet, OutputAdSetID, adSetID, desired.Attributes[AttrAdSet])
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote ad %s ad-set relationship: %w", id, err)
	}
	creativeObject, err := decodeJSONObject(item.Creative)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote ad %s has invalid creative: %w", id, err)
	}
	creativeID, ok := objectIDFromAny(creativeObject["id"])
	if !ok {
		return resource.RemoteResource{}, fmt.Errorf("remote ad %s creative is missing id", id)
	}
	creative, err := p.managedRefAttr(ctx, TypeAdCreative, OutputAdCreativeID, creativeID, desired.Attributes[AttrCreative])
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote ad %s creative relationship: %w", id, err)
	}
	status := strings.ToUpper(strings.TrimSpace(item.ConfiguredStatus))
	if status == "" {
		status = strings.ToUpper(strings.TrimSpace(item.Status))
	}
	attrs := resource.Attributes{AttrName: strings.TrimSpace(item.Name), AttrAdSet: adSet, AttrCreative: creative, AttrStatus: status}
	res := resource.Resource{Address: desired.Address, Attributes: attrs}
	if _, err := normalizeAd(res); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote ad %s cannot be represented: %w", id, err)
	}
	computed := resource.Attributes{OutputAdID: id}
	if effective := strings.ToUpper(strings.TrimSpace(item.EffectiveStatus)); effective != "" {
		computed["effectiveStatus"] = effective
	}
	return resource.RemoteResource{Address: desired.Address, Identity: resource.Identity{ID: id}, Attributes: attrs, Computed: computed}, nil
}

func normalizeAd(res resource.Resource) (normalizedAd, error) {
	name, err := requiredString(res, AttrName)
	if err != nil {
		return normalizedAd{}, err
	}
	adSet, err := requiredTypedRef(res, AttrAdSet, TypeAdSet)
	if err != nil {
		return normalizedAd{}, err
	}
	creative, err := requiredTypedRef(res, AttrCreative, TypeAdCreative)
	if err != nil {
		return normalizedAd{}, err
	}
	status, err := campaignEnum(res, AttrStatus, adStatuses, false, adStatusPaused)
	if err != nil {
		return normalizedAd{}, err
	}
	return normalizedAd{Name: name, AdSet: adSet, Creative: creative, Status: status}, nil
}

func adAttributes(a normalizedAd) resource.Attributes {
	return resource.Attributes{AttrName: a.Name, AttrAdSet: a.AdSet, AttrCreative: a.Creative, AttrStatus: a.Status}
}

func (p *Provider) adForm(a normalizedAd) (url.Values, error) {
	adSetID, err := p.refID(a.AdSet, OutputAdSetID)
	if err != nil {
		return nil, fmt.Errorf("adSet %w", err)
	}
	creativeID, err := p.refID(a.Creative, OutputAdCreativeID)
	if err != nil {
		return nil, fmt.Errorf("creative %w", err)
	}
	raw, _ := json.Marshal(map[string]string{"creative_id": creativeID})
	return url.Values{"name": {a.Name}, "adset_id": {adSetID}, "creative": {string(raw)}, "status": {a.Status}}, nil
}

func validateAdTransition(addr resource.Address, want, got normalizedAd) error {
	if want.AdSet.Address != got.AdSet.Address {
		return fmt.Errorf("resource %s: adSet is immutable after create; create a new meta.ad instead", addr)
	}
	return nil
}

func (p *Provider) destroyAd(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	id, bound, err := boundIdentity(res)
	if err != nil {
		return provider.DestroyResult{}, err
	}
	if !bound {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: missing persisted identity", res.Address)
	}
	if _, err := p.readAdByID(ctx, res, id); errors.Is(err, provider.ErrNotFound) {
		return provider.DestroyResult{Status: provider.DestroyStatusAlreadyAbsent}, nil
	} else if err != nil {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: %w", res.Address, err)
	}
	c, err := p.Client()
	if err != nil {
		return provider.DestroyResult{}, err
	}
	var result map[string]any
	if err := c.Delete(ctx, id, nil, &result); err != nil {
		if client.IsNotFound(err) {
			return provider.DestroyResult{Status: provider.DestroyStatusAlreadyAbsent}, nil
		}
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: %w", res.Address, err)
	}
	if success, ok := result["success"].(bool); ok && !success {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: API did not report success", res.Address)
	}
	if _, err := p.readAdByID(ctx, res, id); errors.Is(err, provider.ErrNotFound) {
		return provider.DestroyResult{Status: provider.DestroyStatusRemoved}, nil
	} else if err != nil {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: DELETE succeeded but confirming terminal state failed: %w", res.Address, err)
	}
	return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: ad %s is still active after DELETE", res.Address, id)
}

func (p *Provider) ensureAdAccount(accountID string) error {
	got := strings.TrimPrefix(strings.TrimSpace(accountID), "act_")
	want := strings.TrimPrefix(strings.TrimSpace(p.cfg.AdAccountID), "act_")
	if got == "" {
		return fmt.Errorf("remote ad is missing account_id")
	}
	if got != want {
		return fmt.Errorf("remote ad belongs to ad account act_%s, not configured account act_%s", got, want)
	}
	return nil
}
