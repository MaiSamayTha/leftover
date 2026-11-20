package scan

import "testing"

func TestPrivate(t *testing.T) {
	cases := []struct {
		path, goos, want string
	}{
		{"/Users/a/Pictures/Photos Library.photoslibrary", "darwin", "Photos"},
		{"/Volumes/Backup/Old.photoslibrary", "darwin", "Photos"},
		{"/Users/a/Library/Application Support/AddressBook", "darwin", "Contacts"},
		{"/Users/a/Library/Calendars", "darwin", "Calendar"},
		{"/Users/a/Pictures", "darwin", ""},
		{"/home/a/Photos Library.photoslibrary", "linux", ""},
	}
	for _, c := range cases {
		if got := private(c.path, c.goos); got != c.want {
			t.Errorf("private(%q, %s) = %q, want %q", c.path, c.goos, got, c.want)
		}
	}
}
