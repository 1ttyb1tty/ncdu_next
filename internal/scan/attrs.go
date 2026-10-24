package scan

import "strings"

// Windows FILE_ATTRIBUTE_* bits, mirrored here as plain constants so they can be
// referenced from cross-platform code (the walker only populates Attrs on
// Windows; ui renders it). Values match the Win32 definitions.
const (
	AttrReadonly   uint32 = 0x00000001
	AttrHidden     uint32 = 0x00000002
	AttrSystem     uint32 = 0x00000004
	AttrArchive    uint32 = 0x00000020
	AttrTemporary  uint32 = 0x00000100
	AttrSparseFile uint32 = 0x00000200
	AttrReparsePnt uint32 = 0x00000400
	AttrCompressed uint32 = 0x00000800
	AttrOffline    uint32 = 0x00001000
	AttrEncrypted  uint32 = 0x00004000
)

// attrLetter pairs a FILE_ATTRIBUTE_* bit with its short display letters. Order
// is fixed so the rendered string is deterministic.
var attrLetter = []struct {
	bit    uint32
	letter string
}{
	{AttrReadonly, "R"},
	{AttrHidden, "H"},
	{AttrSystem, "S"},
	{AttrArchive, "A"},
	{AttrTemporary, "T"},
	{AttrSparseFile, "SP"},
	{AttrReparsePnt, "RP"},
	{AttrCompressed, "C"},
	{AttrOffline, "O"},
	{AttrEncrypted, "E"},
}

// AttrLetters renders a FILE_ATTRIBUTE_* bitmask as space-separated short
// letters (R/H/S/A/...). An empty or unrecognized mask yields "".
func AttrLetters(attrs uint32) string {
	if attrs == 0 {
		return ""
	}
	var parts []string
	for _, a := range attrLetter {
		if attrs&a.bit != 0 {
			parts = append(parts, a.letter)
		}
	}
	return strings.Join(parts, " ")
}
