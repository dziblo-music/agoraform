//go:build windows

package apply_test

import (
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
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	restore := func() {
		_ = syscall.CloseHandle(handle)
	}
	t.Cleanup(restore)
	return restore
}
