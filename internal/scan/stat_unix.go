//go:build !windows

package scan

import (
	"io/fs"
	"syscall"
)

// fileStat pulls the parts of a stat result the sizer needs. On Unix the
// on-disk size comes from allocated blocks, so sparse files like a VM disk
// image count for what they really take, not for their nominal length.
func fileStat(info fs.FileInfo) (dev, ino uint64, links uint64, onDisk int64) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, 1, info.Size()
	}
	return uint64(st.Dev), uint64(st.Ino), uint64(st.Nlink), int64(st.Blocks) * 512
}

// deviceOf returns the device a path's metadata says it lives on.
func deviceOf(info fs.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Dev)
	}
	return 0
}
