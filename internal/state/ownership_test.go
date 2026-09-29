package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestOwnershipRoundTripAndLegacyManaged(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), DefaultFilename)
	st, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	external := mustAddr(t, "matomo.container.main")
	legacy := mustAddr(t, "fake.widget.homepage")
	if err := st.RecordExternal(external, resource.Identity{ID: "Aa000001"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(legacy, resource.Identity{ID: "widget-1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	ownership, ok, err := loaded.Ownership(external)
	if err != nil || !ok || ownership != resource.OwnershipExternal {
		t.Fatalf("external ownership = (%s, %v, %v)", ownership, ok, err)
	}
	ownership, ok, err = loaded.Ownership(legacy)
	if err != nil || !ok || ownership != resource.OwnershipManaged {
		t.Fatalf("legacy ownership = (%s, %v, %v)", ownership, ok, err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(string(raw), `"ownership": "external"`, `"remoteId": "widget-1"`) {
		t.Fatalf("state = %s", raw)
	}
	if containsAll(string(raw), `"fake.widget.homepage": {
      "provider": "fake",
      "remoteId": "widget-1",
      "ownership"`) {
		t.Fatalf("legacy binding should omit ownership:\n%s", raw)
	}

	if err := loaded.Bind(external, resource.Identity{ID: "Aa000001", Fingerprint: "abc"}); err != nil {
		t.Fatal(err)
	}
	ownership, _, err = loaded.Ownership(external)
	if err != nil || ownership != resource.OwnershipExternal {
		t.Fatalf("bind preserved ownership = %s, err %v", ownership, err)
	}
	id, _, err := loaded.Identity(external)
	if err != nil || id.Fingerprint != "abc" {
		t.Fatalf("fingerprint = %+v, err %v", id, err)
	}

	if err := loaded.SetOwnership(external, id, resource.OwnershipManaged); err != nil {
		t.Fatal(err)
	}
	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	ownership, ok, err = again.Ownership(external)
	if err != nil || !ok || ownership != resource.OwnershipManaged {
		t.Fatalf("adopted ownership = (%s, %v, %v)", ownership, ok, err)
	}
}

func TestLoadRejectsInvalidOwnership(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), DefaultFilename)
	body := `{
  "version": 1,
  "resources": {
    "fake.widget.homepage": {
      "provider": "fake",
      "remoteId": "widget-1",
      "ownership": "shared"
    }
  }
}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load succeeded, want invalid ownership")
	}
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !contains(s, part) {
			return false
		}
	}
	return true
}

func contains(s, part string) bool {
	return len(part) == 0 || (len(s) >= len(part) && (s == part || len(s) > 0 && stringIndex(s, part) >= 0))
}

func stringIndex(s, part string) int {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}
