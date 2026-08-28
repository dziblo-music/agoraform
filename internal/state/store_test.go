package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestLoadMissingFileIsEmpty(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), DefaultFilename)
	st, err := Load(path)
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	addr := mustAddr(t, "matomo.goal.trial_started")
	if _, ok := st.Lookup(addr); ok {
		t.Fatal("empty store should have no records")
	}
}

func TestSaveLoadRoundTripAndDeterministic(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), DefaultFilename)
	st, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	first := mustAddr(t, "matomo.goal.trial_started")
	second := mustAddr(t, "fake.widget.homepage")
	if err := st.Bind(second, resource.Identity{ID: "widget-1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(first, resource.Identity{ID: "12"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "version": 1,
  "resources": {
    "fake.widget.homepage": {
      "provider": "fake",
      "remoteId": "widget-1"
    },
    "matomo.goal.trial_started": {
      "provider": "matomo",
      "remoteId": "12"
    }
  }
}
`
	if string(got) != want {
		t.Fatalf("serialized state mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}

	again, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := again.Bind(first, resource.Identity{ID: "12"}); err != nil {
		t.Fatal(err)
	}
	if err := again.Bind(second, resource.Identity{ID: "widget-1"}); err != nil {
		t.Fatal(err)
	}
	if err := again.Save(); err != nil {
		t.Fatal(err)
	}
	got2, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got2) != want {
		t.Fatalf("second save was not deterministic\n%s", got2)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	id, ok, err := loaded.Identity(first)
	if err != nil || !ok || id.ID != "12" {
		t.Fatalf("Identity(%s) = (%v, %v, %v)", first, id, ok, err)
	}
}

func TestAddressByRemoteID(t *testing.T) {
	t.Parallel()

	st, err := New(filepath.Join(t.TempDir(), DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}
	goal := mustAddr(t, "matomo.goal.trial_started")
	tag := mustAddr(t, "matomo.tag.trial_started")
	trigger := mustAddr(t, "matomo.trigger.trial_started")
	if err := st.Bind(goal, resource.Identity{ID: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(tag, resource.Identity{ID: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(trigger, resource.Identity{ID: "4"}); err != nil {
		t.Fatal(err)
	}

	got, ok, err := st.AddressByRemoteID("matomo", "tag", "1")
	if err != nil || !ok || got != tag {
		t.Fatalf("AddressByRemoteID(tag,1) = (%v,%v,%v), want %s", got, ok, err, tag)
	}
	got, ok, err = st.AddressByRemoteID("matomo", "goal", "1")
	if err != nil || !ok || got != goal {
		t.Fatalf("AddressByRemoteID(goal,1) = (%v,%v,%v), want %s", got, ok, err, goal)
	}
	if _, ok, err := st.AddressByRemoteID("matomo", "trigger", "1"); err != nil || ok {
		t.Fatalf("AddressByRemoteID(trigger,1) = ok=%v err=%v, want missing", ok, err)
	}
	got, ok, err = st.AddressByRemoteID("matomo", "trigger", "4")
	if err != nil || !ok || got != trigger {
		t.Fatalf("AddressByRemoteID(trigger,4) = (%v,%v,%v), want %s", got, ok, err, trigger)
	}
	if _, ok, err := st.AddressByRemoteID("matomo", "tag", "99"); err != nil || ok {
		t.Fatalf("missing remote id: ok=%v err=%v", ok, err)
	}

	bindings, err := st.Bindings("matomo", "tag")
	if err != nil {
		t.Fatalf("Bindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0].Address != tag || bindings[0].RemoteID != "1" {
		t.Fatalf("Bindings(tag) = %+v, want %s/1", bindings, tag)
	}
}

func TestSaveReplacesFileAtomically(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, DefaultFilename)
	st, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustAddr(t, "fake.widget.a"), resource.Identity{ID: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustAddr(t, "fake.widget.b"), resource.Identity{ID: "2"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Lookup(mustAddr(t, "fake.widget.b")); !ok {
		t.Fatal("replaced state missing second resource")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Fatalf("temporary file left behind: %s", e.Name())
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatalf("replaced file is not valid JSON: %s", raw)
	}
}

func TestLoadMalformedAndUnsupportedVersion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad); err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("malformed error = %v", err)
	}

	unsupported := filepath.Join(dir, "v2.json")
	if err := os.WriteFile(unsupported, []byte("{\n  \"version\": 2,\n  \"resources\": {}\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(unsupported); err == nil || !strings.Contains(err.Error(), "unsupported version") {
		t.Fatalf("unsupported version error = %v", err)
	}
}

func TestLoadInvalidRecords(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"invalid address": `{
  "version": 1,
  "resources": {
    "not-an-address": {"provider": "fake", "remoteId": "1"}
  }
}`,
		"empty identity": `{
  "version": 1,
  "resources": {
    "fake.widget.homepage": {"provider": "fake", "remoteId": ""}
  }
}`,
		"provider mismatch": `{
  "version": 1,
  "resources": {
    "fake.widget.homepage": {"provider": "matomo", "remoteId": "1"}
  }
}`,
		"duplicate identity": `{
  "version": 1,
  "resources": {
    "fake.widget.a": {"provider": "fake", "remoteId": "same"},
    "fake.widget.b": {"provider": "fake", "remoteId": "same"}
  }
}`,
	}

	dir := t.TempDir()
	for name, body := range cases {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(dir, strings.ReplaceAll(name, " ", "_")+".json")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("Load succeeded, want error")
			}
		})
	}
}

func TestBindRejectsEmptyAndConflictingIdentity(t *testing.T) {
	t.Parallel()

	st, err := New(filepath.Join(t.TempDir(), DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}
	addr := mustAddr(t, "fake.widget.homepage")
	if err := st.Bind(addr, resource.Identity{}); err == nil {
		t.Fatal("empty identity should fail")
	}
	if err := st.Bind(addr, resource.Identity{ID: "widget-1"}); err != nil {
		t.Fatal(err)
	}
	other := mustAddr(t, "fake.widget.other")
	err = st.Bind(other, resource.Identity{ID: "widget-1"})
	if err == nil {
		t.Fatal("duplicate identity should fail")
	}
	var conflict *DuplicateIdentityError
	if !errors.As(err, &conflict) || conflict.RemoteID != "widget-1" {
		t.Fatalf("error = %v, want DuplicateIdentityError for widget-1", err)
	}
}

func TestRecordCreateUpdateImport(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), DefaultFilename)
	st, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	addr := mustAddr(t, "fake.widget.homepage")

	if err := st.RecordCreate(addr, resource.RemoteResource{}); err == nil {
		t.Fatal("create with empty identity should fail")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("failed create should not write state: %v", err)
	}

	live := resource.RemoteResource{Address: addr, Identity: resource.Identity{ID: "widget-9"}}
	if err := st.RecordCreate(addr, live); err != nil {
		t.Fatal(err)
	}
	if err := st.RecordUpdate(addr, resource.RemoteResource{Address: addr}); err != nil {
		t.Fatal(err)
	}
	id, ok, err := st.Identity(addr)
	if err != nil || !ok || id.ID != "widget-9" {
		t.Fatalf("after update Identity = (%v,%v,%v)", id, ok, err)
	}

	imported := mustAddr(t, "fake.widget.imported")
	if err := st.RecordImport(imported, "widget-imported"); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := loaded.Identity(imported)
	if err != nil || !ok || got.ID != "widget-imported" {
		t.Fatalf("imported identity = (%v,%v,%v)", got, ok, err)
	}
}

func TestStateContainsNoSecrets(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), DefaultFilename)
	st, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	secret := "super-secret-token-value"
	if err := st.Bind(mustAddr(t, "matomo.goal.trial_started"), resource.Identity{ID: "12"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatal("state file contained a secret")
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for key := range decoded {
		switch key {
		case "version", "resources":
		default:
			t.Fatalf("unexpected top-level state field %q", key)
		}
	}
	resources, _ := decoded["resources"].(map[string]any)
	for _, rawRec := range resources {
		rec, _ := rawRec.(map[string]any)
		for key := range rec {
			switch key {
			case "provider", "remoteId":
			default:
				t.Fatalf("unexpected record field %q", key)
			}
		}
	}
}

func TestPathForManifest(t *testing.T) {
	t.Parallel()

	if got := PathForManifest("agoraform.yaml"); got != DefaultFilename {
		t.Fatalf("PathForManifest(agoraform.yaml) = %q", got)
	}
	got := PathForManifest(filepath.Join("site", "agoraform.yaml"))
	want := filepath.Join("site", DefaultFilename)
	if got != want {
		t.Fatalf("PathForManifest = %q, want %q", got, want)
	}
}

func TestAddressesAreDeterministic(t *testing.T) {
	t.Parallel()

	st, err := New(filepath.Join(t.TempDir(), DefaultFilename))
	if err != nil {
		t.Fatal(err)
	}
	second := mustAddr(t, "fake.widget.homepage")
	first := mustAddr(t, "matomo.goal.trial_started")
	if err := st.Bind(second, resource.Identity{ID: "widget-1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(first, resource.Identity{ID: "12"}); err != nil {
		t.Fatal(err)
	}

	got, err := st.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].String() != "fake.widget.homepage" || got[1].String() != "matomo.goal.trial_started" {
		t.Fatalf("Addresses() = %v, want [fake.widget.homepage matomo.goal.trial_started]", got)
	}
}

func TestRemoveDeletesBindingAndIsIdempotent(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), DefaultFilename)
	st, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	keep := mustAddr(t, "fake.widget.keep")
	drop := mustAddr(t, "fake.widget.drop")
	if err := st.Bind(keep, resource.Identity{ID: "keep-1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(drop, resource.Identity{ID: "drop-1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	if err := st.Remove(drop); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.Identity(drop); err != nil || ok {
		t.Fatalf("Identity(drop) after Remove = (%v, %v)", ok, err)
	}
	id, ok, err := st.Identity(keep)
	if err != nil || !ok || id.ID != "keep-1" {
		t.Fatalf("Identity(keep) = (%v, %v, %v)", id, ok, err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := loaded.Identity(drop); err != nil || ok {
		t.Fatal("removed binding still on disk")
	}

	if err := st.Remove(drop); err != nil {
		t.Fatalf("idempotent Remove: %v", err)
	}
}

func mustAddr(t *testing.T, s string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress(s)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}
