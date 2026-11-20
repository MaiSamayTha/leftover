//go:build !darwin && !linux && !windows

package scan

func DiskSpace(path string) (total, free int64, ok bool) { return 0, 0, false }
