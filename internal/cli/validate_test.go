package cli_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

const validManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: Homepage banner
`

const emptyResourcesManifest = `apiVersion: agoraform.io/v1alpha1
resources: []
`

const matomoGoalManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.goal.trial_started
    attributes:
      name: Trial Started
      matchAttribute: event_action
      pattern: trialStarted
`

const matomoGoalIncompleteManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.goal.trial_started
    attributes:
      name: Trial Started
      matchAttribute: event_action
`

const invalidManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: not-an-address
    attributes:
      title: Homepage banner
`

func TestValidateSuccess(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", emptyResourcesManifest)
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
	if !strings.Contains(stdout.String(), "0 resources") {
		t.Fatalf("stdout = %q, want resource count", stdout.String())
	}
}

func TestValidatePositionalFile(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "site.yaml", emptyResourcesManifest)
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
	if err := os.WriteFile(filepath.Join(dir, "agoraform.yaml"), []byte(emptyResourcesManifest), 0o600); err != nil {
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

func TestValidateMatomoGoalMissingCredentials(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", matomoGoalManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	errOut := stderr.String()
	if strings.Contains(errOut, "unknown resource type") {
		t.Fatalf("stderr = %q, matomo.goal should be registered", errOut)
	}
	if !strings.Contains(errOut, "MATOMO_URL") && !strings.Contains(errOut, "required") {
		t.Fatalf("stderr = %q, want missing credential error", errOut)
	}
}

func TestValidateWithFakeProvider(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", validManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 resource") {
		t.Fatalf("stdout = %q, want 1 resource", stdout.String())
	}
}

func TestValidateResourceReference(t *testing.T) {
	t.Parallel()

	const referenced = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: Homepage banner
  - address: fake.widget.banner
    attributes:
      title: Banner
      parent:
        $ref: fake.widget.homepage
`
	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", referenced)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "2 resources") {
		t.Fatalf("stdout = %q, want 2 resources", stdout.String())
	}
}

func TestValidateMissingReference(t *testing.T) {
	t.Parallel()

	const missing = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.banner
    attributes:
      title: Banner
      parent:
        $ref: fake.widget.missing
`
	path := writeManifest(t, "agoraform.yaml", missing)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	errOut := stderr.String()
	if !strings.Contains(errOut, "unknown resource") || !strings.Contains(errOut, "fake.widget.missing") {
		t.Fatalf("stderr = %q, want missing reference", errOut)
	}
}

func TestValidateSelfReference(t *testing.T) {
	t.Parallel()

	const selfRef = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.loop
    attributes:
      title: Loop
      parent:
        $ref: fake.widget.loop
`
	path := writeManifest(t, "agoraform.yaml", selfRef)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "itself") {
		t.Fatalf("stderr = %q, want self-reference", stderr.String())
	}
}

func TestValidateDependencyCycle(t *testing.T) {
	t.Parallel()

	const cycle = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.alpha
    attributes:
      title: Alpha
      parent:
        $ref: fake.widget.beta
  - address: fake.widget.beta
    attributes:
      title: Beta
      parent:
        $ref: fake.widget.alpha
`
	path := writeManifest(t, "agoraform.yaml", cycle)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"validate", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "cyclic dependency") {
		t.Fatalf("stderr = %q, want cycle", stderr.String())
	}
}

func TestValidateV01ManifestWithoutReferences(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", validManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 resource") {
		t.Fatalf("stdout = %q, want 1 resource", stdout.String())
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

func TestValidateMatomoGoalWithProvider(t *testing.T) {
	t.Parallel()

	p, _ := matomoGoalTestProvider(t, `[]`)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoGoalManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 resource") {
		t.Fatalf("stdout = %q, want 1 resource", stdout.String())
	}
}

func TestValidateMatomoGoalMissingPattern(t *testing.T) {
	t.Parallel()

	p, _ := matomoGoalTestProvider(t, `[]`)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoGoalIncompleteManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "pattern") {
		t.Fatalf("stderr = %q, want pattern validation error", stderr.String())
	}
}

func matomoGoalTestProvider(t *testing.T, getGoalsBody string) (*matomo.Provider, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "API.getMatomoVersion") {
			_, _ = io.WriteString(w, `"5.2.0"`)
			return
		}
		_, _ = io.WriteString(w, getGoalsBody)
	}))
	t.Cleanup(srv.Close)
	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:    srv.URL,
		TokenAuth:  "cli-test-token",
		SiteID:     "3",
		HTTPClient: srv.Client(),
	}, srv.Client())
	return p, srv
}

func writeManifest(t *testing.T, name, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}