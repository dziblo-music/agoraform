package meta_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/meta"
)

func TestDestroyAdSetIgnoresUnrepresentableProviderFields(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedAdSet(testAdSetID, graphObject{
		"targeting": graphObject{
			"geo_locations":          graphObject{"countries": []string{"US"}},
			"user_age_unknown":       true,
			"future_provider_option": graphObject{"enabled": true},
		},
		"promoted_object": "not-an-object",
	})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	res := adSetResource(t, "instagram", standardAdSetAttrs(t))
	res.Identity = resource.Identity{ID: testAdSetID}

	got, err := p.Destroy(context.Background(), res)
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if got.Status != provider.DestroyStatusRemoved {
		t.Fatalf("status = %q, want removed", got.Status)
	}
	if _, deletes := srv.mutationCounts(); deletes != 1 {
		t.Fatalf("deletes = %d, want 1", deletes)
	}
}

func TestDestroyRechecksObjectStillReadableAfterDelete(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedCreative(testCreativeID, nil)
	srv.staleCreativeReads = 2
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	var slept []time.Duration
	meta.SetDestroyConfirmForTest(p, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}, func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	})
	res := creativeResource(t, "instagram", standardImageCreativeAttrs())
	res.Identity = resource.Identity{ID: testCreativeID}

	got, err := p.Destroy(context.Background(), res)
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if got.Status != provider.DestroyStatusDestroyed {
		t.Fatalf("status = %q, want destroyed", got.Status)
	}
	if len(slept) != 2 {
		t.Fatalf("waited %v, want two re-checks before the delete was visible", slept)
	}
	if _, deletes := srv.mutationCounts(); deletes != 1 {
		t.Fatalf("deletes = %d, want exactly one DELETE", deletes)
	}
}

func TestDestroyReportsUncertainDeleteAfterRechecks(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedCreative(testCreativeID, nil)
	srv.staleCreativeReads = 100
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	var slept []time.Duration
	meta.SetDestroyConfirmForTest(p, []time.Duration{time.Millisecond, time.Millisecond}, func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	})
	res := creativeResource(t, "instagram", standardImageCreativeAttrs())
	res.Identity = resource.Identity{ID: testCreativeID}

	_, err := p.Destroy(context.Background(), res)
	if err == nil {
		t.Fatal("Destroy succeeded, want uncertain-outcome error")
	}
	for _, want := range []string{"DELETE was accepted", "still readable after 3 checks", "rerun agoraform destroy"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err, want)
		}
	}
	if len(slept) != 2 {
		t.Fatalf("waited %v, want every configured re-check", slept)
	}
	if _, deletes := srv.mutationCounts(); deletes != 1 {
		t.Fatalf("deletes = %d, want a single DELETE", deletes)
	}
}

func TestDestroyRefusesObjectFromAnotherAdAccount(t *testing.T) {
	t.Parallel()
	srv := newGraphServer(t)
	srv.seedCampaign(testCampaignID, graphObject{"account_id": "999999999999999"})
	httpSrv := srv.start()
	defer httpSrv.Close()
	p := testProvider(t, httpSrv)
	res := campaignResource(t, "acquisition", standardCampaignAttrs())
	res.Identity = resource.Identity{ID: testCampaignID}

	_, err := p.Destroy(context.Background(), res)
	if err == nil || !strings.Contains(err.Error(), "not configured account") {
		t.Fatalf("Destroy = %v, want wrong-account refusal", err)
	}
	if _, deletes := srv.mutationCounts(); deletes != 0 {
		t.Fatalf("deletes = %d, want 0", deletes)
	}
}
