package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
)

const validManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: Homepage banner
`

const invalidManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: not-an-address
    attributes:
      title: Homepage banner
`

func TestValidateSuccess(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", validManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Validated") {
		t.Fatalf("stdout = %q, want Validated message", stdout.String())
	}
	if !strings.Contains(stdout.String(), "1 resource") {
		t.Fatalf("stdout = %q, want resource count", stdout.String())
	}
}

func TestValidatePositionalFile(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "site.yaml", validManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", path})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
}

func TestValidateInvalidManifest(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "bad.yaml", invalidManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	errOut := stderr.String()
	if !strings.Contains(errOut, "invalid") && !strings.Contains(errOut, "provider.type.name") {
		t.Fatalf("stderr = %q, want address error", errOut)
	}
}

func TestValidateMissingFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing.yaml")
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d", code, cli.ExitError)
	}
	if !strings.Contains(stderr.String(), "read manifest") {
		t.Fatalf("stderr = %q, want read manifest error", stderr.String())
	}
}

func TestValidateConflictingFileArgs(t *testing.T) {
	t.Parallel()

	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", "a.yaml", "b.yaml"})
	if code != cli.ExitUsage {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "not both") {
		t.Fatalf("stderr = %q, want conflicting path message", stderr.String())
	}
}

func TestValidateDefaultFilename(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agoraform.yaml"), []byte(validManifest), 0o600); err != nil {
		t.Fatal(err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate"})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "agoraform.yaml") {
		t.Fatalf("stdout = %q, want default filename", stdout.String())
	}
}

func TestValidateHelp(t *testing.T) {
	t.Parallel()

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "--help"})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "validate") || !strings.Contains(out, "agoraform.yaml") {
		t.Fatalf("help output missing expected text:\n%s", out)
	}
}

func writeManifest(t *testing.T, name, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
