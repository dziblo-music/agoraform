package destroy

import (
	"testing"

	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestResolveDesiredPreservesUnmanagedCrossProviderOutputRefs(t *testing.T) {
	t.Parallel()

	tagAddr := resource.Address{Provider: "matomo", Type: "tag", Name: "google_ads_trial_started"}
	conversionAddr := resource.Address{Provider: "googleads", Type: "conversion_action", Name: "trial_started"}
	triggerAddr := resource.Address{Provider: "matomo", Type: "trigger", Name: "trial_started"}
	containerAddr := resource.Address{Provider: "matomo", Type: "container", Name: "main"}

	conversionIDRef := resource.Ref{Address: conversionAddr, Output: "conversionId"}
	conversionLabelRef := resource.Ref{Address: conversionAddr, Output: "conversionLabel"}
	res := resource.Resource{
		Address: tagAddr,
		Attributes: resource.Attributes{
			"type":            "googleAdsConversion",
			"conversionId":    conversionIDRef,
			"conversionLabel": conversionLabelRef,
			"trigger":         resource.Ref{Address: triggerAddr},
			"container":       resource.Ref{Address: containerAddr},
		},
	}

	runtime := map[string]resource.Resolved{
		triggerAddr.String(): {
			Address:  triggerAddr,
			Identity: resource.Identity{ID: "trigger-26"},
		},
		containerAddr.String(): {
			Address:  containerAddr,
			Identity: resource.Identity{ID: "container-main"},
		},
	}

	got, err := resolveDesired(res, runtime)
	if err != nil {
		t.Fatalf("resolveDesired: %v", err)
	}

	gotConversionID, ok := resource.AsRef(got.Attributes["conversionId"])
	if !ok || gotConversionID != conversionIDRef {
		t.Fatalf("conversionId = %#v, want preserved ref %#v", got.Attributes["conversionId"], conversionIDRef)
	}
	gotConversionLabel, ok := resource.AsRef(got.Attributes["conversionLabel"])
	if !ok || gotConversionLabel != conversionLabelRef {
		t.Fatalf("conversionLabel = %#v, want preserved ref %#v", got.Attributes["conversionLabel"], conversionLabelRef)
	}

	gotTrigger, ok := resource.AsResolved(got.Attributes["trigger"])
	if !ok || gotTrigger.Address != triggerAddr || gotTrigger.Identity.ID != "trigger-26" {
		t.Fatalf("trigger = %#v, want resolved trigger identity", got.Attributes["trigger"])
	}
	gotContainer, ok := resource.AsResolved(got.Attributes["container"])
	if !ok || gotContainer.Address != containerAddr || gotContainer.Identity.ID != "container-main" {
		t.Fatalf("container = %#v, want resolved container identity", got.Attributes["container"])
	}
}
