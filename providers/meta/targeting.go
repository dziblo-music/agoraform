package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strings"

	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/meta/client"
)

const (
	targetCustomAudiences         = "customAudiences"
	targetExcludedCustomAudiences = "excludedCustomAudiences"
	targetInterests               = "interests"

	customAudienceFields = "id,account_id,name,subtype"
)

// targetingEntity is a stable Meta identifier with an optional display name.
// Names never participate in identity, plan comparison, or API writes.
type targetingEntity struct {
	ID   string
	Name string
}

var supportedCustomAudienceSubtypes = map[string]struct{}{
	"APP":                {},
	"APP_COMBINATION":    {},
	"CHAT":               {},
	"CLAIM":              {},
	"CUSTOM":             {},
	"DATA_SET":           {},
	"ENGAGEMENT":         {},
	"EVENT":              {},
	"IG_BUSINESS":        {},
	"LOOKALIKE":          {},
	"LOOKALIKE_VALUE":    {},
	"MANAGED":            {},
	"OFFLINE":            {},
	"OFFLINE_CONVERSION": {},
	"PARTNER":            {},
	"STORE_VISITS":       {},
	"VIDEO":              {},
	"WEBSITE":            {},
}

func (t normalizedTargeting) sameIDs(other normalizedTargeting) bool {
	return reflect.DeepEqual(comparableTargeting(t), comparableTargeting(other))
}

func comparableTargeting(t normalizedTargeting) normalizedTargeting {
	t.CustomAudiences = dropEntityNames(t.CustomAudiences)
	t.ExcludedCustomAudiences = dropEntityNames(t.ExcludedCustomAudiences)
	t.Interests = dropEntityNames(t.Interests)
	return t
}

func dropEntityNames(in []targetingEntity) []targetingEntity {
	if len(in) == 0 {
		return nil
	}
	out := make([]targetingEntity, len(in))
	for i, item := range in {
		out[i] = targetingEntity{ID: item.ID}
	}
	return out
}

func normalizeTargetingEntities(addr resource.Address, path string, v any, remote bool) ([]targetingEntity, error) {
	if v == nil {
		return nil, nil
	}
	items, err := anySlice(v)
	if err != nil {
		return nil, fmt.Errorf("resource %s: %s must be a list", addr, path)
	}
	if len(items) == 0 {
		return nil, nil
	}
	seen := map[string]struct{}{}
	out := make([]targetingEntity, 0, len(items))
	for i, item := range items {
		entity, err := normalizeTargetingEntity(addr, path, i, item, remote)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[entity.ID]; dup {
			return nil, fmt.Errorf("resource %s: %s contains duplicate id %s", addr, path, entity.ID)
		}
		seen[entity.ID] = struct{}{}
		out = append(out, entity)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func normalizeTargetingEntity(addr resource.Address, path string, i int, v any, remote bool) (targetingEntity, error) {
	if s, err := coerceString(v); err == nil {
		id, nerr := normalizeObjectID(s)
		if nerr != nil {
			return targetingEntity{}, fmt.Errorf("resource %s: %s[%d] must be a numeric Meta identifier", addr, path, i)
		}
		return targetingEntity{ID: id}, nil
	}
	m, ok := stringMap(v)
	if !ok {
		return targetingEntity{}, fmt.Errorf("resource %s: %s[%d] must be an object with a numeric id", addr, path, i)
	}
	if !remote {
		for key := range m {
			if key != "id" && key != "name" {
				return targetingEntity{}, fmt.Errorf("resource %s: %s[%d] has unsupported field %q", addr, path, i, key)
			}
		}
	}
	rawID, hasID := m["id"]
	rawName, hasName := m["name"]
	name := ""
	if hasName && rawName != nil {
		s, err := coerceString(rawName)
		if err != nil {
			return targetingEntity{}, fmt.Errorf("resource %s: %s[%d].name must be a string", addr, path, i)
		}
		name = strings.TrimSpace(s)
	}
	if !hasID || rawID == nil {
		if name != "" {
			return targetingEntity{}, fmt.Errorf("resource %s: %s[%d] must declare a numeric id; name-only matching is not supported", addr, path, i)
		}
		return targetingEntity{}, fmt.Errorf("resource %s: %s[%d] must declare a numeric id", addr, path, i)
	}
	s, err := coerceString(rawID)
	if err != nil {
		return targetingEntity{}, fmt.Errorf("resource %s: %s[%d].id must be a numeric Meta identifier", addr, path, i)
	}
	id, err := normalizeObjectID(s)
	if err != nil {
		return targetingEntity{}, fmt.Errorf("resource %s: %s[%d].id must be a numeric Meta identifier", addr, path, i)
	}
	return targetingEntity{ID: id, Name: name}, nil
}

func entityAttrs(entities []targetingEntity, includeNames bool) []any {
	out := make([]any, len(entities))
	for i, entity := range entities {
		item := map[string]any{"id": entity.ID}
		if includeNames && entity.Name != "" {
			item["name"] = entity.Name
		}
		out[i] = item
	}
	return out
}

func entityAPIObjects(entities []targetingEntity) []map[string]string {
	out := make([]map[string]string, len(entities))
	for i, entity := range entities {
		out[i] = map[string]string{"id": entity.ID}
	}
	return out
}

func overlappingEntityIDs(include, exclude []targetingEntity) []string {
	excluded := map[string]struct{}{}
	for _, entity := range exclude {
		excluded[entity.ID] = struct{}{}
	}
	var overlap []string
	for _, entity := range include {
		if _, ok := excluded[entity.ID]; ok {
			overlap = append(overlap, entity.ID)
		}
	}
	sort.Strings(overlap)
	return overlap
}

func parseRemoteInterests(addr resource.Address, m map[string]any) ([]targetingEntity, error) {
	flexible, hasFlexible := m["flexible_spec"]
	topLevel, hasTop := m["interests"]
	if hasFlexible && !isEmptyTargetingValue(flexible) {
		interests, err := parseFlexibleSpecInterests(addr, flexible)
		if err != nil {
			return nil, err
		}
		if hasTop && !isEmptyTargetingValue(topLevel) {
			also, err := normalizeTargetingEntities(addr, AttrTargeting+"."+targetInterests, topLevel, true)
			if err != nil {
				return nil, err
			}
			if !entityIDsEqual(interests, also) {
				return nil, fmt.Errorf("interests and flexible_spec describe different interest sets")
			}
		}
		return interests, nil
	}
	if !hasTop {
		return nil, nil
	}
	return normalizeTargetingEntities(addr, AttrTargeting+"."+targetInterests, topLevel, true)
}

func parseFlexibleSpecInterests(addr resource.Address, v any) ([]targetingEntity, error) {
	items, err := anySlice(v)
	if err != nil {
		return nil, fmt.Errorf("flexible_spec must be a list")
	}
	if len(items) == 0 {
		return nil, nil
	}
	if len(items) != 1 {
		return nil, fmt.Errorf("flexible_spec must contain a single interests group")
	}
	group, ok := stringMap(items[0])
	if !ok {
		return nil, fmt.Errorf("flexible_spec[0] must be an object")
	}
	for key := range group {
		if key != "interests" {
			return nil, fmt.Errorf("unsupported flexible_spec field %q", key)
		}
	}
	return normalizeTargetingEntities(addr, AttrTargeting+"."+targetInterests, group["interests"], true)
}

func entityIDsEqual(a, b []targetingEntity) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			return false
		}
	}
	return true
}

func isEmptyTargetingValue(v any) bool {
	if v == nil {
		return true
	}
	if items, err := anySlice(v); err == nil {
		return len(items) == 0
	}
	if m, ok := stringMap(v); ok {
		return len(m) == 0
	}
	return false
}

func (p *Provider) ensureTargetingReferences(ctx context.Context, addr resource.Address, targeting normalizedTargeting) error {
	if len(targeting.CustomAudiences) == 0 && len(targeting.ExcludedCustomAudiences) == 0 && len(targeting.Interests) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	for _, entity := range targeting.CustomAudiences {
		if err := p.ensureCustomAudience(ctx, addr, targetCustomAudiences, entity.ID, seen); err != nil {
			return err
		}
	}
	for _, entity := range targeting.ExcludedCustomAudiences {
		if err := p.ensureCustomAudience(ctx, addr, targetExcludedCustomAudiences, entity.ID, seen); err != nil {
			return err
		}
	}
	if err := p.ensureInterests(ctx, addr, targeting.Interests); err != nil {
		return err
	}
	return nil
}

func (p *Provider) ensureCustomAudience(ctx context.Context, addr resource.Address, field, id string, seen map[string]struct{}) error {
	if _, ok := seen[id]; ok {
		return nil
	}
	seen[id] = struct{}{}
	c, err := p.Client()
	if err != nil {
		return err
	}
	var item struct {
		ID        string `json:"id"`
		AccountID string `json:"account_id"`
		Name      string `json:"name"`
		Subtype   string `json:"subtype"`
	}
	if err := c.Get(ctx, id, url.Values{"fields": {customAudienceFields}}, &item); err != nil {
		return classifyTargetingReadError(addr, field, "custom audience", id, err)
	}
	gotID, err := normalizeObjectID(item.ID)
	if err != nil || gotID != id {
		return fmt.Errorf("resource %s: targeting.%s: custom audience %s is not a Custom Audience", addr, field, id)
	}
	subtype := strings.ToUpper(strings.TrimSpace(item.Subtype))
	if subtype == "" {
		return fmt.Errorf("resource %s: targeting.%s: id %s is not a Custom Audience", addr, field, id)
	}
	if _, ok := supportedCustomAudienceSubtypes[subtype]; !ok {
		return fmt.Errorf("resource %s: targeting.%s: custom audience %s has unsupported subtype %s", addr, field, id, subtype)
	}
	if item.AccountID != "" {
		got := strings.TrimPrefix(strings.TrimSpace(item.AccountID), "act_")
		want := strings.TrimPrefix(c.AdAccountID(), "act_")
		if got != "" && got != want {
			return fmt.Errorf("resource %s: targeting.%s: custom audience %s belongs to ad account %s, not the configured %s", addr, field, id, item.AccountID, c.AdAccountID())
		}
	}
	return nil
}

func (p *Provider) ensureInterests(ctx context.Context, addr resource.Address, interests []targetingEntity) error {
	if len(interests) == 0 {
		return nil
	}
	c, err := p.Client()
	if err != nil {
		return err
	}
	ids := make([]string, len(interests))
	for i, entity := range interests {
		ids[i] = entity.ID
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("resource %s: targeting.interests: %w", addr, err)
	}
	var result struct {
		Data []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Valid *bool  `json:"valid"`
		} `json:"data"`
	}
	if err := c.Get(ctx, "search", url.Values{"type": {"adinterestvalid"}, "interest_fbid_list": {string(raw)}}, &result); err != nil {
		return classifyTargetingReadError(addr, targetInterests, "interest", strings.Join(ids, ","), err)
	}
	found := map[string]bool{}
	for _, item := range result.Data {
		id, nerr := normalizeObjectID(item.ID)
		if nerr != nil {
			continue
		}
		valid := item.Valid == nil || *item.Valid
		found[id] = valid
	}
	for _, entity := range interests {
		valid, ok := found[entity.ID]
		if !ok || !valid {
			return fmt.Errorf("resource %s: targeting.interests: interest %s was not found", addr, entity.ID)
		}
	}
	return nil
}

func classifyTargetingReadError(addr resource.Address, field, kind, id string, err error) error {
	var api *client.Error
	if errors.As(err, &api) {
		if api.IsAuthentication() {
			return fmt.Errorf("resource %s: targeting.%s: authorization failed while reading %s %s: %w", addr, field, kind, id, err)
		}
		if api.IsPermission() {
			return fmt.Errorf("resource %s: targeting.%s: %s %s is not accessible with the configured token", addr, field, kind, id)
		}
		if client.IsNotFound(err) {
			return fmt.Errorf("resource %s: targeting.%s: %s %s was not found", addr, field, kind, id)
		}
	}
	return fmt.Errorf("resource %s: targeting.%s: could not read %s %s: %w", addr, field, kind, id, err)
}
