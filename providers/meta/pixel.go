package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/meta/client"
)

const (
	// TypePixel is the website Pixel/Dataset event-source type used in
	// addresses such as meta.pixel.website.
	TypePixel = "pixel"

	// OutputPixelID is the declared non-secret Pixel/Dataset identifier.
	OutputPixelID = "pixelId"

	pixelCreateGuidance = "Agoraform does not create Meta Pixel/Dataset event sources; they are owned in Events Manager / Business Manager. Import an existing pixel with agoraform import or adopt the unique account pixel that matches name"
	pixelFields         = "id,name,is_unavailable,owner_business"
)

var (
	supportedPixelAttrs = map[string]struct{}{
		AttrName: {},
	}

	computedPixelAttrs = map[string]struct{}{
		"id":             {},
		"pixelId":        {},
		"code":           {},
		"owner_business": {},
		"ownerBusiness":  {},
		"is_unavailable": {},
		"isUnavailable":  {},
		"creation_time":  {},
		"creationTime":   {},
	}
)

type adsPixel struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	IsUnavailable  bool            `json:"is_unavailable"`
	OwnerBusiness  json.RawMessage `json:"owner_business"`
	OwnerAdAccount json.RawMessage `json:"owner_ad_account"`
}

func (p *Provider) validatePixel(res resource.Resource) error {
	if err := p.requireAdAccount(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}
	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedPixelAttrs[key]; ok {
			continue
		}
		if _, computed := computedPixelAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; meta.pixel supports %s", res.Address, key, joinSorted(keys(supportedPixelAttrs)))
	}
	if _, err := requiredString(res, AttrName); err != nil {
		return err
	}
	if _, _, err := boundIdentity(res); err != nil {
		return err
	}
	return nil
}

func (p *Provider) readPixel(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validatePixel(res); err != nil {
		return resource.RemoteResource{}, err
	}
	id, bound, err := boundIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if !bound {
		// Unbound reads never discover by name. Name matching is adopt-only so
		// an equivalent remote pixel cannot silently skip the local binding.
		return resource.RemoteResource{}, fmt.Errorf("meta: read %s: %w", res.Address, provider.ErrNotFound)
	}
	live, err := p.readPixelByID(ctx, res.Address, id)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: read %s: %w", res.Address, err)
	}
	return live, nil
}

func (p *Provider) createPixel(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validatePixel(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("meta: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}

	name, err := requiredString(res, AttrName)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	matches, err := p.listAccountPixels(ctx)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: adopt %s: %w", res.Address, err)
	}
	var unique []adsPixel
	for _, pixel := range matches {
		if strings.TrimSpace(pixel.Name) == name {
			unique = append(unique, pixel)
		}
	}
	switch len(unique) {
	case 1:
		live, err := remotePixel(res.Address, unique[0])
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("meta: adopt %s: %w", res.Address, err)
		}
		return p.rememberLive(live), nil
	case 0:
		return resource.RemoteResource{}, fmt.Errorf("meta: adopt %s: no account Pixel/Dataset named %q; %s", res.Address, name, pixelCreateGuidance)
	default:
		ids := make([]string, 0, len(unique))
		for _, pixel := range unique {
			ids = append(ids, pixel.ID)
		}
		sort.Strings(ids)
		return resource.RemoteResource{}, fmt.Errorf("meta: adopt %s: multiple account Pixel/Dataset objects named %q (ids %s); refusing to guess", res.Address, name, strings.Join(ids, ", "))
	}
}

func (p *Provider) updatePixel(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validatePixel(desired); err != nil {
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

	want, err := comparablePixel(desired.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: %w", desired.Address, err)
	}
	got, err := comparablePixel(actual.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: %w", desired.Address, err)
	}
	if want[AttrName] != got[AttrName] {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: name is not updated through Agoraform; Pixel/Dataset rename remains an Events Manager operation", desired.Address)
	}
	live, err := p.readPixelByID(ctx, desired.Address, actual.Identity.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: update %s: %w", desired.Address, err)
	}
	return live, nil
}

func (p *Provider) importPixel(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	id, err := p.canonicalPixelImportID(addr, rawID)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireConfig(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("meta: import %s: %w", addr, err)
	}
	live, err := p.readPixelByID(ctx, addr, id)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("meta: import %s: remote Pixel/Dataset %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("meta: import %s: %w", addr, err)
	}
	return live, nil
}

func (p *Provider) canonicalPixelImportID(_ resource.Address, raw string) (string, error) {
	id, err := normalizeObjectID(raw)
	if err != nil {
		return "", fmt.Errorf("meta: import identity is invalid: %w", err)
	}
	return id, nil
}

func (p *Provider) normalizePixelComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	want, err := comparablePixel(desired.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if live == nil {
		return want, nil, nil
	}
	if id, bound, err := boundIdentity(desired); err != nil {
		return nil, nil, err
	} else if bound {
		if live.Identity.IsZero() || live.Identity.ID != id {
			return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, id, live.Identity.ID)
		}
	}
	got, err := comparablePixel(live.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	return want, got, nil
}

func (p *Provider) readPixelByID(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	var pixel adsPixel
	q := url.Values{"fields": {pixelFields}}
	if err := c.Get(ctx, id, q, &pixel); err != nil {
		if client.IsNotFound(err) {
			return resource.RemoteResource{}, provider.ErrNotFound
		}
		return resource.RemoteResource{}, err
	}
	live, err := remotePixel(addr, pixel)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	return p.rememberLive(live), nil
}

func (p *Provider) listAccountPixels(ctx context.Context) ([]adsPixel, error) {
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	raw, err := c.List(ctx, c.AdAccountID()+"/adspixels", url.Values{"fields": {pixelFields}})
	if err != nil {
		return nil, err
	}
	out := make([]adsPixel, 0, len(raw))
	for _, item := range raw {
		var pixel adsPixel
		if err := json.Unmarshal(item, &pixel); err != nil {
			return nil, fmt.Errorf("malformed AdsPixel list element")
		}
		out = append(out, pixel)
	}
	return out, nil
}

func remotePixel(addr resource.Address, pixel adsPixel) (resource.RemoteResource, error) {
	id, err := normalizeObjectID(pixel.ID)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("remote Pixel/Dataset id is invalid: %w", err)
	}
	name := strings.TrimSpace(pixel.Name)
	if name == "" {
		return resource.RemoteResource{}, fmt.Errorf("remote Pixel/Dataset %s is missing a name", id)
	}
	computed := resource.Attributes{
		OutputPixelID: id,
	}
	if pixel.IsUnavailable {
		computed["isUnavailable"] = true
	}
	return resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: id},
		Attributes: resource.Attributes{AttrName: name},
		Computed:   computed,
	}, nil
}

func comparablePixel(attrs resource.Attributes) (resource.Attributes, error) {
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	name, err := coerceString(attrs[AttrName])
	if err != nil {
		return nil, fmt.Errorf("attribute %q must be a non-empty string", AttrName)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("attribute %q must be a non-empty string", AttrName)
	}
	return resource.Attributes{AttrName: name}, nil
}
