package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	if c == nil {
		return "", fmt.Errorf("googleads: client is nil")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.tokenExpiry) {
		return c.token, nil
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", c.cfg.ClientID)
	form.Set("client_secret", c.cfg.ClientSecret)
	form.Set("refresh_token", c.cfg.RefreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", c.sanitize("oauth", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	if err := ctx.Err(); err != nil {
		return "", c.sanitize("oauth", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", c.sanitize("oauth", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		return "", responseReadError("oauth", resp.StatusCode)
	}
	if len(body) > maxResponseBody {
		return "", responseTooLargeError("oauth", resp.StatusCode)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", parseAPIError("oauth", resp.StatusCode, "", body, c.secrets())
	}
	if !json.Valid(body) {
		return "", malformedResponseError("oauth", resp.StatusCode)
	}

	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil || strings.TrimSpace(tok.AccessToken) == "" {
		return "", malformedResponseError("oauth", resp.StatusCode)
	}

	expiresIn := tok.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}
	c.token = tok.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(expiresIn)*time.Second - tokenSkew)
	if !c.tokenExpiry.After(time.Now()) {
		// expires_in was shorter than the refresh skew; use the token once.
		c.tokenExpiry = time.Now().Add(time.Second)
	}
	return c.token, nil
}
