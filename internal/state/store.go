package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dziblo-music/agoraform/internal/resource"
)

// Store is an in-memory view of a local state file.
type Store struct {
	path    string
	version int
	records map[string]Record
}

// New returns an empty store that will write to path.
func New(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("state: path is required")
	}
	return &Store{
		path:    path,
		version: Version,
		records: make(map[string]Record),
	}, nil
}

// PathForManifest returns the default state path beside the given manifest.
func PathForManifest(manifestPath string) string {
	dir := filepath.Dir(strings.TrimSpace(manifestPath))
	if dir == "" || dir == "." {
		return DefaultFilename
	}
	return filepath.Join(dir, DefaultFilename)
}

// Load reads state from path. A missing file is an empty store.
func Load(path string) (*Store, error) {
	st, err := New(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errorsIsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return nil, fmt.Errorf("state: read %s: %w", path, err)
	}
	f, err := decodeFile(path, data)
	if err != nil {
		return nil, err
	}
	st.version = f.Version
	st.records = f.Resources
	return st, nil
}

func errorsIsNotExist(err error) bool {
	return err != nil && os.IsNotExist(err)
}

// Path returns the backing file path.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// Identity implements plan.Identities.
func (s *Store) Identity(addr resource.Address) (resource.Identity, bool, error) {
	if s == nil {
		return resource.Identity{}, false, nil
	}
	rec, ok := s.records[addr.String()]
	if !ok {
		return resource.Identity{}, false, nil
	}
	if err := validateRecord(addr, rec); err != nil {
		return resource.Identity{}, true, fmt.Errorf("state: %w", err)
	}
	return resource.Identity{ID: rec.RemoteID}, true, nil
}

// AddressByRemoteID finds the logical address bound to a provider-native
// identity for the given resource type. Ownership is scoped the same way as
// validateRecords: provider + resource type + remote id.
func (s *Store) AddressByRemoteID(provider, resourceType, remoteID string) (resource.Address, bool, error) {
	if s == nil {
		return resource.Address{}, false, nil
	}
	provider = strings.TrimSpace(provider)
	resourceType = strings.TrimSpace(resourceType)
	remoteID = strings.TrimSpace(remoteID)
	if provider == "" || resourceType == "" || remoteID == "" {
		return resource.Address{}, false, nil
	}
	for addrStr, rec := range s.records {
		if strings.TrimSpace(rec.Provider) != provider || strings.TrimSpace(rec.RemoteID) != remoteID {
			continue
		}
		addr, err := resource.ParseAddress(addrStr)
		if err != nil {
			return resource.Address{}, false, fmt.Errorf("state: %w", err)
		}
		if addr.Type != resourceType {
			continue
		}
		if err := validateRecord(addr, rec); err != nil {
			return resource.Address{}, true, fmt.Errorf("state: %w", err)
		}
		return addr, true, nil
	}
	return resource.Address{}, false, nil
}

// Lookup returns the stored record for addr.
func (s *Store) Lookup(addr resource.Address) (Record, bool) {
	if s == nil {
		return Record{}, false
	}
	rec, ok := s.records[addr.String()]
	return rec, ok
}

// Bind records an identity in memory without writing the file.
func (s *Store) Bind(addr resource.Address, id resource.Identity) error {
	if s == nil {
		return fmt.Errorf("state: store is nil")
	}
	if err := addr.Validate(); err != nil {
		return fmt.Errorf("state: %w", err)
	}
	rec := Record{Provider: addr.Provider, RemoteID: strings.TrimSpace(id.ID)}
	if err := validateRecord(addr, rec); err != nil {
		return fmt.Errorf("state: %w", err)
	}
	key := addr.String()
	next := cloneRecords(s.records)
	next[key] = rec
	if err := validateRecords(next); err != nil {
		return fmt.Errorf("state: %w", err)
	}
	s.records = next
	return nil
}

// Save writes the store to disk atomically.
func (s *Store) Save() error {
	if s == nil {
		return fmt.Errorf("state: store is nil")
	}
	data, err := marshalFile(file{
		Version:   s.version,
		Resources: cloneRecords(s.records),
	})
	if err != nil {
		return fmt.Errorf("state: encode %s: %w", s.path, err)
	}
	if err := writeAtomic(s.path, data); err != nil {
		return fmt.Errorf("state: cannot write %s atomically: %w", s.path, err)
	}
	return nil
}

// RecordCreate persists the identity returned by a successful Create.
func (s *Store) RecordCreate(addr resource.Address, live resource.RemoteResource) error {
	return s.persist(addr, live.Identity, "create")
}

// RecordUpdate retains or refreshes identity after a successful Update.
func (s *Store) RecordUpdate(addr resource.Address, live resource.RemoteResource) error {
	id := live.Identity
	if id.IsZero() {
		existing, ok, err := s.Identity(addr)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("state: cannot persist update for %s: missing identity", addr)
		}
		id = existing
	}
	return s.persist(addr, id, "update")
}

// RecordImport persists a mapping from a logical address to an existing remote identity.
func (s *Store) RecordImport(addr resource.Address, remoteID string) error {
	return s.persist(addr, resource.Identity{ID: remoteID}, "import")
}

func (s *Store) persist(addr resource.Address, id resource.Identity, op string) error {
	prev := cloneRecords(s.records)
	if err := s.Bind(addr, id); err != nil {
		return fmt.Errorf("state: %s %s: %w", op, addr, err)
	}
	if err := s.Save(); err != nil {
		s.records = prev
		return err
	}
	return nil
}

func cloneRecords(in map[string]Record) map[string]Record {
	out := make(map[string]Record, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
