package scan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanUnicodeDeep(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a😀")
	os.MkdirAll(p, 0755)
	os.WriteFile(filepath.Join(p, "b😀.txt"), []byte("x"), 0644)
	// deep
	d := dir
	for i := 0; i < 20; i++ {
		d = filepath.Join(d, "d")
		os.MkdirAll(d, 0755)
	}
	os.WriteFile(filepath.Join(d, "leaf.txt"), []byte("yy"), 0644)
	w := New(Options{Root: dir, Workers: 8})
	n, err := w.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if n.Size < 3 {
		t.Fatalf("size %d", n.Size)
	}
	if !strings.Contains(n.Path, dir) {
		t.Fatal("path")
	}
}
