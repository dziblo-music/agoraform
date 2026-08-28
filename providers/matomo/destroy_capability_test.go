package matomo

import (
	"testing"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestDestroyCapabilityDistinguishesGoalRemoval(t *testing.T) {
	t.Parallel()

	p := New(Config{})
	tests := []struct {
		name string
		typ  string
		want provider.DestroyCapability
	}{
		{name: "goal", typ: TypeGoal, want: provider.DestroyRemove},
		{name: "container", typ: TypeContainer, want: provider.DestroyDelete},
		{name: "variable", typ: TypeVariable, want: provider.DestroyDelete},
		{name: "trigger", typ: TypeTrigger, want: provider.DestroyDelete},
		{name: "tag", typ: TypeTag, want: provider.DestroyDelete},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := p.DestroyCapability(resource.Resource{Address: resource.Address{
				Provider: Name,
				Type:     tc.typ,
				Name:     "example",
			}})
			if err != nil {
				t.Fatalf("DestroyCapability: %v", err)
			}
			if got != tc.want {
				t.Fatalf("capability = %q, want %q", got, tc.want)
			}
		})
	}
}
