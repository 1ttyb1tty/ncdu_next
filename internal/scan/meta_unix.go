//go:build !windows && !linux

package scan

import "os"

// fileMeta is the portable Unix fallback: only the modification time is read
// from os.FileInfo; ctime/atime require per-OS syscall field names and are left
// zero. Windows attributes are always 0 off Windows.
func fileMeta(fi os.FileInfo) (mtime, ctime, atime int64, attrs uint32) {
	if fi == nil {
		return 0, 0, 0, 0
	}
	return fi.ModTime().UnixNano(), 0, 0, 0
}
