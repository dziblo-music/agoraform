package matomo_test

import (
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/providers/matomo"
)

func TestPlanMatomoConfigurationUserIDMismatchRedactsRemoteValue(t *testing.T) {
	t.Parallel()

	const sensitiveUserID = "alice@example.com"

	srv := newVariableServer(t)
	srv.seed(apiVariable{ID: 2, Name: "User ID", Type: "DataLayer", Key: "userId"})
	srv.seed(apiVariable{
		ID:   20,
		Name: "Matomo Configuration",
		Type: "MatomoConfiguration",
		Parameters: map[string]any{
			"matomoUrl": "https://matomo.example.com",
			"idSite":    "1",
			"userId":    sensitiveUserID,
		},
	})

	p := testVariableProvider(t, srv)
	got := mustPlanVariables(t, p,
		variableResource(t, "user_id", userIDDataLayerAttrs()),
		variableResource(t, "config", configVariableAttrs(resource.Attributes{
			matomo.AttrUserID: variableRef(t, "user_id"),
		})),
	)

	change := changeByAddr(t, got, "matomo.variable.config")
	if change.Action != plan.ActionUpdate {
		t.Fatalf("config change = %+v, want update", change)
	}

	output := plan.Format(got)
	if strings.Contains(output, sensitiveUserID) {
		t.Fatalf("plan output leaked remote userId: %s", output)
	}
	if !strings.Contains(output, "matomo.variable.user_id") {
		t.Fatalf("plan output omitted desired userId reference: %s", output)
	}
	if !strings.Contains(output, matomo.AttrUserID) {
		t.Fatalf("plan output omitted userId drift: %s", output)
	}
}
