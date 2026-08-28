package matomo_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
	"github.com/dziblo-music/agoraform/internal/state"
	"github.com/dziblo-music/agoraform/providers/matomo"
	"github.com/dziblo-music/agoraform/providers/matomo/client"
)

func TestPlanContainerRejectsDiscoveredImmutableContextMismatch(t *testing.T) {
	t.Parallel()

	srv := newContainerServer(t)
	srv.seed(apiContainer{
		ID:      "Aa000001",
		Name:    "Main Website",
		Context: "android",
	})
	p := testContainerProvider(t, srv)
	res := containerResource(t, "main", resource.Attributes{
		matomo.AttrName:    "Main Website",
		matomo.AttrContext: "web",
	})

	_, err := plan.Build(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return p, nil
	})
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("plan.Build = %v, want immutable context error", err)
	}
	if srv.updateCount() != 0 {
		t.Fatalf("plan mutated remote container: updates=%d", srv.updateCount())
	}
}

func TestImportVariableExternalModeIgnoresStaleManagedContainerState(t *testing.T) {
	t.Parallel()

	const externalID = "Bb000002"

	cases := []struct {
		name    string
		staleID string
	}{
		{name: "different stale id", staleID: "Aa000001"},
		{name: "matching stale id", staleID: externalID},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := newContainerServer(t)
			srv.seed(apiContainer{ID: externalID, Name: "External Website", Context: "web"})
			srv.seedVariable(externalID, apiContainerVariable{ID: 2, Name: "userId", Type: "DataLayer", Key: "userId"})

			p := matomo.NewWithHTTPClient(client.Config{
				BaseURL:     srv.server.URL,
				TokenAuth:   "test-token",
				SiteID:      "3",
				ContainerID: externalID,
				HTTPClient:  srv.server.Client(),
			}, srv.server.Client())

			st, err := state.New(filepath.Join(t.TempDir(), state.DefaultFilename))
			if err != nil {
				t.Fatal(err)
			}
			containerAddr, err := resource.ParseAddress("matomo.container.main")
			if err != nil {
				t.Fatal(err)
			}
			if err := st.Bind(containerAddr, resource.Identity{ID: tc.staleID}); err != nil {
				t.Fatal(err)
			}
			p.SetIdentityCatalog(st)

			variableAddr, err := resource.ParseAddress("matomo.variable.user_id")
			if err != nil {
				t.Fatal(err)
			}
			live, err := p.Import(context.Background(), variableAddr, "2")
			if err != nil {
				t.Fatalf("Import: %v", err)
			}
			if _, ok := live.Attributes[matomo.AttrContainer]; ok {
				t.Fatalf("external import reconstructed stale managed container ref: %#v", live.Attributes[matomo.AttrContainer])
			}
			if got := srv.lastVariableReadContainer(); got != externalID {
				t.Fatalf("import read container %q, want explicit external container %q", got, externalID)
			}
		})
	}
}
