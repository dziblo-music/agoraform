package cli_test

import (
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
)

func TestPlanResourceReference(t *testing.T) {
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
	code := cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, reg)
	if code != cli.ExitChanges {
		t.Fatalf("exit code = %d, want %d; stderr=%q stdout=%q", code, cli.ExitChanges, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "+ fake.widget.banner") || !strings.Contains(out, "+ fake.widget.homepage") {
		t.Fatalf("plan missing logical addresses:\n%s", out)
	}
	if !strings.Contains(out, `parent: "fake.widget.homepage"`) {
		t.Fatalf("plan missing logical reference:\n%s", out)
	}
	if strings.Contains(out, "widget-") {
		t.Fatalf("plan leaked provider-native identity:\n%s", out)
	}

	streams, stdout2, stderr := testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, reg)
	if code != cli.ExitChanges {
		t.Fatalf("second plan exit = %d; stderr=%q", code, stderr.String())
	}
	if stdout.String() != stdout2.String() {
		t.Fatalf("plan was not stable\nfirst:\n%s\nsecond:\n%s", stdout.String(), stdout2.String())
	}
}

func TestPlanRejectsCycle(t *testing.T) {
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
	code := cli.ExecuteWith(streams, []string{"plan", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "cyclic dependency") {
		t.Fatalf("stderr = %q, want cycle", stderr.String())
	}
}
