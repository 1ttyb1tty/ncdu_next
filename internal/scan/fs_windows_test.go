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

// TestVolumeAvailability pins the three-state decision: an unreadable volume
// (empty filesystem name) is unavailable with a reason, never "available" with a
// blank type.
func TestVolumeAvailability(t *testing.T) {
	if a, r := volumeAvailability("NTFS"); a != FSAvailable || r != "" {
		t.Fatalf("NTFS -> %v, %q; want available with no reason", a, r)
	}
	if a, r := volumeAvailability(""); a != FSUnavailable || r == "" {
		t.Fatalf("empty name -> %v, %q; want unavailable with a reason", a, r)
	}
}
