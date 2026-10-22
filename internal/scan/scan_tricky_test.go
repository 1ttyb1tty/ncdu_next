package scan

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/1ttyb1tty/ncdu-next/internal/export"
	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

func TestScanExcludes(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "drop.tmp"), []byte("x"), 0644)
	os.MkdirAll(filepath.Join(dir, "skip"), 0755)
	os.WriteFile(filepath.Join(dir, "skip", "inner.txt"), []byte("x"), 0644)
	w := New(Options{Root: dir, Workers: 2, Excludes: []string{"*.tmp", "skip"}})
	n, err := w.Scan()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	var walk func(*tree.Node)
	walk = func(node *tree.Node) {
		for _, c := range node.Children {
			names[c.Name] = true
			walk(c)
		}
	}
	walk(n)
	if names["drop.tmp"] {
		t.Fatal("excluded *.tmp present")
	}
	if names["skip"] {
		t.Fatal("excluded skip dir present")
	}
	if !names["keep.txt"] {
		t.Fatal("keep.txt missing")
	}
}

func TestScanHardlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hardlinks need privileges on windows")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world"), 0644)
	if err := os.Link(filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")); err != nil {
		t.Skipf("cannot link: %v", err)
	}
	dedup := New(Options{Root: dir, Workers: 2, CountLinks: false})
	nd, err := dedup.Scan()
	if err != nil {
		t.Fatal(err)
	}
	count := New(Options{Root: dir, Workers: 2, CountLinks: true})
	nc, err := count.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if nd.Size != int64(len("hello world")) {
		t.Fatalf("dedup size %d", nd.Size)
	}
	if nc.Size != int64(2*len("hello world")) {
		t.Fatalf("count size %d", nc.Size)
	}
}

func TestScanSymlinkCycleFollows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on windows")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0644)
	if err := os.Symlink(dir, filepath.Join(dir, "loop")); err != nil {
		t.Skipf("cannot symlink: %v", err)
	}
	w := New(Options{Root: dir, Workers: 4, FollowSym: true})
	done := make(chan struct{})
	go func() {
		_, _ = w.Scan()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("scan did not terminate on symlink cycle")
	}
}

func TestScanPermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions model differs on windows")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "ok.txt"), []byte("x"), 0644)
	deny := filepath.Join(dir, "deny")
	os.MkdirAll(deny, 0755)
	os.WriteFile(filepath.Join(deny, "hidden.txt"), []byte("x"), 0644)
	os.Chmod(deny, 0000)
	defer os.Chmod(deny, 0755)
	w := New(Options{Root: dir, Workers: 2})
	n, err := w.Scan()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	var walk func(*tree.Node)
	walk = func(node *tree.Node) {
		for _, c := range node.Children {
			if c.Name == "deny" && c.Err != nil {
				found = true
			}
			walk(c)
		}
	}
	walk(n)
	if !found {
		t.Fatal("denied dir not recorded")
	}
	if n.Size != int64(len("x")) {
		t.Fatalf("size %d", n.Size)
	}
}

func TestTreeDeleteRecomputes(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	a := &tree.Node{Name: "a", IsDir: false, Size: 10, AppSize: 10}
	b := &tree.Node{Name: "b", IsDir: false, Size: 30, AppSize: 30}
	root.AddChild(a)
	root.AddChild(b)
	root.Recompute()
	if root.Size != 40 {
		t.Fatalf("initial %d", root.Size)
	}
	b.Delete()
	if root.Size != 10 {
		t.Fatalf("after delete %d", root.Size)
	}
	if len(root.Children) != 1 || root.Children[0].Name != "a" {
		t.Fatal("child not removed")
	}
	root.Delete()
	if root.Parent == nil {
		t.Log("root delete no-op ok")
	}
}

func TestExportImportRoundtrip(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("bye"), 0644)
	w := New(Options{Root: dir, Workers: 2})
	n, err := w.Scan()
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.json")
	if err := export.Write(out, n); err != nil {
		t.Fatal(err)
	}
	back, err := export.Read(out)
	if err != nil {
		t.Fatal(err)
	}
	if back.Size != n.Size {
		t.Fatalf("size mismatch %d %d", back.Size, n.Size)
	}
	if len(back.Children) != len(n.Children) {
		t.Fatal("children mismatch")
	}
}
