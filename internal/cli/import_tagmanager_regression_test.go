package cli_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/state"
)

func TestImportMatomoTagRejectsWhitespaceAroundManagedVariableTemplate(t *testing.T) {
	t.Parallel()

	p, srv := matomoTagManagerServerProvider(t)
	srv.seedVariable(cliTMVariable{ID: 2, Name: "userId", Type: "DataLayer", Key: "userId"})
	srv.seedTrigger(cliTMTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(cliTMTag{
		ID:            1,
		Name:          "trialStarted",
		FireTriggerID: 4,
		Category:      " {{userId}} ",
		Action:        "trialStarted",
	})

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(t.TempDir(), "agoraform.yaml")
	importDependency(t, reg, manifestPath, "matomo.variable.user_id", "2")
	importDependency(t, reg, manifestPath, "matomo.trigger.trial_started", "4")

	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "matomo.tag.trial_started", "1"}, reg)
	if code != cli.ExitError {
		t.Fatalf("tag import exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), `attribute "eventCategory" must not have leading or trailing whitespace`) {
		t.Fatalf("stderr = %q, want whitespace validation guidance", stderr.String())
	}
	assertNoImportedTagBinding(t, manifestPath)
	if srv.mutationCount() != 0 {
		t.Fatalf("failed import mutated remote: %d", srv.mutationCount())
	}
}

func TestImportMatomoTagRejectsDuplicateVariableNames(t *testing.T) {
	t.Parallel()

	p, srv := matomoTagManagerServerProvider(t)
	srv.seedVariable(cliTMVariable{ID: 2, Name: "userId", Type: "DataLayer", Key: "userId"})
	srv.seedVariable(cliTMVariable{ID: 3, Name: "userId", Type: "DataLayer", Key: "alternateUserId"})
	srv.seedTrigger(cliTMTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(cliTMTag{
		ID:            1,
		Name:          "trialStarted",
		FireTriggerID: 4,
		Category:      "{{userId}}",
		Action:        "trialStarted",
	})

	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(t.TempDir(), "agoraform.yaml")
	importDependency(t, reg, manifestPath, "matomo.variable.user_id", "2")
	importDependency(t, reg, manifestPath, "matomo.trigger.trial_started", "4")

	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, "matomo.tag.trial_started", "1"}, reg)
	if code != cli.ExitError {
		t.Fatalf("tag import exit = %d, want %d; stderr=%q", code, cli.ExitError, stderr.String())
	}
	if !strings.Contains(stderr.String(), `variable template "{{userId}}" matches multiple active remote variables`) {
		t.Fatalf("stderr = %q, want duplicate variable guidance", stderr.String())
	}
	if !strings.Contains(stderr.String(), "ids 2, 3") {
		t.Fatalf("stderr = %q, want deterministic duplicate ids", stderr.String())
	}
	assertNoImportedTagBinding(t, manifestPath)
	if srv.mutationCount() != 0 {
		t.Fatalf("failed import mutated remote: %d", srv.mutationCount())
	}
}

func importDependency(t *testing.T, reg *provider.Registry, manifestPath, address, remoteID string) {
	t.Helper()
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"import", "-f", manifestPath, address, remoteID}, reg)
	if code != cli.ExitOK {
		t.Fatalf("import %s exit = %d, want %d; stderr=%q stdout=%q", address, code, cli.ExitOK, stderr.String(), stdout.String())
	}
}

func assertNoImportedTagBinding(t *testing.T, manifestPath string) {
	t.Helper()
	st, err := state.Load(state.PathForManifest(manifestPath))
	if err != nil {
		t.Fatal(err)
	}
	if id, ok, err := st.Identity(mustCLIAddress(t, "matomo.tag.trial_started")); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatalf("failed tag import persisted identity %q", id.ID)
	}
}
