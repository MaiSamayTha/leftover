//go:build windows

package vault

import (
	"os"
	"path/filepath"
	"strings"
)

func sameVolume(a, b string) bool {
	va, vb := filepath.VolumeName(a), filepath.VolumeName(b)
	return va != "" && strings.EqualFold(va, vb)
}

func mountPoint(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.VolumeName(path) + `\`, nil
}

func isVolumeRoot(path string) bool {
	clean := filepath.Clean(path)
	return clean == filepath.VolumeName(clean)+`\` || clean == filepath.VolumeName(clean)
}

// makeWritable clears the read-only attribute so RemoveAll can delete files.
func makeWritable(root string) {
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if info.Mode().Perm()&0o200 == 0 {
			os.Chmod(p, 0o666)
		}
		return nil
	})
}
