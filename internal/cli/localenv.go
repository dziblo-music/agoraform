package cli

import (
	"path/filepath"
	"strings"
)

func localEnvDirectory(args []string) string {
	if path, ok := manifestFileFlag(args); ok {
		return filepath.Dir(path)
	}

	commandIndex := -1
	command := ""
	for i, arg := range args {
		switch arg {
		case "validate", "plan", "apply", "import":
			commandIndex = i
			command = arg
		}
		if commandIndex >= 0 {
			break
		}
	}

	if commandIndex < 0 || command == "import" {
		return "."
	}

	for i := commandIndex + 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if i+1 < len(args) {
				return filepath.Dir(args[i+1])
			}
			break
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return filepath.Dir(arg)
	}

	return "."
}

func manifestFileFlag(args []string) (string, bool) {
	for i, arg := range args {
		switch arg {
		case "-f", "--file":
			if i+1 < len(args) && args[i+1] != "" {
				return args[i+1], true
			}
		default:
			if strings.HasPrefix(arg, "--file=") {
				if value := strings.TrimPrefix(arg, "--file="); value != "" {
					return value, true
				}
			}
			if strings.HasPrefix(arg, "-f=") {
				if value := strings.TrimPrefix(arg, "-f="); value != "" {
					return value, true
				}
			}
		}
	}
	return "", false
}
