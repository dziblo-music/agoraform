package matomo

import (
	"testing"

	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestFingerprintIgnoresProviderNativeIDs(t *testing.T) {
	t.Parallel()

	draft, err := fingerprintContainer(
		[]client.Variable{{
			IDVariable:         "2",
			IDContainerVersion: "9",
			Type:               "DataLayer",
			Name:               "userId",
			Status:             "active",
			Parameters:         map[string]string{"dataLayerName": "userId"},
			LookupTable:        []byte(`[]`),
		}},
		[]client.Trigger{{
			IDTrigger:          "4",
			IDContainerVersion: "9",
			Type:               "CustomEvent",
			Name:               "trialStarted",
			Status:             "active",
			Parameters:         map[string]string{"eventName": "trialStarted"},
			Conditions:         []byte(`[]`),
		}},
		[]client.Tag{{
			IDTag:              "7",
			IDContainerVersion: "9",
			Type:               "Matomo",
			Name:               "trialStarted",
			Status:             "active",
			FireTriggerIDs:     []string{"4"},
			Parameters: map[string]any{
				"trackingType":  "event",
				"eventCategory": "signup",
				"eventAction":   "trialStarted",
				"matomoConfig":  map[string]any{"name": "Matomo Configuration", "type": "MatomoConfiguration"},
			},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}

	published, err := fingerprintContainer(
		[]client.Variable{{
			IDVariable:         "20",
			IDContainerVersion: "12",
			Type:               "DataLayer",
			Name:               "userId",
			Status:             "deleted",
			Parameters:         map[string]string{"dataLayerName": "userId"},
			LookupTable:        []byte(`null`),
		}},
		[]client.Trigger{{
			IDTrigger:          "40",
			IDContainerVersion: "12",
			Type:               "CustomEvent",
			Name:               "trialStarted",
			Parameters:         map[string]string{"eventName": "trialStarted"},
			Conditions:         []byte(`null`),
		}},
		[]client.Tag{{
			IDTag:          "70",
			Type:           "Matomo",
			Name:           "trialStarted",
			FireTriggerIDs: []string{"40"},
			Parameters: map[string]any{
				"trackingType":  "event",
				"eventCategory": "signup",
				"eventAction":   "trialStarted",
				"matomoConfig":  "{{Matomo Configuration}}",
			},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if draft != published {
		t.Fatalf("fingerprints differ:\ndraft=%s\npublished=%s", draft, published)
	}
}

func TestFingerprintDetectsTagChange(t *testing.T) {
	t.Parallel()

	before, err := fingerprintContainer(nil, nil, []client.Tag{{
		Type: "Matomo",
		Name: "trialStarted",
		Parameters: map[string]any{
			"eventAction": "trialStarted",
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	after, err := fingerprintContainer(nil, nil, []client.Tag{{
		Type: "Matomo",
		Name: "trialStarted",
		Parameters: map[string]any{
			"eventAction": "trial_started",
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("expected fingerprint change when eventAction changes")
	}
}
