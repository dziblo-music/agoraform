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

func TestUpdateTriggerPreservesDescription(t *testing.T) {
	t.Parallel()

	srv := newTriggerServer(t)
	srv.seed(apiTrigger{
		ID:          8,
		Name:        "trialStarted",
		Type:        "CustomEvent",
		Event:       "oldEvent",
		Description: "keep me",
	})
	p := testTriggerProvider(t, srv)

	desired := triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
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
}

func TestTriggerConfigErrorUsesTagManagerResourceWording(t *testing.T) {
	t.Parallel()

	p := matomo.NewWithHTTPClient(client.Config{
		BaseURL:   "https://matomo.example.com",
		TokenAuth: "token",
		SiteID:    "3",
	}, http.DefaultClient)

	err := p.Validate(context.Background(), triggerResource(t, "trial_started", resource.Attributes{
		matomo.AttrType:  "customEvent",
		matomo.AttrEvent: "trialStarted",
	}))
	if err == nil {
		t.Fatal("expected missing container configuration error")
	}
	if !strings.Contains(err.Error(), "Tag Manager resources") {
		t.Fatalf("error = %q, want generic Tag Manager resource wording", err)
	}
	if strings.Contains(err.Error(), "Tag Manager variables") {
		t.Fatalf("error = %q, must not describe trigger configuration as variable-only", err)
	}
}
