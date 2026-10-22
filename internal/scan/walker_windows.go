//go:build windows

package scan

import "os"

func fileKey(fi os.FileInfo) string { return "" }

func deviceOf(fi os.FileInfo) (uint64, bool) { return 0, false }
