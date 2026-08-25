package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Variable is a Matomo Tag Manager variable as returned by
// TagManager.getContainerVariables.
//
// Field names follow the API JSON response. Values are normalized to
// strings so callers do not have to handle Matomo's mixed encodings.
// IDVariable is the provider-native identifier within the draft version.
type Variable struct {
	IDVariable         string
	IDContainerVersion string
	IDSite             string
	Type               string
	Name               string
	Status             string
	Description        string
	DefaultValue       string
	Parameters         map[string]string
	LookupTable        json.RawMessage
}

// VariableInput is the configurable subset sent to
// TagManager.addContainerVariable and TagManager.updateContainerVariable.
type VariableInput struct {
	Type       string
	Name       string
	Parameters map[string]string
}

// VariablePreservedFields is the unmanaged portion of a Tag Manager
// variable that must be carried forward on update. Matomo's update API
// replaces omitted parameters, default values, lookup tables, and
// descriptions with defaults.
type VariablePreservedFields struct {
	DefaultValue string
	Description  string
	LookupTable  json.RawMessage
}

type rawVariable struct {
	IDVariable         flexibleString  `json:"idvariable"`
	IDContainerVersion flexibleString  `json:"idcontainerversion"`
	IDSite             flexibleString  `json:"idsite"`
	Type               flexibleString  `json:"type"`
	Name               flexibleString  `json:"name"`
	Status             flexibleString  `json:"status"`
	Description        flexibleString  `json:"description"`
	DefaultValue       flexibleString  `json:"default_value"`
	Parameters         json.RawMessage `json:"parameters"`
	LookupTable        json.RawMessage `json:"lookup_table"`
}

func (v rawVariable) variable() (Variable, error) {
	params, err := decodeStringMap(v.Parameters)
	if err != nil {
		return Variable{}, err
	}
	return Variable{
		IDVariable:         string(v.IDVariable),
		IDContainerVersion: string(v.IDContainerVersion),
		IDSite:             string(v.IDSite),
		Type:               string(v.Type),
		Name:               string(v.Name),
		Status:             string(v.Status),
		Description:        string(v.Description),
		DefaultValue:       string(v.DefaultValue),
		Parameters:         params,
		LookupTable:        bytes.TrimSpace(v.LookupTable),
	}, nil
}

// GetContainerVariables lists user-created variables in a container version.
func (t *TagManager) GetContainerVariables(ctx context.Context, idContainerVersion string) ([]Variable, error) {
	if t == nil || t.c == nil {
		return nil, fmt.Errorf("matomo: tag manager client is nil")
	}
	idContainerVersion = strings.TrimSpace(idContainerVersion)
	if idContainerVersion == "" {
		return nil, fmt.Errorf("matomo: idContainerVersion is required")
	}
	params := url.Values{}
	params.Set("idContainerVersion", idContainerVersion)
	raw, err := t.Call(ctx, "getContainerVariables", params)
	if err != nil {
		return nil, err
	}
	vars, err := decodeVariables(raw)
	if err != nil {
		return nil, malformedResponseError("TagManager.getContainerVariables", 0)
	}
	return vars, nil
}

// AddContainerVariable creates a variable in the given container version
// and returns the new idvariable.
func (t *TagManager) AddContainerVariable(ctx context.Context, idContainerVersion string, in VariableInput) (string, error) {
	if t == nil || t.c == nil {
		return "", fmt.Errorf("matomo: tag manager client is nil")
	}
	idContainerVersion = strings.TrimSpace(idContainerVersion)
	if idContainerVersion == "" {
		return "", fmt.Errorf("matomo: idContainerVersion is required")
	}
	params := variableInputValues(in)
	params.Set("idContainerVersion", idContainerVersion)
	raw, err := t.Call(ctx, "addContainerVariable", params)
	if err != nil {
		return "", err
	}
	id, err := decodeVariableID(raw)
	if err != nil {
		return "", err
	}
	return id, nil
}

// UpdateContainerVariable updates a variable while carrying forward
// unmanaged Matomo fields.
func (t *TagManager) UpdateContainerVariable(ctx context.Context, idContainerVersion, idVariable string, in VariableInput, preserved VariablePreservedFields) error {
	if t == nil || t.c == nil {
		return fmt.Errorf("matomo: tag manager client is nil")
	}
	idContainerVersion = strings.TrimSpace(idContainerVersion)
	if idContainerVersion == "" {
		return fmt.Errorf("matomo: idContainerVersion is required")
	}
	idVariable = strings.TrimSpace(idVariable)
	if idVariable == "" {
		return fmt.Errorf("matomo: idVariable is required")
	}
	params := variableInputValues(in)
	params.Del("type") // updateContainerVariable does not accept type
	params.Set("idContainerVersion", idContainerVersion)
	params.Set("idVariable", idVariable)
	params.Set("description", preserved.Description)
	params.Set("defaultValue", preserved.DefaultValue)
	if err := setFormJSON(params, "lookupTable", preserved.LookupTable); err != nil {
		return err
	}
	_, err := t.Call(ctx, "updateContainerVariable", params)
	return err
}

func variableInputValues(in VariableInput) url.Values {
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

func decodeVariables(raw json.RawMessage) ([]Variable, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	switch raw[0] {
	case '[':
		var items []rawVariable
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		out := make([]Variable, 0, len(items))
		for _, item := range items {
			v, err := item.variable()
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case '{':
		var keyed map[string]json.RawMessage
		if err := json.Unmarshal(raw, &keyed); err != nil {
			return nil, err
		}
		if _, hasType := keyed["type"]; hasType || hasVariableID(keyed) {
			var item rawVariable
			if err := json.Unmarshal(raw, &item); err != nil {
				return nil, err
			}
			v, err := item.variable()
			if err != nil {
				return nil, err
			}
			return []Variable{v}, nil
		}
		out := make([]Variable, 0, len(keyed))
		for key, value := range keyed {
			value = bytes.TrimSpace(value)
			if len(value) == 0 || value[0] != '{' {
				return nil, fmt.Errorf("unexpected TagManager.getContainerVariables payload")
			}
			var item rawVariable
			if err := json.Unmarshal(value, &item); err != nil {
				return nil, err
			}
			v, err := item.variable()
			if err != nil {
				return nil, err
			}
			if v.IDVariable == "" {
				v.IDVariable = strings.TrimSpace(key)
			}
			out = append(out, v)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unexpected TagManager.getContainerVariables payload")
	}
}

func hasVariableID(keyed map[string]json.RawMessage) bool {
	_, ok := keyed["idvariable"]
	return ok
}

func decodeVariableID(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", fmt.Errorf("matomo: TagManager.addContainerVariable returned no idVariable")
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

	return "", fmt.Errorf("matomo: unexpected TagManager.addContainerVariable payload")
}

func decodeStringMap(raw json.RawMessage) (map[string]string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]string{}, nil
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(object))
	for key, value := range object {
		s, err := flexibleValueString(value)
		if err != nil {
			return nil, err
		}
		out[key] = s
	}
	return out, nil
}

func flexibleValueString(v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "", nil
	case string:
		return x, nil
	case float64:
		return formatJSONFloat(x), nil
	case json.Number:
		return x.String(), nil
	case bool:
		if x {
			return "true", nil
		}
		return "false", nil
	default:
		return "", fmt.Errorf("expected string or number")
	}
}

func formatJSONFloat(n float64) string {
	if n == float64(int64(n)) && n >= -1e15 && n <= 1e15 {
		return strconv.FormatInt(int64(n), 10)
	}
	return strconv.FormatFloat(n, 'g', -1, 64)
}

func setFormJSON(params url.Values, key string, raw json.RawMessage) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return malformedResponseError("TagManager.updateContainerVariable", 0)
	}
	setFormValue(params, key, v)
	return nil
}

func setFormValue(params url.Values, key string, v any) {
	switch x := v.(type) {
	case nil:
		return
	case map[string]any:
		if len(x) == 0 {
			return
		}
		for k, val := range x {
			setFormValue(params, key+"["+k+"]", val)
		}
	case []any:
		if len(x) == 0 {
			return
		}
		for i, val := range x {
			setFormValue(params, fmt.Sprintf("%s[%d]", key, i), val)
		}
	case string:
		params.Set(key, x)
	case float64:
		params.Set(key, formatJSONFloat(x))
	case json.Number:
		params.Set(key, x.String())
	case bool:
		if x {
			params.Set(key, "1")
		} else {
			params.Set(key, "0")
		}
	default:
		params.Set(key, fmt.Sprint(x))
	}
}
