package googleads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

const (
	// TypeResponsiveSearchAd is the Google Ads RSA type used in addresses
	// such as googleads.responsive_search_ad.brand.
	TypeResponsiveSearchAd = "responsive_search_ad"

	// AttrFinalUrls is the list of landing-page URLs for an RSA.
	AttrFinalUrls = "finalUrls"
	// AttrHeadlines is the list of RSA headlines.
	AttrHeadlines = "headlines"
	// AttrDescriptions is the list of RSA descriptions.
	AttrDescriptions = "descriptions"
	// AttrPath1 is an optional display-path segment after the domain.
	AttrPath1 = "path1"
	// AttrPath2 is an optional second display-path segment.
	AttrPath2 = "path2"
	// AttrPin is an optional served-asset field pin on a headline or description.
	AttrPin = "pin"

	adTypeResponsiveSearchAd = "RESPONSIVE_SEARCH_AD"
	adGroupAdsCollection     = "adGroupAds"
	adsCollection            = "ads"
	rsaStatusPaused          = "PAUSED"
	rsaStatusEnabled         = "ENABLED"

	minRSAHeadlines        = 3
	maxRSAHeadlines        = 15
	maxRSAHeadlineRunes    = 30
	minRSADescriptions     = 2
	maxRSADescriptions     = 4
	maxRSADescriptionRunes = 90
	maxRSAPathRunes        = 15
	maxRSAFinalURLBytes    = 2084
)

var (
	supportedRSAAttrs = map[string]struct{}{
		AttrAdGroup:      {},
		AttrFinalUrls:    {},
		AttrHeadlines:    {},
		AttrDescriptions: {},
		AttrPath1:        {},
		AttrPath2:        {},
		AttrStatus:       {},
	}

	computedRSAAttrs = map[string]struct{}{
		"id":                      {},
		"resourceName":            {},
		"resource_name":           {},
		"adId":                    {},
		"ad_id":                   {},
		"adResourceName":          {},
		"ad_resource_name":        {},
		"type":                    {},
		"primaryStatus":           {},
		"primary_status":          {},
		"primaryStatusReasons":    {},
		"assetPerformanceLabel":   {},
		"asset_performance_label": {},
		"policySummaryInfo":       {},
		"policy_summary_info":     {},
		"approvalStatus":          {},
		"approval_status":         {},
	}

	rsaStatuses = map[string]struct{}{
		rsaStatusPaused:  {},
		rsaStatusEnabled: {},
	}

	rsaHeadlinePins = map[string]struct{}{
		"HEADLINE_1": {},
		"HEADLINE_2": {},
		"HEADLINE_3": {},
	}

	rsaDescriptionPins = map[string]struct{}{
		"DESCRIPTION_1": {},
		"DESCRIPTION_2": {},
	}

	rsaSelect = strings.Join([]string{
		"SELECT",
		"ad_group_ad.resource_name,",
		"ad_group_ad.status,",
		"ad_group_ad.ad_group,",
		"ad_group_ad.ad.id,",
		"ad_group_ad.ad.resource_name,",
		"ad_group_ad.ad.type,",
		"ad_group_ad.ad.final_urls,",
		"ad_group_ad.ad.responsive_search_ad.headlines,",
		"ad_group_ad.ad.responsive_search_ad.descriptions,",
		"ad_group_ad.ad.responsive_search_ad.path1,",
		"ad_group_ad.ad.responsive_search_ad.path2,",
		"ad_group_ad.primary_status",
		"FROM ad_group_ad",
	}, " ")
)

func (p *Provider) validateResponsiveSearchAd(res resource.Resource) error {
	if err := p.requireCustomerID(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}

	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedRSAAttrs[key]; ok {
			continue
		}
		if _, computed := computedRSAAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; googleads.responsive_search_ad supports %s", res.Address, key, joinSorted(keys(supportedRSAAttrs)))
	}

	if _, err := requiredAdGroupRef(res); err != nil {
		return err
	}
	if _, err := requiredRSAFinalURLs(res); err != nil {
		return err
	}
	if _, err := requiredRSAHeadlines(res); err != nil {
		return err
	}
	if _, err := requiredRSADescriptions(res); err != nil {
		return err
	}
	path1, path1Set, err := optionalRSAPath(res, AttrPath1)
	if err != nil {
		return err
	}
	path2, path2Set, err := optionalRSAPath(res, AttrPath2)
	if err != nil {
		return err
	}
	if path2Set && path2 != "" && (!path1Set || path1 == "") {
		return fmt.Errorf("resource %s: attribute %q requires %q", res.Address, AttrPath2, AttrPath1)
	}
	if _, _, err := optionalEnum(res, AttrStatus, rsaStatuses); err != nil {
		return err
	}
	if err := p.ensureRSAIdentityMatches(res); err != nil {
		return err
	}
	return nil
}

func (p *Provider) readResponsiveSearchAd(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateResponsiveSearchAd(res); err != nil {
		return resource.RemoteResource{}, err
	}

	id, bound, err := boundRSAIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if bound {
		live, err := p.readRSAByID(ctx, res.Address, id, res.Attributes)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return p.rememberLive(live), nil
	}

	adGroupID, ok := p.adGroupIDFromRef(res.Attributes[AttrAdGroup])
	if !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, provider.ErrNotFound)
	}
	where := strings.Join([]string{
		"ad_group.id = " + adGroupID,
		"ad_group_ad.ad.type = " + gaqlString(adTypeResponsiveSearchAd),
		"ad_group_ad.status != " + gaqlString("REMOVED"),
	}, " AND ")
	candidates, err := p.queryRSAs(ctx, where)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
	}
	want, err := comparableRSA(res.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
	}
	matches := make([]rsaData, 0, len(candidates))
	for _, item := range candidates {
		live, lerr := p.remoteRSA(res.Address, item, res.Attributes)
		if lerr != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, lerr)
		}
		got, gerr := comparableRSA(live.Attributes)
		if gerr != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, gerr)
		}
		if rsaCreativeEquivalent(want, got) {
			matches = append(matches, item)
		}
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, provider.ErrNotFound)
	case 1:
		live, err := p.remoteRSA(res.Address, matches[0], res.Attributes)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return p.rememberLive(live), nil
	default:
		ids := make([]string, 0, len(matches))
		for _, item := range matches {
			ids = append(ids, rsaID(item.AdGroupID, item.AdID))
		}
		sort.Strings(ids)
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: multiple remote responsive search ads match this creative in ad group %s (ids %s); bind one with agoraform import", res.Address, adGroupID, strings.Join(ids, ", "))
	}
}

func (p *Provider) createResponsiveSearchAd(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateResponsiveSearchAd(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundRSAIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}

	body, err := p.rsaCreateBody(res)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}
	if _, ok := body["status"]; !ok {
		body["status"] = rsaStatusPaused
	}

	raw, err := c.Mutate(ctx, adGroupAdsCollection, []map[string]any{
		{"create": body},
	})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}
	id, err := parseRSAMutateID(raw, c.CustomerID())
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}

	live, err := p.readRSAByID(ctx, res.Address, id, res.Attributes)
	if err == nil {
		return p.rememberLive(live), nil
	}
	fallback, ferr := p.remoteRSAFromDesired(res, id, c.CustomerID())
	if ferr != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s succeeded but refreshing responsive search ad %q failed: %w", res.Address, id, err)
	}
	return p.rememberLive(fallback), nil
}

func (p *Provider) updateResponsiveSearchAd(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateResponsiveSearchAd(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: missing remote identity", desired.Address)
	}
	if id, bound, err := boundRSAIdentity(desired); err != nil {
		return resource.RemoteResource{}, err
	} else if bound && id != actual.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: persisted identity %q does not match planned remote identity %q", desired.Address, id, actual.Identity.ID)
	}

	live, err := p.readRSAByID(ctx, desired.Address, actual.Identity.ID, desired.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: refreshing current responsive search ad: %w", desired.Address, err)
	}

	want, got, err := p.normalizeRSAComparable(desired, &live)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := rejectImmutableRSAChanges(want, got); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}
	if reflect.DeepEqual(want, got) {
		return p.rememberLive(live), nil
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}

	if rsaCreativeChanged(want, got) {
		adName := adResourceName(c.CustomerID(), actual.Identity.ID)
		body, mask, err := p.rsaAdMutateBody(desired, adName)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
		}
		if len(mask) > 0 {
			_, err = c.Mutate(ctx, adsCollection, []map[string]any{
				{
					"updateMask": strings.Join(mask, ","),
					"update":     body,
				},
			})
			if err != nil {
				return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: replacing responsive search ad creative: %w", desired.Address, err)
			}
		}
	}

	if !reflect.DeepEqual(want[AttrStatus], got[AttrStatus]) {
		resourceName := adGroupAdResourceName(c.CustomerID(), actual.Identity.ID)
		_, err = c.Mutate(ctx, adGroupAdsCollection, []map[string]any{
			{
				"updateMask": "status",
				"update": map[string]any{
					"resourceName": resourceName,
					"status":       want[AttrStatus],
				},
			},
		})
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
		}
	}

	refreshed, err := p.readRSAByID(ctx, desired.Address, actual.Identity.ID, desired.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s succeeded but refreshing responsive search ad %q failed: %w", desired.Address, actual.Identity.ID, err)
	}
	return p.rememberLive(refreshed), nil
}

func (p *Provider) importResponsiveSearchAd(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	id, err := p.canonicalRSAImportID(addr, rawID)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireCustomerID(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	live, err := p.readRSAByID(ctx, addr, id, nil)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: remote responsive search ad %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	if _, ok := live.Attributes[AttrAdGroup]; !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: ad group is not bound in local state; import the googleads.ad_group resource first (or apply it), then re-import this responsive search ad", addr)
	}
	return p.rememberLive(live), nil
}

func (p *Provider) normalizeRSAComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	if err := p.validateResponsiveSearchAd(desired); err != nil {
		return nil, nil, err
	}
	want, err := comparableRSA(desired.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if live == nil {
		return want, nil, nil
	}
	if id, bound, err := boundRSAIdentity(desired); err != nil {
		return nil, nil, err
	} else if bound {
		if live.Identity.IsZero() || live.Identity.ID != id {
			return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, id, live.Identity.ID)
		}
	}
	gotFull, err := comparableRSA(live.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	got := resource.Attributes{}
	for key := range want {
		if v, ok := gotFull[key]; ok {
			got[key] = intersectRSAValue(want[key], v)
			continue
		}
		if v, ok := live.Attributes[key]; ok {
			got[key] = intersectRSAValue(want[key], v)
		}
	}
	if err := rejectImmutableRSAChanges(want, got); err != nil {
		return nil, nil, err
	}
	return want, got, nil
}

func (p *Provider) readRSAByID(ctx context.Context, addr resource.Address, id string, desired resource.Attributes) (resource.RemoteResource, error) {
	adGroupID, adID, err := parseRSAID(addr, id)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	where := "ad_group.id = " + adGroupID + " AND ad_group_ad.ad.id = " + adID
	matches, err := p.queryRSAs(ctx, where)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, provider.ErrNotFound
	case 1:
		return p.remoteRSA(addr, matches[0], desired)
	default:
		return resource.RemoteResource{}, fmt.Errorf("multiple remote responsive search ads returned for id %s", id)
	}
}

func (p *Provider) queryRSAs(ctx context.Context, where string) ([]rsaData, error) {
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	query := rsaSelect
	if strings.TrimSpace(where) != "" {
		query += " WHERE " + where
	}
	query += " ORDER BY ad_group_ad.ad.id"
	rows, err := c.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]rsaData, 0, len(rows))
	for _, row := range rows {
		item, err := decodeRSARow(row, c.CustomerID())
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AdGroupID != out[j].AdGroupID {
			return out[i].AdGroupID < out[j].AdGroupID
		}
		return out[i].AdID < out[j].AdID
	})
	return out, nil
}

type rsaData struct {
	ResourceName   string
	AdGroup        string
	AdGroupID      string
	AdID           string
	AdResourceName string
	Status         string
	Type           string
	FinalURLs      []string
	Headlines      []rsaAsset
	Descriptions   []rsaAsset
	Path1          string
	Path2          string
	PrimaryStatus  string
}

type rsaAsset struct {
	Text string
	Pin  string
}

func (a rsaAsset) comparable() any {
	if a.Pin == "" {
		return a.Text
	}
	return map[string]any{AttrText: a.Text, AttrPin: a.Pin}
}

func (a rsaAsset) mutate() map[string]any {
	out := map[string]any{"text": a.Text}
	if a.Pin != "" {
		out["pinnedField"] = a.Pin
	}
	return out
}

type rsaJSON struct {
	ResourceName  string `json:"resourceName"`
	Status        string `json:"status"`
	AdGroup       string `json:"adGroup"`
	PrimaryStatus string `json:"primaryStatus"`
	Ad            *struct {
		ID                 json.Number  `json:"id"`
		ResourceName       string       `json:"resourceName"`
		Type               string       `json:"type"`
		FinalURLs          []string     `json:"finalUrls"`
		ResponsiveSearchAd *rsaInfoJSON `json:"responsiveSearchAd"`
	} `json:"ad"`
}

type rsaInfoJSON struct {
	Headlines    []rsaAssetJSON `json:"headlines"`
	Descriptions []rsaAssetJSON `json:"descriptions"`
	Path1        string         `json:"path1"`
	Path2        string         `json:"path2"`
}

type rsaAssetJSON struct {
	Text        string `json:"text"`
	PinnedField string `json:"pinnedField"`
}

func decodeRSARow(raw json.RawMessage, configuredCustomerID string) (rsaData, error) {
	malformed := func(detail string) (rsaData, error) {
		if detail == "" {
			return rsaData{}, fmt.Errorf("malformed responsive search ad result")
		}
		return rsaData{}, fmt.Errorf("malformed responsive search ad result: %s", detail)
	}

	var envelope struct {
		AdGroupAd json.RawMessage `json:"adGroupAd"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.AdGroupAd) == 0 {
		return malformed("")
	}
	var body rsaJSON
	if err := json.Unmarshal(envelope.AdGroupAd, &body); err != nil {
		return malformed("")
	}

	resourceName := strings.TrimSpace(body.ResourceName)
	resourceCustomerID, adGroupID, adID, ok := splitAdGroupAdResourceName(resourceName)
	if !ok {
		return malformed("invalid resourceName")
	}
	if configuredCustomerID != "" && resourceCustomerID != configuredCustomerID {
		return malformed("resourceName belongs to a different customer")
	}

	if body.Ad == nil {
		return malformed("missing ad")
	}
	id := strings.TrimSpace(body.Ad.ID.String())
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		return malformed("invalid ad id")
	}
	if id != adID {
		return malformed("ad id does not match resourceName")
	}

	adGroupResourceName := strings.TrimSpace(body.AdGroup)
	adGroupCustomerID, parsedAdGroupID, ok := splitAdGroupResourceName(adGroupResourceName)
	if !ok {
		return malformed("invalid adGroup")
	}
	if configuredCustomerID != "" && adGroupCustomerID != configuredCustomerID {
		return malformed("adGroup belongs to a different customer")
	}
	if parsedAdGroupID != adGroupID {
		return malformed("adGroup does not match resourceName")
	}

	adType := normalizeEnum(body.Ad.Type)
	if adType == "" {
		return malformed("missing ad type")
	}
	if adType != adTypeResponsiveSearchAd {
		return rsaData{}, fmt.Errorf("ad group ad %s has type %s; googleads.responsive_search_ad only manages RESPONSIVE_SEARCH_AD ads", rsaID(adGroupID, id), adType)
	}
	if body.Ad.ResponsiveSearchAd == nil {
		return malformed("missing responsiveSearchAd")
	}

	status := normalizeEnum(body.Status)
	if status == "REMOVED" {
		return rsaData{}, fmt.Errorf("responsive search ad %s has status REMOVED; googleads.responsive_search_ad does not manage removed ads", rsaID(adGroupID, id))
	}
	if status != "" {
		if _, ok := rsaStatuses[status]; !ok {
			return rsaData{}, fmt.Errorf("responsive search ad %s has status %s; googleads.responsive_search_ad supports %s", rsaID(adGroupID, id), status, joinSorted(keys(rsaStatuses)))
		}
	}

	headlines, err := decodeRSAAssets(body.Ad.ResponsiveSearchAd.Headlines, rsaHeadlinePins, maxRSAHeadlineRunes, "headline")
	if err != nil {
		return rsaData{}, fmt.Errorf("responsive search ad %s: %w", rsaID(adGroupID, id), err)
	}
	descriptions, err := decodeRSAAssets(body.Ad.ResponsiveSearchAd.Descriptions, rsaDescriptionPins, maxRSADescriptionRunes, "description")
	if err != nil {
		return rsaData{}, fmt.Errorf("responsive search ad %s: %w", rsaID(adGroupID, id), err)
	}

	urls := make([]string, 0, len(body.Ad.FinalURLs))
	for _, rawURL := range body.Ad.FinalURLs {
		normalized, nerr := normalizeRSAFinalURL(rawURL)
		if nerr != nil {
			return rsaData{}, fmt.Errorf("responsive search ad %s has invalid final URL %q: %w", rsaID(adGroupID, id), rawURL, nerr)
		}
		urls = append(urls, normalized)
	}

	adResourceName := strings.TrimSpace(body.Ad.ResourceName)
	if adResourceName == "" {
		adResourceName = adResourceNameFromParts(resourceCustomerID, id)
	}

	return rsaData{
		ResourceName:   resourceName,
		AdGroup:        adGroupResourceName,
		AdGroupID:      adGroupID,
		AdID:           id,
		AdResourceName: adResourceName,
		Status:         status,
		Type:           adType,
		FinalURLs:      urls,
		Headlines:      headlines,
		Descriptions:   descriptions,
		Path1:          strings.TrimSpace(body.Ad.ResponsiveSearchAd.Path1),
		Path2:          strings.TrimSpace(body.Ad.ResponsiveSearchAd.Path2),
		PrimaryStatus:  normalizeEnum(body.PrimaryStatus),
	}, nil
}

func decodeRSAAssets(raw []rsaAssetJSON, allowedPins map[string]struct{}, maxRunes int, kind string) ([]rsaAsset, error) {
	out := make([]rsaAsset, 0, len(raw))
	for i, item := range raw {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			return nil, fmt.Errorf("malformed %s at index %d: missing text", kind, i)
		}
		if utf8.RuneCountInString(text) > maxRunes {
			return nil, fmt.Errorf("%s %q exceeds %d characters", kind, text, maxRunes)
		}
		pin := normalizeEnum(item.PinnedField)
		if pin == "" || pin == "UNSPECIFIED" || pin == "UNKNOWN" {
			pin = ""
		} else if _, ok := allowedPins[pin]; !ok {
			return nil, fmt.Errorf("%s %q has unsupported pin %s", kind, text, pin)
		}
		out = append(out, rsaAsset{Text: text, Pin: pin})
	}
	return out, nil
}

func (p *Provider) remoteRSA(addr resource.Address, item rsaData, desired resource.Attributes) (resource.RemoteResource, error) {
	attrs := resource.Attributes{
		AttrFinalUrls:    comparableAnyList(item.FinalURLs),
		AttrHeadlines:    comparableRSAAssets(item.Headlines),
		AttrDescriptions: comparableRSAAssets(item.Descriptions),
	}
	if item.Status != "" {
		attrs[AttrStatus] = item.Status
	}
	if desired != nil {
		if _, ok := desired[AttrPath1]; ok {
			attrs[AttrPath1] = item.Path1
		}
		if _, ok := desired[AttrPath2]; ok {
			attrs[AttrPath2] = item.Path2
		}
	} else {
		if item.Path1 != "" {
			attrs[AttrPath1] = item.Path1
		}
		if item.Path2 != "" {
			attrs[AttrPath2] = item.Path2
		}
	}

	adGroup, err := p.liveAdGroupAttr(addr, item.AdGroup, desired[AttrAdGroup])
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if adGroup != nil {
		attrs[AttrAdGroup] = adGroup
	}

	id := rsaID(item.AdGroupID, item.AdID)
	computed := resource.Attributes{}
	setComputed(computed, "id", id)
	setComputed(computed, "resourceName", item.ResourceName)
	setComputed(computed, "adId", item.AdID)
	setComputed(computed, "adResourceName", item.AdResourceName)
	setComputed(computed, "type", item.Type)
	setComputed(computed, "primaryStatus", item.PrimaryStatus)

	return resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: id},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func (p *Provider) remoteRSAFromDesired(res resource.Resource, id, customerID string) (resource.RemoteResource, error) {
	attrs, err := comparableRSA(res.Attributes)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	_, adID, err := parseRSAID(res.Address, id)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	computed := resource.Attributes{
		"id":             id,
		"resourceName":   adGroupAdResourceName(customerID, id),
		"adId":           adID,
		"adResourceName": adResourceNameFromParts(customerID, adID),
		"type":           adTypeResponsiveSearchAd,
	}
	return resource.RemoteResource{
		Address:    res.Address,
		Identity:   resource.Identity{ID: id},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func comparableRSA(attrs resource.Attributes) (resource.Attributes, error) {
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	adGroup, err := comparableAdGroupAttr(attrs[AttrAdGroup])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrAdGroup, err)
	}
	urls, err := parseRSAFinalURLs(attrs[AttrFinalUrls])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrFinalUrls, err)
	}
	headlines, err := parseRSAAssets(attrs[AttrHeadlines], rsaHeadlinePins, minRSAHeadlines, maxRSAHeadlines, maxRSAHeadlineRunes, "headline")
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrHeadlines, err)
	}
	descriptions, err := parseRSAAssets(attrs[AttrDescriptions], rsaDescriptionPins, minRSADescriptions, maxRSADescriptions, maxRSADescriptionRunes, "description")
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrDescriptions, err)
	}

	status := rsaStatusPaused
	if _, ok := attrs[AttrStatus]; ok {
		raw, err := coerceString(attrs[AttrStatus])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrStatus, err)
		}
		status = normalizeEnum(raw)
	}

	out := resource.Attributes{
		AttrAdGroup:      adGroup,
		AttrFinalUrls:    comparableAnyList(urls),
		AttrHeadlines:    comparableRSAAssets(headlines),
		AttrDescriptions: comparableRSAAssets(descriptions),
		AttrStatus:       status,
	}
	if _, ok := attrs[AttrPath1]; ok {
		path1, err := normalizeRSAPath(attrs[AttrPath1])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrPath1, err)
		}
		out[AttrPath1] = path1
	}
	if _, ok := attrs[AttrPath2]; ok {
		path2, err := normalizeRSAPath(attrs[AttrPath2])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrPath2, err)
		}
		out[AttrPath2] = path2
	}
	return out, nil
}

func (p *Provider) rsaCreateBody(res resource.Resource) (map[string]any, error) {
	comparable, err := comparableRSA(res.Attributes)
	if err != nil {
		return nil, err
	}
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	adGroupName, err := p.adGroupResourceNameFromRef(res.Attributes[AttrAdGroup], c.CustomerID())
	if err != nil {
		return nil, err
	}

	ad := map[string]any{
		"finalUrls": comparable[AttrFinalUrls],
		"responsiveSearchAd": map[string]any{
			"headlines":    rsaMutateAssets(comparable[AttrHeadlines]),
			"descriptions": rsaMutateAssets(comparable[AttrDescriptions]),
		},
	}
	rsa := ad["responsiveSearchAd"].(map[string]any)
	if path1, ok := comparable[AttrPath1].(string); ok && path1 != "" {
		rsa["path1"] = path1
	}
	if path2, ok := comparable[AttrPath2].(string); ok && path2 != "" {
		rsa["path2"] = path2
	}

	body := map[string]any{
		"adGroup": adGroupName,
		"ad":      ad,
	}
	if status, ok := comparable[AttrStatus].(string); ok {
		body["status"] = status
	}
	return body, nil
}

func (p *Provider) rsaAdMutateBody(res resource.Resource, adResourceName string) (map[string]any, []string, error) {
	comparable, err := comparableRSA(res.Attributes)
	if err != nil {
		return nil, nil, err
	}
	rsa := map[string]any{
		"headlines":    rsaMutateAssets(comparable[AttrHeadlines]),
		"descriptions": rsaMutateAssets(comparable[AttrDescriptions]),
	}
	mask := []string{
		"responsiveSearchAd.headlines",
		"responsiveSearchAd.descriptions",
		"finalUrls",
	}
	if path1, ok := comparable[AttrPath1].(string); ok {
		rsa["path1"] = path1
		mask = append(mask, "responsiveSearchAd.path1")
	}
	if path2, ok := comparable[AttrPath2].(string); ok {
		rsa["path2"] = path2
		mask = append(mask, "responsiveSearchAd.path2")
	}
	sort.Strings(mask)
	return map[string]any{
		"resourceName":       adResourceName,
		"finalUrls":          comparable[AttrFinalUrls],
		"responsiveSearchAd": rsa,
	}, mask, nil
}

func requiredRSAFinalURLs(res resource.Resource) ([]string, error) {
	v, ok := res.Attributes[AttrFinalUrls]
	if !ok {
		return nil, fmt.Errorf("resource %s: missing required attribute %q", res.Address, AttrFinalUrls)
	}
	urls, err := parseRSAFinalURLs(v)
	if err != nil {
		return nil, fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrFinalUrls, err)
	}
	return urls, nil
}

func requiredRSAHeadlines(res resource.Resource) ([]rsaAsset, error) {
	v, ok := res.Attributes[AttrHeadlines]
	if !ok {
		return nil, fmt.Errorf("resource %s: missing required attribute %q", res.Address, AttrHeadlines)
	}
	assets, err := parseRSAAssets(v, rsaHeadlinePins, minRSAHeadlines, maxRSAHeadlines, maxRSAHeadlineRunes, "headline")
	if err != nil {
		return nil, fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrHeadlines, err)
	}
	return assets, nil
}

func requiredRSADescriptions(res resource.Resource) ([]rsaAsset, error) {
	v, ok := res.Attributes[AttrDescriptions]
	if !ok {
		return nil, fmt.Errorf("resource %s: missing required attribute %q", res.Address, AttrDescriptions)
	}
	assets, err := parseRSAAssets(v, rsaDescriptionPins, minRSADescriptions, maxRSADescriptions, maxRSADescriptionRunes, "description")
	if err != nil {
		return nil, fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrDescriptions, err)
	}
	return assets, nil
}

func optionalRSAPath(res resource.Resource, key string) (string, bool, error) {
	v, ok := res.Attributes[key]
	if !ok {
		return "", false, nil
	}
	path, err := normalizeRSAPath(v)
	if err != nil {
		return "", true, fmt.Errorf("resource %s: attribute %q %w", res.Address, key, err)
	}
	return path, true, nil
}

func parseRSAFinalURLs(v any) ([]string, error) {
	list, err := asAnyList(v)
	if err != nil {
		return nil, fmt.Errorf("must be a list of HTTPS landing-page URLs")
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("must include at least one final URL")
	}
	out := make([]string, 0, len(list))
	seen := map[string]struct{}{}
	for i, item := range list {
		raw, err := coerceString(item)
		if err != nil {
			return nil, fmt.Errorf("index %d must be a URL string", i)
		}
		normalized, err := normalizeRSAFinalURL(raw)
		if err != nil {
			return nil, fmt.Errorf("index %d %w", i, err)
		}
		if _, ok := seen[normalized]; ok {
			return nil, fmt.Errorf("duplicate final URL %q", normalized)
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out, nil
}

func normalizeRSAFinalURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("must be a non-empty URL")
	}
	if len(s) > maxRSAFinalURLBytes {
		return "", fmt.Errorf("must be at most %d bytes", maxRSAFinalURLBytes)
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("must be an absolute http or https URL")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("must be an absolute http or https URL")
	}
	return s, nil
}

func parseRSAAssets(v any, allowedPins map[string]struct{}, minCount, maxCount, maxRunes int, kind string) ([]rsaAsset, error) {
	list, err := asAnyList(v)
	if err != nil {
		return nil, fmt.Errorf("must be a list of %ss", kind)
	}
	if len(list) < minCount {
		return nil, fmt.Errorf("must include at least %d %ss", minCount, kind)
	}
	if len(list) > maxCount {
		return nil, fmt.Errorf("must include at most %d %ss", maxCount, kind)
	}
	out := make([]rsaAsset, 0, len(list))
	seenText := map[string]struct{}{}
	for i, item := range list {
		asset, err := parseRSAAsset(item, allowedPins, maxRunes, kind)
		if err != nil {
			return nil, fmt.Errorf("index %d %w", i, err)
		}
		key := strings.ToLower(asset.Text)
		if _, ok := seenText[key]; ok {
			return nil, fmt.Errorf("duplicate %s %q", kind, asset.Text)
		}
		seenText[key] = struct{}{}
		out = append(out, asset)
	}
	return out, nil
}

func parseRSAAsset(v any, allowedPins map[string]struct{}, maxRunes int, kind string) (rsaAsset, error) {
	if s, err := coerceString(v); err == nil {
		text := strings.TrimSpace(s)
		if text == "" {
			return rsaAsset{}, fmt.Errorf("must be a non-empty %s", kind)
		}
		if utf8.RuneCountInString(text) > maxRunes {
			return rsaAsset{}, fmt.Errorf("must be at most %d characters", maxRunes)
		}
		return rsaAsset{Text: text}, nil
	}
	m, ok := asMapValue(v)
	if !ok {
		return rsaAsset{}, fmt.Errorf("must be a string or an object with %q", AttrText)
	}
	for key := range m {
		if key != AttrText && key != AttrPin {
			return rsaAsset{}, fmt.Errorf("unsupported field %q; %ss support %s and optional %s", key, kind, AttrText, AttrPin)
		}
	}
	rawText, ok := m[AttrText]
	if !ok {
		return rsaAsset{}, fmt.Errorf("must include %q", AttrText)
	}
	text, err := coerceString(rawText)
	if err != nil {
		return rsaAsset{}, fmt.Errorf("%s %w", AttrText, err)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return rsaAsset{}, fmt.Errorf("must be a non-empty %s", kind)
	}
	if utf8.RuneCountInString(text) > maxRunes {
		return rsaAsset{}, fmt.Errorf("must be at most %d characters", maxRunes)
	}
	asset := rsaAsset{Text: text}
	if rawPin, ok := m[AttrPin]; ok {
		pin, err := coerceString(rawPin)
		if err != nil {
			return rsaAsset{}, fmt.Errorf("%s %w", AttrPin, err)
		}
		pin = normalizeEnum(pin)
		if _, allowed := allowedPins[pin]; !allowed {
			return rsaAsset{}, fmt.Errorf("%s must be one of %s", AttrPin, joinSorted(keys(allowedPins)))
		}
		asset.Pin = pin
	}
	return asset, nil
}

func normalizeRSAPath(v any) (string, error) {
	s, err := coerceString(v)
	if err != nil {
		return "", fmt.Errorf("must be a string")
	}
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		return "", fmt.Errorf("must not contain '/'")
	}
	if utf8.RuneCountInString(s) > maxRSAPathRunes {
		return "", fmt.Errorf("must be at most %d characters", maxRSAPathRunes)
	}
	return s, nil
}

func comparableRSAAssets(assets []rsaAsset) []any {
	out := make([]any, len(assets))
	for i, asset := range assets {
		out[i] = asset.comparable()
	}
	return out
}

func comparableAnyList(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}

func rsaMutateAssets(v any) []map[string]any {
	list, _ := v.([]any)
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		switch x := item.(type) {
		case string:
			out = append(out, map[string]any{"text": x})
		case map[string]any:
			asset := rsaAsset{Text: stringifyComparable(x[AttrText]), Pin: stringifyComparable(x[AttrPin])}
			out = append(out, asset.mutate())
		}
	}
	return out
}

func stringifyComparable(v any) string {
	s, _ := coerceString(v)
	return s
}

func asAnyList(v any) ([]any, error) {
	if v == nil {
		return nil, fmt.Errorf("must be a list")
	}
	switch list := v.(type) {
	case []any:
		return list, nil
	default:
		rv := reflect.ValueOf(v)
		if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
			return nil, fmt.Errorf("must be a list")
		}
		out := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = rv.Index(i).Interface()
		}
		return out, nil
	}
}

func intersectRSAValue(want, got any) any {
	wantList, wantErr := asAnyList(want)
	gotList, gotErr := asAnyList(got)
	if wantErr != nil || gotErr != nil || len(wantList) != len(gotList) {
		return got
	}
	out := make([]any, len(wantList))
	for i := range wantList {
		out[i] = intersectRSAAsset(wantList[i], gotList[i])
	}
	return out
}

func intersectRSAAsset(want, got any) any {
	if _, err := coerceString(want); err == nil {
		if s, err := coerceString(got); err == nil {
			return s
		}
		if _, ok := asMapValue(got); ok {
			return got
		}
		return got
	}
	return intersectComparableValue(want, got)
}

func rejectImmutableRSAChanges(want, got resource.Attributes) error {
	if !sameRef(want[AttrAdGroup], got[AttrAdGroup]) {
		return fmt.Errorf("adGroup is immutable and cannot be changed from %s to %s; create a new googleads.responsive_search_ad resource instead of moving this ad", logicalRef(got[AttrAdGroup]).Address, logicalRef(want[AttrAdGroup]).Address)
	}
	return nil
}

func rsaCreativeEquivalent(want, got resource.Attributes) bool {
	for _, key := range []string{AttrFinalUrls, AttrHeadlines, AttrDescriptions, AttrPath1, AttrPath2} {
		wv, wOK := want[key]
		if !wOK {
			continue
		}
		gv, gOK := got[key]
		if !gOK {
			return false
		}
		if !reflect.DeepEqual(wv, intersectRSAValue(wv, gv)) {
			return false
		}
	}
	return true
}

func rsaCreativeChanged(want, got resource.Attributes) bool {
	return !rsaCreativeEquivalent(want, got)
}

func rsaNaturalKey(res resource.Resource) (string, error) {
	ref, err := requiredAdGroupRef(res)
	if err != nil {
		return "", err
	}
	comparable, err := comparableRSA(res.Attributes)
	if err != nil {
		return "", err
	}
	return ref.Address.String() + "\x00" + rsaCreativeFingerprint(comparable), nil
}

func rsaCreativeFingerprint(attrs resource.Attributes) string {
	var b strings.Builder
	b.WriteString(fingerprintList(attrs[AttrHeadlines]))
	b.WriteByte('\x00')
	b.WriteString(fingerprintList(attrs[AttrDescriptions]))
	b.WriteByte('\x00')
	b.WriteString(fingerprintList(attrs[AttrFinalUrls]))
	b.WriteByte('\x00')
	if v, ok := attrs[AttrPath1].(string); ok {
		b.WriteString(v)
	}
	b.WriteByte('\x00')
	if v, ok := attrs[AttrPath2].(string); ok {
		b.WriteString(v)
	}
	return b.String()
}

func fingerprintList(v any) string {
	list, err := asAnyList(v)
	if err != nil {
		return ""
	}
	parts := make([]string, 0, len(list))
	for _, item := range list {
		if s, err := coerceString(item); err == nil {
			parts = append(parts, s)
			continue
		}
		if m, ok := asMapValue(item); ok {
			parts = append(parts, stringifyComparable(m[AttrText])+"\x01"+stringifyComparable(m[AttrPin]))
			continue
		}
		parts = append(parts, fmt.Sprint(item))
	}
	return strings.Join(parts, "\x02")
}

func (p *Provider) ensureRSAIdentityMatches(res resource.Resource) error {
	id, bound, err := boundRSAIdentity(res)
	if err != nil || !bound {
		return err
	}
	adGroupID, ok := p.adGroupIDFromRef(res.Attributes[AttrAdGroup])
	if !ok {
		return nil
	}
	gotAdGroupID, _, err := parseRSAID(res.Address, id)
	if err != nil {
		return err
	}
	if gotAdGroupID != adGroupID {
		return fmt.Errorf("resource %s: persisted identity %q does not match referenced ad group %s", res.Address, id, adGroupID)
	}
	return nil
}

func boundRSAIdentity(res resource.Resource) (string, bool, error) {
	if res.Identity.IsZero() {
		return "", false, nil
	}
	id, err := parseRSAIdentity(res.Address, res.Identity.ID)
	if err != nil {
		return "", true, err
	}
	return id, true, nil
}

func parseRSAIdentity(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("resource %s: persisted identity is empty; a Google Ads responsive search ad id of the form adGroupId~adId is required", addr)
	}
	adGroupID, adID, err := parseRSAID(addr, raw)
	if err != nil {
		return "", err
	}
	return rsaID(adGroupID, adID), nil
}

func (p *Provider) canonicalRSAImportID(addr resource.Address, raw string) (string, error) {
	id, err := parseImportRSAID(addr, raw)
	if err != nil {
		return "", err
	}
	if customerID, adGroupID, adID, ok := splitAdGroupAdResourceName(id); ok {
		configured, err := p.configuredCustomerID()
		if err != nil {
			return "", fmt.Errorf("googleads: import %s: %w", addr, err)
		}
		got, err := NormalizeCustomerID(customerID)
		if err != nil {
			return "", fmt.Errorf("googleads: import %s: %w", addr, err)
		}
		if got != configured {
			return "", fmt.Errorf("googleads: import %s: resource name customer %s does not match configured %s", addr, got, configured)
		}
		return rsaID(adGroupID, adID), nil
	}
	return id, nil
}

func parseImportRSAID(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("googleads: import %s: remote identifier is empty; expected adGroupId~adId or resource name customers/{customerId}/adGroupAds/{adGroupId}~{adId}", addr)
	}
	if _, adGroupID, adID, ok := splitAdGroupAdResourceName(raw); ok {
		if err := rsaPartIDError(addr, adGroupID); err != nil {
			return "", err
		}
		if err := rsaPartIDError(addr, adID); err != nil {
			return "", err
		}
		return raw, nil
	}
	adGroupID, adID, err := parseRSAID(addr, raw)
	if err != nil {
		return "", fmt.Errorf("googleads: import %s: %q is not a valid Google Ads responsive search ad id; expected adGroupId~adId or resource name customers/{customerId}/adGroupAds/{adGroupId}~{adId}", addr, raw)
	}
	return rsaID(adGroupID, adID), nil
}

func parseRSAID(addr resource.Address, raw string) (adGroupID, adID string, err error) {
	raw = strings.TrimSpace(raw)
	if _, agID, parsedAdID, ok := splitAdGroupAdResourceName(raw); ok {
		return agID, parsedAdID, nil
	}
	parts := strings.Split(raw, "~")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("resource %s: %q is not a valid Google Ads responsive search ad id; expected adGroupId~adId", addr, raw)
	}
	adGroupID = strings.TrimSpace(parts[0])
	adID = strings.TrimSpace(parts[1])
	if n, parseErr := strconv.ParseInt(adGroupID, 10, 64); parseErr != nil || n <= 0 {
		return "", "", fmt.Errorf("resource %s: %q is not a valid Google Ads responsive search ad id; expected adGroupId~adId", addr, raw)
	}
	if n, parseErr := strconv.ParseInt(adID, 10, 64); parseErr != nil || n <= 0 {
		return "", "", fmt.Errorf("resource %s: %q is not a valid Google Ads responsive search ad id; expected adGroupId~adId", addr, raw)
	}
	return adGroupID, adID, nil
}

func rsaPartIDError(addr resource.Address, id string) error {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("googleads: import %s: %q is not a valid Google Ads responsive search ad id; expected adGroupId~adId or resource name customers/{customerId}/adGroupAds/{adGroupId}~{adId}", addr, id)
	}
	return nil
}

func splitAdGroupAdResourceName(name string) (customerID, adGroupID, adID string, ok bool) {
	name = strings.TrimSpace(name)
	parts := strings.Split(name, "/")
	if len(parts) != 4 || parts[0] != "customers" || parts[2] != adGroupAdsCollection {
		return "", "", "", false
	}
	if _, err := NormalizeCustomerID(parts[1]); err != nil {
		return "", "", "", false
	}
	ids := strings.Split(parts[3], "~")
	if len(ids) != 2 {
		return "", "", "", false
	}
	if n, err := strconv.ParseInt(ids[0], 10, 64); err != nil || n <= 0 {
		return "", "", "", false
	}
	if n, err := strconv.ParseInt(ids[1], 10, 64); err != nil || n <= 0 {
		return "", "", "", false
	}
	return parts[1], ids[0], ids[1], true
}

func rsaID(adGroupID, adID string) string {
	return adGroupID + "~" + adID
}

func adGroupAdResourceName(customerID, id string) string {
	return "customers/" + customerID + "/" + adGroupAdsCollection + "/" + id
}

func adResourceName(customerID, compositeID string) string {
	_, adID, ok := strings.Cut(compositeID, "~")
	if !ok || strings.TrimSpace(adID) == "" {
		return adResourceNameFromParts(customerID, compositeID)
	}
	return adResourceNameFromParts(customerID, adID)
}

func adResourceNameFromParts(customerID, adID string) string {
	return "customers/" + customerID + "/" + adsCollection + "/" + adID
}

func parseRSAMutateID(raw json.RawMessage, customerID string) (string, error) {
	var resp struct {
		Results []struct {
			ResourceName string `json:"resourceName"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || len(resp.Results) == 0 {
		return "", fmt.Errorf("malformed mutate response")
	}
	resourceName := strings.TrimSpace(resp.Results[0].ResourceName)
	gotCustomer, adGroupID, adID, ok := splitAdGroupAdResourceName(resourceName)
	if !ok {
		return "", fmt.Errorf("malformed mutate response")
	}
	if customerID != "" && gotCustomer != customerID {
		return "", fmt.Errorf("mutate returned responsive search ad %s for a different customer", rsaID(adGroupID, adID))
	}
	return rsaID(adGroupID, adID), nil
}
