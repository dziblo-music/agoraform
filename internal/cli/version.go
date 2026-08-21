package cli

// Version is the Agoraform CLI version string.
//
// The default value marks local and CI builds as development builds.
// Release builds should override it via:
//
//	-ldflags "-X github.com/dziblo-music/agoraform/internal/cli.Version=<version>"
var Version = "0.0.0-dev"
