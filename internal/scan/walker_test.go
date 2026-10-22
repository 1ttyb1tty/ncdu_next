package scan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

func TestScanBasic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("world!!"), 0644)
	w := New(Options{Root: dir, Workers: 2})
	n, err := w.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if !n.IsDir {
		t.Fatal("root not dir")
	}
	if n.Size != 12 {
		t.Fatalf("size %d", n.Size)
	}
	_ = tree.SortBySize
}

func TestScanEmpty(t *testing.T) {
	dir := t.TempDir()
	w := New(Options{Root: dir, Workers: 1})
	n, err := w.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if n.Size != 0 {
		t.Fatalf("size %d", n.Size)
	}
}

func TestScanFileRoot(t *testing.T) {
	f := filepath.Join(t.TempDir(), "x.txt")
	os.WriteFile(f, []byte("abc"), 0644)
	w := New(Options{Root: f, Workers: 1})
	n, err := w.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if n.IsDir || n.Size != 3 {
		t.Fatalf("bad node")
	}
}
