package matomo_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestUpdateTagPreservesDescriptionAndBlockTriggers(t *testing.T) {
	t.Parallel()

	srv := newTagServer(t)
	srv.seedTrigger(apiTagTrigger{ID: 4, Name: "trialStarted", Event: "trialStarted"})
	srv.seedTag(apiTag{
		ID:            8,
		Name:          "trialStarted",
		Type:          "Matomo",
		FireTriggerID: 4,
		Category:      "old",
		Action:        "trialStarted",
		Description:   "keep me",
		BlockIDs:      []int{2},
	})
	p := testTagProvider(t, srv)

	desired := tagResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:          "matomoAnalytics",
		matomo.AttrTrigger:       resolvedTrigger(t, "trial_started", "4"),
		matomo.AttrEventCategory: "signup",
		matomo.AttrEventAction:   "trialStarted",
	})
	if _, err := p.Update(context.Background(), desired, resource.RemoteResource{
		Address:  desired.Address,
		Identity: resource.Identity{ID: "8"},
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	if got := srv.lastUpdateValues().Get("description"); got != "keep me" {
		t.Fatalf("description = %q, want preserved value", got)
	}
	if got := srv.lastUpdateValues().Get("blockTriggerIds[0]"); got != "2" {
		t.Fatalf("blockTriggerIds = %q, want preserved value", got)
	}
}

func TestTagConfigErrorUsesTagManagerResourceWording(t *testing.T) {
	t.Parallel()

	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:   "https://matomo.example.com",
		TokenAuth: "token",
		SiteID:    "3",
	}, http.DefaultClient)

	err := p.Validate(context.Background(), tagResource(t, "trial_started", validTagAttrs(t)))
	if err == nil {
		t.Fatal("expected missing container configuration error")
	}
	if !strings.Contains(err.Error(), "Tag Manager resources") {
		t.Fatalf("error = %q, want generic Tag Manager resource wording", err)
	}
}
