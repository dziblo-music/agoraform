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

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

const (
	// TypeCampaignNegativeKeyword is the Google Ads campaign negative
	// keyword criterion type used in addresses such as
	// googleads.campaign_negative_keyword.jobs.
	TypeCampaignNegativeKeyword = "campaign_negative_keyword"
)

var (
	supportedCampaignNegativeKeywordAttrs = map[string]struct{}{
		AttrCampaign:  {},
		AttrText:      {},
		AttrMatchType: {},
	}

	computedCampaignNegativeKeywordAttrs = map[string]struct{}{
		"id":            {},
		"resourceName":  {},
		"resource_name": {},
		"criterionId":   {},
		"criterion_id":  {},
		"type":          {},
		"status":        {},
		AttrNegative:    {},
		AttrAdGroup:     {},
		AttrCpcBid:      {},
	}

	campaignNegativeKeywordSelect = strings.Join([]string{
		"SELECT",
		"campaign_criterion.resource_name,",
		"campaign_criterion.criterion_id,",
		"campaign_criterion.campaign,",
		"campaign_criterion.negative,",
		"campaign_criterion.type,",
		"campaign_criterion.status,",
		"campaign_criterion.keyword.text,",
		"campaign_criterion.keyword.match_type",
		"FROM campaign_criterion",
	}, " ")
)

func (p *Provider) validateCampaignNegativeKeyword(res resource.Resource) error {
	if err := p.requireCustomerID(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}

	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedCampaignNegativeKeywordAttrs[key]; ok {
			continue
		}
		if _, computed := computedCampaignNegativeKeywordAttrs[key]; computed {
			switch key {
			case AttrNegative:
				return fmt.Errorf("resource %s: campaign negative keywords are always negative; omit attribute %q", res.Address, AttrNegative)
			case AttrAdGroup:
				return fmt.Errorf("resource %s: campaign negative keywords attach to a campaign, not an ad group; use attribute %q with a $ref to googleads.campaign", res.Address, AttrCampaign)
			case AttrCpcBid:
				return fmt.Errorf("resource %s: attribute %q cannot be set on campaign negative keywords", res.Address, AttrCpcBid)
			default:
				return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
			}
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; googleads.campaign_negative_keyword supports %s", res.Address, key, joinSorted(keys(supportedCampaignNegativeKeywordAttrs)))
	}

	if _, err := requiredCampaignRef(res); err != nil {
		return err
	}
	if _, err := requiredKeywordText(res); err != nil {
		return err
	}
	if _, err := requiredKeywordMatchType(res); err != nil {
		return err
	}
	return p.ensureCampaignCriterionIdentityMatches(res)
}

func (p *Provider) readCampaignNegativeKeyword(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateCampaignNegativeKeyword(res); err != nil {
		return resource.RemoteResource{}, err
	}

	id, bound, err := boundCampaignCriterionIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if bound {
		live, err := p.readCampaignNegativeKeywordByID(ctx, res.Address, id, res.Attributes)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return p.rememberLive(live), nil
	}

	campaignID, ok := p.campaignIDFromRef(res.Attributes[AttrCampaign])
	if !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, provider.ErrNotFound)
	}
	text, _ := requiredKeywordText(res)
	matchType, _ := requiredKeywordMatchType(res)
	// GAQL string equality is case-sensitive. Keyword text is compared after
	// normalizeKeywordText so capitalization-only differences still match.
	where := strings.Join([]string{
		"campaign.id = " + campaignID,
		"campaign_criterion.type = " + gaqlString(keywordTypeKeyword),
		"campaign_criterion.keyword.match_type = " + gaqlString(matchType),
		"campaign_criterion.negative = " + gaqlBool(true),
		"campaign_criterion.status != " + gaqlString("REMOVED"),
	}, " AND ")
	candidates, err := p.queryCampaignNegativeKeywords(ctx, where)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
	}
	matches := make([]campaignNegativeKeywordData, 0, len(candidates))
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
		live, err := p.remoteCampaignNegativeKeyword(res.Address, matches[0], res.Attributes)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return p.rememberLive(live), nil
	default:
		ids := make([]string, 0, len(matches))
		for _, item := range matches {
			ids = append(ids, campaignCriterionID(item.CampaignID, item.CriterionID))
		}
		sort.Strings(ids)
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: multiple remote campaign negative keywords for %q (%s) in campaign %s (ids %s); keyword text and match type must be unique within a campaign", res.Address, text, matchType, campaignID, strings.Join(ids, ", "))
	}
}

func (p *Provider) createCampaignNegativeKeyword(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateCampaignNegativeKeyword(res); err != nil {
		return resource.RemoteResource{}, err
	}
	if _, bound, err := boundCampaignCriterionIdentity(res); err != nil {
		return resource.RemoteResource{}, err
	} else if bound {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: resource already has persisted identity %q", res.Address, res.Identity.ID)
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	body, err := p.campaignNegativeKeywordMutateBody(res, "")
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}

	raw, err := c.Mutate(ctx, campaignCriteriaCollection, []map[string]any{
		{"create": body},
	})
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}
	id, err := parseCampaignCriterionMutateID(raw, c.CustomerID())
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s: %w", res.Address, err)
	}

	live, err := p.readCampaignNegativeKeywordByID(ctx, res.Address, id, res.Attributes)
	if err == nil {
		return p.rememberLive(live), nil
	}
	fallback, ferr := p.remoteCampaignNegativeKeywordFromDesired(res, id, c.CustomerID())
	if ferr != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s succeeded but refreshing campaign negative keyword %q failed: %w", res.Address, id, err)
	}
	return p.rememberLive(fallback), nil
}

func (p *Provider) updateCampaignNegativeKeyword(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateCampaignNegativeKeyword(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: missing remote identity", desired.Address)
	}
	if id, bound, err := boundCampaignCriterionIdentity(desired); err != nil {
		return resource.RemoteResource{}, err
	} else if bound && id != actual.Identity.ID {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: persisted identity %q does not match planned remote identity %q", desired.Address, id, actual.Identity.ID)
	}

	live, err := p.readCampaignNegativeKeywordByID(ctx, desired.Address, actual.Identity.ID, desired.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: refreshing current campaign negative keyword: %w", desired.Address, err)
	}
	if _, _, err := p.normalizeCampaignNegativeKeywordComparable(desired, &live); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}
	return p.rememberLive(live), nil
}

func (p *Provider) importCampaignNegativeKeyword(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	id, err := p.canonicalCampaignCriterionImportID(addr, rawID)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireCustomerID(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	live, err := p.readCampaignNegativeKeywordByID(ctx, addr, id, nil)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: remote campaign negative keyword %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	if _, ok := live.Attributes[AttrCampaign]; !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: campaign is not bound in local state; import the googleads.campaign resource first (or apply it), then re-import this campaign negative keyword", addr)
	}
	return p.rememberLive(live), nil
}

func (p *Provider) normalizeCampaignNegativeKeywordComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	if err := p.validateCampaignNegativeKeyword(desired); err != nil {
		return nil, nil, err
	}
	want, err := comparableCampaignNegativeKeyword(desired.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if live == nil {
		return want, nil, nil
	}
	if id, bound, err := boundCampaignCriterionIdentity(desired); err != nil {
		return nil, nil, err
	} else if bound {
		if live.Identity.IsZero() || live.Identity.ID != id {
			return nil, nil, fmt.Errorf("resource %s: persisted identity %q does not match remote identity %q", desired.Address, id, live.Identity.ID)
		}
	}
	got, err := comparableCampaignNegativeKeyword(live.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if err := rejectImmutableCampaignNegativeKeywordChanges(want, got); err != nil {
		return nil, nil, err
	}
	return want, got, nil
}

func (p *Provider) readCampaignNegativeKeywordByID(ctx context.Context, addr resource.Address, id string, desired resource.Attributes) (resource.RemoteResource, error) {
	campaignID, criterionID, err := parseCampaignCriterionID(addr, id)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	where := "campaign.id = " + campaignID + " AND campaign_criterion.criterion_id = " + criterionID
	matches, err := p.queryCampaignNegativeKeywords(ctx, where)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, provider.ErrNotFound
	case 1:
		return p.remoteCampaignNegativeKeyword(addr, matches[0], desired)
	default:
		return resource.RemoteResource{}, fmt.Errorf("multiple remote campaign negative keywords returned for id %s", id)
	}
}

func (p *Provider) queryCampaignNegativeKeywords(ctx context.Context, where string) ([]campaignNegativeKeywordData, error) {
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	query := campaignNegativeKeywordSelect
	if strings.TrimSpace(where) != "" {
		query += " WHERE " + where
	}
	query += " ORDER BY campaign_criterion.criterion_id"
	rows, err := c.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]campaignNegativeKeywordData, 0, len(rows))
	for _, row := range rows {
		item, err := decodeCampaignNegativeKeywordRow(row, c.CustomerID())
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CampaignID != out[j].CampaignID {
			return out[i].CampaignID < out[j].CampaignID
		}
		return out[i].CriterionID < out[j].CriterionID
	})
	return out, nil
}

type campaignNegativeKeywordData struct {
	ResourceName string
	Campaign     string
	CampaignID   string
	CriterionID  string
	Negative     bool
	Type         string
	Status       string
	Text         string
	MatchType    string
}

type campaignNegativeKeywordJSON struct {
	ResourceName string      `json:"resourceName"`
	CriterionID  json.Number `json:"criterionId"`
	Campaign     string      `json:"campaign"`
	Negative     *bool       `json:"negative"`
	Type         string      `json:"type"`
	Status       string      `json:"status"`
	Keyword      *struct {
		Text      string `json:"text"`
		MatchType string `json:"matchType"`
	} `json:"keyword"`
}

func decodeCampaignNegativeKeywordRow(raw json.RawMessage, configuredCustomerID string) (campaignNegativeKeywordData, error) {
	malformed := func(detail string) (campaignNegativeKeywordData, error) {
		if detail == "" {
			return campaignNegativeKeywordData{}, fmt.Errorf("malformed campaign negative keyword result")
		}
		return campaignNegativeKeywordData{}, fmt.Errorf("malformed campaign negative keyword result: %s", detail)
	}

	var envelope struct {
		CampaignCriterion json.RawMessage `json:"campaignCriterion"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.CampaignCriterion) == 0 {
		return malformed("")
	}
	var body campaignNegativeKeywordJSON
	if err := json.Unmarshal(envelope.CampaignCriterion, &body); err != nil {
		return malformed("")
	}

	resourceName := strings.TrimSpace(body.ResourceName)
	resourceCustomerID, campaignID, criterionID, ok := splitCampaignCriterionResourceName(resourceName)
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

	campaignResourceName := strings.TrimSpace(body.Campaign)
	campaignCustomerID, parsedCampaignID, ok := splitCampaignResourceName(campaignResourceName)
	if !ok {
		return malformed("invalid campaign")
	}
	if configuredCustomerID != "" && campaignCustomerID != configuredCustomerID {
		return malformed("campaign belongs to a different customer")
	}
	if parsedCampaignID != campaignID {
		return malformed("campaign does not match resourceName")
	}

	criterionType := normalizeEnum(body.Type)
	if criterionType == "" {
		return malformed("missing type")
	}
	if criterionType != keywordTypeKeyword {
		return campaignNegativeKeywordData{}, fmt.Errorf("campaign criterion %s has type %s; googleads.campaign_negative_keyword only manages KEYWORD criteria", campaignCriterionID(campaignID, id), criterionType)
	}
	if body.Keyword == nil {
		return malformed("missing keyword")
	}

	text, err := normalizeKeywordText(body.Keyword.Text)
	if err != nil {
		return campaignNegativeKeywordData{}, fmt.Errorf("campaign negative keyword %s has invalid text: %w", campaignCriterionID(campaignID, id), err)
	}
	matchType := normalizeEnum(body.Keyword.MatchType)
	if _, ok := keywordMatchTypes[matchType]; !ok {
		return campaignNegativeKeywordData{}, fmt.Errorf("campaign negative keyword %s has match type %s; googleads.campaign_negative_keyword supports %s", campaignCriterionID(campaignID, id), matchType, joinSorted(keys(keywordMatchTypes)))
	}

	status := normalizeEnum(body.Status)
	if status == "REMOVED" {
		return campaignNegativeKeywordData{}, fmt.Errorf("campaign negative keyword %s has status REMOVED; googleads.campaign_negative_keyword does not manage removed keyword criteria", campaignCriterionID(campaignID, id))
	}

	negative := false
	if body.Negative != nil {
		negative = *body.Negative
	}
	if !negative {
		return campaignNegativeKeywordData{}, fmt.Errorf("campaign criterion %s is a positive keyword; googleads.campaign_negative_keyword only manages negative campaign keyword criteria", campaignCriterionID(campaignID, id))
	}

	return campaignNegativeKeywordData{
		ResourceName: resourceName,
		Campaign:     campaignResourceName,
		CampaignID:   campaignID,
		CriterionID:  id,
		Negative:     true,
		Type:         criterionType,
		Status:       status,
		Text:         text,
		MatchType:    matchType,
	}, nil
}

func (p *Provider) remoteCampaignNegativeKeyword(addr resource.Address, item campaignNegativeKeywordData, desired resource.Attributes) (resource.RemoteResource, error) {
	attrs := resource.Attributes{
		AttrText:      item.Text,
		AttrMatchType: item.MatchType,
	}
	campaign, err := p.liveCampaignGoalAttr(addr, item.Campaign, desired[AttrCampaign])
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if campaign != nil {
		attrs[AttrCampaign] = campaign
	}

	id := campaignCriterionID(item.CampaignID, item.CriterionID)
	computed := resource.Attributes{}
	setComputed(computed, "id", id)
	setComputed(computed, "resourceName", item.ResourceName)
	setComputed(computed, "type", item.Type)
	if item.Status != "" {
		setComputed(computed, "status", item.Status)
	}
	computed[AttrNegative] = true

	return resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: id},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func (p *Provider) remoteCampaignNegativeKeywordFromDesired(res resource.Resource, id, customerID string) (resource.RemoteResource, error) {
	attrs, err := comparableCampaignNegativeKeyword(res.Attributes)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	computed := resource.Attributes{
		"id":           id,
		"resourceName": campaignCriterionResourceName(customerID, id),
		"type":         keywordTypeKeyword,
		AttrNegative:   true,
	}
	return resource.RemoteResource{
		Address:    res.Address,
		Identity:   resource.Identity{ID: id},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func comparableCampaignNegativeKeyword(attrs resource.Attributes) (resource.Attributes, error) {
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	campaign, err := comparableCampaignAttr(attrs[AttrCampaign])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrCampaign, err)
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
	return resource.Attributes{
		AttrCampaign:  campaign,
		AttrText:      text,
		AttrMatchType: matchType,
	}, nil
}

func (p *Provider) campaignNegativeKeywordMutateBody(res resource.Resource, resourceName string) (map[string]any, error) {
	comparable, err := comparableCampaignNegativeKeyword(res.Attributes)
	if err != nil {
		return nil, err
	}
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	if resourceName != "" {
		body["resourceName"] = resourceName
		return body, nil
	}
	campaignName, err := p.campaignResourceNameFromRef(res.Attributes[AttrCampaign], c.CustomerID())
	if err != nil {
		return nil, err
	}
	body["campaign"] = campaignName
	body["negative"] = true
	body["keyword"] = map[string]any{
		"text":      comparable[AttrText],
		"matchType": comparable[AttrMatchType],
	}
	return body, nil
}

func rejectImmutableCampaignNegativeKeywordChanges(want, got resource.Attributes) error {
	if !sameRef(want[AttrCampaign], got[AttrCampaign]) {
		return fmt.Errorf("campaign is immutable and cannot be changed from %s to %s; create a new googleads.campaign_negative_keyword resource instead of mutating this criterion", logicalRef(got[AttrCampaign]).Address, logicalRef(want[AttrCampaign]).Address)
	}
	if !reflect.DeepEqual(want[AttrText], got[AttrText]) {
		return fmt.Errorf("text is immutable and cannot be changed from %q to %q; create a new googleads.campaign_negative_keyword resource instead of mutating this criterion", got[AttrText], want[AttrText])
	}
	if !reflect.DeepEqual(want[AttrMatchType], got[AttrMatchType]) {
		return fmt.Errorf("matchType is immutable and cannot be changed from %s to %s; create a new googleads.campaign_negative_keyword resource instead of mutating this criterion", got[AttrMatchType], want[AttrMatchType])
	}
	return nil
}

func campaignNegativeKeywordNaturalKey(res resource.Resource) (string, error) {
	ref, err := requiredCampaignRef(res)
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
