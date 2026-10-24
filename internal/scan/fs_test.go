package scan

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

func TestParseMountinfo(t *testing.T) {
	// A bind-style duplicate and an escaped space, neither of which /proc/mounts
	// distinguishes. The expected device ids are computed independently.
	const fixture = "" +
		"36 35 98:0 /mnt1 /mnt2 rw,noatime master:1 - ext3 /dev/root rw,errors=continue\n" +
		"37 36 8:1 / / rw,relatime shared:1 - ext4 /dev/sda1 rw\n" +
		"38 37 0:24 / /mnt/with\\040space rw - tmpfs tmpfs rw\n" +
		"39 37 0:24 / /mnt/with\\040space rw - tmpfs tmpfs rw\n"

	got, err := parseMountinfo(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("parseMountinfo: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d mounts, want 4: %+v", len(got), got)
	}
	want := []FSInfo{
		{MountPoint: "/mnt2", FSType: "ext3", Avail: FSAvailable, Device: uint64(98)<<32 | 0},
		{MountPoint: "/", FSType: "ext4", Avail: FSAvailable, Device: uint64(8)<<32 | 1},
		{MountPoint: "/mnt/with space", FSType: "tmpfs", Avail: FSAvailable, Device: uint64(0)<<32 | 24},
		{MountPoint: "/mnt/with space", FSType: "tmpfs", Avail: FSAvailable, Device: uint64(0)<<32 | 24},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("mount %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	// A reader error must be reported, not swallowed.
	if _, err := parseMountinfo(errReader{}); err == nil {
		t.Error("expected an error from a failing reader")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestPathPrefixBoundary(t *testing.T) {
	cases := []struct {
		mount, path string
		want        bool
	}{
		{"/a/b", "/a/b", true},
		{"/a/b", "/a/b/c", true},
		{"/a/b", "/a/bc", false},
		{"/a/b/", "/a/b/c", true},
		{"/", "/anything", true},
	}
	for _, c := range cases {
		if got := pathPrefix(c.mount, c.path); got != c.want {
			t.Errorf("pathPrefix(%q, %q) = %v, want %v", c.mount, c.path, got, c.want)
		}
	}
}

func TestMountsUnder(t *testing.T) {
	table := []FSInfo{
		{MountPoint: "/a/b", FSType: "root"},
		{MountPoint: "/a/b/c", FSType: "nested"},
		{MountPoint: "/a/bc", FSType: "sibling"},
		{MountPoint: "/a", FSType: "above"},
	}
	got := mountsUnder("/a/b", table)
	if len(got) != 1 || got[0].MountPoint != "/a/b/c" {
		t.Fatalf("mountsUnder = %+v, want only /a/b/c", got)
	}
}

func TestFSForLongestPrefix(t *testing.T) {
	table := []FSInfo{
		{MountPoint: "/", FSType: "root"},
		{MountPoint: "/a", FSType: "a"},
		{MountPoint: "/a/b", FSType: "ab"},
	}
	if info, ok := fsFor("/a/b/c", table); !ok || info.FSType != "ab" {
		t.Fatalf("fsFor(/a/b/c) = %+v, %v; want ab", info, ok)
	}
	if info, ok := fsFor("/a/z", table); !ok || info.FSType != "a" {
		t.Fatalf("fsFor(/a/z) = %+v, %v; want a", info, ok)
	}
	if _, ok := fsFor("/a/b", []FSInfo{{MountPoint: "/other"}}); ok {
		t.Fatal("a path under no mount must not resolve")
	}
}

func TestFSLookupNilAndRoot(t *testing.T) {
	if _, ok := FSLookup(nil, "/x"); ok {
		t.Fatal("a nil report must have no answer")
	}
	fs := &FSReport{
		Root:   FSInfo{MountPoint: "/", FSType: "rootfs", Avail: FSAvailable},
		Mounts: []FSInfo{{MountPoint: "/data", FSType: "ntfs", Avail: FSAvailable}},
	}
	if info, ok := FSLookup(fs, "/data/x"); !ok || info.FSType != "ntfs" {
		t.Fatalf("lookup under a mount = %+v, %v; want ntfs", info, ok)
	}
	if info, ok := FSLookup(fs, "/etc/x"); !ok || info.FSType != "rootfs" {
		t.Fatalf("lookup outside mounts = %+v, %v; want rootfs", info, ok)
	}
}

// stubTable replaces the mount-table seam for one test, restoring it before the
// test returns (Scan's goroutines are joined by then). Tests using it must not
// run in parallel.
func stubTable(t *testing.T, table []FSInfo, err error) {
	t.Helper()
	old := osMountTableFn
	osMountTableFn = func() ([]FSInfo, error) { return table, err }
	t.Cleanup(func() { osMountTableFn = old })
}

// TestFSAvailable is the "available" state: the OS named the filesystem.
func TestFSAvailable(t *testing.T) {
	dir := t.TempDir()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	stubTable(t, []FSInfo{{MountPoint: abs, FSType: "testfs", Avail: FSAvailable, Device: 42}}, nil)

	w := New(Options{Root: dir, Workers: 2})
	node, err := w.Scan()
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if node == nil {
		t.Fatal("expected a node")
	}
	rep := w.FSReport()
	if rep == nil {
		t.Fatal("expected a report")
	}
	if rep.Root.Avail != FSAvailable || rep.Root.FSType != "testfs" {
		t.Fatalf("root = %+v, want available testfs", rep.Root)
	}
}

// TestFSUnavailable is the "unavailable" state: the mount table read failed, but
// the scan still succeeds.
func TestFSUnavailable(t *testing.T) {
	dir := t.TempDir()
	stubTable(t, nil, errors.New("mount table exploded"))

	w := New(Options{Root: dir, Workers: 2})
	node, err := w.Scan()
	if err != nil {
		t.Fatalf("scan must not fail when the mount table does: %v", err)
	}
	if node == nil {
		t.Fatal("expected a node")
	}
	rep := w.FSReport()
	if rep == nil || rep.Root.Avail != FSUnavailable {
		t.Fatalf("root = %+v, want unavailable", rep)
	}
	if rep.Root.Reason == "" {
		t.Fatal("unavailable must carry a reason")
	}
}

// TestFSNotApplicable is the third state: the scan root is a regular file, so
// there is nothing to read (not the same as an unreadable filesystem).
func TestFSNotApplicable(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := New(Options{Root: file, Workers: 2})
	if _, err := w.Scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	rep := w.FSReport()
	if rep == nil || rep.Root.Avail != FSNotApplicable {
		t.Fatalf("root = %+v, want not-applicable", rep)
	}
	if rep.Root.Reason == "" {
		t.Fatal("not-applicable must carry a reason")
	}
}

// stubDevice replaces the OneFS identity seam for one test.
func stubDevice(t *testing.T, fn func(fi os.FileInfo, path string, table []FSInfo) (uint64, bool)) {
	t.Helper()
	old := deviceOfEntryFn
	deviceOfEntryFn = fn
	t.Cleanup(func() { deviceOfEntryFn = old })
}

func childNames(n *tree.Node) []string {
	if n == nil {
		return nil
	}
	var out []string
	for _, c := range n.Children {
		out = append(out, c.Name)
	}
	return out
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func TestOneFileSystemDeviceChange(t *testing.T) {
	// A device change skips the child; an unchanged device descends; an unknown
	// identity fails open (descends), so OneFS never silently hides data.
	const sub = "sub"

	cases := []struct {
		name    string
		fn      func(fi os.FileInfo, path string, table []FSInfo) (uint64, bool)
		wantSub bool
	}{
		{
			name: "changed device skips",
			fn: func(fi os.FileInfo, path string, table []FSInfo) (uint64, bool) {
				if filepath.Base(path) == sub {
					return 2, true
				}
				return 1, true
			},
			wantSub: false,
		},
		{
			name: "same device descends",
			fn: func(fi os.FileInfo, path string, table []FSInfo) (uint64, bool) {
				return 1, true
			},
			wantSub: true,
		},
		{
			name: "unknown identity fails open",
			fn: func(fi os.FileInfo, path string, table []FSInfo) (uint64, bool) {
				return 0, false
			},
			wantSub: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, sub), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, sub, "in.txt"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			stubTable(t, nil, errors.New("stub"))
			stubDevice(t, c.fn)

			w := New(Options{Root: dir, OneFS: true, Workers: 2})
			node, err := w.Scan()
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if got := contains(childNames(node), sub); got != c.wantSub {
				t.Fatalf("subdir present = %v, want %v (names %v)", got, c.wantSub, childNames(node))
			}
		})
	}
}
