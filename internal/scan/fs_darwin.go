//go:build darwin

package scan

import (
	"path/filepath"
	"strings"
	"syscall"
)

// mntNowait is syscall.MNT_NOWAIT (2), which the syscall package does not export.
const mntNowait = 2

// osMountTable enumerates mounted filesystems with getfsstat, which reports the
// mounted-at path, the filesystem type name, and the filesystem id directly.
// It is best-effort: this repository's CI cannot execute it.
func osMountTable() ([]FSInfo, error) {
	n, err := syscall.Getfsstat(nil, mntNowait)
	if err != nil {
		return nil, err
	}
	if n <= 0 {
		return nil, nil
	}
	buf := make([]syscall.Statfs_t, n)
	n, err = syscall.Getfsstat(buf, mntNowait)
	if err != nil {
		return nil, err
	}
	if n > len(buf) {
		n = len(buf)
	}
	out := make([]FSInfo, 0, n)
	for i := 0; i < n; i++ {
		s := &buf[i]
		out = append(out, FSInfo{
			MountPoint: cstr(s.Mntonname[:]),
			FSType:     cstr(s.Fstypename[:]),
			Avail:      FSAvailable,
			// Fsid is a pair of int32; fold it into an opaque identity.
			Device: uint64(uint32(s.Fsid.Val[0]))<<32 | uint64(uint32(s.Fsid.Val[1])),
		})
	}
	return out, nil
}

// cstr converts a NUL-terminated int8 array field to a Go string.
func cstr(b []int8) string {
	n := 0
	for n < len(b) && b[n] != 0 {
		n++
	}
	bs := make([]byte, n)
	for i := 0; i < n; i++ {
		bs[i] = byte(b[i])
	}
	return string(bs)
}

// pathPrefix reports whether path is at or under mountPoint, comparing at a
// separator boundary. Darwin paths are case-sensitive by default.
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
