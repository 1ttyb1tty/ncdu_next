package scan

// RefineType is the optional magic-byte signature detection pass.
// It is a stub for v2: v1 relies purely on extension heuristics via
// detectFileType. When --detect-format is enabled this is where
// per-file header magic bytes would be read to confirm or correct the
// extension-based type (e.g. catching a renamed .vdi or .qcow2).
//
// current is the extension-based type already assigned; the stub returns
// it unchanged. v2 implementations inspect file header bytes here and may
// return a more specific type (e.g. "qcow2", "vmdk-footer", "vhd-dynamic").
func RefineType(path string, current string) string {
	_ = path
	return current
}

// RefineText is the optional encoding + line-ending detection pass.
// It is a stub for v2: v1 leaves Encoding and LineEnding empty. When
// --detect-format is enabled and the file type is text-ish, this is where
// per-file header bytes would be read to detect encoding (BOM/UTF-8/ASCII/
// Latin-1) and line-ending variation (CRLF/LF/CR/mixed).
//
// path is the file being inspected; currentType is the extension-based
// type already assigned. The stub returns empty strings; v2 implementations
// inspect file header bytes here.
func RefineText(path string, currentType string) (encoding string, lineEnding string) {
	_ = path
	_ = currentType
	return "", ""
}
