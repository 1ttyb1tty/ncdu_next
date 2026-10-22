//go:build unix

package scan

import (
	"os"
	"syscall"
)

func fileKey(fi os.FileInfo) string {
	s, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	return joinKey(s.Dev, s.Ino)
}

func deviceOf(fi os.FileInfo) (uint64, bool) {
	s, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return s.Dev, true
}
