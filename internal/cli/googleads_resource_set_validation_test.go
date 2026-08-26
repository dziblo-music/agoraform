package cli_test

import (
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
	"github.com/dziblo-music/agoraform/internal/provider"
)

const googleAdsMismatchedGoalReferenceManifest = `apiVersion: agoraform.io/v1alpha1
resources:
  - address: googleads.conversion_action.purchase
    attributes:
      name: Purchase
      category: PURCHASE
  - address: googleads.customer_conversion_goal.signup
    attributes:
      category: SIGNUP
      origin: WEBSITE
      biddable: true
      conversionAction:
        $ref: googleads.conversion_action.purchase
`

func TestValidateGoogleAdsCustomerGoalReferenceCategoryMismatch(t *testing.T) {
	p, _ := googleAdsTestProvider(t)
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		t.Fatal(err)
	}

	path := writeManifest(t, "agoraform.yaml", googleAdsMismatchedGoalReferenceManifest)
	streams, _, stderr := testStreams()
	code := cli.ExecuteWithRegistry(streams, []string{"validate", "-f", path}, reg)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d", code, cli.ExitError)
	}
	errOut := stderr.String()
	if !strings.Contains(errOut, "PURCHASE") || !strings.Contains(errOut, "SIGNUP") || !strings.Contains(errOut, "must match") {
		t.Fatalf("stderr = %q, want actionable category mismatch", errOut)
	}
}
