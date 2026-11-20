package scan

import (
	"path/filepath"
	"runtime"
	"strings"
)

// Private says which personal library a folder is, if reading inside it
// would make macOS ask for permission (Photos, Contacts, Calendars).
// Leftover never opens these: nothing a dev tool leaves behind lives there,
// and a scan shouldn't pop up a request for your photos.
func Private(path string) string { return private(path, runtime.GOOS) }

func private(path, goos string) string {
	if goos != "darwin" {
		return ""
	}
	p := filepath.ToSlash(path)
	switch {
	case strings.HasSuffix(strings.ToLower(filepath.Base(p)), ".photoslibrary"):
		return "Photos"
	case strings.HasSuffix(p, "/Library/Application Support/AddressBook"):
		return "Contacts"
	case strings.HasSuffix(p, "/Library/Calendars"):
		return "Calendar"
	}
	return ""
}
