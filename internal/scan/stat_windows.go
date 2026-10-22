//go:build windows

package scan

import "os"

func devOf(fi os.FileInfo) uint64 { return 0 }
func inoOf(fi os.FileInfo) uint64 { return 0 }
