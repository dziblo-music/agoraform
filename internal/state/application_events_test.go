package state_test

import (
	"path/filepath"
	"testing"

	"github.com/dziblo-music/agoraform/internal/state"
)

func TestApplicationEventFingerprintsPersist(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), state.DefaultFilename)
	st, err := state.New(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"trial_started": "abc123"}
	if err := st.RecordApplicationEvents(want); err != nil {
		t.Fatal(err)
	}
	loaded, err := state.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.ApplicationEventFingerprints()
	if got["trial_started"] != "abc123" {
		t.Fatalf("fingerprint = %q, want abc123", got["trial_started"])
	}
}

func TestApplicationEventFingerprintsEmptyStateCompatible(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), state.DefaultFilename)
	st, err := state.New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := state.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.ApplicationEventFingerprints()) != 0 {
		t.Fatalf("expected no application event fingerprints")
	}
}
