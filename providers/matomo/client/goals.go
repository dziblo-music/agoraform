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

// Goal is a Matomo analytics goal as returned by Goals.getGoals.
//
// Field names follow the API JSON response. Values are normalized to
// strings so callers do not have to handle Matomo's mixed number/string
// encodings. IDGoal is the provider-native identifier.
type Goal struct {
	IDGoal              string
	IDSite              string
	Name                string
	Description         string
	MatchAttribute      string
	Pattern             string
	PatternType         string
	CaseSensitive       string
	AllowMultiple       string
	Revenue             string
	Deleted             string
	EventValueAsRevenue string
}

// GoalInput is the configurable subset sent to Goals.addGoal and
// Goals.updateGoal. Request parameter names follow the Matomo API.
type GoalInput struct {
	Name           string
	MatchAttribute string
	Pattern        string
	PatternType    string
}

type rawGoal struct {
	IDGoal              flexibleString `json:"idgoal"`
	IDSite              flexibleString `json:"idsite"`
	Name                flexibleString `json:"name"`
	Description         flexibleString `json:"description"`
	MatchAttribute      flexibleString `json:"match_attribute"`
	Pattern             flexibleString `json:"pattern"`
	PatternType         flexibleString `json:"pattern_type"`
	CaseSensitive       flexibleString `json:"case_sensitive"`
	AllowMultiple       flexibleString `json:"allow_multiple"`
	Revenue             flexibleString `json:"revenue"`
	Deleted             flexibleString `json:"deleted"`
	EventValueAsRevenue flexibleString `json:"event_value_as_revenue"`
}

func (g rawGoal) goal() Goal {
	return Goal{
		IDGoal:              string(g.IDGoal),
		IDSite:              string(g.IDSite),
		Name:                string(g.Name),
		Description:         string(g.Description),
		MatchAttribute:      string(g.MatchAttribute),
		Pattern:             string(g.Pattern),
		PatternType:         string(g.PatternType),
		CaseSensitive:       string(g.CaseSensitive),
		AllowMultiple:       string(g.AllowMultiple),
		Revenue:             string(g.Revenue),
		Deleted:             string(g.Deleted),
		EventValueAsRevenue: string(g.EventValueAsRevenue),
	}
}

// GetGoals lists active goals for the configured site.
func (a *Analytics) GetGoals(ctx context.Context) ([]Goal, error) {
	if a == nil || a.c == nil {
		return nil, fmt.Errorf("matomo: analytics client is nil")
	}
	raw, err := a.Call(ctx, "Goals.getGoals", nil)
	if err != nil {
		return nil, err
	}
	goals, err := decodeGoals(raw)
	if err != nil {
		return nil, malformedResponseError("Goals.getGoals", 0)
	}
	return goals, nil
}

// AddGoal creates a goal and returns the new idGoal.
func (a *Analytics) AddGoal(ctx context.Context, in GoalInput) (string, error) {
	if a == nil || a.c == nil {
		return "", fmt.Errorf("matomo: analytics client is nil")
	}
	raw, err := a.Call(ctx, "Goals.addGoal", goalInputValues(in))
	if err != nil {
		return "", err
	}
	id, err := decodeGoalID(raw)
	if err != nil {
		return "", err
	}
	return id, nil
}

// UpdateGoal updates an existing goal identified by idGoal.
func (a *Analytics) UpdateGoal(ctx context.Context, idGoal string, in GoalInput) error {
	if a == nil || a.c == nil {
		return fmt.Errorf("matomo: analytics client is nil")
	}
	idGoal = strings.TrimSpace(idGoal)
	if idGoal == "" {
		return fmt.Errorf("matomo: idGoal is required")
	}
	params := goalInputValues(in)
	params.Set("idGoal", idGoal)
	_, err := a.Call(ctx, "Goals.updateGoal", params)
	return err
}

func goalInputValues(in GoalInput) url.Values {
	params := url.Values{}
	params.Set("name", in.Name)
	params.Set("matchAttribute", in.MatchAttribute)
	params.Set("pattern", in.Pattern)
	params.Set("patternType", in.PatternType)
	return params
}

func decodeGoals(raw json.RawMessage) ([]Goal, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	switch raw[0] {
	case '[':
		var items []rawGoal
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		out := make([]Goal, 0, len(items))
		for _, item := range items {
			out = append(out, item.goal())
		}
		return out, nil
	case '{':
		var keyed map[string]json.RawMessage
		if err := json.Unmarshal(raw, &keyed); err != nil {
			return nil, err
		}
		if _, hasID := keyed["idgoal"]; hasID {
			var item rawGoal
			if err := json.Unmarshal(raw, &item); err != nil {
				return nil, err
			}
			return []Goal{item.goal()}, nil
		}
		out := make([]Goal, 0, len(keyed))
		for key, value := range keyed {
			value = bytes.TrimSpace(value)
			if len(value) == 0 || value[0] != '{' {
				return nil, fmt.Errorf("unexpected Goals.getGoals payload")
			}
			var item rawGoal
			if err := json.Unmarshal(value, &item); err != nil {
				return nil, err
			}
			g := item.goal()
			if g.IDGoal == "" {
				g.IDGoal = strings.TrimSpace(key)
			}
			out = append(out, g)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unexpected Goals.getGoals payload")
	}
}

func decodeGoalID(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", fmt.Errorf("matomo: Goals.addGoal returned no idGoal")
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

	return "", fmt.Errorf("matomo: unexpected Goals.addGoal payload")
}

// flexibleString accepts Matomo's mixed JSON encodings (string, number, bool).
type flexibleString string

func (s *flexibleString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*s = ""
		return nil
	}

	var str string
	if err := json.Unmarshal(b, &str); err == nil {
		*s = flexibleString(str)
		return nil
	}

	var num json.Number
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&num); err == nil {
		*s = flexibleString(num.String())
		return nil
	}

	var flag bool
	if err := json.Unmarshal(b, &flag); err == nil {
		*s = flexibleString(strconv.FormatBool(flag))
		return nil
	}

	return fmt.Errorf("expected string or number")
}
