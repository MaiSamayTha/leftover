package ui

import (
	"syscall"
	"unsafe"
)

var getDiskFreeSpaceEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

// diskSpace reports the size of the disk holding path and how much of it is
// free for this user.
func diskSpace(path string) (total, free int64, ok bool) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, false
	}
	var avail, size, all uint64
	r, _, _ := getDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&avail)), uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&all)))
	if r == 0 {
		return 0, 0, false
	}
	return int64(size), int64(avail), true
}
