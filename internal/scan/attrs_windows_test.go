//go:build windows

package scan

import (
	"syscall"
	"testing"
)

// TestAttrsMatchSyscall pins the hand-mirrored FILE_ATTRIBUTE_* constants to the
// Win32 definitions in the stdlib, so a mis-transcribed value fails loudly here
// instead of silently mis-decoding real Windows attributes.
func TestAttrsMatchSyscall(t *testing.T) {
	cases := []struct {
		name string
		got  uint32
		want uint32
	}{
		{"Readonly", AttrReadonly, syscall.FILE_ATTRIBUTE_READONLY},
		{"Hidden", AttrHidden, syscall.FILE_ATTRIBUTE_HIDDEN},
		{"System", AttrSystem, syscall.FILE_ATTRIBUTE_SYSTEM},
		{"Archive", AttrArchive, syscall.FILE_ATTRIBUTE_ARCHIVE},
		{"ReparsePnt", AttrReparsePnt, syscall.FILE_ATTRIBUTE_REPARSE_POINT},
		// stdlib syscall does not declare these; pin them to the Win32 values.
		{"Temporary", AttrTemporary, 0x00000100},
		{"SparseFile", AttrSparseFile, 0x00000200},
		{"Compressed", AttrCompressed, 0x00000800},
		{"Offline", AttrOffline, 0x00001000},
		{"Encrypted", AttrEncrypted, 0x00004000},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("Attr%s = %#x, want syscall value %#x", c.name, c.got, c.want)
		}
	}
}
