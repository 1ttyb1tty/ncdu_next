package scan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

func TestDetectFileType(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"disk.vdi", "vdi"},
		{"disk.VMDK", "vmdk"},
		{"disk.vmdk", "vmdk"},
		{"disk.vhd", "vhd"},
		{"disk.ova", "ova"},
		{"disk.ovf", "ovf"},
		{"machine.vbox", "vbox"},
		{"disk.vzv", "vzv"},
		{"disk.vzvhd", "vzvhd"},
		{"disk.ploop", "ploop"},
		{"disk.xva", "xva"},
		{"disk.hdd", "hdd"},
		{"disk.hdds", "hdds"},
		{"vm.pvm", "pvm"},
		{"disk.qcow2", "qcow2"},
		{"disk.raw", "raw"},
		{"disk.vhdx", "vhdx"},
		{"image.tar", "tar"},
		{"image.oci", "oci"},
		{"image.iso", "iso"},
		{"dump.vmem", "vmem"},
		{"snapshot.vmss", "vmss"},
		{"state.vmsn", "vmsn"},
		{"crash.dmp", "dmp"},
		{"mem.mem", "mem"},
		{"document.txt", "txt"},
		{"photo.jpeg", "jpg"},
		{"photo.JPG", "jpg"},
		{"scan.tif", "tiff"},
		{"pic.heif", "heic"},
		{"index.htm", "html"},
		{"readme.markdown", "md"},
		{"clip.mpg", "mpeg"},
		{"image.img", ""},
		{"noext", ""},
	}
	for _, c := range cases {
		if got := detectFileType(c.path); got != c.want {
			t.Errorf("detectFileType(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

// TestFileTypesHaveKind guards the two-table contract: every Type id the
// extension table can produce must map to a broad Kind in the tree package.
func TestFileTypesHaveKind(t *testing.T) {
	for ext, id := range fileTypes {
		if k := tree.KindForType(id); k == tree.KindUnknown {
			t.Errorf("extension %q -> Type %q has no Kind mapping", ext, id)
		}
	}
}

func TestClassifyType(t *testing.T) {
	cases := []struct {
		id   string
		want tree.Kind
	}{
		{"", tree.KindOther},
		{"vdi", tree.KindDiskImage},
		{"tar", tree.KindContainerImage},
		{"dmp", tree.KindMemoryState},
		{"pdf", tree.KindOfficeDoc},
		{"png", tree.KindImage},
		{"mp3", tree.KindAudio},
		{"mp4", tree.KindVideo},
		{"log", tree.KindText},
		{"go", tree.KindSource},
		{"json", tree.KindMarkup},
	}
	for _, c := range cases {
		if got := classifyType(c.id); got != c.want {
			t.Errorf("classifyType(%q) = %v, want %v", c.id, got, c.want)
		}
	}
}

func TestScanSetsType(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "disk.vdi"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("world"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "snap.qcow2"), []byte("x"), 0644)

	w := New(Options{Root: dir, Workers: 2})
	n, err := w.Scan()
	if err != nil {
		t.Fatal(err)
	}

	type rec struct {
		typ  string
		kind tree.Kind
	}
	got := map[string]rec{}
	var walk func(*tree.Node)
	walk = func(node *tree.Node) {
		if !node.IsDir {
			got[node.Name] = rec{node.Type, node.Kind}
		}
		for _, c := range node.Children {
			walk(c)
		}
	}
	walk(n)

	if r := got["disk.vdi"]; r.typ != "vdi" || r.kind != tree.KindDiskImage {
		t.Errorf("disk.vdi = %+v, want {vdi disk-image}", r)
	}
	if r := got["notes.txt"]; r.typ != "txt" || r.kind != tree.KindText {
		t.Errorf("notes.txt = %+v, want {txt text}", r)
	}
	if r := got["snap.qcow2"]; r.typ != "qcow2" || r.kind != tree.KindDiskImage {
		t.Errorf("snap.qcow2 = %+v, want {qcow2 disk-image}", r)
	}
}

// TestScanSetsTimestamps checks that modification times are populated on file
// and directory records after a scan.
func TestScanSetsTimestamps(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)

	w := New(Options{Root: dir, Workers: 2})
	n, err := w.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if n.Mtime == 0 {
		t.Error("root dir Mtime is zero")
	}
	for _, c := range n.Children {
		if c.Mtime == 0 {
			t.Errorf("%s Mtime is zero", c.Name)
		}
	}
}

func TestAttrLetters(t *testing.T) {
	cases := []struct {
		attrs uint32
		want  string
	}{
		{0, ""},
		{AttrReadonly, "R"},
		{AttrReadonly | AttrHidden, "R H"},
		{AttrArchive | AttrCompressed, "A C"},
		{AttrReparsePnt, "RP"},
		{AttrSparseFile, "SP"},
	}
	for _, c := range cases {
		if got := AttrLetters(c.attrs); got != c.want {
			t.Errorf("AttrLetters(%#x) = %q, want %q", c.attrs, got, c.want)
		}
	}
}

func TestFilePropsDetectFormat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello world\n"), 0644)

	w := New(Options{Root: dir, Workers: 2, DetectFormat: true})
	ft, enc, le := w.fileProps(filepath.Join(dir, "notes.txt"))
	if ft != "txt" {
		t.Errorf("fileType %q, want txt", ft)
	}
	// RefineText is a stub in v1, returns empty strings
	if enc != "" {
		t.Errorf("encoding %q, want empty (stub)", enc)
	}
	if le != "" {
		t.Errorf("lineEnding %q, want empty (stub)", le)
	}

	// Non-text type under DetectFormat should not call RefineText
	os.WriteFile(filepath.Join(dir, "disk.vdi"), []byte("hello"), 0644)
	ft2, enc2, le2 := w.fileProps(filepath.Join(dir, "disk.vdi"))
	if ft2 != "vdi" {
		t.Errorf("fileType2 %q, want vdi", ft2)
	}
	if enc2 != "" || le2 != "" {
		t.Errorf("encoding/lineEnding %q/%q want empty for non-text", enc2, le2)
	}
}

func TestIsTextType(t *testing.T) {
	cases := []struct {
		fileType string
		want     bool
	}{
		{"txt", true},
		{"log", true},
		{"go", true},
		{"py", true},
		{"html", true},
		{"json", true},
		{"vdi", false},
		{"png", false},
		{"mp3", false},
		{"mp4", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isTextType(c.fileType); got != c.want {
			t.Errorf("isTextType(%q) = %v, want %v", c.fileType, got, c.want)
		}
	}
}

func TestFilePropsNoDetectFormat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0644)

	w := New(Options{Root: dir, Workers: 2, DetectFormat: false})
	ft, enc, le := w.fileProps(filepath.Join(dir, "notes.txt"))
	if ft != "txt" {
		t.Errorf("fileType %q, want txt", ft)
	}
	if enc != "" {
		t.Errorf("encoding %q, want empty (no detect-format)", enc)
	}
	if le != "" {
		t.Errorf("lineEnding %q, want empty (no detect-format)", le)
	}
}
