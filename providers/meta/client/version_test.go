package client_test

import (
	"testing"

	"github.com/dziblo-music/agoraform/providers/meta/client"
)

// TestPinnedAPIVersion fails when the pinned Meta Graph and Marketing API
// version changes. Every other test compares request paths against
// client.Version, so they pass for any value and cannot detect an upgrade.
// Changing this constant requires reviewing Meta's version changelog and
// migration guidance and re-verifying the provider resources against the new
// version, so the release policy that patch releases never silently switch
// API versions needs an assertion on the literal value.
func TestPinnedAPIVersion(t *testing.T) {
	t.Parallel()
	if client.Version != "v26.0" {
		t.Fatalf("pinned API version = %q, want v26.0; see the API version policy in providers/meta/README.md", client.Version)
	}
	if client.DefaultBaseURL != "https://graph.facebook.com" {
		t.Fatalf("default base URL = %q", client.DefaultBaseURL)
	}
}
