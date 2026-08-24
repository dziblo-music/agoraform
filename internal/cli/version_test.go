package cli

import "testing"

func TestNormalizeVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in, want string
	}{
		{in: "0.1.0", want: "0.1.0"},
		{in: "v0.1.0", want: "0.1.0"},
		{in: "  v0.1.0-rc.1  ", want: "0.1.0-rc.1"},
		{in: developmentVersion, want: developmentVersion},
	}
	for _, tt := range tests {
		if got := normalizeVersion(tt.in); got != tt.want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestResolveVersionPrefersInjected(t *testing.T) {
	t.Parallel()

	if got := resolveVersion("v0.1.0"); got != "0.1.0" {
		t.Fatalf("resolveVersion(v0.1.0) = %q, want 0.1.0", got)
	}
	if got := resolveVersion("0.1.0"); got != "0.1.0" {
		t.Fatalf("resolveVersion(0.1.0) = %q, want 0.1.0", got)
	}
}

func TestVersionFromBuildInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in, want string
	}{
		{in: "", want: developmentVersion},
		{in: "(devel)", want: developmentVersion},
		{in: "v0.1.0", want: "0.1.0"},
		{in: "0.1.0", want: "0.1.0"},
		{in: "v0.1.0-rc.1", want: "0.1.0-rc.1"},
		{in: "v0.0.0-20260824165146-82b5fc14a477+dirty", want: developmentVersion},
		{in: "v0.0.0-20260824165146-82b5fc14a477", want: developmentVersion},
	}
	for _, tt := range tests {
		if got := versionFromBuildInfo(tt.in); got != tt.want {
			t.Errorf("versionFromBuildInfo(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
