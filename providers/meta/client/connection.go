package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const adsManagementPermission = "ads_management"

// CheckConnection verifies the configured token, ads_management permission,
// and ad-account access using GET requests only.
func (c *Client) CheckConnection(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("meta: client is nil")
	}

	permissions, err := c.List(ctx, "me/permissions", url.Values{"fields": {"permission,status"}})
	if err != nil {
		return connectionError("could not inspect token permissions", err)
	}
	granted := false
	for _, raw := range permissions {
		var permission struct {
			Name   string `json:"permission"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(raw, &permission); err != nil {
			return fmt.Errorf("meta: check connection: malformed permissions response")
		}
		if permission.Name == adsManagementPermission && strings.EqualFold(permission.Status, "granted") {
			granted = true
		}
	}
	if !granted {
		return fmt.Errorf("meta: check connection: token does not have required %s permission", adsManagementPermission)
	}

	var account struct {
		ID string `json:"id"`
	}
	if err := c.Get(ctx, c.adAccountID, url.Values{"fields": {"id,account_status"}}, &account); err != nil {
		return connectionError("configured ad account is not accessible", err)
	}
	wantNumeric := strings.TrimPrefix(c.adAccountID, "act_")
	if account.ID != wantNumeric && account.ID != c.adAccountID {
		return fmt.Errorf("meta: check connection: configured ad account %s was not returned by the API", c.adAccountID)
	}
	return nil
}

func connectionError(message string, err error) error {
	var apiErr *Error
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.IsAuthentication():
			return fmt.Errorf("meta: check connection: access token was rejected: %w", apiErr)
		case apiErr.IsPermission():
			return fmt.Errorf("meta: check connection: insufficient permission: %w", apiErr)
		}
	}
	return fmt.Errorf("meta: check connection: %s: %w", message, err)
}
