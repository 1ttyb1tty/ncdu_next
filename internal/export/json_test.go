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
	if len(back.Children) != 1 || back.Children[0].Type != "vbox-disk" {
		t.Fatalf("child type %q, want vbox-disk", back.Children[0].Type)
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
