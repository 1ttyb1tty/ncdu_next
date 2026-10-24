//go:build !linux && !darwin && !windows

package scan

import (
	"path/filepath"
	"strings"
)

// osMountTable has no reader on this platform (the BSDs have incompatible
// Statfs_t layouts and cannot be compiled here). Every scan still succeeds; the
// report simply says the type is unavailable and names the reason.
func osMountTable(_ string) ([]FSInfo, error) { return nil, errUnsupported }

// pathPrefix reports whether path is at or under mountPoint, comparing at a
// separator boundary.
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
