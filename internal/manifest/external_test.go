package manifest_test

import (
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestParseExternalLifecycle(t *testing.T) {
	t.Parallel()

	got, err := manifest.Parse([]byte(`
apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.container.main
    lifecycle:
      ownership: external
      id: 123
  - address: matomo.trigger.trial_started
    attributes:
      container:
        $ref: matomo.container.main
      type: customEvent
`), "external.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Resources) != 2 {
		t.Fatalf("resources = %d", len(got.Resources))
	}
	container := got.Resources[0]
	if !container.IsExternal() || container.ExternalID != "123" {
		t.Fatalf("container = %+v", container)
	}
	if got.Resources[1].IsExternal() {
		t.Fatal("trigger should stay managed")
	}
}

func TestParseExternalLifecycleRejectsAttributesAndUnknownFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "attributes",
			yaml: `
apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.container.main
    lifecycle:
      ownership: external
      id: Aa000001
    attributes:
      name: Main
`,
			want: "cannot declare attributes",
		},
		{
			name: "unknown field",
			yaml: `
apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.container.main
    lifecycle:
      ownership: external
      mode: reference
`,
			want: "unknown lifecycle field",
		},
		{
			name: "id on managed",
			yaml: `
apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.container.main
    lifecycle:
      ownership: managed
      id: Aa000001
`,
			want: "lifecycle.id is only valid",
		},
		{
			name: "bad ownership",
			yaml: `
apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.container.main
    lifecycle:
      ownership: shared
`,
			want: "ownership",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := manifest.Parse([]byte(tc.yaml), "external.yaml")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParseManagedResourceOmitsLifecycle(t *testing.T) {
	t.Parallel()

	got, err := manifest.Parse([]byte(`
apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: Homepage
`), "managed.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got.Resources[0].Ownership != resource.OwnershipManaged || got.Resources[0].ExternalID != "" {
		t.Fatalf("resource = %+v", got.Resources[0])
	}
}
