//go:build !windows

package state

import "os"

// replaceFile atomically renames a temporary file over the destination on
// platforms where rename-over-existing has replacement semantics. The temp
// file is created in the destination directory, so the operation stays on one
// filesystem.
func replaceFile(tmp, dest string) error {
	return os.Rename(tmp, dest)
}
