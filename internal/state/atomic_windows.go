//go:build windows

package state

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	moveFileReplaceExisting = 0x1
	moveFileWriteThrough    = 0x8
)

var moveFileExW = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

// replaceFile uses MoveFileExW because os.Rename on Windows does not replace
// an existing destination. MOVEFILE_REPLACE_EXISTING avoids the unsafe
// delete-then-rename window, and WRITE_THROUGH asks Windows not to report
// success before the move has completed.
func replaceFile(tmp, dest string) error {
	from, err := syscall.UTF16PtrFromString(tmp)
	if err != nil {
		return fmt.Errorf("encode temporary path: %w", err)
	}
	to, err := syscall.UTF16PtrFromString(dest)
	if err != nil {
		return fmt.Errorf("encode destination path: %w", err)
	}

	r1, _, callErr := moveFileExW.Call(
		uintptr(unsafe.Pointer(from)),
		uintptr(unsafe.Pointer(to)),
		uintptr(moveFileReplaceExisting|moveFileWriteThrough),
	)
	if r1 != 0 {
		return nil
	}
	if callErr != syscall.Errno(0) {
		return callErr
	}
	return fmt.Errorf("MoveFileExW failed")
}
