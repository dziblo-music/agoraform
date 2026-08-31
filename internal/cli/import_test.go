package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
)

func TestImportSuccessThenPlanUnchanged(t *testing.T) {
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

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "fake.widget.homepage", "widget-1"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Imported fake.widget.homepage (remote identity widget-1).") {
		t.Fatalf("stdout missing import confirmation:\n%s", out)
	}
	if strings.Contains(out, "serial") {
		t.Fatalf("computed field leaked into import output:\n%s", out)
	}

	yamlText := extractYAML(out)
	parsed, err := manifest.Parse([]byte(yamlText), "generated")
	if err != nil {
		t.Fatalf("generated YAML: %v\n%s", err, out)
	}
	if _, ok := parsed.Resources[0].Attributes[fake.AttrSerial]; ok {
		t.Fatal("computed serial present in generated attributes")
	}
	if err := os.WriteFile(manifestPath, []byte(yamlText), 0o600); err != nil {
		t.Fatal(err)
	}

	st, err := state.Load(state.PathForManifest(manifestPath))
	if err != nil {
		t.Fatal(err)
	}
	id, ok, err := st.Identity(addr)
	if err != nil || !ok || id.ID != "widget-1" {
		t.Fatalf("persisted identity = (%v,%v,%v)", id, ok, err)
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after import exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after import = %q, want no changes", stdout.String())
	}

	_, creates, updates, imports := p.Calls()
	if creates != 0 || updates != 0 {
		t.Fatalf("import/plan mutated provider: creates=%d updates=%d imports=%d", creates, updates, imports)
	}
}

func TestImportReconstructsCrossProviderOutputRefThenPlanUnchanged(t *testing.T) {
	t.Parallel()

	widgets := fake.New()
	notes := fake.NewAlt()
	parent := mustCLIAddress(t, "fake.widget.homepage")
	note := mustCLIAddress(t, "alt.note.banner")
	if err := widgets.Seed(resource.RemoteResource{
		Address:    parent,
		Identity:   resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage banner"},
		Computed:   resource.Attributes{fake.AttrSerial: 4, fake.OutputToken: "tok-homepage", fake.OutputSecret: "s3cret"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := notes.Seed(resource.RemoteResource{
		Address:    note,
		Identity:   resource.Identity{ID: "note-1"},
		Attributes: resource.Attributes{fake.AttrText: "tok-homepage"},
	}); err != nil {
		t.Fatal(err)
	}

	reg := provider.NewRegistry()
	if err := reg.Register(widgets); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(notes); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, parent.String(), "widget-1"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("widget import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	widgetYAML := extractYAML(stdout.String())

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, note.String(), "note-1"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("note import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	noteOut := stdout.String()
	if !strings.Contains(noteOut, "$ref: fake.widget.homepage") || !strings.Contains(noteOut, "output: token") {
		t.Fatalf("note import missing reconstructed output ref:\n%s", noteOut)
	}
	if strings.Contains(noteOut, "tok-homepage") || strings.Contains(noteOut, "s3cret") {
		t.Fatalf("note import leaked output values:\n%s", noteOut)
	}
	noteYAML := extractYAML(noteOut)

	combined := combineManifestResources(t, widgetYAML, noteYAML)
	if err := os.WriteFile(manifestPath, []byte(combined), 0o600); err != nil {
		t.Fatal(err)
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after output-ref import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after output-ref import = %q", stdout.String())
	}

	_, widgetCreates, widgetUpdates, _ := widgets.Calls()
	_, noteCreates, noteUpdates, _ := notes.Calls()
	if widgetCreates != 0 || widgetUpdates != 0 || noteCreates != 0 || noteUpdates != 0 {
		t.Fatalf("import/plan mutated providers: widget creates=%d updates=%d note creates=%d updates=%d", widgetCreates, widgetUpdates, noteCreates, noteUpdates)
	}
}

func TestImportStaleIdentityIsNotCreate(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustCLIAddress(t, "fake.widget.homepage")
	if err := p.Seed(resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage banner"},
	}); err != nil {
		t.Fatal(err)
	}
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, addr.String(), "widget-1"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import exit = %d; stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	if err := os.WriteFile(manifestPath, []byte(extractYAML(stdout.String())), 0o600); err != nil {
		t.Fatal(err)
	}
	if !p.Remove("widget-1") {
		t.Fatal("failed to remove remote identity")
	}

	streams, _, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitError {
		t.Fatalf("plan exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "persisted identity") {
		t.Fatalf("stderr = %q, want stale identity diagnostic", stderr.String())
	}
}

func TestImportConflictingState(t *testing.T) {
	t.Parallel()

	p := fake.New()
	addr := mustCLIAddress(t, "fake.widget.homepage")
	if err := p.Seed(resource.RemoteResource{
		Address:    addr,
		Identity:   resource.Identity{ID: "widget-1"},
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage banner"},
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
	if err := st.Bind(addr, resource.Identity{ID: "already-bound"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}

	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, addr.String(), "widget-1"}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "already bound") {
		t.Fatalf("stderr = %q, want already bound", stderr.String())
	}
}

func TestImportNotFound(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", emptyResourcesManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, "fake.widget.homepage", "missing"}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "was not found") {
		t.Fatalf("stderr = %q, want not found", stderr.String())
	}
}

func TestImportInvalidAddress(t *testing.T) {
	t.Parallel()

	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"import", "not-an-address", "1"})
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "resource address") {
		t.Fatalf("stderr = %q, want address diagnostic", stderr.String())
	}
}

func TestImportEmptyRemoteID(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "fake.widget.homepage", "  "}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "remote identifier is empty") {
		t.Fatalf("stderr = %q, want empty identifier", stderr.String())
	}
}

func TestImportUnknownResourceType(t *testing.T) {
	t.Parallel()

	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "fake.gadget.homepage", "1"}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown resource type") {
		t.Fatalf("stderr = %q, want unknown resource type", stderr.String())
	}
}

func TestImportMissingArgs(t *testing.T) {
	t.Parallel()

	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"import"})
	if code != cli.ExitUsage {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitUsage, stderr.String())
	}
}

func TestImportMalformedStateFile(t *testing.T) {
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
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, "fake.widget.homepage", "widget-1"}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "malformed") && !strings.Contains(stderr.String(), "state") {
		t.Fatalf("stderr = %q, want malformed state diagnostic", stderr.String())
	}
}

func TestImportNoRegisteredProviders(t *testing.T) {
	t.Parallel()

	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "fake.widget.homepage", "1"}, provider.NewRegistry())
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "registered provider") {
		t.Fatalf("stderr = %q, want registered provider error", stderr.String())
	}
}

func TestImportHelpDocumentsBehavior(t *testing.T) {
	t.Parallel()

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"import", "--help"})
	if code != cli.ExitOK {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"import", "ADDRESS", "REMOTE-ID", "agoraform.state.json", "never creates"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing %q:\n%s", want, out)
		}
	}
}

func TestImportMatomoGoalPersistsIdentityNotManifest(t *testing.T) {
	t.Parallel()

	p, srv := matomoGoalServerProvider(t)
	srv.mu.Lock()
	srv.goals["12"] = map[string]any{
		"idgoal":          "12",
		"idsite":          "3",
		"name":            "Trial Started",
		"match_attribute": "event_action",
		"pattern":         "trialStarted",
		"pattern_type":    "contains",
		"revenue":         "0",
	}
	srv.mu.Unlock()

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "agoraform.yaml")
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "matomo.goal.trial_started", "12"}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if strings.Contains(out, "cli-test-token") {
		t.Fatalf("import output leaked token:\n%s", out)
	}
	if strings.Contains(out, "idGoal") || strings.Contains(out, "idgoal") {
		t.Fatalf("Matomo identity leaked into generated YAML:\n%s", out)
	}

	yamlText := extractYAML(out)
	parsed, err := manifest.Parse([]byte(yamlText), "generated")
	if err != nil {
		t.Fatalf("generated YAML: %v\n%s", err, yamlText)
	}
	if _, ok := parsed.Resources[0].Attributes["idGoal"]; ok {
		t.Fatal("idGoal present in generated attributes")
	}
	if err := os.WriteFile(manifestPath, []byte(yamlText), 0o600); err != nil {
		t.Fatal(err)
	}

	st, err := state.Load(state.PathForManifest(manifestPath))
	if err != nil {
		t.Fatal(err)
	}
	id, ok, err := st.Identity(mustCLIAddress(t, "matomo.goal.trial_started"))
	if err != nil || !ok || id.ID != "12" {
		t.Fatalf("persisted matomo identity = (%v,%v,%v), want 12", id, ok, err)
	}
	raw, err := os.ReadFile(state.PathForManifest(manifestPath))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "cli-test-token") {
		t.Fatal("state file leaked token")
	}

	streams, stdout, stderr = testStreams()
	code = cli.ExecuteWithRegistry(streams, []string{"plan", "-f", manifestPath}, reg)
	if code != cli.ExitOK {
		t.Fatalf("plan after matomo import exit = %d, want %d; stderr=%q stdout=%q", code, cli.ExitOK, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "No changes.") {
		t.Fatalf("plan after matomo import = %q, want no changes", stdout.String())
	}
	srv.mu.Lock()
	creates, updates := srv.creates, srv.updates
	srv.mu.Unlock()
	if creates != 0 || updates != 0 {
		t.Fatalf("matomo import mutated remote goals: creates=%d updates=%d", creates, updates)
	}
}

func TestImportMatomoGoalInvalidID(t *testing.T) {
	t.Parallel()

	p, _ := matomoGoalApplyProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	path := writeManifest(t, "agoraform.yaml", emptyResourcesManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", path, "matomo.goal.trial_started", "abc"}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "valid Matomo goal id") && !strings.Contains(stderr.String(), "identity") {
		t.Fatalf("stderr = %q, want invalid id diagnostic", stderr.String())
	}
}

func extractYAML(out string) string {
	const marker = "apiVersion:"
	i := strings.Index(out, marker)
	if i < 0 {
		return out
	}
	return out[i:]
}
