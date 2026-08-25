package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/dziblo-music/agoraform/internal/resource"
)

const (
	// Version is the v0.1 local state schema version.
	Version = 1

	// DefaultFilename is the state file stored next to a manifest.
	DefaultFilename = "agoraform.state.json"
)

// ErrStaleIdentity reports that a persisted identity no longer exists remotely.
var ErrStaleIdentity = errors.New("persisted identity was not found remotely")

// DuplicateIdentityError reports that the same provider-native identity is
// bound to more than one logical address of the same resource type.
type DuplicateIdentityError struct {
	Provider     string
	ResourceType string
	RemoteID     string
	Existing     string
	Address      resource.Address
}

func (e *DuplicateIdentityError) Error() string {
	if e == nil {
		return "duplicate identity"
	}
	return fmt.Sprintf("duplicate identity %q for provider %q resource type %q on %s and %s", e.RemoteID, e.Provider, e.ResourceType, e.Existing, e.Address)
}

// OwnerOtherThan returns the logical address that already owns RemoteID,
// excluding addr when addr is one of the conflicting bindings.
func (e *DuplicateIdentityError) OwnerOtherThan(addr resource.Address) string {
	if e == nil {
		return ""
	}
	key := addr.String()
	if e.Existing != "" && e.Existing != key {
		return e.Existing
	}
	if e.Address.String() != "" && e.Address.String() != key {
		return e.Address.String()
	}
	return e.Existing
}

// Record is the persisted management metadata for one logical resource.
type Record struct {
	Provider string `json:"provider"`
	RemoteID string `json:"remoteId"`
}

type file struct {
	Version   int               `json:"version"`
	Resources map[string]Record `json:"resources"`
}

func marshalFile(f file) ([]byte, error) {
	if f.Resources == nil {
		f.Resources = map[string]Record{}
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func decodeFile(path string, data []byte) (file, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var f file
	if err := dec.Decode(&f); err != nil {
		return file{}, fmt.Errorf("state: malformed %s: %w", path, err)
	}
	if err := consumeEOF(dec); err != nil {
		return file{}, fmt.Errorf("state: malformed %s: %w", path, err)
	}
	if f.Version != Version {
		return file{}, fmt.Errorf("state: unsupported version %d in %s (supported version is %d)", f.Version, path, Version)
	}
	if f.Resources == nil {
		f.Resources = map[string]Record{}
	}
	if err := validateRecords(f.Resources); err != nil {
		return file{}, fmt.Errorf("state: invalid %s: %w", path, err)
	}
	return f, nil
}

func consumeEOF(dec *json.Decoder) error {
	tok, err := dec.Token()
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("unexpected trailing data %v", tok)
}

func validateRecords(records map[string]Record) error {
	seenRemote := make(map[string]string, len(records))
	for addrStr, rec := range records {
		addr, err := resource.ParseAddress(addrStr)
		if err != nil {
			return fmt.Errorf("invalid resource address %q: %w", addrStr, err)
		}
		if err := validateRecord(addr, rec); err != nil {
			return err
		}

		// Provider IDs are opaque and are not necessarily globally unique
		// across a provider. Scope duplicate ownership checks to the provider
		// resource type so, for example, goal ID "1" and tag ID "1" can
		// coexist safely.
		dupKey := rec.Provider + "\x00" + addr.Type + "\x00" + rec.RemoteID
		if other, ok := seenRemote[dupKey]; ok {
			return &DuplicateIdentityError{
				Provider:     rec.Provider,
				ResourceType: addr.Type,
				RemoteID:     rec.RemoteID,
				Existing:     other,
				Address:      addr,
			}
		}
		seenRemote[dupKey] = addr.String()
	}
	return nil
}

func validateRecord(addr resource.Address, rec Record) error {
	provider := strings.TrimSpace(rec.Provider)
	remoteID := strings.TrimSpace(rec.RemoteID)
	if provider == "" {
		return fmt.Errorf("resource %s: provider is required", addr)
	}
	if remoteID == "" {
		return fmt.Errorf("resource %s: remote identity is empty", addr)
	}
	if provider != addr.Provider {
		return fmt.Errorf("resource %s: stored provider %q does not match address provider %q", addr, provider, addr.Provider)
	}
	return nil
}
