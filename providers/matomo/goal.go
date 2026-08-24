package matomo

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

const (
	// TypeGoal is the Matomo Goal resource type used in addresses such as
	// matomo.goal.trial_started.
	TypeGoal = "goal"

	// Configurable v0.1 attributes. Names follow Matomo API request
	// parameters rather than a cross-provider abstraction.
	AttrName           = "name"
	AttrMatchAttribute = "matchAttribute"
	AttrPattern        = "pattern"
	AttrPatternType    = "patternType"

	matchManually = "manually"

	defaultPatternType        = "contains"
	defaultNumericPatternType = "greater_than"
)

var (
	supportedGoalAttrs = map[string]struct{}{
		AttrName:           {},
		AttrMatchAttribute: {},
		AttrPattern:        {},
		AttrPatternType:    {},
	}

	computedGoalAttrs = map[string]struct{}{
		"idgoal":                           {},
		"idsite":                           {},
		"deleted":                          {},
		"allow_multiple":                   {},
		"event_value_as_revenue":           {},
		"case_sensitive":                   {},
		"caseSensitive":                    {},
		"revenue":                          {},
		"description":                      {},
		"allowMultipleConversionsPerVisit": {},
		"useEventValueAsRevenue":           {},
	}

	matchAttributes = map[string]matchKind{
		"url":                   matchURL,
		"title":                 matchText,
		"file":                  matchText,
		"external_website":      matchURL,
		"manually":              matchManual,
		"visit_duration":        matchNumeric,
		"visit_total_actions":   matchNumeric,
		"visit_total_pageviews": matchNumeric,
		"event_action":          matchEvent,
		"event_category":        matchEvent,
		"event_name":            matchEvent,
	}

	textPatternTypes    = map[string]struct{}{"contains": {}, "exact": {}, "regex": {}}
	numericPatternTypes = map[string]struct{}{"greater_than": {}}
)

type matchKind int

const (
	matchText matchKind = iota
	matchURL
	matchEvent
	matchNumeric
	matchManual
)

func (p *Provider) validateGoal(res resource.Resource) error {
	if err := p.requireSiteID(); err != nil {
		return fmt.Errorf("resource %s: %w", res.Address, err)
	}

	attrs := res.Attributes
	if attrs == nil {
		attrs = resource.Attributes{}
	}

	for key := range attrs {
		if _, ok := supportedGoalAttrs[key]; ok {
			continue
		}
		if _, computed := computedGoalAttrs[key]; computed {
			return fmt.Errorf("resource %s: %s is computed and cannot be set in configuration", res.Address, key)
		}
		return fmt.Errorf("resource %s: unsupported attribute %q; v0.1 matomo.goal supports %s, %s, %s, and %s", res.Address, key, AttrName, AttrMatchAttribute, AttrPattern, AttrPatternType)
	}

	name, err := requiredString(res, AttrName)
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("resource %s: attribute %q must be a non-empty string", res.Address, AttrName)
	}

	match, err := requiredString(res, AttrMatchAttribute)
	if err != nil {
		return err
	}
	kind, ok := matchAttributes[match]
	if !ok {
		return fmt.Errorf("resource %s: attribute %q must be one of %s", res.Address, AttrMatchAttribute, joinSorted(keys(matchAttributes)))
	}

	pattern, patternSet, err := optionalString(res, AttrPattern)
	if err != nil {
		return err
	}
	patternType, patternTypeSet, err := optionalString(res, AttrPatternType)
	if err != nil {
		return err
	}

	if kind == matchManual {
		if patternSet {
			return fmt.Errorf("resource %s: %s is not used when %s is %q", res.Address, AttrPattern, AttrMatchAttribute, matchManually)
		}
		if patternTypeSet {
			return fmt.Errorf("resource %s: %s is not used when %s is %q", res.Address, AttrPatternType, AttrMatchAttribute, matchManually)
		}
		return nil
	}

	if !patternSet || pattern == "" {
		return fmt.Errorf("resource %s: attribute %q is required when %s is %q", res.Address, AttrPattern, AttrMatchAttribute, match)
	}
	if kind == matchNumeric && !isNumericString(pattern) {
		return fmt.Errorf("resource %s: attribute %q must be numeric when %s is %q", res.Address, AttrPattern, AttrMatchAttribute, match)
	}

	if patternTypeSet {
		if err := validatePatternType(res.Address, match, kind, patternType); err != nil {
			return err
		}
	} else {
		patternType = defaultPatternTypeFor(kind)
	}

	if kind == matchURL && patternType == "exact" && !hasHTTPPrefix(pattern) {
		return fmt.Errorf("resource %s: attribute %q must start with http:// or https:// when %s is \"exact\" and %s is %q", res.Address, AttrPattern, AttrPatternType, AttrMatchAttribute, match)
	}

	return nil
}

func (p *Provider) readGoal(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateGoal(res); err != nil {
		return resource.RemoteResource{}, err
	}

	name, _, _ := optionalString(res, AttrName)
	goals, err := p.listGoals(ctx)
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: %w", res.Address, err)
	}

	matches := findGoalsByName(goals, name)
	switch len(matches) {
	case 0:
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: %w", res.Address, provider.ErrNotFound)
	case 1:
		return remoteGoal(res.Address, matches[0]), nil
	default:
		return resource.RemoteResource{}, fmt.Errorf("matomo: read %s: multiple remote goals named %q (ids %s); names must be unique", res.Address, name, joinGoalIDs(matches))
	}
}

func (p *Provider) createGoal(ctx context.Context, res resource.Resource) (resource.RemoteResource, error) {
	if err := p.validateGoal(res); err != nil {
		return resource.RemoteResource{}, err
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}

	id, err := c.Analytics().AddGoal(ctx, goalInput(res.Attributes))
	if err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: create %s: %w", res.Address, err)
	}

	live, err := p.readGoalByID(ctx, res.Address, id)
	if err == nil {
		return live, nil
	}
	return remoteGoal(res.Address, client.Goal{
		IDGoal:         id,
		Name:           stringAttr(res.Attributes, AttrName),
		MatchAttribute: stringAttr(res.Attributes, AttrMatchAttribute),
		Pattern:        stringAttr(res.Attributes, AttrPattern),
		PatternType:    comparablePatternType(res.Attributes),
	}), nil
}

func (p *Provider) updateGoal(ctx context.Context, desired resource.Resource, actual resource.RemoteResource) (resource.RemoteResource, error) {
	if err := p.validateGoal(desired); err != nil {
		return resource.RemoteResource{}, err
	}
	if actual.Identity.IsZero() {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: missing remote identity", desired.Address)
	}

	c, err := p.Client()
	if err != nil {
		return resource.RemoteResource{}, err
	}
	if err := c.Analytics().UpdateGoal(ctx, actual.Identity.ID, goalInput(desired.Attributes)); err != nil {
		return resource.RemoteResource{}, fmt.Errorf("matomo: update %s: %w", desired.Address, err)
	}

	live, err := p.readGoalByID(ctx, desired.Address, actual.Identity.ID)
	if err == nil {
		return live, nil
	}
	return remoteGoal(desired.Address, client.Goal{
		IDGoal:         actual.Identity.ID,
		Name:           stringAttr(desired.Attributes, AttrName),
		MatchAttribute: stringAttr(desired.Attributes, AttrMatchAttribute),
		Pattern:        stringAttr(desired.Attributes, AttrPattern),
		PatternType:    comparablePatternType(desired.Attributes),
	}), nil
}

func (p *Provider) readGoalByID(ctx context.Context, addr resource.Address, id string) (resource.RemoteResource, error) {
	goals, err := p.listGoals(ctx)
	if err != nil {
		return resource.RemoteResource{}, err
	}
	for _, g := range goals {
		if g.IDGoal == id {
			return remoteGoal(addr, g), nil
		}
	}
	return resource.RemoteResource{}, provider.ErrNotFound
}

func (p *Provider) listGoals(ctx context.Context) ([]client.Goal, error) {
	if err := p.requireSiteID(); err != nil {
		return nil, err
	}
	c, err := p.Client()
	if err != nil {
		return nil, err
	}
	return c.Analytics().GetGoals(ctx)
}

func (p *Provider) requireSiteID() error {
	if p == nil || strings.TrimSpace(p.cfg.SiteID) == "" {
		return fmt.Errorf("%s is required to manage goals", EnvSiteID)
	}
	return nil
}

func (p *Provider) normalizeGoalComparable(desired resource.Resource, live *resource.RemoteResource) (resource.Attributes, resource.Attributes, error) {
	want, err := comparableGoal(desired.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	if live == nil {
		return want, nil, nil
	}
	got, err := comparableGoal(live.Attributes)
	if err != nil {
		return nil, nil, fmt.Errorf("resource %s: %w", desired.Address, err)
	}
	return want, got, nil
}

func comparableGoal(attrs resource.Attributes) (resource.Attributes, error) {
	if attrs == nil {
		attrs = resource.Attributes{}
	}
	name, err := coerceString(attrs[AttrName])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrName, err)
	}
	match, err := coerceString(attrs[AttrMatchAttribute])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrMatchAttribute, err)
	}

	out := resource.Attributes{
		AttrName:           name,
		AttrMatchAttribute: match,
	}
	if match == matchManually {
		return out, nil
	}

	pattern, err := coerceString(attrs[AttrPattern])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrPattern, err)
	}
	patternType, err := coerceString(attrs[AttrPatternType])
	if err != nil {
		return nil, fmt.Errorf("attribute %q %w", AttrPatternType, err)
	}
	if patternType == "" {
		kind := matchAttributes[match]
		patternType = defaultPatternTypeFor(kind)
	}
	out[AttrPattern] = pattern
	out[AttrPatternType] = patternType
	return out, nil
}

func remoteGoal(addr resource.Address, g client.Goal) resource.RemoteResource {
	attrs := resource.Attributes{
		AttrName:           g.Name,
		AttrMatchAttribute: g.MatchAttribute,
	}
	if g.MatchAttribute != matchManually {
		if g.Pattern != "" {
			attrs[AttrPattern] = g.Pattern
		}
		if g.PatternType != "" {
			attrs[AttrPatternType] = g.PatternType
		}
	}

	computed := resource.Attributes{}
	setComputed(computed, "idgoal", g.IDGoal)
	setComputed(computed, "idsite", g.IDSite)
	setComputed(computed, "description", g.Description)
	setComputed(computed, "case_sensitive", g.CaseSensitive)
	setComputed(computed, "allow_multiple", g.AllowMultiple)
	setComputed(computed, "revenue", g.Revenue)
	setComputed(computed, "deleted", g.Deleted)
	setComputed(computed, "event_value_as_revenue", g.EventValueAsRevenue)

	return resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: g.IDGoal},
		Attributes: attrs,
		Computed:   computed,
	}
}

func goalInput(attrs resource.Attributes) client.GoalInput {
	match := stringAttr(attrs, AttrMatchAttribute)
	in := client.GoalInput{
		Name:           stringAttr(attrs, AttrName),
		MatchAttribute: match,
		Pattern:        stringAttr(attrs, AttrPattern),
		PatternType:    stringAttr(attrs, AttrPatternType),
	}
	if match == matchManually {
		// Older Matomo versions reject addGoal without these fields even
		// when matchAttribute is manually. getGoals then omits them.
		if in.Pattern == "" {
			in.Pattern = matchManually
		}
		if in.PatternType == "" {
			in.PatternType = defaultPatternType
		}
		return in
	}
	if in.PatternType == "" {
		in.PatternType = defaultPatternTypeFor(matchAttributes[match])
	}
	return in
}

func findGoalsByName(goals []client.Goal, name string) []client.Goal {
	var matches []client.Goal
	for _, g := range goals {
		if g.Deleted == "1" || strings.EqualFold(g.Deleted, "true") {
			continue
		}
		if g.Name == name {
			matches = append(matches, g)
		}
	}
	return matches
}

func joinGoalIDs(goals []client.Goal) string {
	ids := make([]string, 0, len(goals))
	for _, g := range goals {
		ids = append(ids, g.IDGoal)
	}
	sort.Strings(ids)
	return strings.Join(ids, ", ")
}

func setComputed(attrs resource.Attributes, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	attrs[key] = value
}

func requiredString(res resource.Resource, key string) (string, error) {
	v, ok := res.Attributes[key]
	if !ok {
		return "", fmt.Errorf("resource %s: missing required attribute %q", res.Address, key)
	}
	s, err := coerceString(v)
	if err != nil {
		return "", fmt.Errorf("resource %s: attribute %q %w", res.Address, key, err)
	}
	return s, nil
}

func optionalString(res resource.Resource, key string) (string, bool, error) {
	v, ok := res.Attributes[key]
	if !ok {
		return "", false, nil
	}
	s, err := coerceString(v)
	if err != nil {
		return "", true, fmt.Errorf("resource %s: attribute %q %w", res.Address, key, err)
	}
	return s, true, nil
}

func stringAttr(attrs resource.Attributes, key string) string {
	s, err := coerceString(attrs[key])
	if err != nil {
		return ""
	}
	return s
}

func comparablePatternType(attrs resource.Attributes) string {
	match := stringAttr(attrs, AttrMatchAttribute)
	patternType := stringAttr(attrs, AttrPatternType)
	if patternType != "" {
		return patternType
	}
	return defaultPatternTypeFor(matchAttributes[match])
}

func coerceString(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	switch x := v.(type) {
	case string:
		return x, nil
	case int:
		return strconv.Itoa(x), nil
	case int8:
		return strconv.FormatInt(int64(x), 10), nil
	case int16:
		return strconv.FormatInt(int64(x), 10), nil
	case int32:
		return strconv.FormatInt(int64(x), 10), nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case uint:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint8:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint64:
		return strconv.FormatUint(x, 10), nil
	case float32:
		return formatFloat(float64(x)), nil
	case float64:
		return formatFloat(x), nil
	default:
		return "", fmt.Errorf("must be a string")
	}
}

func formatFloat(n float64) string {
	if n == float64(int64(n)) && n >= -1e15 && n <= 1e15 {
		return strconv.FormatInt(int64(n), 10)
	}
	return strconv.FormatFloat(n, 'g', -1, 64)
}

func validatePatternType(addr resource.Address, match string, kind matchKind, patternType string) error {
	allowed := textPatternTypes
	if kind == matchNumeric {
		allowed = numericPatternTypes
	}
	if _, ok := allowed[patternType]; ok {
		return nil
	}
	return fmt.Errorf("resource %s: attribute %q must be one of %s when %s is %q", addr, AttrPatternType, joinSorted(keys(allowed)), AttrMatchAttribute, match)
}

func defaultPatternTypeFor(kind matchKind) string {
	if kind == matchNumeric {
		return defaultNumericPatternType
	}
	return defaultPatternType
}

func hasHTTPPrefix(pattern string) bool {
	lower := strings.ToLower(pattern)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func isNumericString(s string) bool {
	if s == "" {
		return false
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	return false
}

func keys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func joinSorted(values []string) string {
	sort.Strings(values)
	return strings.Join(values, ", ")
}
