package cli_test

import (
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
)

func TestApplyResourceReferenceOrderAndIdempotentPlan(t *testing.T) {
	t.Parallel()

	const referenced = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.banner
    attributes:
      title: Banner
      parent:
        $ref: fake.widget.homepage
  - address: fake.widget.homepage
    attributes:
      title: Homepage banner
`
	reg := provider.NewRegistry()
	p := fake.New()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", referenced)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"apply", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("apply exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	out := stdout.String()
	home := strings.Index(out, "fake.widget.homepage: created")
	banner := strings.Index(out, "fake.widget.banner: created")
	if home < 0 || banner < 0 {
		t.Fatalf("apply stdout missing creates:\n%s", out)
	}
	if home > banner {
		t.Fatalf("dependent created before prerequisite:\n%s", out)
	}
	if strings.Contains(out, "widget-") {
		t.Fatalf("apply leaked provider-native identity:\n%s", out)
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after apply exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after referenced apply = %q, want no changes", stdout.String())
	}
}

func TestApplyRejectsCycleWithoutMutation(t *testing.T) {
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
	reg := provider.NewRegistry()
	p := fake.New()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", cycle)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"apply", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "cyclic dependency") {
		t.Fatalf("stderr = %q, want cycle", stderr.String())
	}
	_, creates, updates, imports := p.Calls()
	if creates != 0 || updates != 0 || imports != 0 {
		t.Fatalf("cycle mutated provider: creates=%d updates=%d imports=%d", creates, updates, imports)
	}
}

func TestApplyOutputReferenceOrderAndIdempotentPlan(t *testing.T) {
	t.Parallel()

	const src = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: alt.note.banner
    attributes:
      text:
        $ref: fake.widget.homepage
        output: token
  - address: fake.widget.homepage
    attributes:
      title: Homepage banner
`
	reg := provider.NewRegistry()
	widgets := fake.New()
	notes := fake.NewAlt()
	if err := reg.Register(widgets); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(notes); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", src)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"apply", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("apply exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	out := stdout.String()
	home := strings.Index(out, "fake.widget.homepage: created")
	banner := strings.Index(out, "alt.note.banner: created")
	if home < 0 || banner < 0 {
		t.Fatalf("apply stdout missing creates:\n%s", out)
	}
	if home > banner {
		t.Fatalf("dependent created before prerequisite:\n%s", out)
	}
	if strings.Contains(out, "tok-") || strings.Contains(out, "widget-") {
		t.Fatalf("apply leaked native output or identity:\n%s", out)
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after apply exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after output apply = %q, want no changes", stdout.String())
	}
}
