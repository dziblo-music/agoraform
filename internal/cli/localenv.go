package cli

import (
	"os"
	"path/filepath"
	"strings"
)

func localEnvDirectory(args []string) string {
	if path, ok := manifestFileFlag(args); ok {
		return configDirectory(path)
	}

	commandIndex := -1
	command := ""
	for i, arg := range args {
		switch arg {
		case "validate", "plan", "apply", "destroy", "integrations", "import":
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
				return configDirectory(args[i+1])
			}
			break
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return configDirectory(arg)
	}

	return "."
}

func configDirectory(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "."
	}
	// Match manifest.Load's filesystem-based file/directory decision. A
	// directory can itself end in .yaml or .yml; do not infer its type
	// from the extension or strip root directory separators.
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			return path
		}
		return filepath.Dir(path)
	}
	// Retain single-file behavior for paths that do not exist yet.
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".yaml" || ext == ".yml" {
		return filepath.Dir(path)
	}
	return path
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
