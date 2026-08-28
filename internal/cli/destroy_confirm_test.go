package cli

import (
	"errors"
	"strings"
	"testing"
)

func TestConfirmDestroyMode(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	if err := confirmDestroyMode(strings.NewReader("yes\n"), &out, false, true); err != nil {
		t.Fatalf("yes: %v", err)
	}

	out.Reset()
	err := confirmDestroyMode(strings.NewReader("no\n"), &out, false, true)
	if !errors.Is(err, errDestroyCancelled) {
		t.Fatalf("no: err=%v, want cancelled", err)
	}
	if !strings.Contains(out.String(), "Destroy cancelled.") {
		t.Fatalf("output = %q, want cancelled message", out.String())
	}

	if err := confirmDestroyMode(strings.NewReader(""), &out, true, false); err != nil {
		t.Fatalf("auto-approve: %v", err)
	}

	err = confirmDestroyMode(strings.NewReader(""), &out, false, false)
	if err == nil || !strings.Contains(err.Error(), "--auto-approve") {
		t.Fatalf("non-interactive = %v, want --auto-approve", err)
	}
}
