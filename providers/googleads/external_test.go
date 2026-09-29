package googleads_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestExternalCampaignReadDoesNotMutate(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	mutates := 0
	body := `{"results":[{"campaign":{"resourceName":"customers/` + testCustomerID + `/campaigns/99","id":"99","name":"Brand","advertisingChannelType":"SEARCH","status":"PAUSED"}}]}`
	p, _ := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/oauth/token") {
			writeToken(w)
			return
		}
		if strings.Contains(r.URL.Path, ":mutate") {
			mu.Lock()
			mutates++
			mu.Unlock()
			http.Error(w, "mutate", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})
	if !p.SupportsExternal(googleads.TypeCampaign) || p.SupportsExternal(googleads.TypeCampaignBudget) {
		t.Fatalf("SupportsExternal campaign=%v budget=%v", p.SupportsExternal(googleads.TypeCampaign), p.SupportsExternal(googleads.TypeCampaignBudget))
	}

	addr := mustGoogleAddress(t, "googleads.campaign.brand")
	live, err := p.ReadExternal(context.Background(), addr, "99")
	if err != nil {
		t.Fatalf("ReadExternal: %v", err)
	}
	if live.Identity.ID != "99" || live.Address != addr {
		t.Fatalf("live = %+v", live)
	}

	res := resource.Resource{Address: addr, Ownership: resource.OwnershipExternal, ExternalID: "99"}
	planned, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	}, nil)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if planned.ExternalCount() != 1 || planned.HasChanges() {
		t.Fatalf("plan = %+v hasChanges=%v", planned.Changes, planned.HasChanges())
	}
	mu.Lock()
	defer mu.Unlock()
	if mutates != 0 {
		t.Fatalf("mutates = %d", mutates)
	}
}

func TestExternalCampaignAmbiguousAndMissing(t *testing.T) {
	t.Parallel()

	ambiguous := `{"results":[
		{"campaign":{"resourceName":"customers/` + testCustomerID + `/campaigns/99","id":"99","name":"One","advertisingChannelType":"SEARCH","status":"PAUSED"}},
		{"campaign":{"resourceName":"customers/` + testCustomerID + `/campaigns/99","id":"99","name":"Two","advertisingChannelType":"SEARCH","status":"PAUSED"}}
	]}`
	p, _ := testProvider(t, googleSearchHandler(t, ambiguous))
	addr := mustGoogleAddress(t, "googleads.campaign.brand")
	_, err := p.ReadExternal(context.Background(), addr, "99")
	if err == nil || !strings.Contains(err.Error(), "multiple remote campaigns") {
		t.Fatalf("ambiguous error = %v", err)
	}

	p, _ = testProvider(t, googleSearchHandler(t, `{"results":[]}`))
	_, err = p.ReadExternal(context.Background(), addr, "99")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing error = %v", err)
	}
}

func TestExternalCampaignRejectsOtherTypes(t *testing.T) {
	t.Parallel()

	p, _ := testProvider(t, nil)
	addr := mustGoogleAddress(t, "googleads.campaign_budget.daily")
	if _, err := p.ReadExternal(context.Background(), addr, "15"); err == nil || !strings.Contains(err.Error(), "does not support external ownership") {
		t.Fatalf("error = %v", err)
	}
}

func googleSearchHandler(t *testing.T, body string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/oauth/token") {
			writeToken(w)
			return
		}
		if strings.Contains(r.URL.Path, ":mutate") {
			t.Errorf("unexpected mutate %s", r.URL.Path)
			http.Error(w, "mutate", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}
}

func mustGoogleAddress(t *testing.T, raw string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress(raw)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}
