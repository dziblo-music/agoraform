package googleads_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/googleads"
)

func TestCampaignLocationSpecificIDDoesNotPoisonAmbiguousNameCache(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, newTargetingFake())

	byID := resolvedCampaignLocationAttrs(t, "21")
	byID[googleads.AttrLocation] = "geoTargetConstants/1021001"
	_, err := p.Read(context.Background(), campaignLocationResource(t, "springfield_il", byID))
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("specific Springfield read = %v, want ErrNotFound after successful constant resolution", err)
	}

	byName := resolvedCampaignLocationAttrs(t, "21")
	byName[googleads.AttrLocation] = "Springfield"
	_, err = p.Read(context.Background(), campaignLocationResource(t, "springfield", byName))
	if err == nil {
		t.Fatal("expected ambiguous Springfield error")
	}
	if !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "Springfield") {
		t.Fatalf("error = %q, want ambiguous Springfield guidance", err)
	}
}

func TestCampaignLocationAliasDuplicateRejected(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, newTargetingFake())
	campaign := campaignRef(t, "brand")
	resources := []resource.Resource{
		campaignLocationResource(t, "us_code", resource.Attributes{
			googleads.AttrCampaign: campaign,
			googleads.AttrLocation: "US",
		}),
		campaignLocationResource(t, "us_name", resource.Attributes{
			googleads.AttrCampaign: campaign,
			googleads.AttrLocation: "United States",
		}),
	}

	err := p.ValidateResourceSet(context.Background(), resources)
	if err == nil {
		t.Fatal("expected alias duplicate location error")
	}
	if !strings.Contains(err.Error(), "duplicates") || !strings.Contains(err.Error(), "geoTargetConstants/2840") {
		t.Fatalf("error = %q, want canonical duplicate location diagnostic", err)
	}
}

func TestCampaignLocationResourceNameAliasDuplicateRejected(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, newTargetingFake())
	campaign := campaignRef(t, "brand")
	resources := []resource.Resource{
		campaignLocationResource(t, "us_id", resource.Attributes{
			googleads.AttrCampaign: campaign,
			googleads.AttrLocation: "geoTargetConstants/2840",
		}),
		campaignLocationResource(t, "us_name", resource.Attributes{
			googleads.AttrCampaign: campaign,
			googleads.AttrLocation: "United States",
		}),
	}

	err := p.ValidateResourceSet(context.Background(), resources)
	if err == nil || !strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("error = %v, want duplicate location error", err)
	}
}

func TestCampaignLanguageAliasDuplicateRejected(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, newTargetingFake())
	campaign := campaignRef(t, "brand")
	resources := []resource.Resource{
		campaignLanguageResource(t, "en_code", resource.Attributes{
			googleads.AttrCampaign: campaign,
			googleads.AttrLanguage: "en",
		}),
		campaignLanguageResource(t, "en_name", resource.Attributes{
			googleads.AttrCampaign: campaign,
			googleads.AttrLanguage: "English",
		}),
	}

	err := p.ValidateResourceSet(context.Background(), resources)
	if err == nil {
		t.Fatal("expected alias duplicate language error")
	}
	if !strings.Contains(err.Error(), "duplicates") || !strings.Contains(err.Error(), "languageConstants/1000") {
		t.Fatalf("error = %q, want canonical duplicate language diagnostic", err)
	}
}

func TestCampaignLanguageResourceNameAliasDuplicateRejected(t *testing.T) {
	t.Parallel()

	p, _ := testTargetingProvider(t, newTargetingFake())
	campaign := campaignRef(t, "brand")
	resources := []resource.Resource{
		campaignLanguageResource(t, "en_id", resource.Attributes{
			googleads.AttrCampaign: campaign,
			googleads.AttrLanguage: "languageConstants/1000",
		}),
		campaignLanguageResource(t, "en_code", resource.Attributes{
			googleads.AttrCampaign: campaign,
			googleads.AttrLanguage: "en",
		}),
	}

	err := p.ValidateResourceSet(context.Background(), resources)
	if err == nil || !strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("error = %v, want duplicate language error", err)
	}
}
