//go:build windows

package scan

import "io/fs"

// fileStat on Windows has no cheap inode or block count from a directory
// listing, so the logical size is used and hard links aren't folded.
func fileStat(info fs.FileInfo) (dev, ino uint64, links uint64, onDisk int64) {
	return 0, 0, 1, info.Size()
}

func deviceOf(info fs.FileInfo) uint64 { return 0 }
