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
			Parameters:         map[string]any{"dataLayerName": "userId"},
		}},
		nil,
		[]client.Tag{{
			IDTag:              "7",
			IDContainerVersion: "9",
			Type:               "Matomo",
			Name:               "trialStarted",
			Status:             "active",
			Parameters:         map[string]any{"eventAction": "trialStarted"},
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
			Parameters:         map[string]any{"dataLayerName": "userId"},
		}},
		nil,
		[]client.Tag{{
			IDTag:              "70",
			IDContainerVersion: "12",
			Type:               "Matomo",
			Name:               "trialStarted",
			Status:             "",
			Parameters:         map[string]any{"eventAction": "trialStarted"},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if draft != published {
		t.Fatalf("provider-native ids changed fingerprint:\ndraft=%s\npublished=%s", draft, published)
	}
}

func TestFingerprintDetectsTagStatusChange(t *testing.T) {
	t.Parallel()

	active, err := fingerprintContainer(nil, nil, []client.Tag{{
		Type:       "Matomo",
		Name:       "trialStarted",
		Status:     "active",
		Parameters: map[string]any{"eventAction": "trialStarted"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	paused, err := fingerprintContainer(nil, nil, []client.Tag{{
		Type:       "Matomo",
		Name:       "trialStarted",
		Status:     "paused",
		Parameters: map[string]any{"eventAction": "trialStarted"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if active == paused {
		t.Fatal("expected paused vs active tag status to require publication")
	}
}
