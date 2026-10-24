//go:build unix

package scan

import (
	"os"
	"syscall"
)

func fileKey(fi os.FileInfo) string {
	if _, ok := fi.Sys().(*syscall.Stat_t); !ok {
		return ""
	}
	// devOf/inoOf convert the platform's Stat_t fields (Dev is int32 on darwin,
	// uint64 on linux) to the shared uint64 identity.
	return joinKey(devOf(fi), inoOf(fi))
}

func deviceOf(fi os.FileInfo) (uint64, bool) {
	if _, ok := fi.Sys().(*syscall.Stat_t); !ok {
		return 0, false
	}
	return devOf(fi), true
}

// deviceOfEntry returns the --one-file-system identity for a path. On Unix the
// Stat_t device already provides it, so path and table are ignored and the
// identity is byte-for-byte the same as before.
func deviceOfEntry(fi os.FileInfo, path string, table []FSInfo) (uint64, bool) {
	return deviceOf(fi)
}
