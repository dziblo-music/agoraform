package cli_test

import (
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
)

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

func TestValidateOutputReference(t *testing.T) {
	t.Parallel()

	const src = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: Homepage banner
  - address: alt.note.banner
    attributes:
      text:
        $ref: fake.widget.homepage
        output: token
`
	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(fake.NewAlt()); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", src)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "2 resources") {
		t.Fatalf("stdout = %q, want 2 resources", stdout.String())
	}
}

func TestValidateSensitiveOutput(t *testing.T) {
	t.Parallel()

	const src = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: Homepage banner
  - address: alt.note.banner
    attributes:
      text:
        $ref: fake.widget.homepage
        output: secret
`
	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(fake.NewAlt()); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", src)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "sensitive") {
		t.Fatalf("stderr = %q, want sensitive", stderr.String())
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
