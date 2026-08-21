package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dziblo-music/agoraform/internal/cli"
)

func TestRootHelp(t *testing.T) {
	t.Parallel()

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"--help"})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{"agoraform", "validate", "plan", "apply", "import"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing %q:\n%s", want, out)
		}
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"--version"})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	if got != cli.Version {
		t.Fatalf("version = %q, want %q", got, cli.Version)
	}
}

func TestUnimplementedCommands(t *testing.T) {
	t.Parallel()

	commands := []string{"plan", "apply", "import"}
	for _, name := range commands {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			streams, _, stderr := testStreams()
			code := cli.ExecuteWith(streams, []string{name})
			if code != cli.ExitError {
				t.Fatalf("exit code = %d, want %d", code, cli.ExitError)
			}

			errOut := stderr.String()
			if !strings.Contains(errOut, name+": not implemented yet") {
				t.Fatalf("stderr = %q, want not-implemented message for %s", errOut, name)
			}
		})
	}
}

func TestUnknownCommand(t *testing.T) {
	t.Parallel()

	streams, _, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"does-not-exist"})
	if code != cli.ExitUsage {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("stderr = %q, want unknown command message", stderr.String())
	}
}

func TestSubcommandHelp(t *testing.T) {
	t.Parallel()

	streams, stdout, stderr := testStreams()
	code := cli.ExecuteWith(streams, []string{"plan", "--help"})
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, cli.ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "plan") {
		t.Fatalf("help output missing plan:\n%s", stdout.String())
	}
}

func testStreams() (cli.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	return cli.IOStreams{
		In:     strings.NewReader(""),
		Out:    stdout,
		ErrOut: stderr,
	}, stdout, stderr
}
