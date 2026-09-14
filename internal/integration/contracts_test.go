package integration_test

import (
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/integration"
	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestContractFingerprintsDetectExternallyConsumedChanges(t *testing.T) {
	t.Parallel()
	trigger := makeAddr("matomo.trigger.trial_started")
	meta := makeAddr("meta.pixel.main")
	resources := []resource.Resource{
		{Address: trigger, Attributes: resource.Attributes{"event": "trialStarted"}},
		{Address: meta},
	}
	events := map[string]manifest.ApplicationEvent{
		"trial_started": {
			Name: "trial_started",
			Matomo: &manifest.MatomoEventBinding{
				Trigger: resource.Ref{Address: trigger},
				Fields:  []string{"userId"},
			},
			Meta: &manifest.MetaEventBinding{
				EventSource: resource.Ref{Address: meta},
				EventName:   "StartTrial",
				Delivery:    manifest.DeliveryBrowser,
			},
		},
	}
	before, err := integration.ContractFingerprints(events, resources)
	if err != nil {
		t.Fatal(err)
	}

	resources[0].Attributes["event"] = "trialStartedV2"
	after, err := integration.ContractFingerprints(events, resources)
	if err != nil {
		t.Fatal(err)
	}
	changes := integration.DiffContractFingerprints(before, after)
	if len(changes) != 1 || changes[0].Action != integration.ContractUpdate {
		t.Fatalf("changes = %#v, want one update", changes)
	}
	formatted := integration.FormatContractChanges(changes)
	if !strings.Contains(formatted, "~ applicationEvents.trial_started") {
		t.Fatalf("formatted changes = %q", formatted)
	}
}

func TestDiffContractFingerprintsCreateDeleteSorted(t *testing.T) {
	t.Parallel()
	previous := map[string]string{"z": "old", "b": "same"}
	desired := map[string]string{"a": "new", "b": "same"}
	changes := integration.DiffContractFingerprints(previous, desired)
	if len(changes) != 2 {
		t.Fatalf("changes = %#v", changes)
	}
	if changes[0].Name != "a" || changes[0].Action != integration.ContractCreate {
		t.Fatalf("first change = %#v", changes[0])
	}
	if changes[1].Name != "z" || changes[1].Action != integration.ContractDelete {
		t.Fatalf("second change = %#v", changes[1])
	}
}
