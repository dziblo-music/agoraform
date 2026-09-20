package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalEnvDirectory(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "default", args: []string{"plan"}, want: "."},
		{name: "positional manifest", args: []string{"plan", "campaign/agoraform.yaml"}, want: "campaign"},
		{name: "short file flag", args: []string{"validate", "-f", "campaign/agoraform.yaml"}, want: "campaign"},
		{name: "long file flag", args: []string{"apply", "--file", "campaign/agoraform.yaml"}, want: "campaign"},
		{name: "long file equals", args: []string{"plan", "--file=campaign/agoraform.yaml"}, want: "campaign"},
		{name: "short file equals", args: []string{"plan", "-f=campaign/agoraform.yaml"}, want: "campaign"},
		{name: "import file flag", args: []string{"import", "-f", "campaign/agoraform.yaml", "matomo.goal.signup", "1"}, want: "campaign"},
		{name: "import default", args: []string{"import", "matomo.goal.signup", "1"}, want: "."},
		{name: "root command", args: []string{"--version"}, want: "."},
		{name: "positional directory", args: []string{"validate", "campaign"}, want: "campaign"},
		{name: "directory file flag", args: []string{"plan", "-f", "campaign"}, want: "campaign"},
		{name: "destroy positional directory", args: []string{"destroy", "campaign"}, want: "campaign"},
		{name: "integrations positional manifest", args: []string{"integrations", "campaign/agoraform.yaml"}, want: "campaign"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := localEnvDirectory(tt.args); got != tt.want {
				t.Fatalf("localEnvDirectory(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestConfigDirectoryUsesFilesystemType(t *testing.T) {
	dir := t.TempDir()
	yamlDir := filepath.Join(dir, "campaign.yaml")
	if err := os.Mkdir(yamlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ymlDir := filepath.Join(dir, "campaign.yml")
	if err := os.Mkdir(ymlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	plainFile := filepath.Join(dir, "config")
	if err := os.WriteFile(plainFile, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.VolumeName(dir) + string(filepath.Separator)
	for _, tt := range []struct {
		name, path, want string
	}{
		{"yaml directory", yamlDir, yamlDir},
		{"yaml directory with trailing separator", yamlDir + string(filepath.Separator), yamlDir + string(filepath.Separator)},
		{"yml directory", ymlDir, ymlDir},
		{"extensionless file", plainFile, dir},
		{"root directory", root, root},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := configDirectory(tt.path); got != tt.want {
				t.Fatalf("configDirectory(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
