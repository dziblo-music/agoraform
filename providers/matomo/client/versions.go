package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// DefaultEnvironment is the initial v0.2 Tag Manager publish target when the
// manifest does not specify one.
const DefaultEnvironment = "live"

// Environment is a Tag Manager publish target as returned by Tag Manager.
type Environment struct {
	ID   string
	Name string
}

type rawEnvironment struct {
	ID   flexibleString `json:"id"`
	Name flexibleString `json:"name"`
}

// GetAvailableEnvironments lists Tag Manager environments.
func (t *TagManager) GetAvailableEnvironments(ctx context.Context) ([]Environment, error) {
	if t == nil || t.c == nil {
		return nil, fmt.Errorf("matomo: tag manager client is nil")
	}
	raw, err := t.c.Call(ctx, "TagManager.getAvailableEnvironments", nil)
	if err != nil {
		return nil, err
	}
	envs, err := decodeEnvironments(raw)
	if err != nil {
		return nil, malformedResponseError("TagManager.getAvailableEnvironments", 0)
	}
	return envs, nil
}

// GetAvailableEnvironmentsWithPublishCapability lists only environments that
// the current credentials may publish to for the configured site. This is a
// non-mutating preflight used before container version creation.
func (t *TagManager) GetAvailableEnvironmentsWithPublishCapability(ctx context.Context) ([]Environment, error) {
	if t == nil || t.c == nil {
		return nil, fmt.Errorf("matomo: tag manager client is nil")
	}
	raw, err := t.c.Call(ctx, "TagManager.getAvailableEnvironmentsWithPublishCapability", t.c.withSiteID(nil))
	if err != nil {
		return nil, err
	}
	envs, err := decodeEnvironments(raw)
	if err != nil {
		return nil, malformedResponseError("TagManager.getAvailableEnvironmentsWithPublishCapability", 0)
	}
	return envs, nil
}

// CreateContainerVersion snapshots the current draft into a named version
// and returns the provider-native version id.
func (t *TagManager) CreateContainerVersion(ctx context.Context, name, description string) (string, error) {
	if t == nil || t.c == nil {
		return "", fmt.Errorf("matomo: tag manager client is nil")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("matomo: container version name is required")
	}
	params := url.Values{}
	params.Set("name", name)
	if strings.TrimSpace(description) != "" {
		params.Set("description", description)
	}
	raw, err := t.Call(ctx, "createContainerVersion", params)
	if err != nil {
		return "", err
	}
	id, err := decodeVersionID(raw, "TagManager.createContainerVersion")
	if err != nil {
		return "", err
	}
	return id, nil
}

// PublishContainerVersion publishes an existing container version to
// environment.
//
// A successful response must carry a Matomo container release ID. Empty,
// JSON null, unreadable, oversized, and unrelated payloads are rejected.
// Those failures are uncertain outcomes: the publish request has already
// been sent, so callers must inspect the remote container before creating
// another version.
func (t *TagManager) PublishContainerVersion(ctx context.Context, idContainerVersion, environment string) error {
	if t == nil || t.c == nil {
		return fmt.Errorf("matomo: tag manager client is nil")
	}
	idContainerVersion = strings.TrimSpace(idContainerVersion)
	if idContainerVersion == "" {
		return fmt.Errorf("matomo: idContainerVersion is required")
	}
	environment = strings.TrimSpace(environment)
	if environment == "" {
		return fmt.Errorf("matomo: environment is required")
	}
	params := url.Values{}
	params.Set("idContainerVersion", idContainerVersion)
	params.Set("environment", environment)
	raw, err := t.Call(ctx, "publishContainerVersion", params)
	if err != nil {
		if isUnconfirmed(err) {
			return uncertainOutcomeError("TagManager.publishContainerVersion", unconfirmedReason(err), err)
		}
		return err
	}
	if _, err := decodeReleaseID(raw); err != nil {
		return uncertainOutcomeError("TagManager.publishContainerVersion", err.Error(), err)
	}
	return nil
}

func decodeEnvironments(raw json.RawMessage) ([]Environment, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if raw[0] != '[' {
		return nil, fmt.Errorf("unexpected Tag Manager environments payload")
	}
	var items []rawEnvironment
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	out := make([]Environment, 0, len(items))
	for _, item := range items {
		env := Environment{
			ID:   strings.TrimSpace(string(item.ID)),
			Name: strings.TrimSpace(string(item.Name)),
		}
		if env.ID == "" {
			continue
		}
		out = append(out, env)
	}
	return out, nil
}

func decodeVersionID(raw json.RawMessage, method string) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", fmt.Errorf("matomo: %s returned no version id", method)
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

	return "", fmt.Errorf("matomo: unexpected %s payload", method)
}

// decodeReleaseID accepts the release-ID encodings returned by supported
// Matomo Tag Manager versions: a JSON number, a digit string, or the
// historical {"value": ...} scalar wrapper. Empty, null, and unrelated
// payloads are rejected so publication cannot be treated as success without
// evidence that Matomo completed the publish.
func decodeReleaseID(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return "", fmt.Errorf("empty response")
	}
	if string(raw) == "null" {
		return "", fmt.Errorf("null response")
	}

	if id, ok, err := releaseIDFromScalar(raw); ok {
		return id, err
	}

	if raw[0] == '{' {
		var keyed map[string]json.RawMessage
		if err := json.Unmarshal(raw, &keyed); err == nil {
			inner, ok := keyed["value"]
			if !ok {
				return "", fmt.Errorf("unexpected payload")
			}
			inner = bytes.TrimSpace(inner)
			if len(inner) == 0 || string(inner) == "null" {
				return "", fmt.Errorf("empty release id")
			}
			if id, ok, err := releaseIDFromScalar(inner); ok {
				return id, err
			}
		}
	}

	return "", fmt.Errorf("unexpected payload")
}

func releaseIDFromScalar(raw json.RawMessage) (string, bool, error) {
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		id := strings.TrimSpace(str)
		if id == "" {
			return "", true, fmt.Errorf("empty release id")
		}
		if !isReleaseID(id) {
			return "", true, fmt.Errorf("unexpected payload")
		}
		return id, true, nil
	}

	var num json.Number
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&num); err == nil {
		id := strings.TrimSpace(num.String())
		if !isReleaseID(id) {
			return "", true, fmt.Errorf("unexpected payload")
		}
		return id, true, nil
	}

	return "", false, nil
}

func isReleaseID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
