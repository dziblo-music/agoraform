//go:build !windows

package apply_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func blockStateFileReplace(t *testing.T, path string) func() {
	t.Helper()

	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}

	var once sync.Once
	restore := func() {
		once.Do(func() {
			_ = os.Chmod(dir, 0o755)
		})
	}
	t.Cleanup(restore)
	return restore
}
