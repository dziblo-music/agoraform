package manifest_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/manifest"
	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/provider/fake"
	"github.com/dziblo-music/agoraform/internal/resource"
)

var errConnectionFailed = errors.New("connection failed")

type checkingProvider struct {
	*fake.Provider
	checks int
	err    error
}

func (p *checkingProvider) CheckConnection(context.Context) error {
	p.checks++
	return p.err
}

func TestParseValidManifest(t *testing.T) {
	t.Parallel()

	m := loadTestdata(t, "valid.yaml")
	if m.APIVersion != manifest.APIVersion {
		t.Fatalf("APIVersion = %q, want %s", m.APIVersion, manifest.APIVersion)
	}
	if len(m.Resources) != 2 {
		t.Fatalf("resources = %d, want 2", len(m.Resources))
	}

	first := m.Resources[0]
	if first.Address.String() != "fake.widget.homepage" {
		t.Fatalf("first address = %s, want fake.widget.homepage", first.Address)
	}
	if first.Attributes["title"] != "Homepage banner" {
		t.Fatalf("title = %v", first.Attributes["title"])
	}
	if first.Attributes["color"] != "blue" {
		t.Fatalf("color = %v", first.Attributes["color"])
	}
	if _, ok := first.Attributes[fake.AttrSerial]; ok {
		t.Fatal("desired resource must not include computed serial")
	}
}

func TestParseInvalidManifests(t *testing.T) {
	t.Parallel()

	cases := []struct {
		file    string
		wantSub string
	}{
		{file: "malformed.yaml", wantSub: "malformed YAML"},
		{file: "unsupported-version.yaml", wantSub: "unsupported apiVersion"},
		{file: "missing-apiversion.yaml", wantSub: "apiVersion is required"},
		{file: "duplicate-address.yaml", wantSub: "duplicate resource address"},
		{file: "invalid-address.yaml", wantSub: "provider.type.name"},
		{file: "missing-address.yaml", wantSub: "address is required"},
		{file: "missing-reference.yaml", wantSub: "unknown resource"},
		{file: "self-reference.yaml", wantSub: "references itself"},
		{file: "cycle.yaml", wantSub: "cyclic dependency"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()
			_, err := manifest.Parse(readTestdata(t, tc.file), tc.file)
			if err == nil {
				t.Fatal("Parse succeeded, want error")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error %q, want substring %q", err, tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.file) {
				t.Fatalf("error %q, want origin %q", err, tc.file)
			}
		})
	}
}

func TestParseEmptyManifest(t *testing.T) {
	t.Parallel()

	_, err := manifest.Parse([]byte("   \n\t"), "empty.yaml")
	if err == nil {
		t.Fatal("Parse empty succeeded, want error")
	}
	if !strings.Contains(err.Error(), "manifest is empty") {
		t.Fatalf("error = %q", err)
	}
}

func TestLoadFile(t *testing.T) {
	t.Parallel()

	path := testdataPath(t, "valid.yaml")
	m, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if m.Origin != path {
		t.Fatalf("Origin = %q, want %q", m.Origin, path)
	}

	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	if _, err := manifest.LoadFile(missing); err == nil {
		t.Fatal("LoadFile missing path succeeded")
	}
}

func TestCheckProviders(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	valid := loadTestdata(t, "valid.yaml")

	if err := manifest.CheckProviders(ctx, valid, nil); err != nil {
		t.Fatalf("nil registry: %v", err)
	}
	if err := manifest.CheckProviders(ctx, valid, provider.NewRegistry()); err != nil {
		t.Fatalf("empty registry: %v", err)
	}

	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}
	if err := manifest.CheckProviders(ctx, valid, reg); err != nil {
		t.Fatalf("registered fake provider: %v", err)
	}

	unknown := loadTestdata(t, "unknown-provider.yaml")
	if err := manifest.CheckProviders(ctx, unknown, nil); err != nil {
		t.Fatalf("unknown provider with nil registry should be skipped: %v", err)
	}
	err := manifest.CheckProviders(ctx, unknown, reg)
	if err == nil {
		t.Fatal("unknown provider with registry succeeded")
	}
	if !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("error = %q, want unknown provider", err)
	}

	missingAttr := loadTestdata(t, "missing-required-attribute.yaml")
	err = manifest.CheckProviders(ctx, missingAttr, reg)
	if err == nil {
		t.Fatal("missing required attribute succeeded")
	}
	if !strings.Contains(err.Error(), "title") {
		t.Fatalf("error = %q, want title", err)
	}

	unknownType := &manifest.Manifest{
		Origin:     "test",
		APIVersion: manifest.APIVersion,
		Resources: []resource.Resource{{
			Address:    mustAddress(t, "fake.goal.trial_started"),
			Attributes: resource.Attributes{"name": "Trial Started"},
		}},
	}
	err = manifest.CheckProviders(ctx, unknownType, reg)
	if err == nil {
		t.Fatal("unknown type succeeded")
	}
	if !strings.Contains(err.Error(), "unknown resource type") {
		t.Fatalf("error = %q, want unknown resource type", err)
	}
}

func TestCheckProvidersConnectionChecker(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	valid := loadTestdata(t, "valid.yaml")

	ok := &checkingProvider{Provider: fake.New()}
	reg := provider.NewRegistry()
	if err := reg.Register(ok); err != nil {
		t.Fatal(err)
	}
	if err := manifest.CheckProviders(ctx, valid, reg); err != nil {
		t.Fatalf("successful connection check: %v", err)
	}
	if ok.checks != 1 {
		t.Fatalf("checks = %d, want 1", ok.checks)
	}

	failing := &checkingProvider{Provider: fake.New(), err: errConnectionFailed}
	failReg := provider.NewRegistry()
	if err := failReg.Register(failing); err != nil {
		t.Fatal(err)
	}
	err := manifest.CheckProviders(ctx, valid, failReg)
	if err == nil {
		t.Fatal("failed connection check succeeded")
	}
	if !strings.Contains(err.Error(), "connection failed") {
		t.Fatalf("error = %q, want connection failed", err)
	}
	if failing.checks != 1 {
		t.Fatalf("failing checks = %d, want 1", failing.checks)
	}
}

func TestExampleManifestParses(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(thisFile), "..", "..", "examples", "agoraform.yaml")
	m, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("example manifest: %v", err)
	}
	if len(m.Resources) != 1 {
		t.Fatalf("example resources = %d, want 1", len(m.Resources))
	}
	if m.Resources[0].Address.String() != "matomo.goal.trial_started" {
		t.Fatalf("example address = %s", m.Resources[0].Address)
	}
}

func TestParseResourceReference(t *testing.T) {
	t.Parallel()

	m := loadTestdata(t, "with-reference.yaml")
	if len(m.Resources) != 2 {
		t.Fatalf("resources = %d, want 2", len(m.Resources))
	}
	child := m.Resources[1]
	if child.Address.String() != "fake.widget.banner" {
		t.Fatalf("child address = %s", child.Address)
	}
	ref, ok := resource.AsRef(child.Attributes["parent"])
	if !ok {
		t.Fatalf("parent type = %T, want resource.Ref", child.Attributes["parent"])
	}
	if ref.String() != "fake.widget.homepage" {
		t.Fatalf("parent = %s, want fake.widget.homepage", ref)
	}
}

func TestParseNestedReferences(t *testing.T) {
	t.Parallel()

	m := loadTestdata(t, "nested-references.yaml")
	child := m.Resources[2]
	meta, ok := child.Attributes["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta type = %T", child.Attributes["meta"])
	}
	primary, ok := resource.AsRef(meta["primary"])
	if !ok || primary.String() != "fake.widget.left" {
		t.Fatalf("meta.primary = %v (%T)", meta["primary"], meta["primary"])
	}
	extras, ok := meta["extras"].([]any)
	if !ok || len(extras) != 2 {
		t.Fatalf("meta.extras = %v", meta["extras"])
	}
	if extras[0] != "plain-string" {
		t.Fatalf("extras[0] = %v, want plain string", extras[0])
	}
	extraRef, ok := resource.AsRef(extras[1])
	if !ok || extraRef.String() != "fake.widget.right" {
		t.Fatalf("extras[1] = %v (%T)", extras[1], extras[1])
	}
}

func TestParseTransitiveReferences(t *testing.T) {
	t.Parallel()

	m := loadTestdata(t, "transitive-references.yaml")
	if len(m.Resources) != 3 {
		t.Fatalf("resources = %d, want 3", len(m.Resources))
	}
}

func TestCheckProvidersWithReference(t *testing.T) {
	t.Parallel()

	m := loadTestdata(t, "with-reference.yaml")
	reg := provider.NewRegistry()
	if err := reg.Register(fake.New()); err != nil {
		t.Fatal(err)
	}
	if err := manifest.CheckProviders(context.Background(), m, reg); err != nil {
		t.Fatalf("referenced widgets: %v", err)
	}
}

func TestParseV01ManifestWithoutReferencesRemainsValid(t *testing.T) {
	t.Parallel()

	m := loadTestdata(t, "valid.yaml")
	for _, res := range m.Resources {
		resource.WalkRefs(res.Attributes, func(path string, addr resource.Address) {
			t.Fatalf("%s: unexpected reference at %s -> %s", res.Address, path, addr)
		})
	}
}

func TestParseAddressLookingStringRemainsLiteral(t *testing.T) {
	t.Parallel()

	src := []byte(`
apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: Homepage
      pattern: checkout.step.complete
`)
	m, err := manifest.Parse(src, "literal.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := m.Resources[0].Attributes["pattern"]
	if got != "checkout.step.complete" {
		t.Fatalf("pattern = %#v (%T), want literal string", got, got)
	}
	resource.WalkRefs(m.Resources[0].Attributes, func(path string, addr resource.Address) {
		t.Fatalf("unexpected reference at %s -> %s", path, addr)
	})
}

func TestParseRejectsMalformedExplicitReference(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "non-string",
			src: `
apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      parent:
        $ref: 42
`,
			want: "$ref must be a string",
		},
		{
			name: "extra key",
			src: `
apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      parent:
        $ref: fake.widget.other
        note: not-allowed
`,
			want: "must contain only $ref",
		},
		{
			name: "invalid address",
			src: `
apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      parent:
        $ref: not-an-address
`,
			want: "provider.type.name",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := manifest.Parse([]byte(tc.src), tc.name+".yaml")
			if err == nil {
				t.Fatal("Parse succeeded, want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q, want substring %q", err, tc.want)
			}
		})
	}
}

func TestParseNestedAttributes(t *testing.T) {
	t.Parallel()

	src := []byte(`
apiVersion: agoraform.io/v1alpha1
resources:
  - address: fake.widget.homepage
    attributes:
      title: Nested
      meta:
        enabled: true
        tags:
          - a
          - b
`)
	m, err := manifest.Parse(src, "nested.yaml")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	meta, ok := m.Resources[0].Attributes["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta type = %T", m.Resources[0].Attributes["meta"])
	}
	if meta["enabled"] != true {
		t.Fatalf("meta.enabled = %v", meta["enabled"])
	}
}

func loadTestdata(t *testing.T, name string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.Parse(readTestdata(t, name), name)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return m
}

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(testdataPath(t, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func testdataPath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "testdata", name)
}

func mustAddress(t *testing.T, s string) resource.Address {
	t.Helper()
	addr, err := resource.ParseAddress(s)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}
