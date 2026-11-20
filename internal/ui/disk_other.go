//go:build !darwin && !linux && !windows

package ui

func diskSpace(path string) (total, free int64, ok bool) { return 0, 0, false }
