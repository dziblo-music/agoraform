package fake_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

func TestFakeProviderLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := fake.New()
	res := resource.Resource{
		Address:    mustAddress(t, "fake.widget.homepage"),
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage"},
	}

	if err := p.Validate(ctx, res); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	if _, err := p.Read(ctx, res); !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Read before create = %v, want ErrNotFound", err)
	}

	created, err := p.Create(ctx, res)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Identity.IsZero() {
		t.Fatal("Create returned empty identity")
	}
	if created.Attributes[fake.AttrTitle] != "Homepage" {
		t.Fatalf("Create title = %v, want Homepage", created.Attributes[fake.AttrTitle])
	}
	if _, ok := created.Computed[fake.AttrSerial]; !ok {
		t.Fatal("Create missing computed serial")
	}
	if _, ok := created.Attributes[fake.AttrSerial]; ok {
		t.Fatal("computed serial must not appear in configurable attributes")
	}

	read, err := p.Read(ctx, res)
	if err != nil {
		t.Fatalf("Read after create: %v", err)
	}
	if read.Identity != created.Identity {
		t.Fatalf("Read identity = %+v, want %+v", read.Identity, created.Identity)
	}

	updatedDesired := resource.Resource{
		Address: res.Address,
		Attributes: resource.Attributes{
			fake.AttrTitle: "Homepage",
			fake.AttrColor: "blue",
		},
	}
	updated, err := p.Update(ctx, updatedDesired, read)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Attributes[fake.AttrColor] != "blue" {
		t.Fatalf("Update color = %v, want blue", updated.Attributes[fake.AttrColor])
	}
	if updated.Computed[fake.AttrSerial] == created.Computed[fake.AttrSerial] {
		t.Fatal("Update should refresh computed serial")
	}

	imported, err := p.Import(ctx, res.Address, created.Identity.ID)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if imported.Identity != created.Identity {
		t.Fatalf("Import identity = %+v, want %+v", imported.Identity, created.Identity)
	}

	reads, creates, updates, imports := p.Calls()
	if reads != 2 || creates != 1 || updates != 1 || imports != 1 {
		t.Fatalf("Calls() = %d %d %d %d, want 2 1 1 1", reads, creates, updates, imports)
	}
}

func TestFakeProviderValidate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := fake.New()

	cases := []struct {
		name string
		res  resource.Resource
	}{
		{
			name: "missing title",
			res: resource.Resource{
				Address:    mustAddress(t, "fake.widget.homepage"),
				Attributes: resource.Attributes{},
			},
		},
		{
			name: "computed serial in config",
			res: resource.Resource{
				Address: mustAddress(t, "fake.widget.homepage"),
				Attributes: resource.Attributes{
					fake.AttrTitle:  "Homepage",
					fake.AttrSerial: 1,
				},
			},
		},
		{
			name: "unknown attribute",
			res: resource.Resource{
				Address: mustAddress(t, "fake.widget.homepage"),
				Attributes: resource.Attributes{
					fake.AttrTitle:  "Homepage",
					"unknown_field": "nope",
				},
			},
		},
		{
			name: "parent not a reference",
			res: resource.Resource{
				Address: mustAddress(t, "fake.widget.homepage"),
				Attributes: resource.Attributes{
					fake.AttrTitle:  "Homepage",
					fake.AttrParent: "not-an-address",
				},
			},
		},
		{
			name: "unknown type",
			res: resource.Resource{
				Address:    mustAddress(t, "fake.goal.trial_started"),
				Attributes: resource.Attributes{fake.AttrTitle: "Nope"},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := p.Validate(ctx, tc.res); err == nil {
				t.Fatal("Validate succeeded, want error")
			}
		})
	}
}

func TestFakeProviderAcceptsParentReference(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := fake.New()
	parent := resource.Resource{
		Address:    mustAddress(t, "fake.widget.homepage"),
		Attributes: resource.Attributes{fake.AttrTitle: "Homepage"},
	}
	child := resource.Resource{
		Address: mustAddress(t, "fake.widget.banner"),
		Attributes: resource.Attributes{
			fake.AttrTitle:  "Banner",
			fake.AttrParent: resource.Ref{Address: parent.Address},
		},
	}
	if err := p.Validate(ctx, parent); err != nil {
		t.Fatalf("Validate parent: %v", err)
	}
	if err := p.Validate(ctx, child); err != nil {
		t.Fatalf("Validate child: %v", err)
	}
}

func mustAddress(t *testing.T, s string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress(s)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}
