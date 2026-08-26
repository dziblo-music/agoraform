package config

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// LocalEnvFileName is the local, non-versioned provider configuration file.
const LocalEnvFileName = ".agoraform.env"

// LoadLocalEnv loads LocalEnvFileName from dir. Values already present in the
// process environment take precedence over values from the file.
func LoadLocalEnv(dir string) error {
	path := filepath.Join(dir, LocalEnvFileName)
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open %s: %w", LocalEnvFileName, err)
	}
	defer f.Close()

	values, err := parseLocalEnv(f)
	if err != nil {
		return fmt.Errorf("parse %s: %w", LocalEnvFileName, err)
	}

	for key, value := range values {
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set variable %q from %s: %w", key, LocalEnvFileName, err)
		}
	}

	return nil
}

func parseLocalEnv(r io.Reader) (map[string]string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	values := make(map[string]string)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		equals := strings.IndexByte(line, '=')
		if equals <= 0 {
			return nil, fmt.Errorf("line %d: expected KEY=VALUE", lineNumber)
		}

		key := strings.TrimSpace(line[:equals])
		if !validEnvKey(key) {
			return nil, fmt.Errorf("line %d: invalid variable name", lineNumber)
		}

		value, err := parseEnvValue(strings.TrimSpace(line[equals+1:]), lineNumber)
		if err != nil {
			return nil, err
		}
		values[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return values, nil
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		if i == 0 {
			if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_') {
				return false
			}
			continue
		}
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

func parseEnvValue(raw string, lineNumber int) (string, error) {
	if raw == "" {
		return "", nil
	}

	if raw[0] == '\'' || raw[0] == '"' {
		quote := raw[0]
		end := closingQuote(raw, quote)
		if end < 0 {
			return "", fmt.Errorf("line %d: unterminated quoted value", lineNumber)
		}

		trailing := strings.TrimSpace(raw[end+1:])
		if trailing != "" && !strings.HasPrefix(trailing, "#") {
			return "", fmt.Errorf("line %d: unexpected characters after quoted value", lineNumber)
		}

		if quote == '\'' {
			return raw[1:end], nil
		}

		value, err := strconv.Unquote(raw[:end+1])
		if err != nil {
			return "", fmt.Errorf("line %d: invalid quoted value", lineNumber)
		}
		return value, nil
	}

	for i := 1; i < len(raw); i++ {
		if raw[i] == '#' && (raw[i-1] == ' ' || raw[i-1] == '\t') {
			raw = raw[:i]
			break
		}
	}
	return strings.TrimSpace(raw), nil
}

func closingQuote(raw string, quote byte) int {
	escaped := false
	for i := 1; i < len(raw); i++ {
		if quote == '"' && raw[i] == '\\' && !escaped {
			escaped = true
			continue
		}
		if raw[i] == quote && !escaped {
			return i
		}
		escaped = false
	}
	return -1
}
