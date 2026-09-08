package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"unicode"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/meta/client"
)

const (
	adCreativeFields        = "id,account_id,name,status,object_story_spec,url_tags"
	adCreativeStatusDeleted = "DELETED"
	creativeModeImage       = "image"
	creativeModeVideo       = "video"
)

var (
	supportedAdCreativeAttrs = map[string]struct{}{
		AttrName: {}, AttrPageID: {}, AttrInstagramUserID: {}, AttrDestinationURL: {},
		AttrPrimaryText: {}, AttrHeadline: {}, AttrDescription: {}, AttrCallToAction: {},
		AttrImageHash: {}, AttrVideoID: {}, AttrURLTags: {},
	}
	computedAdCreativeAttrs = map[string]struct{}{
		"id": {}, "adCreativeId": {}, "account_id": {}, "accountId": {}, "status": {},
		"effective_object_story_id": {}, "effectiveObjectStoryId": {},
		"thumbnail_url": {}, "thumbnailUrl": {},
	}
	adCreativeCallToActions = map[string]struct{}{
		"GET_STARTED": {}, "LEARN_MORE": {}, "SIGN_UP": {},
	}
)

type adCreative struct {
	ID              string          `json:"id"`
	AccountID       string          `json:"account_id"`
	Name            string          `json:"name"`
	Status          string          `json:"status"`
	ObjectStorySpec json.RawMessage `json:"object_story_spec"`
	URLTags         string          `json:"url_tags"`
}

type normalizedAdCreative struct {
	Name               string
	PageID             string
	InstagramUserID    string
	HasInstagramUserID bool
	DestinationURL     string
	PrimaryText        string
	Headline           string
	Description        string
	HasDescription     bool
	CallToAction       string
	ImageHash          string
	VideoID            string
	Mode               string
	URLTags            string
	HasURLTags         bool
}

func (p *Provider) validateAdCreative(res resource.Resource) error {
	if err := p.requireAdAccount(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}
	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedAdCreativeAttrs[key]; ok {
			continue
		}
		if _, computed := computedAdCreativeAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; meta.ad_creative supports %s", res.Address, key, joinSorted(keys(supportedAdCreativeAttrs)))
	}
	if _, err := normalizeAdCreative(res); err != nil {
		return err
	}
	if _, _, err := boundIdentity(res); err != nil {
		return err
	}
	return nil
}

func (p *Provider) readAdCreative(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateAdCreative(res); err != nil {
		return resource.RemoteResource{}, err
	}
	id, bound, err := boundIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if !bound {
		return resource.RemoteResource{}, fmt.Errorf("meta: read %s: %w", res.Address, provider.ErrNotFound)
	}
	live, err := p.readAdCreativeByID(ctx, res.Address, id)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: read %s: %w", res.Address, err)
	}
	return live, nil
}

func (p *Provider) createAdCreative(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateAdCreative(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}
	normalized, _ := normalizeAdCreative(res)
	form, err := adCreativeForm(normalized)
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
	if err := c.Post(ctx, c.AdAccountID()+"/adcreatives", form, &created); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: %w", res.Address, err)
	}
	id, err := normalizeObjectID(created.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: API returned an invalid id: %w", res.Address, err)
	}
	live, err := p.readAdCreativeByID(ctx, res.Address, id)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s succeeded but refreshing ad creative %q failed: %w", res.Address, id, err)
	}
	return live, nil
}

func (p *Provider) updateAdCreative(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateAdCreative(desired); err != nil {
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
	want, err := normalizeAdCreative(desired)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	got, err := normalizeAdCreative(resource.Resource{Address: desired.Address, Attributes: actual.Attributes})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: live ad creative is invalid: %w", desired.Address, err)
	}
	if err := validateAdCreativeTransition(desired.Address, want, got); err != nil {
		return resource.RemoteResource{}, err
	}
	if want.Name == got.Name {
		return p.readAdCreativeByID(ctx, desired.Address, actual.Identity.ID)
	}
	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	var result map[string]any
	if err := c.Post(ctx, actual.Identity.ID, url.Values{"name": {want.Name}}, &result); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: %w", desired.Address, err)
	}
	if success, ok := result["success"].(bool); ok && !success {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: API did not report success", desired.Address)
	}
	live, err := p.readAdCreativeByID(ctx, desired.Address, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s succeeded but refreshing ad creative %q failed: %w", desired.Address, actual.Identity.ID, err)
	}
	return live, nil
}

func (p *Provider) importAdCreative(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	id, err := p.canonicalCustomConversionImportID(addr, rawID)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireConfig(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: import %s: %w", addr, err)
	}
	live, err := p.readAdCreativeByID(ctx, addr, id)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("meta: import %s: remote ad creative %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("meta: import %s: %w", addr, err)
	}
	return live, nil
}

func (p *Provider) normalizeAdCreativeComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	want, err := normalizeAdCreative(desired)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	wantAttrs := adCreativeAttributes(want)
	if live == nil {
		return wantAttrs, nil, nil
	}
	if id, bound, err := boundIdentity(desired); err != nil {
		return nil, nil, err
	} else if bound && (live.Identity.IsZero() || live.Identity.ID != id) {
		return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, id, live.Identity.ID)
	}
	got, err := normalizeAdCreative(resource.Resource{Address: desired.Address, Attributes: live.Attributes})
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: live ad creative is invalid: %w", desired.Address, err)
	}
	if err := validateAdCreativeTransition(desired.Address, want, got); err != nil {
		return nil, nil, err
	}
	return wantAttrs, adCreativeAttributes(got), nil
}

func (p *Provider) readAdCreativeByID(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	var item adCreative
	if err := c.Get(ctx, id, url.Values{"fields": {adCreativeFields}}, &item); err != nil {
		if client.IsNotFound(err) {
			return resource.RemoteResource{}, provider.ErrNotFound
		}
		return resource.RemoteResource{}, err
	}
	if strings.EqualFold(strings.TrimSpace(item.Status), adCreativeStatusDeleted) {
		return resource.RemoteResource{}, provider.ErrNotFound
	}
	live, err := p.remoteAdCreative(addr, item)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	return p.rememberLive(live), nil
}

func (p *Provider) remoteAdCreative(addr resource.Address, item adCreative) (resource.RemoteResource, error) {
	id, err := normalizeObjectID(item.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote ad creative id is invalid: %w", err)
	}
	if err := p.ensureAdCreativeAccount(item.AccountID); err != nil {
		return resource.RemoteResource{}, err
	}
	story, err := decodeJSONObject(item.ObjectStorySpec)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote ad creative %s has invalid object_story_spec: %w", id, err)
	}
	pageID, ok := objectIDFromAny(story["page_id"])
	if !ok {
		return resource.RemoteResource{}, fmt.Errorf("remote ad creative %s object_story_spec is missing page_id", id)
	}
	attrs := resource.Attributes{AttrName: strings.TrimSpace(item.Name), AttrPageID: pageID}
	if raw, exists := story["instagram_user_id"]; exists {
		instagramID, valid := objectIDFromAny(raw)
		if !valid {
			return resource.RemoteResource{}, fmt.Errorf("remote ad creative %s has invalid instagram_user_id", id)
		}
		attrs[AttrInstagramUserID] = instagramID
	}
	linkData, hasLink := stringMap(story["link_data"])
	videoData, hasVideo := stringMap(story["video_data"])
	if hasLink == hasVideo {
		return resource.RemoteResource{}, fmt.Errorf("remote ad creative %s must contain exactly one supported link_data or video_data object", id)
	}
	data := linkData
	if hasVideo {
		data = videoData
	}
	message, err := nonEmptyRemoteString(data["message"])
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote ad creative %s has invalid primary text: %w", id, err)
	}
	attrs[AttrPrimaryText] = message
	if hasLink {
		attrs[AttrHeadline], err = nonEmptyRemoteString(data["name"])
		if err == nil {
			attrs[AttrDestinationURL], err = nonEmptyRemoteString(data["link"])
		}
		if err == nil {
			attrs[AttrImageHash], err = nonEmptyRemoteString(data["image_hash"])
		}
		if description, set := optionalRemoteString(data["description"]); set {
			attrs[AttrDescription] = description
		}
	} else {
		attrs[AttrHeadline], err = nonEmptyRemoteString(data["title"])
		if err == nil {
			attrs[AttrVideoID], err = nonEmptyRemoteString(data["video_id"])
		}
		if description, set := optionalRemoteString(data["link_description"]); set {
			attrs[AttrDescription] = description
		}
	}
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote ad creative %s cannot be represented: %w", id, err)
	}
	cta, ctaURL, err := remoteCallToAction(data["call_to_action"])
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote ad creative %s has invalid call_to_action: %w", id, err)
	}
	if hasVideo {
		attrs[AttrDestinationURL] = ctaURL
	} else if attrs[AttrDestinationURL] != ctaURL {
		return resource.RemoteResource{}, fmt.Errorf("remote ad creative %s has conflicting link and call_to_action destination URLs", id)
	}
	attrs[AttrCallToAction] = cta
	if tags := strings.TrimSpace(item.URLTags); tags != "" {
		attrs[AttrURLTags] = tags
	}
	res := resource.Resource{Address: addr, Attributes: attrs}
	if _, err := normalizeAdCreative(res); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote ad creative %s cannot be represented: %w", id, err)
	}
	return resource.RemoteResource{
		Address: addr, Identity: resource.Identity{ID: id}, Attributes: attrs,
		Computed: resource.Attributes{OutputAdCreativeID: id},
	}, nil
}

func normalizeAdCreative(res resource.Resource) (normalizedAdCreative, error) {
	name, err := requiredString(res, AttrName)
	if err != nil {
		return normalizedAdCreative{}, err
	}
	pageID, err := requiredObjectID(res, AttrPageID)
	if err != nil {
		return normalizedAdCreative{}, err
	}
	instagramID, hasInstagram, err := optionalObjectID(res, AttrInstagramUserID)
	if err != nil {
		return normalizedAdCreative{}, err
	}
	destination, err := requiredString(res, AttrDestinationURL)
	if err != nil {
		return normalizedAdCreative{}, err
	}
	if err := validateDestinationURL(destination); err != nil {
		return normalizedAdCreative{}, fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrDestinationURL, err)
	}
	primary, err := requiredString(res, AttrPrimaryText)
	if err != nil {
		return normalizedAdCreative{}, err
	}
	headline, err := requiredString(res, AttrHeadline)
	if err != nil {
		return normalizedAdCreative{}, err
	}
	description, hasDescription, err := optionalNonEmptyString(res, AttrDescription)
	if err != nil {
		return normalizedAdCreative{}, err
	}
	cta, err := requiredString(res, AttrCallToAction)
	if err != nil {
		return normalizedAdCreative{}, err
	}
	cta = strings.ToUpper(cta)
	if _, ok := adCreativeCallToActions[cta]; !ok {
		return normalizedAdCreative{}, fmt.Errorf("resource %s: attribute %q must be one of %s (START_TRIAL is a conversion event type, not a v26.0 creative CTA)", res.Address, AttrCallToAction, joinSorted(keys(adCreativeCallToActions)))
	}
	imageHash, hasImage, err := optionalNonEmptyString(res, AttrImageHash)
	if err != nil {
		return normalizedAdCreative{}, err
	}
	videoID, hasVideo, err := optionalObjectID(res, AttrVideoID)
	if err != nil {
		return normalizedAdCreative{}, err
	}
	if hasImage == hasVideo {
		return normalizedAdCreative{}, fmt.Errorf("resource %s: exactly one of %q or %q is required", res.Address, AttrImageHash, AttrVideoID)
	}
	if hasImage && strings.IndexFunc(imageHash, unicode.IsSpace) >= 0 {
		return normalizedAdCreative{}, fmt.Errorf("resource %s: attribute %q must be one external Meta image hash without whitespace", res.Address, AttrImageHash)
	}
	tags, hasTags, err := optionalNonEmptyString(res, AttrURLTags)
	if err != nil {
		return normalizedAdCreative{}, err
	}
	if hasTags {
		if err := validateURLTags(tags); err != nil {
			return normalizedAdCreative{}, fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrURLTags, err)
		}
	}
	mode := creativeModeImage
	if hasVideo {
		mode = creativeModeVideo
	}
	return normalizedAdCreative{
		Name: name, PageID: pageID, InstagramUserID: instagramID, HasInstagramUserID: hasInstagram,
		DestinationURL: destination, PrimaryText: primary, Headline: headline,
		Description: description, HasDescription: hasDescription, CallToAction: cta,
		ImageHash: imageHash, VideoID: videoID, Mode: mode, URLTags: tags, HasURLTags: hasTags,
	}, nil
}

func adCreativeAttributes(c normalizedAdCreative) resource.Attributes {
	out := resource.Attributes{
		AttrName: c.Name, AttrPageID: c.PageID, AttrDestinationURL: c.DestinationURL,
		AttrPrimaryText: c.PrimaryText, AttrHeadline: c.Headline, AttrCallToAction: c.CallToAction,
	}
	if c.HasInstagramUserID {
		out[AttrInstagramUserID] = c.InstagramUserID
	}
	if c.HasDescription {
		out[AttrDescription] = c.Description
	}
	if c.Mode == creativeModeImage {
		out[AttrImageHash] = c.ImageHash
	} else {
		out[AttrVideoID] = c.VideoID
	}
	if c.HasURLTags {
		out[AttrURLTags] = c.URLTags
	}
	return out
}

func adCreativeForm(c normalizedAdCreative) (url.Values, error) {
	cta := map[string]any{"type": c.CallToAction, "value": map[string]any{"link": c.DestinationURL}}
	story := map[string]any{"page_id": c.PageID}
	if c.HasInstagramUserID {
		story["instagram_user_id"] = c.InstagramUserID
	}
	if c.Mode == creativeModeImage {
		data := map[string]any{"link": c.DestinationURL, "message": c.PrimaryText, "name": c.Headline, "image_hash": c.ImageHash, "call_to_action": cta}
		if c.HasDescription {
			data["description"] = c.Description
		}
		story["link_data"] = data
	} else {
		data := map[string]any{"video_id": c.VideoID, "message": c.PrimaryText, "title": c.Headline, "call_to_action": cta}
		if c.HasDescription {
			data["link_description"] = c.Description
		}
		story["video_data"] = data
	}
	raw, err := json.Marshal(story)
	if err != nil {
		return nil, err
	}
	form := url.Values{"name": {c.Name}, "object_story_spec": {string(raw)}}
	if c.HasURLTags {
		form.Set("url_tags", c.URLTags)
	}
	return form, nil
}

func validateAdCreativeTransition(addr resource.Address, want, got normalizedAdCreative) error {
	wantContent := want
	gotContent := got
	wantContent.Name = ""
	gotContent.Name = ""
	if reflect.DeepEqual(wantContent, gotContent) {
		return nil
	}
	return fmt.Errorf("resource %s: ad creative content is immutable after create; declare a new logical meta.ad_creative resource and repoint the ad instead of replacing it implicitly", addr)
}

func (p *Provider) destroyAdCreative(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	id, bound, err := boundIdentity(res)
	if err != nil {
		return provider.DestroyResult{}, err
	}
	if !bound {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: missing persisted identity", res.Address)
	}
	if _, err := p.readAdCreativeByID(ctx, res.Address, id); errors.Is(err, provider.ErrNotFound) {
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
	if _, err := p.readAdCreativeByID(ctx, res.Address, id); errors.Is(err, provider.ErrNotFound) {
		return provider.DestroyResult{Status: provider.DestroyStatusDestroyed}, nil
	} else if err != nil {
		return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: DELETE succeeded but confirming terminal state failed: %w", res.Address, err)
	}
	return provider.DestroyResult{}, fmt.Errorf("meta: destroy %s: ad creative %s is still active after DELETE", res.Address, id)
}

func (p *Provider) ensureAdCreativeAccount(accountID string) error {
	got := strings.TrimPrefix(strings.TrimSpace(accountID), "act_")
	want := strings.TrimPrefix(strings.TrimSpace(p.cfg.AdAccountID), "act_")
	if got == "" {
		return fmt.Errorf("remote ad creative is missing account_id")
	}
	if got != want {
		return fmt.Errorf("remote ad creative belongs to ad account act_%s, not configured account act_%s", got, want)
	}
	return nil
}

func requiredObjectID(res resource.Resource, key string) (string, error) {
	raw, err := requiredString(res, key)
	if err != nil {
		return "", err
	}
	id, err := normalizeObjectID(raw)
	if err != nil {
		return "", fmt.Errorf("resource %s: attribute %q must be a numeric Meta object identifier", res.Address, key)
	}
	return id, nil
}

func optionalObjectID(res resource.Resource, key string) (string, bool, error) {
	raw, set, err := optionalNonEmptyString(res, key)
	if err != nil || !set {
		return "", set, err
	}
	id, err := normalizeObjectID(raw)
	if err != nil {
		return "", true, fmt.Errorf("resource %s: attribute %q must be a numeric Meta object identifier", res.Address, key)
	}
	return id, true, nil
}

func optionalNonEmptyString(res resource.Resource, key string) (string, bool, error) {
	v, ok := res.Attributes[key]
	if !ok {
		return "", false, nil
	}
	s, err := coerceString(v)
	if err != nil || strings.TrimSpace(s) == "" {
		return "", true, fmt.Errorf("resource %s: attribute %q must be a non-empty string", res.Address, key)
	}
	return strings.TrimSpace(s), true, nil
}

func validateDestinationURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("must be an absolute http or https URL without credentials")
	}
	return nil
}

func validateURLTags(raw string) error {
	if strings.HasPrefix(raw, "?") || strings.HasPrefix(raw, "#") || strings.ContainsAny(raw, "\r\n") {
		return fmt.Errorf("must be a query parameter string without a leading '?' or fragment")
	}
	for _, pair := range strings.Split(raw, "&") {
		key, _, ok := strings.Cut(pair, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return fmt.Errorf("must contain key=value query parameters separated by '&'")
		}
	}
	return nil
}

func nonEmptyRemoteString(v any) (string, error) {
	s, err := coerceString(v)
	if err != nil || strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("expected a non-empty string")
	}
	return strings.TrimSpace(s), nil
}

func optionalRemoteString(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	s, err := coerceString(v)
	if err != nil || strings.TrimSpace(s) == "" {
		return "", false
	}
	return strings.TrimSpace(s), true
}

func remoteCallToAction(v any) (string, string, error) {
	cta, ok := stringMap(v)
	if !ok {
		return "", "", fmt.Errorf("expected an object")
	}
	typ, err := nonEmptyRemoteString(cta["type"])
	if err != nil {
		return "", "", err
	}
	value, ok := stringMap(cta["value"])
	if !ok {
		return "", "", fmt.Errorf("value must be an object")
	}
	link, err := nonEmptyRemoteString(value["link"])
	if err != nil {
		return "", "", fmt.Errorf("value.link %w", err)
	}
	return strings.ToUpper(typ), link, nil
}
