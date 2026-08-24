package plan_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/plan"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

type identityIgnoringReader struct {
	returned resource.Identity
}

func (r identityIgnoringReader) Name() string                                      { return "fake" }
func (r identityIgnoringReader) ResourceTypes() []string                           { return []string{"widget"} }
func (r identityIgnoringReader) Validate(context.Context, resource.Resource) error { return nil }
func (r identityIgnoringReader) Read(_ context.Context, res resource.Resource) (resource.RemoteResource, error) {
	return resource.RemoteResource{
		Address:    res.Address,
		Identity:   r.returned,
		Attributes: res.Attributes.Clone(),
	}, nil
}

func TestBuildWithStateRejectsProviderIdentityMismatch(t *testing.T) {
	t.Parallel()

	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	st := mustStore(t)
	if err := st.Bind(res.Address, resource.Identity{ID: "expected-id"}); err != nil {
		t.Fatal(err)
	}

	_, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return identityIgnoringReader{returned: resource.Identity{ID: "different-id"}}, nil
	}, st)
	if err == nil {
		t.Fatal("BuildWithState succeeded, want identity mismatch error")
	}
	if !strings.Contains(err.Error(), "refusing to rebind") || !strings.Contains(err.Error(), "expected-id") || !strings.Contains(err.Error(), "different-id") {
		t.Fatalf("identity mismatch error = %q", err)
	}
}

func TestBuildWithStateRejectsMissingProviderIdentity(t *testing.T) {
	t.Parallel()

	res := widget(t, "homepage", resource.Attributes{fake.AttrTitle: "Homepage"})
	st := mustStore(t)
	if err := st.Bind(res.Address, resource.Identity{ID: "expected-id"}); err != nil {
		t.Fatal(err)
	}

	_, err := plan.BuildWithState(context.Background(), []resource.Resource{res}, func(resource.Address) (provider.Reader, error) {
		return identityIgnoringReader{}, nil
	}, st)
	if err == nil {
		t.Fatal("BuildWithState succeeded, want missing identity error")
	}
	if !strings.Contains(err.Error(), "returned no identity") || !strings.Contains(err.Error(), "refusing to rebind") {
		t.Fatalf("missing identity error = %q", err)
	}
}
