package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadLocalEnvLoadsValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, LocalEnvFileName)
	contents := strings.Join([]string{
		"# provider configuration",
		"AGORAFORM_TEST_PLAIN=plain-value",
		"AGORAFORM_TEST_DOUBLE=\"value with spaces\"",
		"AGORAFORM_TEST_SINGLE='literal value'",
		"AGORAFORM_TEST_COMMENT=value # ignored comment",
		"export AGORAFORM_TEST_EXPORT=exported",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	keys := []string{
		"AGORAFORM_TEST_PLAIN",
		"AGORAFORM_TEST_DOUBLE",
		"AGORAFORM_TEST_SINGLE",
		"AGORAFORM_TEST_COMMENT",
		"AGORAFORM_TEST_EXPORT",
	}
	for _, key := range keys {
		unsetEnvForTest(t, key)
	}

	if err := LoadLocalEnv(dir); err != nil {
		t.Fatalf("LoadLocalEnv() error = %v", err)
	}

	assertEnv(t, "AGORAFORM_TEST_PLAIN", "plain-value")
	assertEnv(t, "AGORAFORM_TEST_DOUBLE", "value with spaces")
	assertEnv(t, "AGORAFORM_TEST_SINGLE", "literal value")
	assertEnv(t, "AGORAFORM_TEST_COMMENT", "value")
	assertEnv(t, "AGORAFORM_TEST_EXPORT", "exported")
}

func TestLoadLocalEnvProcessEnvironmentWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, LocalEnvFileName)
	if err := os.WriteFile(path, []byte("AGORAFORM_TEST_PRECEDENCE=file-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("AGORAFORM_TEST_PRECEDENCE", "process-value")
	if err := LoadLocalEnv(dir); err != nil {
		t.Fatalf("LoadLocalEnv() error = %v", err)
	}

	assertEnv(t, "AGORAFORM_TEST_PRECEDENCE", "process-value")
}

func TestLoadLocalEnvMissingFileIsIgnored(t *testing.T) {
	if err := LoadLocalEnv(t.TempDir()); err != nil {
		t.Fatalf("LoadLocalEnv() error = %v", err)
	}
}

func TestLoadLocalEnvMalformedInputDoesNotExposeValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, LocalEnvFileName)
	if err := os.WriteFile(path, []byte("AGORAFORM_TEST_SECRET=\"do-not-leak\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := LoadLocalEnv(dir)
	if err == nil {
		t.Fatal("LoadLocalEnv() error = nil, want malformed input error")
	}
	if strings.Contains(err.Error(), "do-not-leak") {
		t.Fatalf("error exposed secret value: %v", err)
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("error = %v, want line number", err)
	}
}

func TestLoadLocalEnvDuplicateKeysUseLastFileValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, LocalEnvFileName)
	if err := os.WriteFile(path, []byte("AGORAFORM_TEST_DUP=first\nAGORAFORM_TEST_DUP=second\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	unsetEnvForTest(t, "AGORAFORM_TEST_DUP")
	if err := LoadLocalEnv(dir); err != nil {
		t.Fatalf("LoadLocalEnv() error = %v", err)
	}

	assertEnv(t, "AGORAFORM_TEST_DUP", "second")
}

func assertEnv(t *testing.T, key, want string) {
	t.Helper()
	if got := os.Getenv(key); got != want {
		t.Fatalf("%s = %q, want %q", key, got, want)
	}
}

func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	old, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, old)
			return
		}
		_ = os.Unsetenv(key)
	})
}
