package googleads

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads/client"
)

const (
	geoTargetEnabledStatus = "ENABLED"
	geoTargetTypeCountry   = "Country"
)

type geoTargetConstant struct {
	ResourceName  string
	ID            string
	Name          string
	CanonicalName string
	CountryCode   string
	TargetType    string
	Status        string
}

type languageConstant struct {
	ResourceName string
	ID           string
	Code         string
	Name         string
	Targetable   bool
}

func (g geoTargetConstant) displayName() string {
	if strings.TrimSpace(g.CanonicalName) != "" {
		return g.CanonicalName
	}
	if strings.TrimSpace(g.Name) != "" {
		return g.Name
	}
	return g.ResourceName
}

func (g geoTargetConstant) cacheKeys() []string {
	keys := []string{
		strings.ToLower(strings.TrimSpace(g.ResourceName)),
		strings.ToLower(strings.TrimSpace(g.ID)),
		strings.ToLower(strings.TrimSpace(g.Name)),
		strings.ToLower(strings.TrimSpace(g.CanonicalName)),
	}
	if strings.EqualFold(g.TargetType, geoTargetTypeCountry) && g.CountryCode != "" {
		keys = append(keys, strings.ToLower(strings.TrimSpace(g.CountryCode)))
	}
	out := make([]string, 0, len(keys))
	seen := map[string]struct{}{}
	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func (l languageConstant) cacheKeys() []string {
	keys := []string{
		strings.ToLower(strings.TrimSpace(l.ResourceName)),
		strings.ToLower(strings.TrimSpace(l.ID)),
		strings.ToLower(strings.TrimSpace(l.Code)),
		strings.ToLower(strings.TrimSpace(l.Name)),
	}
	out := make([]string, 0, len(keys))
	seen := map[string]struct{}{}
	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func (p *Provider) resolveGeoTarget(ctx context.Context, addr resource.Address, raw any) (geoTargetConstant, error) {
	s, err := coerceString(raw)
	if err != nil {
		return geoTargetConstant{}, fmt.Errorf("resource %s: attribute %q %w", addr, AttrLocation, err)
	}
	query, err := normalizeLocationQuery(s)
	if err != nil {
		return geoTargetConstant{}, fmt.Errorf("resource %s: attribute %q %w", addr, AttrLocation, err)
	}
	if cached, ok := p.cachedGeo(query); ok {
		if err := rejectUnusableGeoTarget(addr, query, cached); err != nil {
			return geoTargetConstant{}, err
		}
		return cached, nil
	}

	kind, value := classifyLocationQuery(query)
	var resolved geoTargetConstant
	switch kind {
	case locationQueryID:
		item, err := p.lookupGeoTargetByID(ctx, addr, value)
		if err != nil {
			return geoTargetConstant{}, err
		}
		resolved = item
	case locationQueryCountryCode:
		item, found, err := p.lookupGeoTargetCountryCode(ctx, addr, value)
		if err != nil {
			return geoTargetConstant{}, err
		}
		if found {
			resolved = item
			break
		}
		item, err = p.lookupGeoTargetByName(ctx, addr, query)
		if err != nil {
			return geoTargetConstant{}, err
		}
		resolved = item
	default:
		item, err := p.lookupGeoTargetByName(ctx, addr, query)
		if err != nil {
			return geoTargetConstant{}, err
		}
		resolved = item
	}
	if err := rejectUnusableGeoTarget(addr, query, resolved); err != nil {
		return geoTargetConstant{}, err
	}
	p.storeGeo(query, resolved)
	return resolved, nil
}

func (p *Provider) resolveLanguage(ctx context.Context, addr resource.Address, raw any) (languageConstant, error) {
	s, err := coerceString(raw)
	if err != nil {
		return languageConstant{}, fmt.Errorf("resource %s: attribute %q %w", addr, AttrLanguage, err)
	}
	query, err := normalizeLanguageQuery(s)
	if err != nil {
		return languageConstant{}, fmt.Errorf("resource %s: attribute %q %w", addr, AttrLanguage, err)
	}
	if cached, ok := p.cachedLanguage(query); ok {
		if err := rejectUnusableLanguage(addr, query, cached); err != nil {
			return languageConstant{}, err
		}
		return cached, nil
	}

	kind, value := classifyLanguageQuery(query)
	var resolved languageConstant
	switch kind {
	case languageQueryID:
		item, err := p.lookupLanguageByID(ctx, addr, value)
		if err != nil {
			return languageConstant{}, err
		}
		resolved = item
	case languageQueryCode:
		item, found, err := p.lookupLanguageByCode(ctx, addr, value)
		if err != nil {
			return languageConstant{}, err
		}
		if found {
			resolved = item
			break
		}
		item, err = p.lookupLanguageByName(ctx, addr, query)
		if err != nil {
			return languageConstant{}, err
		}
		resolved = item
	default:
		item, err := p.lookupLanguageByName(ctx, addr, query)
		if err != nil {
			return languageConstant{}, err
		}
		resolved = item
	}
	if err := rejectUnusableLanguage(addr, query, resolved); err != nil {
		return languageConstant{}, err
	}
	p.storeLanguage(query, resolved)
	return resolved, nil
}

func (p *Provider) lookupGeoTargetByID(ctx context.Context, addr resource.Address, id string) (geoTargetConstant, error) {
	resourceName := id
	if _, parsed, ok := splitGeoTargetConstantResourceName(id); ok {
		resourceName = "geoTargetConstants/" + parsed
	} else {
		resourceName = "geoTargetConstants/" + id
	}
	matches, err := p.queryGeoTargetConstants(ctx, "geo_target_constant.resource_name = "+gaqlString(resourceName))
	if err != nil {
		return geoTargetConstant{}, fmt.Errorf("googleads: lookup location for %s: %w", addr, err)
	}
	switch len(matches) {
	case 0:
		return geoTargetConstant{}, fmt.Errorf("resource %s: attribute %q %q is not a known Google Ads geo target constant", addr, AttrLocation, id)
	case 1:
		return matches[0], nil
	default:
		return geoTargetConstant{}, ambiguousGeoTargetError(addr, id, matches)
	}
}

func (p *Provider) lookupGeoTargetCountryCode(ctx context.Context, addr resource.Address, code string) (geoTargetConstant, bool, error) {
	where := strings.Join([]string{
		"geo_target_constant.country_code = " + gaqlString(strings.ToUpper(code)),
		"geo_target_constant.target_type = " + gaqlString(geoTargetTypeCountry),
	}, " AND ")
	matches, err := p.queryGeoTargetConstants(ctx, where)
	if err != nil {
		return geoTargetConstant{}, false, fmt.Errorf("googleads: lookup location for %s: %w", addr, err)
	}
	enabled := enabledGeoTargets(matches)
	switch len(enabled) {
	case 0:
		return geoTargetConstant{}, false, nil
	case 1:
		return enabled[0], true, nil
	default:
		return geoTargetConstant{}, false, ambiguousGeoTargetError(addr, code, enabled)
	}
}

func (p *Provider) lookupGeoTargetByName(ctx context.Context, addr resource.Address, name string) (geoTargetConstant, error) {
	c, err := p.Client()
	if err != nil {
		return geoTargetConstant{}, err
	}
	suggestions, err := c.SuggestGeoTargetConstants(ctx, []string{name})
	if err != nil {
		return geoTargetConstant{}, fmt.Errorf("googleads: lookup location for %s: %w", addr, err)
	}

	var exact []geoTargetConstant
	seen := map[string]struct{}{}
	for _, suggestion := range suggestions {
		item := geoTargetFromClient(suggestion.Constant)
		if item.ResourceName == "" || !geoTargetNameMatches(item, name) {
			continue
		}
		if _, ok := seen[item.ResourceName]; ok {
			continue
		}
		seen[item.ResourceName] = struct{}{}
		exact = append(exact, item)
	}
	enabled := enabledGeoTargets(exact)
	switch len(enabled) {
	case 0:
		return geoTargetConstant{}, fmt.Errorf("resource %s: attribute %q %q was not found as a targetable Google Ads location; use a canonical name such as \"United States\" or \"California, United States\", a country code, or geoTargetConstants/{id}", addr, AttrLocation, name)
	case 1:
		return enabled[0], nil
	default:
		return geoTargetConstant{}, ambiguousGeoTargetError(addr, name, enabled)
	}
}

func (p *Provider) lookupLanguageByID(ctx context.Context, addr resource.Address, id string) (languageConstant, error) {
	resourceName := id
	if !strings.HasPrefix(strings.ToLower(id), "languageconstants/") {
		resourceName = "languageConstants/" + id
	}
	matches, err := p.queryLanguageConstants(ctx, "language_constant.resource_name = "+gaqlString(resourceName))
	if err != nil {
		return languageConstant{}, fmt.Errorf("googleads: lookup language for %s: %w", addr, err)
	}
	switch len(matches) {
	case 0:
		return languageConstant{}, fmt.Errorf("resource %s: attribute %q %q is not a known Google Ads language constant", addr, AttrLanguage, id)
	case 1:
		return matches[0], nil
	default:
		return languageConstant{}, ambiguousLanguageError(addr, id, matches)
	}
}

func (p *Provider) lookupLanguageByCode(ctx context.Context, addr resource.Address, code string) (languageConstant, bool, error) {
	matches, err := p.queryLanguageConstants(ctx, "language_constant.code = "+gaqlString(strings.ToLower(code)))
	if err != nil {
		return languageConstant{}, false, fmt.Errorf("googleads: lookup language for %s: %w", addr, err)
	}
	targetable := targetableLanguages(matches)
	switch len(targetable) {
	case 0:
		return languageConstant{}, false, nil
	case 1:
		return targetable[0], true, nil
	default:
		return languageConstant{}, false, ambiguousLanguageError(addr, code, targetable)
	}
}

func (p *Provider) lookupLanguageByName(ctx context.Context, addr resource.Address, name string) (languageConstant, error) {
	matches, err := p.queryLanguageConstants(ctx, "")
	if err != nil {
		return languageConstant{}, fmt.Errorf("googleads: lookup language for %s: %w", addr, err)
	}
	var exact []languageConstant
	for _, item := range matches {
		if strings.EqualFold(item.Name, name) || strings.EqualFold(item.Code, name) {
			exact = append(exact, item)
		}
	}
	targetable := targetableLanguages(exact)
	switch len(targetable) {
	case 0:
		return languageConstant{}, fmt.Errorf("resource %s: attribute %q %q was not found as a targetable Google Ads language; use an ISO code such as \"en\" or a language name such as \"English\"", addr, AttrLanguage, name)
	case 1:
		return targetable[0], nil
	default:
		return languageConstant{}, ambiguousLanguageError(addr, name, targetable)
	}
}

func (p *Provider) queryGeoTargetConstants(ctx context.Context, where string) ([]geoTargetConstant, error) {
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	query := strings.Join([]string{
		"SELECT",
		"geo_target_constant.resource_name,",
		"geo_target_constant.id,",
		"geo_target_constant.name,",
		"geo_target_constant.canonical_name,",
		"geo_target_constant.country_code,",
		"geo_target_constant.target_type,",
		"geo_target_constant.status",
		"FROM geo_target_constant",
	}, " ")
	if strings.TrimSpace(where) != "" {
		query += " WHERE " + where
	}
	rows, err := c.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]geoTargetConstant, 0, len(rows))
	for _, row := range rows {
		item, err := decodeGeoTargetConstantRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
		p.storeGeo("", item)
	}
	return out, nil
}

func (p *Provider) queryLanguageConstants(ctx context.Context, where string) ([]languageConstant, error) {
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	query := strings.Join([]string{
		"SELECT",
		"language_constant.resource_name,",
		"language_constant.id,",
		"language_constant.code,",
		"language_constant.name,",
		"language_constant.targetable",
		"FROM language_constant",
	}, " ")
	if strings.TrimSpace(where) != "" {
		query += " WHERE " + where
	}
	rows, err := c.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	out := make([]languageConstant, 0, len(rows))
	for _, row := range rows {
		item, err := decodeLanguageConstantRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
		p.storeLanguage("", item)
	}
	return out, nil
}

func decodeGeoTargetConstantRow(raw json.RawMessage) (geoTargetConstant, error) {
	var envelope struct {
		GeoTargetConstant *clientGeoTargetJSON `json:"geoTargetConstant"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.GeoTargetConstant == nil {
		return geoTargetConstant{}, fmt.Errorf("malformed geo target constant result")
	}
	item := geoTargetFromJSON(envelope.GeoTargetConstant)
	if item.ResourceName == "" && item.ID == "" {
		return geoTargetConstant{}, fmt.Errorf("malformed geo target constant result")
	}
	return item, nil
}

func decodeLanguageConstantRow(raw json.RawMessage) (languageConstant, error) {
	var envelope struct {
		LanguageConstant *languageConstantJSON `json:"languageConstant"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.LanguageConstant == nil {
		return languageConstant{}, fmt.Errorf("malformed language constant result")
	}
	id := strings.TrimSpace(envelope.LanguageConstant.ID.String())
	resourceName := strings.TrimSpace(envelope.LanguageConstant.ResourceName)
	if id == "" {
		if _, parsed, ok := splitLanguageConstantResourceName(resourceName); ok {
			id = parsed
		}
	}
	if resourceName == "" && id != "" {
		resourceName = "languageConstants/" + id
	}
	if resourceName == "" {
		return languageConstant{}, fmt.Errorf("malformed language constant result")
	}
	targetable := true
	if envelope.LanguageConstant.Targetable != nil {
		targetable = *envelope.LanguageConstant.Targetable
	}
	return languageConstant{
		ResourceName: resourceName,
		ID:           id,
		Code:         strings.ToLower(strings.TrimSpace(envelope.LanguageConstant.Code)),
		Name:         strings.TrimSpace(envelope.LanguageConstant.Name),
		Targetable:   targetable,
	}, nil
}

type clientGeoTargetJSON struct {
	ResourceName  string      `json:"resourceName"`
	ID            json.Number `json:"id"`
	Name          string      `json:"name"`
	CanonicalName string      `json:"canonicalName"`
	CountryCode   string      `json:"countryCode"`
	TargetType    string      `json:"targetType"`
	Status        string      `json:"status"`
}

type languageConstantJSON struct {
	ResourceName string      `json:"resourceName"`
	ID           json.Number `json:"id"`
	Code         string      `json:"code"`
	Name         string      `json:"name"`
	Targetable   *bool       `json:"targetable"`
}

func geoTargetFromJSON(body *clientGeoTargetJSON) geoTargetConstant {
	if body == nil {
		return geoTargetConstant{}
	}
	id := strings.TrimSpace(body.ID.String())
	resourceName := strings.TrimSpace(body.ResourceName)
	if id == "" {
		if _, parsed, ok := splitGeoTargetConstantResourceName(resourceName); ok {
			id = parsed
		}
	}
	if resourceName == "" && id != "" {
		resourceName = "geoTargetConstants/" + id
	}
	return geoTargetConstant{
		ResourceName:  resourceName,
		ID:            id,
		Name:          strings.TrimSpace(body.Name),
		CanonicalName: strings.TrimSpace(body.CanonicalName),
		CountryCode:   strings.TrimSpace(body.CountryCode),
		TargetType:    strings.TrimSpace(body.TargetType),
		Status:        strings.TrimSpace(body.Status),
	}
}

func geoTargetFromClient(v client.GeoTargetConstant) geoTargetConstant {
	return geoTargetConstant{
		ResourceName:  strings.TrimSpace(v.ResourceName),
		ID:            strings.TrimSpace(v.ID),
		Name:          strings.TrimSpace(v.Name),
		CanonicalName: strings.TrimSpace(v.CanonicalName),
		CountryCode:   strings.TrimSpace(v.CountryCode),
		TargetType:    strings.TrimSpace(v.TargetType),
		Status:        strings.TrimSpace(v.Status),
	}
}

func (p *Provider) cachedGeo(key string) (geoTargetConstant, bool) {
	if p == nil || strings.TrimSpace(key) == "" {
		return geoTargetConstant{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	item, ok := p.geoCache[strings.ToLower(strings.TrimSpace(key))]
	return item, ok
}

func (p *Provider) storeGeo(query string, item geoTargetConstant) {
	if p == nil || item.ResourceName == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.geoCache == nil {
		p.geoCache = map[string]geoTargetConstant{}
	}
	for _, key := range item.cacheKeys() {
		p.geoCache[key] = item
	}
	if q := strings.ToLower(strings.TrimSpace(query)); q != "" {
		p.geoCache[q] = item
	}
}

func (p *Provider) cachedLanguage(key string) (languageConstant, bool) {
	if p == nil || strings.TrimSpace(key) == "" {
		return languageConstant{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	item, ok := p.langCache[strings.ToLower(strings.TrimSpace(key))]
	return item, ok
}

func (p *Provider) storeLanguage(query string, item languageConstant) {
	if p == nil || item.ResourceName == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.langCache == nil {
		p.langCache = map[string]languageConstant{}
	}
	for _, key := range item.cacheKeys() {
		p.langCache[key] = item
	}
	if q := strings.ToLower(strings.TrimSpace(query)); q != "" {
		p.langCache[q] = item
	}
}

func normalizeLocationQuery(raw string) (string, error) {
	query := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if query == "" {
		return "", fmt.Errorf("must be a non-empty location name, country code, or geoTargetConstants/{id}")
	}
	return query, nil
}

func normalizeLanguageQuery(raw string) (string, error) {
	query := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if query == "" {
		return "", fmt.Errorf("must be a non-empty language code, name, or languageConstants/{id}")
	}
	return query, nil
}

const (
	locationQueryID          = "id"
	locationQueryCountryCode = "country"
	locationQueryName        = "name"
	languageQueryID          = "id"
	languageQueryCode        = "code"
	languageQueryName        = "name"
)

func classifyLocationQuery(query string) (kind, value string) {
	if _, id, ok := splitGeoTargetConstantResourceName(query); ok {
		return locationQueryID, id
	}
	if n, err := strconv.ParseInt(query, 10, 64); err == nil && n > 0 {
		return locationQueryID, query
	}
	if isCountryCode(query) {
		return locationQueryCountryCode, query
	}
	return locationQueryName, query
}

func classifyLanguageQuery(query string) (kind, value string) {
	if _, id, ok := splitLanguageConstantResourceName(query); ok {
		return languageQueryID, id
	}
	if n, err := strconv.ParseInt(query, 10, 64); err == nil && n > 0 {
		return languageQueryID, query
	}
	if isLanguageCode(query) {
		return languageQueryCode, strings.ToLower(query)
	}
	return languageQueryName, query
}

func splitGeoTargetConstantResourceName(name string) (prefix, id string, ok bool) {
	name = strings.TrimSpace(name)
	parts := strings.Split(name, "/")
	if len(parts) != 2 || !strings.EqualFold(parts[0], "geoTargetConstants") {
		return "", "", false
	}
	id = strings.TrimSpace(parts[1])
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		return "", "", false
	}
	return parts[0], id, true
}

func splitLanguageConstantResourceName(name string) (prefix, id string, ok bool) {
	name = strings.TrimSpace(name)
	parts := strings.Split(name, "/")
	if len(parts) != 2 || !strings.EqualFold(parts[0], "languageConstants") {
		return "", "", false
	}
	id = strings.TrimSpace(parts[1])
	if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 {
		return "", "", false
	}
	return parts[0], id, true
}

func isCountryCode(s string) bool {
	if len(s) != 2 {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

func isLanguageCode(s string) bool {
	n := len(s)
	if n < 2 || n > 3 {
		return false
	}
	for _, r := range s {
		if r < 'A' || (r > 'Z' && r < 'a') || r > 'z' {
			return false
		}
	}
	return true
}

func geoTargetNameMatches(item geoTargetConstant, query string) bool {
	return strings.EqualFold(item.Name, query) || strings.EqualFold(item.CanonicalName, query)
}

func enabledGeoTargets(items []geoTargetConstant) []geoTargetConstant {
	out := make([]geoTargetConstant, 0, len(items))
	for _, item := range items {
		status := normalizeEnum(item.Status)
		if status == "" || status == geoTargetEnabledStatus {
			out = append(out, item)
		}
	}
	return out
}

func targetableLanguages(items []languageConstant) []languageConstant {
	out := make([]languageConstant, 0, len(items))
	for _, item := range items {
		if item.Targetable {
			out = append(out, item)
		}
	}
	return out
}

func rejectUnusableGeoTarget(addr resource.Address, query string, item geoTargetConstant) error {
	status := normalizeEnum(item.Status)
	if status != "" && status != geoTargetEnabledStatus {
		return fmt.Errorf("resource %s: location %q resolves to Google Ads geo target %s (%s) with status %s; googleads.campaign_location only manages ENABLED target constants", addr, query, item.displayName(), item.ResourceName, status)
	}
	return nil
}

func rejectUnusableLanguage(addr resource.Address, query string, item languageConstant) error {
	if !item.Targetable {
		return fmt.Errorf("resource %s: language %q resolves to Google Ads language %s (%s), which is not targetable", addr, query, item.Name, item.ResourceName)
	}
	return nil
}

func ambiguousGeoTargetError(addr resource.Address, query string, matches []geoTargetConstant) error {
	labels := make([]string, 0, len(matches))
	for _, item := range matches {
		label := item.displayName()
		if item.ResourceName != "" {
			label += " (" + item.ResourceName + ")"
		}
		labels = append(labels, label)
	}
	sort.Strings(labels)
	return fmt.Errorf("resource %s: location %q is ambiguous; matches %s. Use a canonical name, country code, or geoTargetConstants/{id}", addr, query, strings.Join(labels, ", "))
}

func ambiguousLanguageError(addr resource.Address, query string, matches []languageConstant) error {
	labels := make([]string, 0, len(matches))
	for _, item := range matches {
		label := item.Code
		if item.Name != "" {
			label = item.Name + " (" + item.Code + ")"
		}
		labels = append(labels, label)
	}
	sort.Strings(labels)
	return fmt.Errorf("resource %s: language %q is ambiguous; matches %s. Use an ISO language code such as \"en\"", addr, query, strings.Join(labels, ", "))
}

func requiredLocationValue(res resource.Resource) (string, error) {
	raw, err := requiredString(res, AttrLocation)
	if err != nil {
		return "", err
	}
	query, err := normalizeLocationQuery(raw)
	if err != nil {
		return "", fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrLocation, err)
	}
	return query, nil
}

func requiredLanguageValue(res resource.Resource) (string, error) {
	raw, err := requiredString(res, AttrLanguage)
	if err != nil {
		return "", err
	}
	query, err := normalizeLanguageQuery(raw)
	if err != nil {
		return "", fmt.Errorf("resource %s: attribute %q %w", res.Address, AttrLanguage, err)
	}
	return query, nil
}
