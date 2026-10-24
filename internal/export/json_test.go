package export

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/1ttyb1tty/ncdu-next/internal/scan"
	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

func TestExport(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0644)
	w := scan.New(scan.Options{Root: dir, Workers: 2})
	n, err := w.Scan()
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.json")
	if err := Write(out, n); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatal(err)
	}
	if e.Asize != 2 {
		t.Fatalf("asize %d", e.Asize)
	}
}

func TestExportTypeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "disk.vdi"), []byte("hello"), 0644)
	w := scan.New(scan.Options{Root: dir, Workers: 2})
	n, err := w.Scan()
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.json")
	if err := Write(out, n); err != nil {
		t.Fatal(err)
	}
	back, err := Read(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Children) != 1 || back.Children[0].Type != "vdi" {
		t.Fatalf("child type %q, want vdi", back.Children[0].Type)
	}
}

func TestExportKindTimestampsAttrsRoundTrip(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	child := &tree.Node{
		Name: "disk.vdi", IsDir: false, Size: 100, Type: "vdi",
		Kind:  tree.KindDiskImage,
		Mtime: 1_700_000_000_000_000_000,
		Ctime: 1_600_000_000_000_000_000,
		Atime: 1_500_000_000_000_000_000,
		Attrs: 0x20,
	}
	root.AddChild(child)
	out := filepath.Join(t.TempDir(), "out.json")
	if err := Write(out, root); err != nil {
		t.Fatal(err)
	}
	back, err := Read(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(back.Children))
	}
	c := back.Children[0]
	if c.Kind != tree.KindDiskImage {
		t.Errorf("kind %v, want disk-image", c.Kind)
	}
	if c.Mtime != child.Mtime || c.Ctime != child.Ctime || c.Atime != child.Atime {
		t.Errorf("timestamps %d/%d/%d, want %d/%d/%d", c.Mtime, c.Ctime, c.Atime, child.Mtime, child.Ctime, child.Atime)
	}
	if c.Attrs != 0x20 {
		t.Errorf("attrs %#x, want 0x20", c.Attrs)
	}
}

// TestExportDerivesKindFromType covers an export that carries a specific Type id
// but no numeric kind: the Kind is derived from the id.
func TestExportDerivesKindFromType(t *testing.T) {
	data := []byte(`{"name":"root","items":[{"name":"pic.png","type":"png"}]}`)
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatal(err)
	}
	n := ToNode(e)
	if len(n.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(n.Children))
	}
	if n.Children[0].Kind != tree.KindImage {
		t.Errorf("kind %v, want image (derived from type)", n.Children[0].Kind)
	}
}

// TestExportLegacyTypeStaysUnknown pins the boundary: a type id from the retired
// category vocabulary does not resolve, so the entry defaults to KindUnknown
// (the documented missing-field default) rather than silently mis-classifying.
func TestExportLegacyTypeStaysUnknown(t *testing.T) {
	data := []byte(`{"name":"root","items":[{"name":"pic.png","type":"image-png"}]}`)
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatal(err)
	}
	n := ToNode(e)
	if len(n.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(n.Children))
	}
	if n.Children[0].Kind != tree.KindUnknown {
		t.Errorf("legacy id kind %v, want unknown", n.Children[0].Kind)
	}
}

func TestExportEncodingLineEndingRoundTrip(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	child := &tree.Node{Name: "notes.txt", IsDir: false, Size: 100, Type: "text-plain", Encoding: "utf-8", LineEnding: "lf"}
	root.AddChild(child)
	out := filepath.Join(t.TempDir(), "out.json")
	if err := Write(out, root); err != nil {
		t.Fatal(err)
	}
	back, err := Read(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(back.Children))
	}
	if back.Children[0].Encoding != "utf-8" {
		t.Fatalf("encoding %q, want utf-8", back.Children[0].Encoding)
	}
	if back.Children[0].LineEnding != "lf" {
		t.Fatalf("line_ending %q, want lf", back.Children[0].LineEnding)
	}
}
