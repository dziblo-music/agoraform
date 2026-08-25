package cli_test

import (
	"os"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
)

func TestPlanEmptyManifestNoProviders(t *testing.T) {
	t.Parallel()

	const emptyManifest = "apiVersion: agoraform.io/v1alpha1\nresources: []\n"
	path := writeManifest(t, "agoraform.yaml", emptyManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"plan", "-f", path})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("stdout = %q, want no-change plan", stdout.String())
	}
}

func TestPlanNoRegisteredProviders(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", validManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, provider.NewRegistry())
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "registered provider") {
		t.Fatalf("stderr = %q, want registered provider error", stderr.String())
	}
}

func TestPlanMatomoGoalMissingCredentials(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "agoraform.yaml", matomoGoalManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"plan", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if strings.Contains(stderr.String(), "unknown resource type") {
		t.Fatalf("stderr = %q, matomo.goal should be registered", stderr.String())
	}
	if !strings.Contains(stderr.String(), "MATOMO_URL") && !strings.Contains(stderr.String(), "required") {
		t.Fatalf("stderr = %q, want missing credential error", stderr.String())
	}
}

func TestPlanMatomoGoalCreate(t *testing.T) {
	t.Parallel()

	p, _ := matomoGoalTestProvider(t, `[]`)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoGoalManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, reg)
	if code != cli.ExitChanges {
		t.Fatalf("exit code = %d, want %d; stderr=%q stdout=%q", code, cli.ExitChanges, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "+ matomo.goal.trial_started") {
		t.Fatalf("stdout missing create:\n%s", out)
	}
}

func TestPlanMatomoVariableCreate(t *testing.T) {
	t.Parallel()

	p, _ := matomoVariableTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoVariableManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, reg)
	if code != cli.ExitChanges {
		t.Fatalf("exit code = %d, want %d; stderr=%q stdout=%q", code, cli.ExitChanges, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "+ matomo.variable.user_id") {
		t.Fatalf("stdout missing create:\n%s", out)
	}
}

func TestPlanMatomoTriggerCreate(t *testing.T) {
	t.Parallel()

	p, _ := matomoVariableTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoTriggerManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, reg)
	if code != cli.ExitChanges {
		t.Fatalf("exit code = %d, want %d; stderr=%q stdout=%q", code, cli.ExitChanges, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "+ matomo.trigger.trial_started") {
		t.Fatalf("stdout missing create:\n%s", out)
	}
}

func TestPlanMatomoGoalNoChanges(t *testing.T) {
	t.Parallel()

	live := `{
		"1": {
			"idgoal": "1",
			"idsite": "3",
			"name": "Trial Started",
			"match_attribute": "event_action",
			"pattern": "trialStarted",
			"pattern_type": "contains",
			"revenue": "0"
		}
	}`
	p, _ := matomoGoalTestProvider(t, live)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoGoalManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("stdout = %q, want no-change plan", stdout.String())
	}
}

func TestPlanInvalidManifest(t *testing.T) {
	t.Parallel()

	path := writeManifest(t, "bad.yaml", invalidManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"plan", "-f", path})
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
}

func TestPlanConflictingFileArgs(t *testing.T) {
	t.Parallel()

	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"plan", "-f", "a.yaml", "b.yaml"})
	if code != cli.ExitUsage {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitUsage, stderr.String())
	}
}

func TestPlanNoChangesExitOK(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustCLIAddress(t, "fake.widget.homepage")
	if err := p.Seed(resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage banner"},
		Computed:   resource.Attributes{fake.AttrSerial: 4},
	}); err != nil {
		t.Fatal(err)
	}

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", validManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("stdout = %q, want no-change plan", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Plan: 0 to create, 0 to update, 0 to destroy.") {
		t.Fatalf("stdout = %q, want zero-change summary", stdout.String())
	}

	_, creates, updates, imports := p.Calls()
	if creates != 0 || updates != 0 || imports != 0 {
		t.Fatalf("plan mutated provider: creates=%d updates=%d imports=%d", creates, updates, imports)
	}
}

func TestPlanChangesExitCode(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	p := fake.New()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", validManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"plan", path}, reg)
	if code != cli.ExitChanges {
		t.Fatalf("exit code = %d, want %d; stderr=%q stdout=%q", code, cli.ExitChanges, stderr.String(), stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty on successful plan with changes", stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "+ fake.widget.homepage") {
		t.Fatalf("stdout missing create:\n%s", out)
	}
	if !strings.Contains(out, "Plan: 1 to create, 0 to update, 0 to destroy.") {
		t.Fatalf("stdout missing summary:\n%s", out)
	}

	_, creates, updates, imports := p.Calls()
	if creates != 0 || updates != 0 || imports != 0 {
		t.Fatalf("plan mutated provider: creates=%d updates=%d imports=%d", creates, updates, imports)
	}
}

func TestPlanProviderValidateError(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}

	const missingTitle = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      color: blue
`
	path := writeManifest(t, "bad.yaml", missingTitle)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "title") {
		t.Fatalf("stderr = %q, want title validation error", stderr.String())
	}
}

func TestPlanHelpDocumentsExitCodes(t *testing.T) {
	t.Parallel()

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"plan", "--help"})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"plan", "agoraform.yaml", "never creates", "agoraform.state.json", "2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing %q:\n%s", want, out)
		}
	}
}

func TestExitCodeConstants(t *testing.T) {
	t.Parallel()

	if cli.ExitOK != 0 || cli.ExitError != 1 || cli.ExitChanges != 2 || cli.ExitUsage != 3 {
		t.Fatalf("exit codes = %d %d %d %d, want 0 1 2 3", cli.ExitOK, cli.ExitError, cli.ExitChanges, cli.ExitUsage)
	}
}

const matomoGoalWithIDGoalManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.goal.trial_started
    attributes:
      idGoal: "12"
      name: Trial Started
      matchAttribute: event_action
      pattern: trialStarted
`

func TestPlanUsesPersistedIdentity(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustCLIAddress(t, "fake.widget.homepage")
	if err := p.Seed(resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage banner"},
		Computed:   resource.Attributes{fake.AttrSerial: 4},
	}); err != nil {
		t.Fatal(err)
	}

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", validManifest)
	st, err := state.New(state.PathForManifest(path))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(addr, resource.Identity{ID: "widget-1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, reg)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("stdout = %q, want no-change plan", stdout.String())
	}
}

func TestPlanStaleStateIdentity(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", validManifest)
	st, err := state.New(state.PathForManifest(path))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Bind(mustCLIAddress(t, "fake.widget.homepage"), resource.Identity{ID: "missing"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "persisted identity") {
		t.Fatalf("stderr = %q, want stale identity diagnostic", stderr.String())
	}
}

func TestPlanMalformedStateFile(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", validManifest)
	if err := os.WriteFile(state.PathForManifest(path), []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}

	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "malformed") && !strings.Contains(stderr.String(), "state") {
		t.Fatalf("stderr = %q, want malformed state diagnostic", stderr.String())
	}
}

func TestPlanRejectsManifestIDGoal(t *testing.T) {
	t.Parallel()

	p, _ := matomoGoalTestProvider(t, `{"12":{"idgoal":"12","name":"Trial Started","match_attribute":"event_action","pattern":"trialStarted","pattern_type":"contains"}}`)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", matomoGoalWithIDGoalManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"plan", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "idGoal") {
		t.Fatalf("stderr = %q, want idGoal diagnostic", stderr.String())
	}
}

func mustCLIAddress(t *testing.T, s string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress(s)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}
