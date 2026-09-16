package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// AddContainerTriggerWithConditions creates a trigger with an explicit native
// Matomo conditions array. It is used when a provider-owned trigger condition
// must be declared without exposing Matomo's generic condition language.
func (t *TagManager) AddContainerTriggerWithConditions(ctx context.Context, idContainerVersion string, in TriggerInput, conditions json.RawMessage) (string, error) {
	if t == nil || t.c == nil {
		return "", fmt.Errorf("matomo: tag manager client is nil")
	}
	idContainerVersion = strings.TrimSpace(idContainerVersion)
	if idContainerVersion == "" {
		return "", fmt.Errorf("matomo: idContainerVersion is required")
	}
	params := triggerInputValues(in)
	if err := setFormJSON(params, "conditions", conditions, "TagManager.addContainerTrigger"); err != nil {
		return "", err
	}
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

// UpdateContainerTriggerWithConditions updates a trigger while replacing only
// the caller-owned conditions array and preserving the other unmanaged fields.
func (t *TagManager) UpdateContainerTriggerWithConditions(ctx context.Context, idContainerVersion, idTrigger string, in TriggerInput, conditions json.RawMessage, preserved TriggerPreservedFields) error {
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
	params.Del("type")
	params.Set("idContainerVersion", idContainerVersion)
	params.Set("idTrigger", idTrigger)
	params.Set("description", preserved.Description)
	if len(strings.TrimSpace(string(conditions))) == 0 {
		conditions = preserved.Conditions
	}
	if err := setFormJSON(params, "conditions", conditions, "TagManager.updateContainerTrigger"); err != nil {
		return err
	}
	_, err := t.Call(ctx, "updateContainerTrigger", params)
	return err
}
