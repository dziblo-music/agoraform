package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// GeoTargetConstant is a provider-native Google Ads geo target constant.
type GeoTargetConstant struct {
	ResourceName  string
	ID            string
	Name          string
	CanonicalName string
	CountryCode   string
	TargetType    string
	Status        string
}

// GeoTargetSuggestion is one result from geoTargetConstants:suggest.
type GeoTargetSuggestion struct {
	Constant   GeoTargetConstant
	SearchTerm string
}

// SuggestGeoTargetConstants looks up geo target constants by location name.
//
// This is the Google Ads-supported name lookup. Callers must treat multiple
// distinct ENABLED constants as ambiguous rather than picking a match.
func (c *Client) SuggestGeoTargetConstants(ctx context.Context, names []string) ([]GeoTargetSuggestion, error) {
	if c == nil {
		return nil, fmt.Errorf("googleads: client is nil")
	}
	cleaned := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		cleaned = append(cleaned, name)
	}
	if len(cleaned) == 0 {
		return nil, fmt.Errorf("googleads: geo target location name is required")
	}

	raw, err := c.doJSON(ctx, "suggest geo target constants", http.MethodPost, "geoTargetConstants:suggest", map[string]any{
		"locale":        "en",
		"locationNames": map[string]any{"names": cleaned},
	})
	if err != nil {
		return nil, err
	}

	var resp struct {
		GeoTargetConstantSuggestions []struct {
			GeoTargetConstant *geoTargetConstantJSON `json:"geoTargetConstant"`
			SearchTerm        string                 `json:"searchTerm"`
		} `json:"geoTargetConstantSuggestions"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, malformedResponseError("suggest geo target constants", http.StatusOK)
	}

	out := make([]GeoTargetSuggestion, 0, len(resp.GeoTargetConstantSuggestions))
	for _, item := range resp.GeoTargetConstantSuggestions {
		constant, ok := decodeGeoTargetConstantJSON(item.GeoTargetConstant)
		if !ok {
			continue
		}
		out = append(out, GeoTargetSuggestion{
			Constant:   constant,
			SearchTerm: strings.TrimSpace(item.SearchTerm),
		})
	}
	return out, nil
}

type geoTargetConstantJSON struct {
	ResourceName  string      `json:"resourceName"`
	ID            json.Number `json:"id"`
	Name          string      `json:"name"`
	CanonicalName string      `json:"canonicalName"`
	CountryCode   string      `json:"countryCode"`
	TargetType    string      `json:"targetType"`
	Status        string      `json:"status"`
}

func decodeGeoTargetConstantJSON(body *geoTargetConstantJSON) (GeoTargetConstant, bool) {
	if body == nil {
		return GeoTargetConstant{}, false
	}
	resourceName := strings.TrimSpace(body.ResourceName)
	id := strings.TrimSpace(body.ID.String())
	if id == "" {
		if _, parsed, ok := splitGeoTargetConstantResourceName(resourceName); ok {
			id = parsed
		}
	}
	if id == "" && resourceName == "" {
		return GeoTargetConstant{}, false
	}
	if resourceName == "" {
		resourceName = "geoTargetConstants/" + id
	}
	return GeoTargetConstant{
		ResourceName:  resourceName,
		ID:            id,
		Name:          strings.TrimSpace(body.Name),
		CanonicalName: strings.TrimSpace(body.CanonicalName),
		CountryCode:   strings.TrimSpace(body.CountryCode),
		TargetType:    strings.TrimSpace(body.TargetType),
		Status:        strings.TrimSpace(body.Status),
	}, true
}

func splitGeoTargetConstantResourceName(name string) (string, string, bool) {
	name = strings.TrimSpace(name)
	parts := strings.Split(name, "/")
	if len(parts) != 2 || parts[0] != "geoTargetConstants" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
