package googleads_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/importer"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestImportConversionActionRejectsUnsupportedStatus(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seed(map[string]any{
		"id":           "21",
		"name":         "Trial Started",
		"category":     "SIGNUP",
		"status":       "UNKNOWN",
		"type":         "WEBPAGE",
		"countingType": "ONE_PER_CLICK",
	})
	p, _ := testConversionActionProvider(t, fake)
	addr := mustConversionActionAddress(t, "trial_started")
	st := mustGoogleAdsImportStore(t)

	_, err := importer.Run(context.Background(), addr, "21", lookupGoogleAds(p), st)
	if err == nil {
		t.Fatal("expected unsupported status error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported status must not look like not found")
	}
	if !strings.Contains(err.Error(), "status UNKNOWN") || !strings.Contains(err.Error(), "ENABLED") {
		t.Fatalf("error = %q, want actionable supported-status guidance", err)
	}
	if _, ok, stateErr := st.Identity(addr); stateErr != nil {
		t.Fatalf("state identity: %v", stateErr)
	} else if ok {
		t.Fatal("unsupported status import persisted identity")
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("unsupported status import mutated remote: %v", fake.mutates)
	}
	assertNoProviderSecret(t, err.Error())
}

func TestImportConversionActionRejectsUnsupportedCountingType(t *testing.T) {
	t.Parallel()

	fake := newConversionActionFake()
	fake.seed(map[string]any{
		"id":           "22",
		"name":         "Trial Started",
		"category":     "SIGNUP",
		"status":       "ENABLED",
		"type":         "WEBPAGE",
		"countingType": "UNKNOWN",
	})
	p, _ := testConversionActionProvider(t, fake)
	addr := mustConversionActionAddress(t, "trial_started")
	st := mustGoogleAdsImportStore(t)

	_, err := importer.Run(context.Background(), addr, "22", lookupGoogleAds(p), st)
	if err == nil {
		t.Fatal("expected unsupported counting type error")
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Fatal("unsupported counting type must not look like not found")
	}
	if !strings.Contains(err.Error(), "counting type UNKNOWN") || !strings.Contains(err.Error(), "ONE_PER_CLICK") {
		t.Fatalf("error = %q, want actionable supported-counting guidance", err)
	}
	if _, ok, stateErr := st.Identity(addr); stateErr != nil {
		t.Fatalf("state identity: %v", stateErr)
	} else if ok {
		t.Fatal("unsupported counting type import persisted identity")
	}
	if len(fake.mutates) != 0 {
		t.Fatalf("unsupported counting type import mutated remote: %v", fake.mutates)
	}
	assertNoProviderSecret(t, err.Error())
}

var _ = googleads.AttrCount
