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
		{"disk.vdi", "vbox-disk"},
		{"disk.VMDK", "vbox-disk"},
		{"disk.vmdk", "vbox-disk"},
		{"disk.vhd", "vbox-disk"},
		{"disk.ova", "vbox-ova"},
		{"disk.ovf", "vbox-ova"},
		{"machine.vbox", "vbox-vm"},
		{"disk.vzv", "vz-disk"},
		{"disk.vzvhd", "vz-disk"},
		{"disk.ploop", "vz-disk"},
		{"disk.xva", "xen-disk"},
		{"disk.hdd", "parallels-disk"},
		{"disk.hdds", "parallels-disk"},
		{"vm.pvm", "parallels-vm"},
		{"disk.qcow2", "utm-disk"},
		{"disk.raw", "utm-disk"},
		{"disk.vhdx", "utm-disk"},
		{"image.tar", "container-image"},
		{"image.oci", "container-image"},
		{"image.iso", "container-image"},
		{"dump.vmem", "memory-state"},
		{"snapshot.vmss", "memory-state"},
		{"state.vmsn", "memory-state"},
		{"crash.dmp", "memory-state"},
		{"mem.mem", "memory-state"},
		{"document.txt", "text-plain"},
		{"image.img", ""},
		{"noext", ""},
	}
	for _, c := range cases {
		if got := detectFileType(c.path); got != c.want {
			t.Errorf("detectFileType(%q) = %q, want %q", c.path, got, c.want)
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

	got := map[string]string{}
	var walk func(*tree.Node)
	walk = func(node *tree.Node) {
		if !node.IsDir {
			got[node.Name] = node.Type
		}
		for _, c := range node.Children {
			walk(c)
		}
	}
	walk(n)

	if got["disk.vdi"] != "vbox-disk" {
		t.Errorf("disk.vdi type = %q, want vbox-disk", got["disk.vdi"])
	}
	if got["notes.txt"] != "text-plain" {
		t.Errorf("notes.txt type = %q, want text-plain", got["notes.txt"])
	}
	if got["snap.qcow2"] != "utm-disk" {
		t.Errorf("snap.qcow2 type = %q, want utm-disk", got["snap.qcow2"])
	}
}

func TestFilePropsDetectFormat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello world\n"), 0644)

	w := New(Options{Root: dir, Workers: 2, DetectFormat: true})
	ft, enc, le := w.fileProps(filepath.Join(dir, "notes.txt"))
	if ft != "text-plain" {
		t.Errorf("fileType %q, want text-plain", ft)
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
	if ft2 != "vbox-disk" {
		t.Errorf("fileType2 %q, want vbox-disk", ft2)
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
		{"text-plain", true},
		{"text-log", true},
		{"source-go", true},
		{"source-python", true},
		{"markup-html", true},
		{"markup-json", true},
		{"vbox-disk", false},
		{"image-png", false},
		{"audio-mp3", false},
		{"video-mp4", false},
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
	if ft != "text-plain" {
		t.Errorf("fileType %q, want text-plain", ft)
	}
	if enc != "" {
		t.Errorf("encoding %q, want empty (no detect-format)", enc)
	}
	if le != "" {
		t.Errorf("lineEnding %q, want empty (no detect-format)", le)
	}
}
