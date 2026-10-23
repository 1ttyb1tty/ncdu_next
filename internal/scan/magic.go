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
