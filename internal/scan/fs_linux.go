//go:build linux

package scan

import (
	"os"
	"path/filepath"
	"strings"
)

// osMountTable reads the Linux mount table. /proc/self/mountinfo lists every
// mount, including bind mounts and btrfs subvolumes, which /proc/mounts does not
// distinguish. The root is unused: the table is one local file read.
func osMountTable(_ string) ([]FSInfo, error) {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseMountinfo(f)
}

// pathPrefix reports whether path is at or under mountPoint, comparing at a
// separator boundary so /a/b is a prefix of /a/b/c but not of /a/bc. Linux
// paths are case-sensitive.
func pathPrefix(mountPoint, path string) bool {
	mp := filepath.Clean(mountPoint)
	p := filepath.Clean(path)
	if mp == p {
		return true
	}
	if mp == string(filepath.Separator) {
		return strings.HasPrefix(p, string(filepath.Separator))
	}
	return strings.HasPrefix(p, mp+string(filepath.Separator))
}
