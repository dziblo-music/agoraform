package cli_test

import (
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
)

const integrationsManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.trigger.trial_started
    attributes:
      type: customEvent
      event: trialStarted
applicationEvents:
  trial_started:
    matomo:
      trigger:
        $ref: matomo.trigger.trial_started
      fields:
        - userId
`

const integrationsEmptyManifest = `apiVersion: agoraform.io/v1alpha1
resources: []
`

const integrationsGoogleAdsManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: googleads.conversion_action.trial_started
    attributes:
      name: Trial Started
      category: SIGNUP
      value: 0
      count: ONE
      primaryForGoal: true
applicationEvents:
  trial_started:
    googleAds:
      conversion:
        $ref: googleads.conversion_action.trial_started
`

const integrationsMetaManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: meta.pixel.main
    attributes:
      name: Website
applicationEvents:
  purchase:
    meta:
      eventSource:
        $ref: meta.pixel.main
      eventName: Purchase
      delivery: browser
`

const integrationsMultiProviderManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.trigger.page_view
    attributes:
      type: customEvent
      event: pageView
  - address: matomo.trigger.signup
    attributes:
      type: customEvent
      event: signedUp
applicationEvents:
  page_view:
    matomo:
      trigger:
        $ref: matomo.trigger.page_view
  signup:
    matomo:
      trigger:
        $ref: matomo.trigger.signup
      fields:
        - planId
`

func TestIntegrationsNoEvents(t *testing.T) {
	t.Parallel()
	path := writeManifest(t, "agoraform.yaml", integrationsEmptyManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"integrations", "-f", path})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No application events") {
		t.Fatalf("stdout = %q, want no-events message", stdout.String())
	}
}

func TestIntegrationsMatomoOnly(t *testing.T) {
	t.Parallel()
	path := writeManifest(t, "agoraform.yaml", integrationsManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"integrations", "-f", path})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"trial_started", "trialStarted", "userId"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestIntegrationsMultipleEvents(t *testing.T) {
	t.Parallel()
	path := writeManifest(t, "agoraform.yaml", integrationsMultiProviderManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"integrations", "-f", path})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	out := stdout.String()
	if strings.Index(out, "page_view") > strings.Index(out, "signup") {
		t.Fatalf("events not sorted:\n%s", out)
	}
}

func TestIntegrationsGoogleAdsNotAppliedDoesNotRequireCredentials(t *testing.T) {
	t.Parallel()
	path := writeManifest(t, "agoraform.yaml", integrationsGoogleAdsManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"integrations", "-f", path})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "trial_started") || !strings.Contains(out, "not yet applied") {
		t.Fatalf("expected unapplied Google Ads contract:\n%s", out)
	}
}

func TestIntegrationsMetaNotAppliedDoesNotRequireCredentials(t *testing.T) {
	t.Parallel()
	path := writeManifest(t, "agoraform.yaml", integrationsMetaManifest)
	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"integrations", "-f", path})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "purchase") || !strings.Contains(out, "not yet applied") {
		t.Fatalf("expected unapplied Meta contract:\n%s", out)
	}
}

func TestIntegrationsFlagFile(t *testing.T) {
	t.Parallel()
	path := writeManifest(t, "agoraform.yaml", integrationsManifest)
	streams, _, stderr := testStreams()
	if code := cli.ExecuteWith(streams, []string{"integrations", "--file", path}); code != cli.ExitOK {
		t.Fatalf("exit code = %d; stderr=%q", code, stderr.String())
	}
}

func TestIntegrationsTooManyArgs(t *testing.T) {
	t.Parallel()
	streams, _, _ := testStreams()
	if code := cli.ExecuteWith(streams, []string{"integrations", "arg1", "arg2"}); code != cli.ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, cli.ExitUsage)
	}
}

func TestIntegrations_HelpIncludesCommand(t *testing.T) {
	t.Parallel()
	streams, stdout, _ := testStreams()
	cli.ExecuteWith(streams, []string{"--help"})
	if !strings.Contains(stdout.String(), "integrations") {
		t.Errorf("help output does not mention integrations:\n%s", stdout.String())
	}
}

func TestIntegrations_InvalidManifest(t *testing.T) {
	t.Parallel()
	path := writeManifest(t, "agoraform.yaml", "not: valid: yaml: [[[")
	streams, _, _ := testStreams()
	if code := cli.ExecuteWith(streams, []string{"integrations", "-f", path}); code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d", code, cli.ExitError)
	}
}
