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
	// TypeCampaignLanguage is the Google Ads campaign language criterion type
	// used in addresses such as googleads.campaign_language.english.
	TypeCampaignLanguage = "campaign_language"

	// AttrLanguage is a human-reviewable language code, name, or
	// languageConstants/{id} identifier.
	AttrLanguage = "language"

	campaignCriterionTypeLanguage = "LANGUAGE"
)

var (
	supportedCampaignLanguageAttrs = map[string]struct{}{
		AttrCampaign: {},
		AttrLanguage: {},
	}

	computedCampaignLanguageAttrs = map[string]struct{}{
		"id":                {},
		"resourceName":      {},
		"resource_name":     {},
		"criterionId":       {},
		"criterion_id":      {},
		"type":              {},
		"languageConstant":  {},
		"language_constant": {},
		"status":            {},
		"negative":          {},
	}

	campaignLanguageSelect = strings.Join([]string{
		"SELECT",
		"campaign_criterion.resource_name,",
		"campaign_criterion.criterion_id,",
		"campaign_criterion.campaign,",
		"campaign_criterion.type,",
		"campaign_criterion.status,",
		"campaign_criterion.language.language_constant",
		"FROM campaign_criterion",
	}, " ")
)

func (p *Provider) validateCampaignLanguage(res resource.Resource) error {
	if err := p.requireCustomerID(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}

	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	for key := range attrs {
		if _, ok := supportedCampaignLanguageAttrs[key]; ok {
			continue
		}
		if _, computed := computedCampaignLanguageAttrs[key]; computed {
			if key == AttrNegative {
				return fmt.Errorf("resource %s: language criteria cannot be negative; googleads.campaign_language only manages included campaign languages", res.Address)
			}
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; googleads.campaign_language supports %s", res.Address, key, joinSorted(keys(supportedCampaignLanguageAttrs)))
	}

	if _, err := requiredCampaignRef(res); err != nil {
		return err
	}
	if _, err := requiredLanguageValue(res); err != nil {
		return err
	}
	return p.ensureCampaignCriterionIdentityMatches(res)
}

func (p *Provider) readCampaignLanguage(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateCampaignLanguage(res); err != nil {
		return resource.RemoteResource{}, err
	}

	id, bound, err := boundCampaignCriterionIdentity(res)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if bound {
		live, err := p.readCampaignLanguageByID(ctx, res.Address, id, res.Attributes)
		if err != nil {
			return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
		}
		return p.rememberLive(live), nil
	}

	campaignID, ok := p.campaignIDFromRef(res.Attributes[AttrCampaign])
	if !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, provider.ErrNotFound)
	}
	lang, err := p.resolveLanguage(ctx, res.Address, res.Attributes[AttrLanguage])
	if err != nil {
		return resource.RemoteResource{}, err
	}
	where := strings.Join([]string{
		"campaign.id = " + campaignID,
		"campaign_criterion.type = " + gaqlString(campaignCriterionTypeLanguage),
		"campaign_criterion.language.language_constant = " + gaqlString(lang.ResourceName),
		"campaign_criterion.status != " + gaqlString("REMOVED"),
	}, " AND ")
	matches, err := p.queryCampaignLanguages(ctx, where)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, err)
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: %w", res.Address, provider.ErrNotFound)
	case 1:
		live, err := p.remoteCampaignLanguage(ctx, res.Address, matches[0], res.Attributes)
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
		return resource.RemoteResource{}, fmt.Errorf("googleads: read %s: multiple remote language criteria for %s in campaign %s (ids %s); a campaign may target a language at most once", res.Address, lang.Code, campaignID, strings.Join(ids, ", "))
	}
}

func (p *Provider) createCampaignLanguage(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateCampaignLanguage(res); err != nil {
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
	body, err := p.campaignLanguageMutateBody(ctx, res, "")
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

	live, err := p.readCampaignLanguageByID(ctx, res.Address, id, res.Attributes)
	if err == nil {
		return p.rememberLive(live), nil
	}
	fallback, ferr := p.remoteCampaignLanguageFromDesired(ctx, res, id, c.CustomerID())
	if ferr != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: create %s succeeded but refreshing language criterion %q failed: %w", res.Address, id, err)
	}
	return p.rememberLive(fallback), nil
}

func (p *Provider) updateCampaignLanguage(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateCampaignLanguage(desired); err != nil {
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

	live, err := p.readCampaignLanguageByID(ctx, desired.Address, actual.Identity.ID, desired.Attributes)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: refreshing current language criterion: %w", desired.Address, err)
	}
	if _, _, err := p.normalizeCampaignLanguageComparable(ctx, desired, &live); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: update %s: %w", desired.Address, err)
	}
	return p.rememberLive(live), nil
}

func (p *Provider) importCampaignLanguage(ctx context.Context, addr resource.Address, rawID string) (resource.RemoteResource, error) {
	id, err := p.canonicalCampaignCriterionImportID(addr, rawID)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := p.requireCustomerID(); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	live, err := p.readCampaignLanguageByID(ctx, addr, id, nil)
	if err != nil {
		if errors.Is(err, provider.ErrNotFound) {
			return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: remote language criterion %q was not found: %w", addr, id, err)
		}
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: %w", addr, err)
	}
	if _, ok := live.Attributes[AttrCampaign]; !ok {
		return resource.RemoteResource{}, fmt.Errorf("googleads: import %s: campaign is not bound in local state; import the googleads.campaign resource first (or apply it), then re-import this language criterion", addr)
	}
	return p.rememberLive(live), nil
}

func (p *Provider) normalizeCampaignLanguageComparable(ctx context.Context, desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	if err := p.validateCampaignLanguage(desired); err != nil {
		return nil, nil, err
	}
	want, err := p.comparableCampaignLanguage(ctx, desired.Address, desired.Attributes)
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
	got, err := p.comparableCampaignLanguage(ctx, desired.Address, live.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if err := rejectImmutableCampaignLanguageChanges(want, got); err != nil {
		return nil, nil, err
	}
	return want, got, nil
}

func (p *Provider) readCampaignLanguageByID(ctx context.Context, addr resource.Address, id string, desired resource.Attributes) (resource.RemoteResource, error) {
	campaignID, criterionID, err := parseCampaignCriterionID(addr, id)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	where := "campaign.id = " + campaignID + " AND campaign_criterion.criterion_id = " + criterionID
	matches, err := p.queryCampaignLanguages(ctx, where)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, provider.ErrNotFound
	case 1:
		return p.remoteCampaignLanguage(ctx, addr, matches[0], desired)
	default:
		return resource.RemoteResource{}, fmt.Errorf("multiple remote language criteria returned for id %s", id)
	}
}

func (p *Provider) queryCampaignLanguages(ctx context.Context, where string) ([]campaignLanguageData, error) {
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	query := campaignLanguageSelect
	if strings.TrimSpace(where) != "" {
		query += " WHERE " + where
	}
	query += " ORDER BY campaign_criterion.criterion_id"
	rows, err := c.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]campaignLanguageData, 0, len(rows))
	for _, row := range rows {
		item, err := decodeCampaignLanguageRow(row, c.CustomerID())
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

type campaignLanguageData struct {
	ResourceName     string
	Campaign         string
	CampaignID       string
	CriterionID      string
	Type             string
	Status           string
	LanguageConstant string
}

type campaignLanguageJSON struct {
	ResourceName string      `json:"resourceName"`
	CriterionID  json.Number `json:"criterionId"`
	Campaign     string      `json:"campaign"`
	Type         string      `json:"type"`
	Status       string      `json:"status"`
	Language     *struct {
		LanguageConstant string `json:"languageConstant"`
	} `json:"language"`
}

func decodeCampaignLanguageRow(raw json.RawMessage, configuredCustomerID string) (campaignLanguageData, error) {
	malformed := func(detail string) (campaignLanguageData, error) {
		if detail == "" {
			return campaignLanguageData{}, fmt.Errorf("malformed language criterion result")
		}
		return campaignLanguageData{}, fmt.Errorf("malformed language criterion result: %s", detail)
	}

	var envelope struct {
		CampaignCriterion json.RawMessage `json:"campaignCriterion"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.CampaignCriterion) == 0 {
		return malformed("")
	}
	var body campaignLanguageJSON
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
	if criterionType != campaignCriterionTypeLanguage {
		return campaignLanguageData{}, fmt.Errorf("campaign criterion %s has type %s; googleads.campaign_language only manages LANGUAGE criteria", campaignCriterionID(campaignID, id), criterionType)
	}
	if body.Language == nil || strings.TrimSpace(body.Language.LanguageConstant) == "" {
		return malformed("missing language")
	}

	status := normalizeEnum(body.Status)
	if status == "REMOVED" {
		return campaignLanguageData{}, fmt.Errorf("language criterion %s has status REMOVED; googleads.campaign_language does not manage removed language criteria", campaignCriterionID(campaignID, id))
	}

	return campaignLanguageData{
		ResourceName:     resourceName,
		Campaign:         campaignResourceName,
		CampaignID:       campaignID,
		CriterionID:      id,
		Type:             criterionType,
		Status:           status,
		LanguageConstant: strings.TrimSpace(body.Language.LanguageConstant),
	}, nil
}

func (p *Provider) remoteCampaignLanguage(ctx context.Context, addr resource.Address, item campaignLanguageData, desired resource.Attributes) (resource.RemoteResource, error) {
	lang, err := p.resolveLanguage(ctx, addr, item.LanguageConstant)
	if err != nil {
		lang = languageConstant{ResourceName: item.LanguageConstant, Targetable: true}
		if _, id, ok := splitLanguageConstantResourceName(item.LanguageConstant); ok {
			lang.ID = id
		}
	}

	language := lang.Code
	if language == "" {
		language = item.LanguageConstant
	}
	if desired != nil {
		if raw, ok := desired[AttrLanguage]; ok {
			if query, nerr := coerceString(raw); nerr == nil {
				if resolved, rerr := p.resolveLanguage(ctx, addr, query); rerr == nil && resolved.ResourceName == lang.ResourceName && strings.TrimSpace(query) != "" {
					if isLanguageCode(strings.TrimSpace(query)) {
						language = strings.ToLower(strings.TrimSpace(query))
					}
				}
			}
		}
	}

	attrs := resource.Attributes{
		AttrLanguage: language,
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
	setComputed(computed, "languageConstant", item.LanguageConstant)
	setComputed(computed, "status", item.Status)

	return resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: id},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func (p *Provider) remoteCampaignLanguageFromDesired(ctx context.Context, res resource.Resource, id, customerID string) (resource.RemoteResource, error) {
	attrs, err := p.comparableCampaignLanguage(ctx, res.Address, res.Attributes)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	lang, err := p.resolveLanguage(ctx, res.Address, res.Attributes[AttrLanguage])
	if err != nil {
		return resource.RemoteResource{}, err
	}
	computed := resource.Attributes{
		"id":               id,
		"resourceName":     campaignCriterionResourceName(customerID, id),
		"type":             campaignCriterionTypeLanguage,
		"languageConstant": lang.ResourceName,
	}
	return resource.RemoteResource{
		Address:    res.Address,
		Identity:   resource.Identity{ID: id},
		Attributes: attrs,
		Computed:   computed,
	}, nil
}

func (p *Provider) comparableCampaignLanguage(ctx context.Context, addr resource.Address, attrs resource.Attributes) (resource.Attributes, error) {
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	campaign, err := comparableCampaignAttr(attrs[AttrCampaign])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrCampaign, err)
	}
	raw, err := coerceString(attrs[AttrLanguage])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrLanguage, err)
	}
	lang, err := p.resolveLanguage(ctx, addr, raw)
	if err != nil {
		return nil, err
	}
	code := lang.Code
	if code == "" {
		code = strings.ToLower(strings.TrimSpace(raw))
	}
	return resource.Attributes{
		AttrCampaign: campaign,
		AttrLanguage: code,
	}, nil
}

func (p *Provider) campaignLanguageMutateBody(ctx context.Context, res resource.Resource, resourceName string) (map[string]any, error) {
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	if resourceName != "" {
		body["resourceName"] = resourceName
		return body, nil
	}
	lang, err := p.resolveLanguage(ctx, res.Address, res.Attributes[AttrLanguage])
	if err != nil {
		return nil, err
	}
	campaignName, err := p.campaignResourceNameFromRef(res.Attributes[AttrCampaign], c.CustomerID())
	if err != nil {
		return nil, err
	}
	body["campaign"] = campaignName
	body["language"] = map[string]any{"languageConstant": lang.ResourceName}
	return body, nil
}

func rejectImmutableCampaignLanguageChanges(want, got resource.Attributes) error {
	if !sameRef(want[AttrCampaign], got[AttrCampaign]) {
		return fmt.Errorf("campaign is immutable and cannot be changed from %s to %s; create a new googleads.campaign_language resource instead of mutating this criterion", logicalRef(got[AttrCampaign]).Address, logicalRef(want[AttrCampaign]).Address)
	}
	if !reflect.DeepEqual(want[AttrLanguage], got[AttrLanguage]) {
		return fmt.Errorf("language is immutable and cannot be changed from %q to %q; create a new googleads.campaign_language resource instead of mutating this criterion", got[AttrLanguage], want[AttrLanguage])
	}
	return nil
}

func campaignLanguageNaturalKey(res resource.Resource) (string, error) {
	ref, err := requiredCampaignRef(res)
	if err != nil {
		return "", err
	}
	language, err := requiredLanguageValue(res)
	if err != nil {
		return "", err
	}
	return ref.Address.String() + "\x00" + strings.ToLower(language), nil
}
