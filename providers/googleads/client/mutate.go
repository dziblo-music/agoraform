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
	return c.doJSON(ctx, "mutate", http.MethodPost, path, map[string]any{
		"operations": operations,
	})
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
