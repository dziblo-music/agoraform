package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode"
)

// Mutate posts operations to customers/{id}/{collection}:mutate using the
// configured customer. collection is the REST collection name, for example
// conversionActions.
func (c *Client) Mutate(ctx context.Context, collection string, operations any) (json.RawMessage, error) {
	return c.MutateCustomer(ctx, "", collection, operations)
}

// MutateCustomer posts operations against customerID. An empty customerID
// uses the configured default.
func (c *Client) MutateCustomer(ctx context.Context, customerID, collection string, operations any) (json.RawMessage, error) {
	if c == nil {
		return nil, fmt.Errorf("googleads: client is nil")
	}
	if err := validateCollection(collection); err != nil {
		return nil, err
	}
	if operations == nil {
		return nil, fmt.Errorf("googleads: mutate operations are required")
	}
	id, err := c.resolveCustomerID(customerID)
	if err != nil {
		return nil, err
	}

	path := "customers/" + id + "/" + collection + ":mutate"
	raw, err := c.doJSON(ctx, "mutate", http.MethodPost, path, map[string]any{
		"operations": operations,
	})
	if err != nil {
		return nil, err
	}

	// Remove operations are destructive and their success must be confirmed
	// before callers can safely discard local state. Google Ads mutate remove
	// responses return one result per operation with the removed resource name.
	// Treat a 2xx response that does not confirm the requested removals as
	// malformed instead of silently reporting success.
	if expected, ok := removeOperationResourceNames(operations); ok {
		if err := validateRemoveMutateResponse(raw, expected); err != nil {
			return nil, err
		}
	}
	return raw, nil
}

func removeOperationResourceNames(operations any) ([]string, bool) {
	encoded, err := json.Marshal(operations)
	if err != nil {
		return nil, false
	}
	var ops []map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &ops); err != nil || len(ops) == 0 {
		return nil, false
	}

	names := make([]string, 0, len(ops))
	for _, op := range ops {
		raw, ok := op["remove"]
		if !ok || len(op) != 1 {
			return nil, false
		}
		var name string
		if err := json.Unmarshal(raw, &name); err != nil {
			return nil, false
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, false
		}
		names = append(names, name)
	}
	return names, true
}

func validateRemoveMutateResponse(raw json.RawMessage, expected []string) error {
	var resp struct {
		Results []struct {
			ResourceName string `json:"resourceName"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || len(resp.Results) != len(expected) {
		return malformedResponseError("mutate", http.StatusOK)
	}
	for i, result := range resp.Results {
		if strings.TrimSpace(result.ResourceName) != expected[i] {
			return malformedResponseError("mutate", http.StatusOK)
		}
	}
	return nil
}

func validateCollection(collection string) error {
	collection = strings.TrimSpace(collection)
	if collection == "" {
		return fmt.Errorf("googleads: mutate collection is required")
	}
	for i, r := range collection {
		if i == 0 {
			if r < 'a' || r > 'z' {
				return fmt.Errorf("googleads: mutate collection %q is invalid", collection)
			}
			continue
		}
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && !unicode.IsDigit(r) {
			return fmt.Errorf("googleads: mutate collection %q is invalid", collection)
		}
	}
	return nil
}
