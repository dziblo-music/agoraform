package googleads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

const (
	// TypeKeyword is the Google Ads Search keyword criterion type used in
	// addresses such as googleads.keyword.brand_exact.
	TypeKeyword = "keyword"

	// AttrAdGroup is a $ref to a googleads.ad_group.
	AttrAdGroup = "adGroup"
	// AttrText is the keyword text without match-type punctuation.
	AttrText = "text"
	// AttrMatchType is EXACT, PHRASE, or BROAD.
	AttrMatchType = "matchType"
	// AttrNegative marks a negative keyword criterion.
	AttrNegative = "negative"

	keywordMatchExact         = "EXACT"
	keywordMatchPhrase        = "PHRASE"
	keywordMatchBroad         = "BROAD"
	keywordTypeKeyword        = "KEYWORD"
	keywordStatusPaused       = "PAUSED"
	keywordStatusEnabled      = "ENABLED"
	adGroupCriteriaCollection = "adGroupCriteria"

	maxKeywordTextRunes = 80
	maxKeywordWords     = 10
)

var (
	supportedKeywordAttrs = map[string]struct{}{
		AttrAdGroup:   {},
		AttrText:      {},
		AttrMatchType: {},
		AttrNegative:  {},
		AttrStatus:    {},
		AttrCpcBid:    {},
	}

	computedKeywordAttrs = map[string]struct{}{
		"id":                       {},
		"resourceName":             {},
		"resource_name":            {},
		"criterionId":              {},
		"criterion_id":             {},
		"type":                     {},
		"cpcBidMicros":             {},
		"cpc_bid_micros":           {},
		"effectiveCpcBidMicros":    {},
		"effective_cpc_bid_micros": {},
		"qualityInfo":              {},
		"quality_info":             {},
		"approvalStatus":           {},
		"approval_status":          {},
		"systemServingStatus":      {},
		"system_serving_status":    {},
		"primaryStatus":            {},
		"primary_status":           {},
		"primaryStatusReasons":     {},
	}

	keywordMatchTypes = map[string]struct{}{
		keywordMatchExact:  {},
		keywordMatchPhrase: {},
		keywordMatchBroad:  {},
	}

	keywordStatuses = map[string]struct{}{
		keywordStatusPaused:  {},
		keywordStatusEnabled: {},
	}

	keywordSelect = strings.Join([]string{
		"SELECT",
		"ad_group_criterion.resource_name,",
		"ad_group_criterion.criterion_id,",
		"ad_group_criterion.ad_group,",
		"ad_group_criterion.status,",
		"ad_group_criterion.type,",
		"ad_group_criterion.negative,",
		"ad_group_criterion.keyword.text,",
		"ad_group_criterion.keyword.match_type,",
		"ad_group_criterion.cpc_bid_micros,",
		"ad_group_criterion.final_urls,",
		"ad_group_criterion.final_mobile_urls,",
		"ad_group_criterion.final_url_suffix,",
		"ad_group_criterion.tracking_url_template,",
		"ad_group_criterion.url_custom_parameters",
		"FROM ad_group_criterion",
	}, " ")
)

func (p *Provider) validateKeyword(res resource.Resource) error {
	if err := p.requireCustomerID(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}

	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedKeywordAttrs[key]; ok {
			continue
		}
		if _, computed := computedKeywordAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; googleads.keyword supports %s", res.Address, key, joinSorted(keys(supportedKeywordAttrs)))
	}

	if _, err := requiredAdGroupRef(res); err != nil {
		return err
	}
	if _, err := requiredKeywordText(res); err != nil {
		return err
	}
	if _, err := requiredKeywordMatchType(res); err != nil {
		return err
	}
	negative, _, err := optionalBool(res, AttrNegative)
	if err != nil {
		return err
	}
	status, statusSet, err := optionalEnum(res, AttrStatus, keywordStatuses)
	if err != nil {
		return err
	}
	if negative && statusSet && status != keywordStatusEnabled {
		return fmt.Errorf("resource %s: attribute %q must be %s for negative keywords; Google Ads does not support paused negative ad-group criteria", res.Address, AttrStatus, keywordStatusEnabled)
	}
	if _, set, err := optionalAdGroupCpcBidMicros(res); err != nil {
		return err
	} else if set && negative {
		return fmt.Errorf("resource %s: attribute %q cannot be set on negative keywords", res.Address, AttrCpcBid)
	}
	if err := p.ensureKeywordIdentityMatches(res); err != nil {
		return err
	}
	return nil
}

func (p *Provider) readKeyword(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateKeyword(res); err != nil {
		return resource.RemoteResource{}, err
	}

	id, bound, err := boundKeywordIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if bound {
		live, err := p.readKeywordByID(ctx, res.Address, id, res.Attributes)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return p.rememberLive(live), nil
	}

	adGroupID, ok := p.adGroupIDFromRef(res.Attributes[AttrAdGroup])
	if !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, provider.ErrNotFound)
	}
	text, _ := requiredKeywordText(res)
	matchType, _ := requiredKeywordMatchType(res)
	negative, _, _ := optionalBool(res, AttrNegative)
	// GAQL string equality is case-sensitive. Keyword text is compared after
	// normalizeKeywordText so capitalization-only differences still match.
	where := strings.Join([]string{
		"ad_group.id = " + adGroupID,
		"ad_group_criterion.type = " + gaqlString(keywordTypeKeyword),
		"ad_group_criterion.keyword.match_type = " + gaqlString(matchType),
		"ad_group_criterion.negative = " + gaqlBool(negative),
		"ad_group_criterion.status != " + gaqlString("REMOVED"),
	}, " AND ")
	candidates, err := p.queryKeywords(ctx, where)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
	}
	matches := make([]keywordData, 0, len(candidates))
	for _, item := range candidates {
		got, nerr := normalizeKeywordText(item.Text)
		if nerr != nil || got != text {
			continue
		}
		matches = append(matches, item)
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, provider.ErrNotFound)
	case 1:
		live, err := p.remoteKeyword(res.Address, matches[0], res.Attributes)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return p.rememberLive(live), nil
	default:
		ids := make([]string, 0, len(matches))
		for _, item := range matches {
			ids = append(ids, keywordID(item.AdGroupID, item.CriterionID))
		}
		sort.Strings(ids)
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: multiple remote keyword criteria for %q (%s) in ad group %s (ids %s); keyword text and match type must be unique within an ad group", res.Address, text, matchType, adGroupID, strings.Join(ids, ", "))
	}
}

func (p *Provider) createKeyword(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateKeyword(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundKeywordIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}

	body, _, err := p.keywordMutateBody(res, "")
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}
	if _, ok := body["status"]; !ok {
		body["status"] = keywordStatusPaused
	}
	if _, ok := body["negative"]; !ok {
		body["negative"] = false
	}

	raw, err := c.Mutate(ctx, adGroupCriteriaCollection, []map[string]any{
		{"create": body},
	})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}
	id, err := parseKeywordMutateID(raw, c.CustomerID())
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}

	live, err := p.readKeywordByID(ctx, res.Address, id, res.Attributes)
	if err == nil {
		return p.rememberLive(live), nil
	}
	fallback, ferr := p.remoteKeywordFromDesired(res, id, c.CustomerID())
	if ferr != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s succeeded but refreshing keyword %q failed: %w", res.Address, id, err)
	}
	return p.rememberLive(fallback), nil
}

func (p *Provider) updateKeyword(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
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

	want, got, err := p.normalizeKeywordComparable(desired, &live)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := rejectImmutableKeywordChanges(want, got); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}
	if reflect.DeepEqual(want, got) {
		return p.rememberLive(live), nil
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	resourceName := adGroupCriterionResourceName(c.CustomerID(), actual.Identity.ID)
	full, fullMask, err := p.keywordMutateBody(desired, resourceName)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}

	changed := changedKeywordAPIFields(want, got)
	body := map[string]any{"resourceName": resourceName}
	mask := make([]string, 0, len(changed))
	for _, field := range fullMask {
		if _, ok := changed[field]; !ok {
			continue
		}
		if value, ok := nestedMutateValue(full, field); ok {
			setNestedMutateValue(body, field, value)
		}
		mask = append(mask, field)
	}
	sort.Strings(mask)
	if len(mask) == 0 {
		return p.rememberLive(live), nil
	}

	_, err = c.Mutate(ctx, adGroupCriteriaCollection, []map[string]any{
		{
			"updateMask": strings.Join(mask, ","),
			"update":     body,
		},
	})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}

	refreshed, err := p.readKeywordByID(ctx, desired.Address, actual.Identity.ID, desired.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s succeeded but refreshing keyword %q failed: %w", desired.Address, actual.Identity.ID, err)
	}
	return p.rememberLive(refreshed), nil
}

func (p *Provider) importKeyword(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	id, err := p.canonicalKeywordImportID(addr, rawID)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireCustomerID(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	live, err := p.readKeywordByID(ctx, addr, id, nil)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: remote keyword %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	if _, ok := live.Attributes[AttrAdGroup]; !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: ad group is not bound in local state; import the googleads.ad_group resource first (or apply it), then re-import this keyword", addr)
	}
	return p.rememberLive(live), nil
}

func (p *Provider) normalizeKeywordComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	if err := p.validateKeyword(desired); err != nil {
		return nil, nil, err
	}
	want, err := comparableKeyword(desired.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if live == nil {
		return want, nil, nil
	}
	if id, bound, err := boundKeywordIdentity(desired); err != nil {
		return nil, nil, err
	} else if bound {
		if live.Identity.IsZero() || live.Identity.ID != id {
			return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, id, live.Identity.ID)
		}
	}
	gotFull, err := comparableKeyword(live.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	got := resource.Attributes{}
	for key := range want {
		if v, ok := gotFull[key]; ok {
			got[key] = intersectComparableValue(want[key], v)
			continue
		}
		if v, ok := live.Attributes[key]; ok {
			got[key] = intersectComparableValue(want[key], v)
		}
	}
	if err := rejectImmutableKeywordChanges(want, got); err != nil {
		return nil, nil, err
	}
	return want, got, nil
}

func (p *Provider) readKeywordByID(ctx context.Context, addr resource.Address, id string, desired resource.Attributes) (resource.RemoteResource, error) {
	adGroupID, criterionID, err := parseKeywordID(addr, id)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	where := "ad_group.id = " + adGroupID + " AND ad_group_criterion.criterion_id = " + criterionID
	matches, err := p.queryKeywords(ctx, where)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, provider.ErrNotFound
	case 1:
		return p.remoteKeyword(addr, matches[0], desired)
	default:
		return resource.RemoteResource{}, fmt.Errorf("multiple remote keyword criteria returned for id %s", id)
	}
}

func (p *Provider) queryKeywords(ctx context.Context, where string) ([]keywordData, error) {
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	query := keywordSelect
	if strings.TrimSpace(where) != "" {
		query += " WHERE " + where
	}
	query += " ORDER BY ad_group_criterion.criterion_id"
	rows, err := c.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]keywordData, 0, len(rows))
	for _, row := range rows {
		item, err := decodeKeywordRow(row, c.CustomerID())
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AdGroupID != out[j].AdGroupID {
			return out[i].AdGroupID < out[j].AdGroupID
		}
		return out[i].CriterionID < out[j].CriterionID
	})
	return out, nil
}

type keywordData struct {
	ResourceName string
	AdGroup      string
	AdGroupID    string
	CriterionID  string
	Status       string
	Type         string
	Negative     bool
	Text         string
	MatchType    string
	CpcBidMicros *int64
}

type keywordJSON struct {
	ResourceName        string                   `json:"resourceName"`
	CriterionID         json.Number              `json:"criterionId"`
	AdGroup             string                   `json:"adGroup"`
	Status              string                   `json:"status"`
	Type                string                   `json:"type"`
	Negative            *bool                    `json:"negative"`
	Keyword             *keywordInfoJSON         `json:"keyword"`
	CpcBidMicros        json.RawMessage          `json:"cpcBidMicros"`
	FinalURLs           []string                 `json:"finalUrls"`
	FinalMobileURLs     []string                 `json:"finalMobileUrls"`
	FinalURLSuffix      string                   `json:"finalUrlSuffix"`
	TrackingURLTemplate string                   `json:"trackingUrlTemplate"`
	URLCustomParameters []urlCustomParameterJSON `json:"urlCustomParameters"`
}

type urlCustomParameterJSON struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type keywordInfoJSON struct {
	Text      string `json:"text"`
	MatchType string `json:"matchType"`
}

func decodeKeywordRow(raw json.RawMessage, configuredCustomerID string) (keywordData, error) {
	malformed := func(detail string) (keywordData, error) {
		if detail == "" {
			return keywordData{}, fmt.Errorf("malformed keyword result")
		}
		return keywordData{}, fmt.Errorf("malformed keyword result: %s", detail)
	}

	var envelope struct {
		AdGroupCriterion json.RawMessage `json:"adGroupCriterion"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.AdGroupCriterion) == 0 {
		return malformed("")
	}
	var body keywordJSON
	if err := json.Unmarshal(envelope.AdGroupCriterion, &body); err != nil {
		return malformed("")
	}

	resourceName := strings.TrimSpace(body.ResourceName)
	resourceCustomerID, adGroupID, criterionID, ok := splitAdGroupCriterionResourceName(resourceName)
	if !ok {
		return malformed("invalid resourceName")
	}
	if configuredCustomerID != "" && resourceCustomerID != configuredCustomerID {
		return malformed("resourceName belongs to a different customer")
	}
	id := strings.TrimSpace(body.CriterionID.String())
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		return malformed("invalid criterionId")
	}
	if id != criterionID {
		return malformed("criterionId does not match resourceName")
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

	criterionType := normalizeEnum(body.Type)
	if criterionType == "" {
		return malformed("missing type")
	}
	if criterionType != keywordTypeKeyword {
		return keywordData{}, fmt.Errorf("ad group criterion %s has type %s; googleads.keyword only manages KEYWORD criteria", keywordID(adGroupID, id), criterionType)
	}
	if err := rejectUnsupportedKeywordURLSettings(keywordID(adGroupID, id), body); err != nil {
		return keywordData{}, err
	}
	if body.Keyword == nil {
		return malformed("missing keyword")
	}

	text, err := normalizeKeywordText(body.Keyword.Text)
	if err != nil {
		return keywordData{}, fmt.Errorf("keyword %s has invalid text: %w", keywordID(adGroupID, id), err)
	}
	matchType := normalizeEnum(body.Keyword.MatchType)
	if _, ok := keywordMatchTypes[matchType]; !ok {
		return keywordData{}, fmt.Errorf("keyword %s has match type %s; googleads.keyword supports %s", keywordID(adGroupID, id), matchType, joinSorted(keys(keywordMatchTypes)))
	}

	status := normalizeEnum(body.Status)
	if status == "REMOVED" {
		return keywordData{}, fmt.Errorf("keyword %s has status REMOVED; googleads.keyword does not manage removed keyword criteria", keywordID(adGroupID, id))
	}
	if status != "" {
		if _, ok := keywordStatuses[status]; !ok {
			return keywordData{}, fmt.Errorf("keyword %s has status %s; googleads.keyword supports %s", keywordID(adGroupID, id), status, joinSorted(keys(keywordStatuses)))
		}
	}

	negative := false
	if body.Negative != nil {
		negative = *body.Negative
	}

	item := keywordData{
		ResourceName: resourceName,
		AdGroup:      adGroupResourceName,
		AdGroupID:    adGroupID,
		CriterionID:  id,
		Status:       status,
		Type:         criterionType,
		Negative:     negative,
		Text:         text,
		MatchType:    matchType,
	}
	if n, err := parseOptionalMicros(body.CpcBidMicros); err == nil {
		item.CpcBidMicros = n
	}
	return item, nil
}

func (p *Provider) remoteKeyword(addr resource.Address, item keywordData, desired resource.Attributes) (resource.RemoteResource, error) {
	attrs := resource.Attributes{
		AttrText:      item.Text,
		AttrMatchType: item.MatchType,
		AttrNegative:  item.Negative,
	}
	if item.Status != "" {
		attrs[AttrStatus] = item.Status
	}

	adGroup, err := p.liveAdGroupAttr(addr, item.AdGroup, desired[AttrAdGroup])
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if adGroup != nil {
		attrs[AttrAdGroup] = adGroup
	}

	if !item.Negative && item.CpcBidMicros != nil && *item.CpcBidMicros > 0 {
		if desired != nil {
			if _, ok := desired[AttrCpcBid]; ok {
				attrs[AttrCpcBid] = amountFromMicros(*item.CpcBidMicros)
			}
		} else {
			attrs[AttrCpcBid] = amountFromMicros(*item.CpcBidMicros)
		}
	}

	id := keywordID(item.AdGroupID, item.CriterionID)
	computed := resource.Attributes{}
	setComputed(computed, "id", id)
	setComputed(computed, "resourceName", item.ResourceName)
	setComputed(computed, "type", item.Type)
	if item.CpcBidMicros != nil {
		setComputed(computed, "cpcBidMicros", strconv.FormatInt(*item.CpcBidMicros, 10))
	}

	return resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: id},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func (p *Provider) remoteKeywordFromDesired(res resource.Resource, id, customerID string) (resource.RemoteResource, error) {
	attrs, err := comparableKeyword(res.Attributes)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	computed := resource.Attributes{
		"id":           id,
		"resourceName": adGroupCriterionResourceName(customerID, id),
		"type":         keywordTypeKeyword,
	}
	return resource.RemoteResource{
		Address:    res.Address,
		Identity:   resource.Identity{ID: id},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func comparableKeyword(attrs resource.Attributes) (resource.Attributes, error) {
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	adGroup, err := comparableAdGroupAttr(attrs[AttrAdGroup])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrAdGroup, err)
	}
	text, err := coerceString(attrs[AttrText])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrText, err)
	}
	text, err = normalizeKeywordText(text)
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrText, err)
	}
	matchType, err := coerceString(attrs[AttrMatchType])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrMatchType, err)
	}
	matchType = normalizeEnum(matchType)
	if _, ok := keywordMatchTypes[matchType]; !ok {
		return nil, fmt.Errorf("attribute %q must be one of %s", AttrMatchType, joinSorted(keys(keywordMatchTypes)))
	}

	negative := false
	if _, ok := attrs[AttrNegative]; ok {
		negative, err = coerceBool(attrs[AttrNegative])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrNegative, err)
		}
	}

	status := keywordStatusPaused
	if negative {
		status = keywordStatusEnabled
	}
	if _, ok := attrs[AttrStatus]; ok {
		raw, err := coerceString(attrs[AttrStatus])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrStatus, err)
		}
		status = normalizeEnum(raw)
	}

	out := resource.Attributes{
		AttrAdGroup:   adGroup,
		AttrText:      text,
		AttrMatchType: matchType,
		AttrNegative:  negative,
		AttrStatus:    status,
	}
	if _, ok := attrs[AttrCpcBid]; ok {
		if negative {
			return nil, fmt.Errorf("attribute %q cannot be set on negative keywords", AttrCpcBid)
		}
		micros, err := adGroupCpcBidToMicros(attrs[AttrCpcBid])
		if err != nil {
			return nil, fmt.Errorf("attribute %q %w", AttrCpcBid, err)
		}
		out[AttrCpcBid] = amountFromMicros(micros)
	}
	return out, nil
}

func (p *Provider) keywordMutateBody(res resource.Resource, resourceName string) (map[string]any, []string, error) {
	comparable, err := comparableKeyword(res.Attributes)
	if err != nil {
		return nil, nil, err
	}
	c, err := p.Client()
	if err != nil {
		return nil, nil, err
	}

	body := map[string]any{}
	var mask []string
	if resourceName != "" {
		body["resourceName"] = resourceName
	}
	if status, ok := comparable[AttrStatus].(string); ok {
		body["status"] = status
		mask = append(mask, "status")
	}
	if resourceName == "" {
		adGroupName, err := p.adGroupResourceNameFromRef(res.Attributes[AttrAdGroup], c.CustomerID())
		if err != nil {
			return nil, nil, err
		}
		body["adGroup"] = adGroupName
		body["keyword"] = map[string]any{
			"text":      comparable[AttrText],
			"matchType": comparable[AttrMatchType],
		}
		if negative, ok := comparable[AttrNegative].(bool); ok {
			body["negative"] = negative
		}
	}
	if _, ok := comparable[AttrCpcBid]; ok {
		micros, err := adGroupCpcBidToMicros(comparable[AttrCpcBid])
		if err != nil {
			return nil, nil, err
		}
		body["cpcBidMicros"] = strconv.FormatInt(micros, 10)
		mask = append(mask, "cpcBidMicros")
	}
	sort.Strings(mask)
	return body, mask, nil
}

func (p *Provider) adGroupResourceNameFromRef(v any, customerID string) (string, error) {
	if resolved, ok := resource.AsResolved(v); ok {
		if name, err := coerceString(resolved.Outputs["resourceName"]); err == nil && strings.TrimSpace(name) != "" {
			return name, nil
		}
		if resolved.Identity.ID != "" {
			return adGroupResourceName(customerID, resolved.Identity.ID), nil
		}
		return "", fmt.Errorf("ad group reference %s has no provider-native identity", resolved.Address)
	}
	ref, ok := resource.AsRef(v)
	if !ok {
		return "", fmt.Errorf("must be a resource reference ($ref) to a %s.%s resource", Name, TypeAdGroup)
	}
	if id := p.lookupID(ref.Address); id != "" {
		return adGroupResourceName(customerID, id), nil
	}
	return "", fmt.Errorf("ad group reference %s has no provider-native identity", ref.Address)
}

func (p *Provider) liveAdGroupAttr(addr resource.Address, adGroupResourceName string, desired any) (any, error) {
	want := logicalRef(desired)
	_, adGroupID, ok := splitAdGroupResourceName(adGroupResourceName)
	if !ok {
		if adGroupResourceName == "" {
			return nil, nil
		}
		return nil, fmt.Errorf("resource %s: remote ad group resource name is invalid", addr)
	}
	if !want.IsZero() {
		wantID := ""
		if resolved, ok := resource.AsResolved(desired); ok {
			wantID = resolved.Identity.ID
		}
		if wantID == "" {
			wantID = p.lookupID(want.Address)
		}
		if wantID != "" && wantID == adGroupID {
			return want, nil
		}
	}
	managed, found, err := p.lookupManagedAddress(TypeAdGroup, adGroupID)
	if err != nil {
		return nil, err
	}
	if found {
		return resource.Ref{Address: managed}, nil
	}
	if !want.IsZero() {
		return adGroupID, nil
	}
	return nil, nil
}

func (p *Provider) adGroupIDFromRef(v any) (string, bool) {
	if resolved, ok := resource.AsResolved(v); ok {
		if resolved.Identity.ID != "" {
			return resolved.Identity.ID, true
		}
	}
	ref := logicalRef(v)
	if ref.IsZero() {
		return "", false
	}
	if id := p.lookupID(ref.Address); id != "" {
		return id, true
	}
	return "", false
}

func requiredAdGroupRef(res resource.Resource) (resource.Ref, error) {
	v, ok := res.Attributes[AttrAdGroup]
	if !ok {
		return resource.Ref{}, fmt.Errorf("resource %s: missing required attribute %q", res.Address, AttrAdGroup)
	}
	ref, err := adGroupRefValue(v)
	if err != nil {
		return resource.Ref{}, fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrAdGroup, err)
	}
	if ref.Address.Provider != Name || ref.Address.Type != TypeAdGroup {
		return resource.Ref{}, fmt.Errorf("resource %s: attribute %q must reference a %s.%s resource", res.Address, AttrAdGroup, Name, TypeAdGroup)
	}
	return ref, nil
}

func adGroupRefValue(v any) (resource.Ref, error) {
	if resolved, ok := resource.AsResolved(v); ok {
		return resource.Ref{Address: resolved.Address}, nil
	}
	ref, ok := resource.AsRef(v)
	if !ok {
		return resource.Ref{}, fmt.Errorf("must be a resource reference ($ref) to a %s.%s resource", Name, TypeAdGroup)
	}
	return ref, nil
}

func comparableAdGroupAttr(v any) (resource.Ref, error) {
	return adGroupRefValue(v)
}

func requiredKeywordText(res resource.Resource) (string, error) {
	raw, err := requiredString(res, AttrText)
	if err != nil {
		return "", err
	}
	text, err := normalizeKeywordText(raw)
	if err != nil {
		return "", fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrText, err)
	}
	return text, nil
}

func requiredKeywordMatchType(res resource.Resource) (string, error) {
	raw, err := requiredString(res, AttrMatchType)
	if err != nil {
		return "", err
	}
	matchType := normalizeEnum(raw)
	if _, ok := keywordMatchTypes[matchType]; !ok {
		return "", fmt.Errorf("resource %s: attribute %q must be one of %s", res.Address, AttrMatchType, joinSorted(keys(keywordMatchTypes)))
	}
	return matchType, nil
}

func normalizeKeywordText(raw string) (string, error) {
	text := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if text == "" {
		return "", fmt.Errorf("must be a non-empty keyword text")
	}
	if (strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]")) ||
		(strings.HasPrefix(text, `"`) && strings.HasSuffix(text, `"`)) {
		return "", fmt.Errorf("must not include match-type punctuation; set matchType to EXACT, PHRASE, or BROAD instead")
	}
	text = strings.ToLower(text)
	if utf8.RuneCountInString(text) > maxKeywordTextRunes {
		return "", fmt.Errorf("must be at most %d characters", maxKeywordTextRunes)
	}
	if len(strings.Fields(text)) > maxKeywordWords {
		return "", fmt.Errorf("must be at most %d words", maxKeywordWords)
	}
	return text, nil
}

func rejectImmutableKeywordChanges(want, got resource.Attributes) error {
	if !sameRef(want[AttrAdGroup], got[AttrAdGroup]) {
		return fmt.Errorf("adGroup is immutable and cannot be changed from %s to %s; keyword text, match type, negative, and ad group identify the Google Ads criterion and require a new resource", logicalRef(got[AttrAdGroup]).Address, logicalRef(want[AttrAdGroup]).Address)
	}
	if !reflect.DeepEqual(want[AttrText], got[AttrText]) {
		return fmt.Errorf("text is immutable and cannot be changed from %q to %q; create a new googleads.keyword resource instead of mutating this criterion", got[AttrText], want[AttrText])
	}
	if !reflect.DeepEqual(want[AttrMatchType], got[AttrMatchType]) {
		return fmt.Errorf("matchType is immutable and cannot be changed from %s to %s; create a new googleads.keyword resource instead of mutating this criterion", got[AttrMatchType], want[AttrMatchType])
	}
	if !reflect.DeepEqual(want[AttrNegative], got[AttrNegative]) {
		return fmt.Errorf("negative is immutable and cannot be changed from %v to %v; create a new googleads.keyword resource instead of mutating this criterion", got[AttrNegative], want[AttrNegative])
	}
	return nil
}

func changedKeywordAPIFields(want, got resource.Attributes) map[string]struct{} {
	changed := map[string]struct{}{}
	if !reflect.DeepEqual(want[AttrStatus], got[AttrStatus]) {
		changed["status"] = struct{}{}
	}
	if !reflect.DeepEqual(want[AttrCpcBid], got[AttrCpcBid]) {
		changed["cpcBidMicros"] = struct{}{}
	}
	return changed
}

func keywordNaturalKey(res resource.Resource) (string, error) {
	ref, err := requiredAdGroupRef(res)
	if err != nil {
		return "", err
	}
	text, err := requiredKeywordText(res)
	if err != nil {
		return "", err
	}
	matchType, err := requiredKeywordMatchType(res)
	if err != nil {
		return "", err
	}
	return ref.Address.String() + "\x00" + text + "\x00" + matchType, nil
}

func (p *Provider) ensureKeywordIdentityMatches(res resource.Resource) error {
	id, bound, err := boundKeywordIdentity(res)
	if err != nil || !bound {
		return err
	}
	adGroupID, ok := p.adGroupIDFromRef(res.Attributes[AttrAdGroup])
	if !ok {
		return nil
	}
	gotAdGroupID, _, err := parseKeywordID(res.Address, id)
	if err != nil {
		return err
	}
	if gotAdGroupID != adGroupID {
		return fmt.Errorf("resource %s: persisted identity %q does not match referenced ad group %s", res.Address, id, adGroupID)
	}
	return nil
}

func boundKeywordIdentity(res resource.Resource) (string, bool, error) {
	if res.Identity.IsZero() {
		return "", false, nil
	}
	id, err := parseKeywordIdentity(res.Address, res.Identity.ID)
	if err != nil {
		return "", true, err
	}
	return id, true, nil
}

func parseKeywordIdentity(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("resource %s: persisted identity is empty; a Google Ads keyword id of the form adGroupId~criterionId is required", addr)
	}
	adGroupID, criterionID, err := parseKeywordID(addr, raw)
	if err != nil {
		return "", err
	}
	return keywordID(adGroupID, criterionID), nil
}

func (p *Provider) canonicalKeywordImportID(addr resource.Address, raw string) (string, error) {
	id, err := parseImportKeywordID(addr, raw)
	if err != nil {
		return "", err
	}
	if customerID, adGroupID, criterionID, ok := splitAdGroupCriterionResourceName(id); ok {
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
		return keywordID(adGroupID, criterionID), nil
	}
	return id, nil
}

func parseImportKeywordID(addr resource.Address, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("googleads: import %s: remote identifier is empty; expected adGroupId~criterionId or resource name customers/{customerId}/adGroupCriteria/{adGroupId}~{criterionId}", addr)
	}
	if _, adGroupID, criterionID, ok := splitAdGroupCriterionResourceName(raw); ok {
		if err := keywordPartIDError(addr, adGroupID); err != nil {
			return "", err
		}
		if err := keywordPartIDError(addr, criterionID); err != nil {
			return "", err
		}
		return raw, nil
	}
	adGroupID, criterionID, err := parseKeywordID(addr, raw)
	if err != nil {
		return "", fmt.Errorf("googleads: import %s: %q is not a valid Google Ads keyword id; expected adGroupId~criterionId or resource name customers/{customerId}/adGroupCriteria/{adGroupId}~{criterionId}", addr, raw)
	}
	return keywordID(adGroupID, criterionID), nil
}

func parseKeywordID(addr resource.Address, raw string) (adGroupID, criterionID string, err error) {
	raw = strings.TrimSpace(raw)
	if _, agID, crID, ok := splitAdGroupCriterionResourceName(raw); ok {
		return agID, crID, nil
	}
	parts := strings.Split(raw, "~")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("resource %s: %q is not a valid Google Ads keyword id; expected adGroupId~criterionId", addr, raw)
	}
	adGroupID = strings.TrimSpace(parts[0])
	criterionID = strings.TrimSpace(parts[1])
	if n, parseErr := strconv.ParseInt(adGroupID, 10, 64); parseErr != nil || n <= 0 {
		return "", "", fmt.Errorf("resource %s: %q is not a valid Google Ads keyword id; expected adGroupId~criterionId", addr, raw)
	}
	if n, parseErr := strconv.ParseInt(criterionID, 10, 64); parseErr != nil || n <= 0 {
		return "", "", fmt.Errorf("resource %s: %q is not a valid Google Ads keyword id; expected adGroupId~criterionId", addr, raw)
	}
	return adGroupID, criterionID, nil
}

func keywordPartIDError(addr resource.Address, id string) error {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("googleads: import %s: %q is not a valid Google Ads keyword id; expected adGroupId~criterionId or resource name customers/{customerId}/adGroupCriteria/{adGroupId}~{criterionId}", addr, id)
	}
	return nil
}

func splitAdGroupCriterionResourceName(name string) (customerID, adGroupID, criterionID string, ok bool) {
	name = strings.TrimSpace(name)
	parts := strings.Split(name, "/")
	if len(parts) != 4 || parts[0] != "customers" || parts[2] != adGroupCriteriaCollection {
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

func keywordID(adGroupID, criterionID string) string {
	return adGroupID + "~" + criterionID
}

func rejectUnsupportedKeywordURLSettings(id string, body keywordJSON) error {
	var fields []string
	if hasNonEmptyStrings(body.FinalURLs) {
		fields = append(fields, "finalUrls")
	}
	if hasNonEmptyStrings(body.FinalMobileURLs) {
		fields = append(fields, "finalMobileUrls")
	}
	if strings.TrimSpace(body.FinalURLSuffix) != "" {
		fields = append(fields, "finalUrlSuffix")
	}
	if strings.TrimSpace(body.TrackingURLTemplate) != "" {
		fields = append(fields, "trackingUrlTemplate")
	}
	for _, param := range body.URLCustomParameters {
		if strings.TrimSpace(param.Key) != "" || strings.TrimSpace(param.Value) != "" {
			fields = append(fields, "urlCustomParameters")
			break
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return fmt.Errorf("ad group criterion %s has keyword-level URL or tracking settings (%s); googleads.keyword does not manage final URLs, mobile URLs, URL suffixes, tracking templates, or custom URL parameters", id, strings.Join(fields, ", "))
}

func hasNonEmptyStrings(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func adGroupCriterionResourceName(customerID, id string) string {
	return "customers/" + customerID + "/" + adGroupCriteriaCollection + "/" + id
}

func parseKeywordMutateID(raw json.RawMessage, customerID string) (string, error) {
	var resp struct {
		Results []struct {
			ResourceName string `json:"resourceName"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || len(resp.Results) == 0 {
		return "", fmt.Errorf("malformed mutate response")
	}
	resourceName := strings.TrimSpace(resp.Results[0].ResourceName)
	gotCustomer, adGroupID, criterionID, ok := splitAdGroupCriterionResourceName(resourceName)
	if !ok {
		return "", fmt.Errorf("malformed mutate response")
	}
	if customerID != "" && gotCustomer != customerID {
		return "", fmt.Errorf("mutate returned keyword %s for a different customer", keywordID(adGroupID, criterionID))
	}
	return keywordID(adGroupID, criterionID), nil
}

func gaqlBool(v bool) string {
	if v {
		return "TRUE"
	}
	return "FALSE"
}
