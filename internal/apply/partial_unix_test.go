//go:build !windows

package apply_test

import (
	"os"
	"path/filepath"
	"testing"
)

func blockStateFileReplace(t *testing.T, path string) func() {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	restore := func() {
		_ = os.Chmod(dir, 0o755)
	}
	t.Cleanup(restore)
	return restore
}
