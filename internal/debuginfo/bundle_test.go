package debuginfo

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1ttyb1tty/ncdu-next/internal/config"
	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

func pinnedNode() *tree.Node {
	root := &tree.Node{Name: "root", Path: "/root", IsDir: true}
	// Two equal sizes so the tie-break is exercised: path ascending must decide.
	root.AddChild(&tree.Node{Name: "b", Path: "/root/b", Size: 10})
	root.AddChild(&tree.Node{Name: "a", Path: "/root/a", Size: 10})
	root.AddChild(&tree.Node{Name: "c", Path: "/root/c", Size: 5})
	root.Recompute()
	return root
}

func pinnedInput() Input {
	return Input{
		Options:   config.Config{Root: "/root", Workers: 4},
		Tabs:      []TabRecord{{Index: 1, Kind: "nav", Status: "done", Root: "/root", NodeCount: 4}},
		Mounts:    []MountRecord{{TabIndex: 1, MountPoint: "/", FSType: "ext4", Avail: "available"}},
		DebugPage: "page text\n",
		Trees:     []TabTree{{TabIndex: 1, Tree: pinnedNode(), NodeCount: 4}},
		BuildInfo: "module example\nversion (devel)\n",
		Runtime:   "alloc 1 B\n",
		Now:       time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func writeTemp(t *testing.T, in Input) string {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "bundle.zip")
	if err := Write(dest, in); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return dest
}

func readEntries(t *testing.T, path string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	out := map[string][]byte{}
	for _, f := range zr.File {
		if !f.Modified.Equal(fixedModTime) {
			t.Errorf("entry %s Modified = %v, want %v", f.Name, f.Modified, fixedModTime)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open entry %s: %v", f.Name, err)
		}
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatalf("read entry %s: %v", f.Name, err)
		}
		rc.Close()
		out[f.Name] = buf.Bytes()
	}
	return out
}

func TestWriteDeterministic(t *testing.T) {
	in := pinnedInput()
	first, err := os.ReadFile(writeTemp(t, in))
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(writeTemp(t, in))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("two Write calls with the same pinned Input differ")
	}
}

func TestDefaultEntrySetHasNoTree(t *testing.T) {
	got := readEntries(t, writeTemp(t, pinnedInput()))
	want := []string{"manifest.json", "README.txt", "debug-tab.txt", "tabs.jsonl", "fs.jsonl", "tree-summary.json"}
	if len(got) != len(want) {
		t.Fatalf("entries = %d, want %d (%v)", len(got), len(want), keys(got))
	}
	for _, name := range want {
		if _, ok := got[name]; !ok {
			t.Errorf("missing entry %s", name)
		}
	}
	if _, ok := got["tree.jsonl"]; ok {
		t.Error("default bundle must not include tree.jsonl")
	}
}

func TestTreeSwitchAddsTree(t *testing.T) {
	in := pinnedInput()
	in.IncludeTree = true
	got := readEntries(t, writeTemp(t, in))
	if _, ok := got["tree.jsonl"]; !ok {
		t.Fatal("tree switch must add tree.jsonl")
	}
	// One line per node, in depth-first order, each a valid object with tab_index.
	lines := strings.Split(strings.TrimSpace(string(got["tree.jsonl"])), "\n")
	if len(lines) != 4 {
		t.Fatalf("tree.jsonl has %d lines, want 4 (root + 3 children)", len(lines))
	}
	for i, ln := range lines {
		var rec TreeRecord
		if err := json.Unmarshal([]byte(ln), &rec); err != nil {
			t.Fatalf("line %d: %v", i, err)
		}
		if rec.TabIndex != 1 {
			t.Errorf("line %d: tab_index = %d, want 1", i, rec.TabIndex)
		}
	}
}

func TestEveryJSONLLineParses(t *testing.T) {
	in := pinnedInput()
	in.IncludeTree = true
	got := readEntries(t, writeTemp(t, in))
	for _, name := range []string{"tabs.jsonl", "fs.jsonl", "tree.jsonl"} {
		data := strings.TrimSpace(string(got[name]))
		if data == "" {
			t.Fatalf("%s is empty", name)
		}
		for i, ln := range strings.Split(data, "\n") {
			var v any
			if err := json.Unmarshal([]byte(ln), &v); err != nil {
				t.Errorf("%s line %d: %v", name, i, err)
			}
		}
	}
	// manifest.json must parse and record the entries actually written.
	var m map[string]any
	if err := json.Unmarshal(got["manifest.json"], &m); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	entries, ok := m["entries"].([]any)
	if !ok {
		t.Fatalf("manifest entries missing: %v", m["entries"])
	}
	if len(entries) != len(got) {
		t.Errorf("manifest lists %d entries, zip has %d", len(entries), len(got))
	}
}

func TestTreeSummaryTieBreak(t *testing.T) {
	got := readEntries(t, writeTemp(t, pinnedInput()))
	var summaries []TreeSummary
	if err := json.Unmarshal(got["tree-summary.json"], &summaries); err != nil {
		t.Fatalf("summary: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summaries = %d, want 1", len(summaries))
	}
	// The summary count is the caller's, so it cannot disagree with tabs.jsonl.
	if summaries[0].NodeCount != 4 {
		t.Errorf("summary node_count = %d, want 4", summaries[0].NodeCount)
	}
	top := summaries[0].Top
	if len(top) != 3 {
		t.Fatalf("top = %d, want 3", len(top))
	}
	// Equal sizes (10, 10) must order by path ascending; the smallest last.
	wantPaths := []string{"/root/a", "/root/b", "/root/c"}
	for i, want := range wantPaths {
		if top[i].Path != want {
			t.Errorf("top[%d].Path = %q, want %q", i, top[i].Path, want)
		}
	}
}

func TestBuildRendering(t *testing.T) {
	stamped := Build{
		Module: "github.com/x/y", Version: "v1.2.3", GoVersion: "go1.26.0",
		GOOS: "windows", GOARCH: "amd64",
		Revision: "0123456789abcdef", VCSTime: "2026-01-02T03:04:05Z",
		VCSModified: true, HasVCS: true, HasInfo: true,
	}
	vs := stamped.VersionString()
	for _, want := range []string{
		"github.com/x/y v1.2.3",
		"revision 0123456789ab (modified) 2026-01-02T03:04:05Z",
		"go1.26.0 windows/amd64",
	} {
		if !strings.Contains(vs, want) {
			t.Errorf("VersionString missing %q:\n%s", want, vs)
		}
	}
	if dl := stamped.DebugLine(); !strings.Contains(dl, "0123456789ab (modified)") || !strings.Contains(dl, "windows/amd64") {
		t.Errorf("DebugLine = %q", dl)
	}
	js, err := stamped.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	for _, want := range []string{`"revision": "0123456789abcdef"`, `"goos": "windows"`, `"vcs_modified": true`, `"has_vcs": true`} {
		if !strings.Contains(js, want) {
			t.Errorf("JSON missing %q:\n%s", want, js)
		}
	}

	// A build with no VCS stamp (tarball, -buildvcs=false) must degrade, not fail.
	bare := Build{GoVersion: "go1.26.0", GOOS: "linux", GOARCH: "arm64"}
	if got := bare.RevisionText(); got != "(no VCS metadata)" {
		t.Errorf("bare RevisionText = %q", got)
	}
	bvs := bare.VersionString()
	if !strings.Contains(bvs, "revision (no VCS metadata)") || !strings.Contains(bvs, "linux/arm64") {
		t.Errorf("bare VersionString = %q", bvs)
	}
	if !strings.Contains(bare.VersionString(), "ncdu-next (unknown)") {
		t.Errorf("bare VersionString should fall back to ncdu-next (unknown): %q", bvs)
	}
}

func TestDefaultName(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if got, want := DefaultName(now), "ncdu-next-debuginfo-20260102T030405.zip"; got != want {
		t.Errorf("DefaultName = %q, want %q", got, want)
	}
}

func TestTopEntriesBoundedAndOrdered(t *testing.T) {
	root := &tree.Node{Name: "root", Path: "/root", IsDir: true}
	for i := 0; i < 100; i++ {
		root.AddChild(&tree.Node{Name: fmt.Sprintf("f%03d", i), Path: fmt.Sprintf("/root/f%03d", i), Size: int64(i)})
	}
	got := topEntries(root, 20)
	if len(got) != 20 {
		t.Fatalf("got %d entries, want 20", len(got))
	}
	if got[0].Size != 99 || got[19].Size != 80 {
		t.Fatalf("top sizes = %d..%d, want 99..80", got[0].Size, got[19].Size)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Size < got[i].Size {
			t.Fatalf("not size-descending at %d: %v", i, got)
		}
	}
	// Fewer nodes than n must return all of them, ordered.
	small := &tree.Node{Name: "r", Path: "/r", IsDir: true}
	small.AddChild(&tree.Node{Name: "a", Path: "/r/a", Size: 1})
	small.AddChild(&tree.Node{Name: "b", Path: "/r/b", Size: 2})
	if got := topEntries(small, 20); len(got) != 2 || got[0].Path != "/r/b" {
		t.Fatalf("small tree top = %v, want [/r/b /r/a]", got)
	}
	if got := topEntries(root, 0); got != nil {
		t.Fatalf("n=0 must yield nothing, got %v", got)
	}
}

func TestReadmeMentionsTreeBoundary(t *testing.T) {
	def := readEntries(t, writeTemp(t, pinnedInput()))
	if !strings.Contains(string(def["README.txt"]), "does NOT contain the full file list") {
		t.Error("default README must state the tree is absent")
	}
	in := pinnedInput()
	in.IncludeTree = true
	full := readEntries(t, writeTemp(t, in))
	if !strings.Contains(string(full["README.txt"]), "DOES contain the full file list") {
		t.Error("tree-enabled README must state the tree is present")
	}
}

func keys(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
