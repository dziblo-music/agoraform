package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Tag is a Matomo Tag Manager tag as returned by TagManager.getContainerTags.
//
// Field names follow the API JSON response. Values are normalized to
// strings so callers do not have to handle Matomo's mixed encodings.
// IDTag is the provider-native identifier within the draft version.
type Tag struct {
	IDTag              string
	IDContainerVersion string
	IDSite             string
	Type               string
	Name               string
	Status             string
	Description        string
	FireLimit          string
	FireDelay          string
	Priority           string
	StartDate          string
	EndDate            string
	FireTriggerIDs     []string
	BlockTriggerIDs    []string
	Parameters         map[string]any
}

// TagInput is the configurable subset sent to TagManager.addContainerTag
// and TagManager.updateContainerTag.
type TagInput struct {
	Type           string
	Name           string
	Parameters     map[string]any
	FireTriggerIDs []string
}

// TagPreservedFields is the unmanaged portion of a Tag Manager tag that
// must be carried forward on update. Matomo's update API replaces omitted
// fire/block triggers, fire limits, dates, description, and parameters
// with defaults.
type TagPreservedFields struct {
	Description     string
	BlockTriggerIDs []string
	FireLimit       string
	FireDelay       string
	Priority        string
	StartDate       string
	EndDate         string
	Parameters      map[string]any
}

type rawTag struct {
	IDTag              flexibleString  `json:"idtag"`
	IDContainerVersion flexibleString  `json:"idcontainerversion"`
	IDSite             flexibleString  `json:"idsite"`
	Type               flexibleString  `json:"type"`
	Name               flexibleString  `json:"name"`
	Status             flexibleString  `json:"status"`
	Description        flexibleString  `json:"description"`
	FireLimit          flexibleString  `json:"fire_limit"`
	FireLimitCamel     flexibleString  `json:"fireLimit"`
	FireDelay          flexibleString  `json:"fire_delay"`
	FireDelayCamel     flexibleString  `json:"fireDelay"`
	Priority           flexibleString  `json:"priority"`
	StartDate          flexibleString  `json:"start_date"`
	StartDateCamel     flexibleString  `json:"startDate"`
	EndDate            flexibleString  `json:"end_date"`
	EndDateCamel       flexibleString  `json:"endDate"`
	FireTriggerIDs     json.RawMessage `json:"fire_trigger_ids"`
	FireTriggerIDsAlt  json.RawMessage `json:"fireTriggerIds"`
	BlockTriggerIDs    json.RawMessage `json:"block_trigger_ids"`
	BlockTriggerIDsAlt json.RawMessage `json:"blockTriggerIds"`
	Parameters         json.RawMessage `json:"parameters"`
}

func (t rawTag) tag() (Tag, error) {
	params, err := decodeAnyMap(t.Parameters)
	if err != nil {
		return Tag{}, err
	}
	fireIDs, err := decodeIDList(firstRaw(t.FireTriggerIDs, t.FireTriggerIDsAlt))
	if err != nil {
		return Tag{}, err
	}
	blockIDs, err := decodeIDList(firstRaw(t.BlockTriggerIDs, t.BlockTriggerIDsAlt))
	if err != nil {
		return Tag{}, err
	}
	return Tag{
		IDTag:              string(t.IDTag),
		IDContainerVersion: string(t.IDContainerVersion),
		IDSite:             string(t.IDSite),
		Type:               string(t.Type),
		Name:               string(t.Name),
		Status:             string(t.Status),
		Description:        string(t.Description),
		FireLimit:          firstNonEmpty(string(t.FireLimit), string(t.FireLimitCamel)),
		FireDelay:          firstNonEmpty(string(t.FireDelay), string(t.FireDelayCamel)),
		Priority:           firstNonEmpty(string(t.Priority)),
		StartDate:          firstNonEmpty(string(t.StartDate), string(t.StartDateCamel)),
		EndDate:            firstNonEmpty(string(t.EndDate), string(t.EndDateCamel)),
		FireTriggerIDs:     fireIDs,
		BlockTriggerIDs:    blockIDs,
		Parameters:         params,
	}, nil
}

// GetContainerTags lists user-created tags in a container version.
func (t *TagManager) GetContainerTags(ctx context.Context, idContainerVersion string) ([]Tag, error) {
	if t == nil || t.c == nil {
		return nil, fmt.Errorf("matomo: tag manager client is nil")
	}
	idContainerVersion = strings.TrimSpace(idContainerVersion)
	if idContainerVersion == "" {
		return nil, fmt.Errorf("matomo: idContainerVersion is required")
	}
	params := url.Values{}
	params.Set("idContainerVersion", idContainerVersion)
	raw, err := t.Call(ctx, "getContainerTags", params)
	if err != nil {
		return nil, err
	}
	tags, err := decodeTags(raw)
	if err != nil {
		return nil, malformedResponseError("TagManager.getContainerTags", 0)
	}
	return tags, nil
}

// AddContainerTag creates a tag in the given container version and
// returns the new idtag.
func (t *TagManager) AddContainerTag(ctx context.Context, idContainerVersion string, in TagInput) (string, error) {
	if t == nil || t.c == nil {
		return "", fmt.Errorf("matomo: tag manager client is nil")
	}
	idContainerVersion = strings.TrimSpace(idContainerVersion)
	if idContainerVersion == "" {
		return "", fmt.Errorf("matomo: idContainerVersion is required")
	}
	params := tagInputValues(in, TagPreservedFields{})
	params.Set("idContainerVersion", idContainerVersion)
	raw, err := t.Call(ctx, "addContainerTag", params)
	if err != nil {
		return "", err
	}
	id, err := decodeTagID(raw)
	if err != nil {
		return "", err
	}
	return id, nil
}

// UpdateContainerTag updates a tag while carrying forward unmanaged
// Matomo fields.
func (t *TagManager) UpdateContainerTag(ctx context.Context, idContainerVersion, idTag string, in TagInput, preserved TagPreservedFields) error {
	if t == nil || t.c == nil {
		return fmt.Errorf("matomo: tag manager client is nil")
	}
	idContainerVersion = strings.TrimSpace(idContainerVersion)
	if idContainerVersion == "" {
		return fmt.Errorf("matomo: idContainerVersion is required")
	}
	idTag = strings.TrimSpace(idTag)
	if idTag == "" {
		return fmt.Errorf("matomo: idTag is required")
	}
	params := tagInputValues(in, preserved)
	params.Del("type") // updateContainerTag does not accept type
	params.Set("idContainerVersion", idContainerVersion)
	params.Set("idTag", idTag)
	params.Set("description", preserved.Description)
	params.Set("fireLimit", firstNonEmpty(preserved.FireLimit, "unlimited"))
	params.Set("fireDelay", firstNonEmpty(preserved.FireDelay, "0"))
	params.Set("priority", firstNonEmpty(preserved.Priority, "999"))
	if preserved.StartDate != "" {
		params.Set("startDate", preserved.StartDate)
	}
	if preserved.EndDate != "" {
		params.Set("endDate", preserved.EndDate)
	}
	for i, id := range preserved.BlockTriggerIDs {
		params.Set(fmt.Sprintf("blockTriggerIds[%d]", i), id)
	}
	_, err := t.Call(ctx, "updateContainerTag", params)
	return err
}

func tagInputValues(in TagInput, preserved TagPreservedFields) url.Values {
	params := url.Values{}
	if in.Type != "" {
		params.Set("type", in.Type)
	}
	params.Set("name", in.Name)
	for i, id := range in.FireTriggerIDs {
		params.Set(fmt.Sprintf("fireTriggerIds[%d]", i), id)
	}
	merged := mergeTagParameters(preserved.Parameters, in.Parameters)
	for key, value := range merged {
		setFormValue(params, "parameters["+key+"]", value)
	}
	return params
}

func mergeTagParameters(preserved, managed map[string]any) map[string]any {
	out := make(map[string]any, len(preserved)+len(managed))
	for k, v := range preserved {
		out[k] = v
	}
	for k, v := range managed {
		out[k] = v
	}
	if cfg, ok := out["matomoConfig"]; ok {
		out["matomoConfig"] = NormalizeMatomoConfig(cfg)
	}
	return out
}

// NormalizeMatomoConfig converts a live matomoConfig value to the
// {{Variable Name}} form expected by add/update APIs.
func NormalizeMatomoConfig(v any) any {
	switch x := v.(type) {
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return s
		}
		if strings.HasPrefix(s, "{{") && strings.HasSuffix(s, "}}") {
			return s
		}
		return "{{" + s + "}}"
	case map[string]any:
		name, _ := flexibleValueString(x["name"])
		name = strings.TrimSpace(name)
		if name == "" {
			return v
		}
		return "{{" + name + "}}"
	default:
		return v
	}
}

func decodeTags(raw json.RawMessage) ([]Tag, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	switch raw[0] {
	case '[':
		var items []rawTag
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		out := make([]Tag, 0, len(items))
		for _, item := range items {
			tag, err := item.tag()
			if err != nil {
				return nil, err
			}
			out = append(out, tag)
		}
		return out, nil
	case '{':
		var keyed map[string]json.RawMessage
		if err := json.Unmarshal(raw, &keyed); err != nil {
			return nil, err
		}
		if _, hasType := keyed["type"]; hasType || hasTagID(keyed) {
			var item rawTag
			if err := json.Unmarshal(raw, &item); err != nil {
				return nil, err
			}
			tag, err := item.tag()
			if err != nil {
				return nil, err
			}
			return []Tag{tag}, nil
		}
		out := make([]Tag, 0, len(keyed))
		for key, value := range keyed {
			value = bytes.TrimSpace(value)
			if len(value) == 0 || value[0] != '{' {
				return nil, fmt.Errorf("unexpected TagManager.getContainerTags payload")
			}
			var item rawTag
			if err := json.Unmarshal(value, &item); err != nil {
				return nil, err
			}
			tag, err := item.tag()
			if err != nil {
				return nil, err
			}
			if tag.IDTag == "" {
				tag.IDTag = strings.TrimSpace(key)
			}
			out = append(out, tag)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unexpected TagManager.getContainerTags payload")
	}
}

func hasTagID(keyed map[string]json.RawMessage) bool {
	_, ok := keyed["idtag"]
	return ok
}

func decodeTagID(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", fmt.Errorf("matomo: TagManager.addContainerTag returned no idTag")
	}

	var direct flexibleString
	if err := json.Unmarshal(raw, &direct); err == nil {
		id := strings.TrimSpace(string(direct))
		if id != "" {
			return id, nil
		}
	}

	var wrapped struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && len(bytes.TrimSpace(wrapped.Value)) > 0 {
		var inner flexibleString
		if err := json.Unmarshal(wrapped.Value, &inner); err == nil {
			id := strings.TrimSpace(string(inner))
			if id != "" {
				return id, nil
			}
		}
	}

	return "", fmt.Errorf("matomo: unexpected TagManager.addContainerTag payload")
}

func decodeAnyMap(raw json.RawMessage) (map[string]any, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]any{}, nil
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	if object == nil {
		return map[string]any{}, nil
	}
	return object, nil
}

func decodeIDList(raw json.RawMessage) ([]string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var items []any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, err := flexibleValueString(item)
		if err != nil {
			return nil, err
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

func firstRaw(values ...json.RawMessage) json.RawMessage {
	for _, raw := range values {
		raw = bytes.TrimSpace(raw)
		if len(raw) > 0 && string(raw) != "null" {
			return raw
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
