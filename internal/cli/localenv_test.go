package cli

import "testing"

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := localEnvDirectory(tt.args); got != tt.want {
				t.Fatalf("localEnvDirectory(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}
