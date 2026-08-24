package cli

import (
	"regexp"
	"runtime/debug"
	"strings"
)

const (
	// developmentVersion is reported by untagged local and CI builds.
	// It is a valid SemVer 2.0 pre-release of 0.0.0.
	developmentVersion = "0.0.0-dev"
)

// goPseudoVersionTimestamp matches the yyyymmddhhmmss segment Go embeds in
// module pseudo-versions for untagged commits.
var goPseudoVersionTimestamp = regexp.MustCompile(`\d{14}`)

// Version is the Agoraform CLI version string (SemVer 2.0, no "v" prefix).
//
// Git tags use the Go module convention vMAJOR.MINOR.PATCH (for example
// v0.1.0). The CLI prints the SemVer identifier without that prefix, so a
// release tagged v0.1.0 reports 0.1.0.
//
// Release builds inject the version via:
//
//	-ldflags "-X github.com/dziblo-music/agoraform/internal/cli.Version=0.1.0"
//
// If Version is still the development default, a tagged module version from
// build info is used so `go install ...@v0.1.0` also reports 0.1.0. Untagged
// local builds stay on 0.0.0-dev.
var Version = developmentVersion

func effectiveVersion() string {
	return resolveVersion(Version)
}

func resolveVersion(current string) string {
	current = normalizeVersion(current)
	if current != developmentVersion && current != "" {
		return current
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return developmentVersion
	}
	return versionFromBuildInfo(info.Main.Version)
}

func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

func versionFromBuildInfo(moduleVersion string) string {
	switch moduleVersion {
	case "", "(devel)":
		return developmentVersion
	default:
		normalized := normalizeVersion(moduleVersion)
		if normalized == "" || isUntaggedBuild(normalized) {
			return developmentVersion
		}
		return normalized
	}
}

func isUntaggedBuild(v string) bool {
	if strings.Contains(v, "+dirty") {
		return true
	}
	return goPseudoVersionTimestamp.MatchString(v)
}
