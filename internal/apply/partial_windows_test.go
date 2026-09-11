//go:build windows

package apply_test

import (
	"os"
	"sync"
	"syscall"
	"testing"
)

func blockStateFileReplace(t *testing.T, path string) func() {
	t.Helper()

	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(
		p,
		syscall.GENERIC_READ,
		0,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Hand the handle to the Go runtime so Close uses normal file cleanup
	// instead of raw CloseHandle, which can corrupt runtime handle tables
	// when many parallel tests run on Windows.
	f := os.NewFile(uintptr(handle), path)

	var once sync.Once
	restore := func() {
		once.Do(func() {
			_ = f.Close()
		})
	}
	t.Cleanup(restore)
	return restore
}
