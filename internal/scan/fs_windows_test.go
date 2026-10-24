//go:build windows

package scan

import (
	"testing"
)

// TestWindowsMountTableResolvesRoot is the Windows smoke test: a real scan of a
// local temporary directory must resolve an available filesystem with a non-empty
// type name (asserted for non-emptiness only, since the volume could be exFAT,
// ReFS, or anything else the OS names).
func TestWindowsMountTableResolvesRoot(t *testing.T) {
	dir := t.TempDir()
	w := New(Options{Root: dir, Workers: 2})
	if _, err := w.Scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	rep := w.FSReport()
	if rep == nil {
		t.Fatal("expected a report")
	}
	if rep.Root.Avail != FSAvailable {
		t.Fatalf("root = %+v, want available on a local volume", rep.Root)
	}
	if rep.Root.FSType == "" {
		t.Fatalf("root type name must be non-empty, got %+v", rep.Root)
	}
}
