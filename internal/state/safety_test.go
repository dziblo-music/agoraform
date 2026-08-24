package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestWriteAtomicReplacementFailurePreservesExistingState(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, DefaultFilename)
	original := []byte("original-valid-state\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	replaceErr := errors.New("simulated replacement failure")
	err := writeAtomicWithReplace(path, []byte("replacement-state\n"), func(tmp, dest string) error {
		if dest != path {
			t.Fatalf("destination = %q, want %q", dest, path)
		}
		if _, err := os.Stat(dest); err != nil {
			t.Fatalf("existing state disappeared before replacement: %v", err)
		}
		if _, err := os.Stat(tmp); err != nil {
			t.Fatalf("temporary state missing before replacement: %v", err)
		}
		return replaceErr
	})
	if !errors.Is(err, replaceErr) {
		t.Fatalf("writeAtomicWithReplace error = %v, want replacement error", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read preserved state: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("existing state changed after failed replacement: got %q want %q", got, original)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".agoraform-state-") {
			t.Fatalf("temporary file left behind after failed replacement: %s", entry.Name())
		}
	}
}

func TestIdentityMayRepeatAcrossResourceTypes(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), DefaultFilename)
	st, err := New(path)
	if err != nil {
		t.Fatal(err)
	}

	widget, err := resource.ParseAddress("fake.widget.homepage")
	if err != nil {
		t.Fatal(err)
	}
	gadget, err := resource.ParseAddress("fake.gadget.homepage")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(widget, resource.Identity{ID: "1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(gadget, resource.Identity{ID: "1"}); err != nil {
		t.Fatalf("same provider ID in a different resource type should be valid: %v", err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("state with type-scoped duplicate IDs failed to load: %v", err)
	}
}
