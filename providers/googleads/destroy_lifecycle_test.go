package googleads

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestDestroyLifecycleMatrixCoversResourceTypes(t *testing.T) {
	t.Parallel()

	p := New(Config{})
	registered := map[string]struct{}{}
	for _, typ := range p.ResourceTypes() {
		registered[typ] = struct{}{}
		spec, ok := destroyLifecycleByType[typ]
		if !ok {
			t.Errorf("resource type %s lacks an explicit destroy lifecycle declaration", typ)
			continue
		}
		switch spec.Capability {
		case provider.DestroyRemove:
			if spec.Collection == "" {
				t.Errorf("%s remove lifecycle is missing the Google Ads mutate collection", typ)
			}
			if spec.MutateOperation != "remove" {
				t.Errorf("%s mutate operation = %q, want remove", typ, spec.MutateOperation)
			}
			if spec.TerminalState != "status=REMOVED" {
				t.Errorf("%s terminal state = %q, want status=REMOVED", typ, spec.TerminalState)
			}
			if !strings.Contains(spec.AlreadyTerminal, "REMOVED") || !strings.Contains(spec.AlreadyTerminal, "not found") {
				t.Errorf("%s already-terminal detection = %q, want REMOVED or not found", typ, spec.AlreadyTerminal)
			}
			if spec.Precondition == "" {
				t.Errorf("%s remove lifecycle missing serving/spend precondition", typ)
			}
		case provider.DestroyProviderOwned:
			if spec.Collection != "" || spec.MutateOperation != "" {
				t.Errorf("%s provider-owned lifecycle must not declare a mutate operation", typ)
			}
			if spec.Precondition == "" {
				t.Errorf("%s provider-owned lifecycle missing guidance", typ)
			}
		default:
			t.Errorf("%s capability %q is not remove or provider-owned", typ, spec.Capability)
		}
	}
	for typ := range destroyLifecycleByType {
		if _, ok := registered[typ]; !ok {
			t.Errorf("lifecycle declares %s which is not registered in ResourceTypes()", typ)
		}
	}
	if len(destroyLifecycleByType) != len(p.ResourceTypes()) {
		t.Errorf("lifecycle table has %d entries, ResourceTypes has %d", len(destroyLifecycleByType), len(p.ResourceTypes()))
	}
}

func TestDestroyCapabilityMatchesLifecycleMatrix(t *testing.T) {
	t.Parallel()

	p := New(Config{})
	for _, typ := range p.ResourceTypes() {
		typ := typ
		t.Run(typ, func(t *testing.T) {
			t.Parallel()
			got, err := p.DestroyCapability(resource.Resource{Address: resource.Address{
				Provider: Name,
				Type:     typ,
				Name:     "example",
			}})
			if err != nil {
				t.Fatalf("DestroyCapability: %v", err)
			}
			want := destroyLifecycleByType[typ].Capability
			if got != want {
				t.Fatalf("capability = %q, want %q", got, want)
			}
		})
	}

	got, err := p.DestroyCapability(resource.Resource{Address: resource.Address{
		Provider: Name,
		Type:     "ad",
		Name:     "brand",
	}})
	if err != nil {
		t.Fatalf("unknown type DestroyCapability: %v", err)
	}
	if got != provider.DestroyUnsupported {
		t.Fatalf("unknown type capability = %q, want unsupported", got)
	}
}

func TestDestroyProviderOwnedRefusesMutation(t *testing.T) {
	t.Parallel()

	p := New(Config{})
	cases := []struct {
		typ       string
		id        string
		wantGuide string
	}{
		{typ: TypeCustomerConversionGoal, id: "SIGNUP~WEBSITE", wantGuide: "cannot be deleted"},
		{typ: TypeCampaignConversionGoal, id: "21~SIGNUP~WEBSITE", wantGuide: "cannot be deleted"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.typ, func(t *testing.T) {
			t.Parallel()
			_, err := p.Destroy(context.Background(), resource.Resource{
				Address:  resource.Address{Provider: Name, Type: tc.typ, Name: "example"},
				Identity: resource.Identity{ID: tc.id},
			})
			if err == nil {
				t.Fatal("Destroy succeeded, want provider-owned refusal")
			}
			if !strings.Contains(err.Error(), tc.wantGuide) {
				t.Fatalf("error = %q, want %q", err, tc.wantGuide)
			}
		})
	}
}
