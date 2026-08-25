package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Trigger is a Matomo Tag Manager trigger as returned by
// TagManager.getContainerTriggers.
//
// Field names follow the API JSON response. Values are normalized to
// strings so callers do not have to handle Matomo's mixed encodings.
// IDTrigger is the provider-native identifier within the draft version.
type Trigger struct {
	IDTrigger          string
	IDContainerVersion string
	IDSite             string
	Type               string
	Name               string
	Status             string
	Description        string
	Parameters         map[string]string
	Conditions         json.RawMessage
}

// TriggerInput is the configurable subset sent to
// TagManager.addContainerTrigger and TagManager.updateContainerTrigger.
type TriggerInput struct {
	Type       string
	Name       string
	Parameters map[string]string
}

// TriggerPreservedFields is the unmanaged portion of a Tag Manager
// trigger that must be carried forward on update. Matomo's update API
// replaces omitted conditions with an empty list and description with an
// empty string.
type TriggerPreservedFields struct {
	Description string
	Conditions  json.RawMessage
}

type rawTrigger struct {
	IDTrigger          flexibleString  `json:"idtrigger"`
	IDContainerVersion flexibleString  `json:"idcontainerversion"`
	IDSite             flexibleString  `json:"idsite"`
	Type               flexibleString  `json:"type"`
	Name               flexibleString  `json:"name"`
	Status             flexibleString  `json:"status"`
	Description        flexibleString  `json:"description"`
	Parameters         json.RawMessage `json:"parameters"`
	Conditions         json.RawMessage `json:"conditions"`
}

func (t rawTrigger) trigger() (Trigger, error) {
	params, err := decodeStringMap(t.Parameters)
	if err != nil {
		return Trigger{}, err
	}
	return Trigger{
		IDTrigger:          string(t.IDTrigger),
		IDContainerVersion: string(t.IDContainerVersion),
		IDSite:             string(t.IDSite),
		Type:               string(t.Type),
		Name:               string(t.Name),
		Status:             string(t.Status),
		Description:        string(t.Description),
		Parameters:         params,
		Conditions:         bytes.TrimSpace(t.Conditions),
	}, nil
}

// GetContainerTriggers lists user-created triggers in a container version.
func (t *TagManager) GetContainerTriggers(ctx context.Context, idContainerVersion string) ([]Trigger, error) {
	if t == nil || t.c == nil {
		return nil, fmt.Errorf("matomo: tag manager client is nil")
	}
	idContainerVersion = strings.TrimSpace(idContainerVersion)
	if idContainerVersion == "" {
		return nil, fmt.Errorf("matomo: idContainerVersion is required")
	}
	params := url.Values{}
	params.Set("idContainerVersion", idContainerVersion)
	raw, err := t.Call(ctx, "getContainerTriggers", params)
	if err != nil {
		return nil, err
	}
	triggers, err := decodeTriggers(raw)
	if err != nil {
		return nil, malformedResponseError("TagManager.getContainerTriggers", 0)
	}
	return triggers, nil
}

// AddContainerTrigger creates a trigger in the given container version
// and returns the new idtrigger.
func (t *TagManager) AddContainerTrigger(ctx context.Context, idContainerVersion string, in TriggerInput) (string, error) {
	if t == nil || t.c == nil {
		return "", fmt.Errorf("matomo: tag manager client is nil")
	}
	idContainerVersion = strings.TrimSpace(idContainerVersion)
	if idContainerVersion == "" {
		return "", fmt.Errorf("matomo: idContainerVersion is required")
	}
	params := triggerInputValues(in)
	params.Set("idContainerVersion", idContainerVersion)
	raw, err := t.Call(ctx, "addContainerTrigger", params)
	if err != nil {
		return "", err
	}
	id, err := decodeTriggerID(raw)
	if err != nil {
		return "", err
	}
	return id, nil
}

// UpdateContainerTrigger updates a trigger while carrying forward
// unmanaged Matomo fields.
func (t *TagManager) UpdateContainerTrigger(ctx context.Context, idContainerVersion, idTrigger string, in TriggerInput, preserved TriggerPreservedFields) error {
	if t == nil || t.c == nil {
		return fmt.Errorf("matomo: tag manager client is nil")
	}
	idContainerVersion = strings.TrimSpace(idContainerVersion)
	if idContainerVersion == "" {
		return fmt.Errorf("matomo: idContainerVersion is required")
	}
	idTrigger = strings.TrimSpace(idTrigger)
	if idTrigger == "" {
		return fmt.Errorf("matomo: idTrigger is required")
	}
	params := triggerInputValues(in)
	params.Del("type") // updateContainerTrigger does not accept type
	params.Set("idContainerVersion", idContainerVersion)
	params.Set("idTrigger", idTrigger)
	params.Set("description", preserved.Description)
	if err := setFormJSON(params, "conditions", preserved.Conditions, "TagManager.updateContainerTrigger"); err != nil {
		return err
	}
	_, err := t.Call(ctx, "updateContainerTrigger", params)
	return err
}

func triggerInputValues(in TriggerInput) url.Values {
	params := url.Values{}
	if in.Type != "" {
		params.Set("type", in.Type)
	}
	params.Set("name", in.Name)
	for key, value := range in.Parameters {
		params.Set("parameters["+key+"]", value)
	}
	return params
}

func decodeTriggers(raw json.RawMessage) ([]Trigger, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	switch raw[0] {
	case '[':
		var items []rawTrigger
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		out := make([]Trigger, 0, len(items))
		for _, item := range items {
			tr, err := item.trigger()
			if err != nil {
				return nil, err
			}
			out = append(out, tr)
		}
		return out, nil
	case '{':
		var keyed map[string]json.RawMessage
		if err := json.Unmarshal(raw, &keyed); err != nil {
			return nil, err
		}
		if _, hasType := keyed["type"]; hasType || hasTriggerID(keyed) {
			var item rawTrigger
			if err := json.Unmarshal(raw, &item); err != nil {
				return nil, err
			}
			tr, err := item.trigger()
			if err != nil {
				return nil, err
			}
			return []Trigger{tr}, nil
		}
		out := make([]Trigger, 0, len(keyed))
		for key, value := range keyed {
			value = bytes.TrimSpace(value)
			if len(value) == 0 || value[0] != '{' {
				return nil, fmt.Errorf("unexpected TagManager.getContainerTriggers payload")
			}
			var item rawTrigger
			if err := json.Unmarshal(value, &item); err != nil {
				return nil, err
			}
			tr, err := item.trigger()
			if err != nil {
				return nil, err
			}
			if tr.IDTrigger == "" {
				tr.IDTrigger = strings.TrimSpace(key)
			}
			out = append(out, tr)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unexpected TagManager.getContainerTriggers payload")
	}
}

func hasTriggerID(keyed map[string]json.RawMessage) bool {
	_, ok := keyed["idtrigger"]
	return ok
}

func decodeTriggerID(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", fmt.Errorf("matomo: TagManager.addContainerTrigger returned no idTrigger")
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

	return "", fmt.Errorf("matomo: unexpected TagManager.addContainerTrigger payload")
}
