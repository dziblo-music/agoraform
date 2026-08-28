package matomo

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"

	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

// fingerprintContainer builds a comparable view of a container version.
//
// Provider-native ids, version numbers, and dates are omitted so a draft
// that was snapshotted into a published version compares equal. Trigger
// references on tags are compared by trigger name, not numeric id. Tag status
// is retained because paused vs active changes runtime behavior.
func fingerprintContainer(vars []client.Variable, triggers []client.Trigger, tags []client.Tag) (string, error) {
	triggerName := make(map[string]string, len(triggers))
	for _, tr := range triggers {
		if tr.IDTrigger != "" {
			triggerName[tr.IDTrigger] = tr.Name
		}
	}

	fp := struct {
		Variables []variableFingerprint `json:"variables"`
		Triggers  []triggerFingerprint  `json:"triggers"`
		Tags      []tagFingerprint      `json:"tags"`
	}{
		Variables: make([]variableFingerprint, 0, len(vars)),
		Triggers:  make([]triggerFingerprint, 0, len(triggers)),
		Tags:      make([]tagFingerprint, 0, len(tags)),
	}

	for _, v := range vars {
		fp.Variables = append(fp.Variables, variableFingerprint{
			Type:         v.Type,
			Name:         v.Name,
			Parameters:   v.Parameters,
			DefaultValue: v.DefaultValue,
			Description:  v.Description,
			LookupTable:  canonicalizeJSON(v.LookupTable),
		})
	}
	sort.Slice(fp.Variables, func(i, j int) bool {
		return fingerprintKey(fp.Variables[i].Type, fp.Variables[i].Name) < fingerprintKey(fp.Variables[j].Type, fp.Variables[j].Name)
	})

	for _, tr := range triggers {
		fp.Triggers = append(fp.Triggers, triggerFingerprint{
			Type:        tr.Type,
			Name:        tr.Name,
			Parameters:  tr.Parameters,
			Description: tr.Description,
			Conditions:  canonicalizeJSON(tr.Conditions),
		})
	}
	sort.Slice(fp.Triggers, func(i, j int) bool {
		return fingerprintKey(fp.Triggers[i].Type, fp.Triggers[i].Name) < fingerprintKey(fp.Triggers[j].Type, fp.Triggers[j].Name)
	})

	for _, tag := range tags {
		params := tag.Parameters
		if params == nil {
			params = map[string]any{}
		} else {
			params = cloneAnyMap(params)
			if cfg, ok := params["matomoConfig"]; ok {
				params["matomoConfig"] = client.NormalizeMatomoConfig(cfg)
			}
		}
		fp.Tags = append(fp.Tags, tagFingerprint{
			Type:          tag.Type,
			Name:          tag.Name,
			Status:        normalizeTagStatus(tag.Status),
			Description:   tag.Description,
			FireLimit:     tag.FireLimit,
			FireDelay:     tag.FireDelay,
			Priority:      tag.Priority,
			StartDate:     tag.StartDate,
			EndDate:       tag.EndDate,
			FireTriggers:  namesForIDs(tag.FireTriggerIDs, triggerName),
			BlockTriggers: namesForIDs(tag.BlockTriggerIDs, triggerName),
			Parameters:    params,
		})
	}
	sort.Slice(fp.Tags, func(i, j int) bool {
		return fingerprintKey(fp.Tags[i].Type, fp.Tags[i].Name) < fingerprintKey(fp.Tags[j].Type, fp.Tags[j].Name)
	})

	raw, err := json.Marshal(fp)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

type variableFingerprint struct {
	Type         string          `json:"type"`
	Name         string          `json:"name"`
	Parameters   map[string]any  `json:"parameters"`
	DefaultValue string          `json:"defaultValue"`
	Description  string          `json:"description"`
	LookupTable  json.RawMessage `json:"lookupTable"`
}

type triggerFingerprint struct {
	Type        string            `json:"type"`
	Name        string            `json:"name"`
	Parameters  map[string]string `json:"parameters"`
	Description string            `json:"description"`
	Conditions  json.RawMessage   `json:"conditions"`
}

type tagFingerprint struct {
	Type          string         `json:"type"`
	Name          string         `json:"name"`
	Status        string         `json:"status"`
	Description   string         `json:"description"`
	FireLimit     string         `json:"fireLimit"`
	FireDelay     string         `json:"fireDelay"`
	Priority      string         `json:"priority"`
	StartDate     string         `json:"startDate"`
	EndDate       string         `json:"endDate"`
	FireTriggers  []string       `json:"fireTriggers"`
	BlockTriggers []string       `json:"blockTriggers"`
	Parameters    map[string]any `json:"parameters"`
}

func normalizeTagStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		return "active"
	}
	return status
}

func fingerprintKey(typ, name string) string {
	return typ + "\x00" + name
}

func namesForIDs(ids []string, names map[string]string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if name, ok := names[id]; ok && name != "" {
			out = append(out, name)
			continue
		}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func canonicalizeJSON(raw json.RawMessage) json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage("[]")
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return json.RawMessage("[]")
	}
	canonical, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("[]")
	}
	return canonical
}

func cloneAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
