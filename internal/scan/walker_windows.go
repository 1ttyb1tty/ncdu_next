//go:build windows

package scan

import (
	"os"
	"syscall"
)

func fileKey(fi os.FileInfo) string { return "" }

func deviceOf(fi os.FileInfo) (uint64, bool) { return 0, false }

// fileMeta extracts timestamps and attributes on Windows. os.FileInfo.Sys() is a
// *syscall.Win32FileAttributeData, carrying the FILE_ATTRIBUTE_* bitmask and the
// FILETIME creation/access times (converted to Unix epoch nanoseconds).
func fileMeta(fi os.FileInfo) (mtime, ctime, atime int64, attrs uint32) {
	if fi == nil {
		return 0, 0, 0, 0
	}
	mtime = fi.ModTime().UnixNano()
	d, ok := fi.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return mtime, 0, 0, 0
	}
	attrs = d.FileAttributes
	if ct := d.CreationTime.Nanoseconds(); ct != 0 {
		ctime = ct
	}
	if at := d.LastAccessTime.Nanoseconds(); at != 0 {
		atime = at
	}
	if lt := d.LastWriteTime.Nanoseconds(); lt != 0 {
		mtime = lt
	}
	return mtime, ctime, atime, attrs
}
