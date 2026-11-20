//go:build darwin || linux

package ui

import "syscall"

// diskSpace reports the size of the disk holding path and how much of it is
// free for this user.
func diskSpace(path string) (total, free int64, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, false
	}
	bs := uint64(st.Bsize)
	return int64(st.Blocks * bs), int64(st.Bavail * bs), true
}
