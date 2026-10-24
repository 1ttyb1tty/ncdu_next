//go:build linux

package scan

import (
	"os"
	"syscall"
)

// fileMeta extracts timestamps on Linux via syscall.Stat_t (attoseconds-aware
// Timespec). Windows attributes are always 0 off Windows.
func fileMeta(fi os.FileInfo) (mtime, ctime, atime int64, attrs uint32) {
	if fi == nil {
		return 0, 0, 0, 0
	}
	mtime = fi.ModTime().UnixNano()
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		mtime = st.Mtim.Nano()
		ctime = st.Ctim.Nano()
		atime = st.Atim.Nano()
	}
	return mtime, ctime, atime, 0
}
