//go:build !windows

package vault

import (
	"os"
	"path/filepath"
	"syscall"
)

func device(path string) (uint64, error) {
	var st syscall.Stat_t
	if err := syscall.Lstat(path, &st); err != nil {
		return 0, err
	}
	return uint64(st.Dev), nil
}

// sameVolume reports whether a rename between a and b can work without copying.
func sameVolume(a, b string) bool {
	da, err1 := device(a)
	db, err2 := device(b)
	return err1 == nil && err2 == nil && da == db
}

// mountPoint walks up from path until the device changes.
func mountPoint(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	dev, err := device(path)
	if err != nil {
		return "", err
	}
	for {
		parent := filepath.Dir(path)
		if parent == path {
			return path, nil
		}
		pd, err := device(parent)
		if err != nil || pd != dev {
			return path, nil
		}
		path = parent
	}
}

func isVolumeRoot(path string) bool {
	mp, err := mountPoint(path)
	return err == nil && filepath.Clean(mp) == filepath.Clean(path)
}

// makeWritable gives the owner write access to a folder before it is
// removed. Some tools (the Go module cache, for one) make files read-only.
func makeWritable(root string) {
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if info.IsDir() && info.Mode().Perm()&0o200 == 0 {
			os.Chmod(p, info.Mode().Perm()|0o700)
		}
		return nil
	})
}
