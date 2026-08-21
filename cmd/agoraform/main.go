// Command agoraform is the Marketing Infrastructure as Code CLI.
package main

import (
	"os"

	"github.com/dziblo-music/agoraform/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
