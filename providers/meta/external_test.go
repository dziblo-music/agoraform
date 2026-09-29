package meta_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/meta"
)

func TestExternalCampaignReadDoesNotMutate(t *testing.T) {
	t.Parallel()

	srv := newGraphServer(t)
	srv.seedCampaign(testCampaignID, graphObject{
		"name":      "Acquisition",
		"objective": "OUTCOME_TRAFFIC",
	})
	p := testProvider(t, srv.start())
	if !p.SupportsExternal(meta.TypeCampaign) || p.SupportsExternal(meta.TypePixel) {
		t.Fatalf("SupportsExternal campaign=%v pixel=%v", p.SupportsExternal(meta.TypeCampaign), p.SupportsExternal(meta.TypePixel))
	}

	addr := mustMetaAddress(t, "meta.campaign.acquisition")
	live, err := p.ReadExternal(context.Background(), addr, testCampaignID)
	if err != nil {
		t.Fatalf("ReadExternal: %v", err)
	}
	if live.Identity.ID != testCampaignID {
		t.Fatalf("identity = %s", live.Identity.ID)
	}

	res := resource.Resource{Address: addr, Ownership: resource.OwnershipExternal, ExternalID: testCampaignID}
	planned, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, nil)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if planned.HasChanges() || planned.ExternalCount() != 1 || planned.Changes[0].Identity.ID != testCampaignID {
		t.Fatalf("plan = %+v", planned.Changes)
	}
	posts, deletes := srv.mutationCounts()
	if posts != 0 || deletes != 0 {
		t.Fatalf("mutations posts=%d deletes=%d", posts, deletes)
	}
}

func TestExternalCampaignMissingAndUnsupported(t *testing.T) {
	t.Parallel()

	srv := newGraphServer(t)
	p := testProvider(t, srv.start())
	addr := mustMetaAddress(t, "meta.campaign.acquisition")
	_, err := p.ReadExternal(context.Background(), addr, testCampaignID)
	if err == nil || !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("missing error = %v", err)
	}

	pixel := mustMetaAddress(t, "meta.pixel.website")
	_, err = p.ReadExternal(context.Background(), pixel, testPixelID)
	if err == nil || !strings.Contains(err.Error(), "does not support external ownership") {
		t.Fatalf("pixel error = %v", err)
	}
	posts, deletes := srv.mutationCounts()
	if posts != 0 || deletes != 0 {
		t.Fatalf("mutations posts=%d deletes=%d", posts, deletes)
	}
}

func mustMetaAddress(t *testing.T, raw string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress(raw)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}
