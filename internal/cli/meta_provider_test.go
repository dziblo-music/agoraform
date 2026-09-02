package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/providers/meta"
)

func TestDefaultRegistryIncludesMeta(t *testing.T) {
	t.Setenv(meta.EnvAccessToken, "")
	t.Setenv(meta.EnvAdAccountID, "")
	dir := t.TempDir()
	path := filepath.Join(dir, "agoraform.yaml")
	if err := os.WriteFile(path, []byte("apiVersion: agoraform.io/v1alpha1\nproviders:\n  meta: {}\nresources: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), meta.EnvAccessToken) || strings.Contains(stderr.String(), "unknown provider") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
