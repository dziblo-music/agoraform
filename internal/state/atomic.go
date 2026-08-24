package state

import (
	"fmt"
	"os"
	"path/filepath"
)

func writeAtomic(path string, data []byte) error {
	return writeAtomicWithReplace(path, data, replaceFile)
}

func writeAtomicWithReplace(path string, data []byte, replace func(tmp, dest string) error) error {
	if replace == nil {
		return fmt.Errorf("replace function is required")
	}

	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	f, err := os.CreateTemp(dir, ".agoraform-state-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tmp := f.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()

	// Establish final permissions before the commit point. There must be no
	// fallible metadata operation after replace succeeds, otherwise callers
	// could observe an error even though new state is already on disk.
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return fmt.Errorf("set temporary file permissions: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}

	// replaceFile is the only commit point. It must replace the destination
	// atomically and must never delete the old state before the replacement is
	// guaranteed to succeed.
	if err := replace(tmp, path); err != nil {
		return fmt.Errorf("replace state file: %w", err)
	}
	cleanup = false
	return nil
}
