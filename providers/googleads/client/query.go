package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Query runs a Google Ads Query Language search against the configured
// customer and returns every result page.
func (c *Client) Query(ctx context.Context, query string) ([]json.RawMessage, error) {
	return c.QueryCustomer(ctx, "", query)
}

// QueryCustomer runs a GAQL search against customerID. An empty customerID
// uses the configured default.
func (c *Client) QueryCustomer(ctx context.Context, customerID, query string) ([]json.RawMessage, error) {
	if c == nil {
		return nil, fmt.Errorf("googleads: client is nil")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("googleads: query is required")
	}
	id, err := c.resolveCustomerID(customerID)
	if err != nil {
		return nil, err
	}

	var all []json.RawMessage
	pageToken := ""
	for page := 0; page < maxQueryPages; page++ {
		req := map[string]any{"query": query}
		if pageToken != "" {
			req["pageToken"] = pageToken
		}
		path := "customers/" + id + "/googleAds:search"
		raw, err := c.doJSON(ctx, "query", http.MethodPost, path, req)
		if err != nil {
			return nil, err
		}
		var resp struct {
			Results       []json.RawMessage `json:"results"`
			NextPageToken string            `json:"nextPageToken"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, malformedResponseError("query", http.StatusOK)
		}
		all = append(all, resp.Results...)
		pageToken = strings.TrimSpace(resp.NextPageToken)
		if pageToken == "" {
			return all, nil
		}
	}
	return nil, fmt.Errorf("googleads: query exceeded the maximum number of result pages")
}
