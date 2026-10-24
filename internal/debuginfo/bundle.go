// Package debuginfo writes the one-file bug-report bundle: what the binary is,
// what it was asked to do, and what each tab holds. It is stdlib-only and never
// imports internal/ui; the package text and records are inputs, so the CLI and
// the TUI share one writer.
package debuginfo

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/1ttyb1tty/ncdu-next/internal/config"
	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

// fixedModTime is stamped on every zip entry so two bundles written from the
// same Input are byte-identical (a real timestamp would differ).
var fixedModTime = time.Unix(0, 0).UTC()

// TabRecord is one tab's state, in tab order.
type TabRecord struct {
	Index        int    `json:"index"`
	Kind         string `json:"kind"`
	Status       string `json:"status"`
	Root         string `json:"root"`
	NodeCount    int    `json:"node_count"`
	EstMemBytes  uint64 `json:"est_mem_bytes"`
	Unit         string `json:"unit"`
	Expanded     bool   `json:"expanded"`
	DetectFormat bool   `json:"detect_format"`
	FilterScope  string `json:"filter_scope"`
	FilterRaw    string `json:"filter_raw"`
	FilterError  string `json:"filter_error"`
	SearchActive bool   `json:"search_active"`
	SearchRaw    string `json:"search_raw"`
	SearchCount  int    `json:"search_result_count"`
}

// MountRecord is one filesystem mount observed by one tab.
type MountRecord struct {
	TabIndex   int    `json:"tab_index"`
	MountPoint string `json:"mount_point"`
	FSType     string `json:"fstype"`
	Avail      string `json:"avail"`
	Device     uint64 `json:"device"`
	Reason     string `json:"reason"`
}

// TabTree pairs a tab index with its tree and the tab's node count. The count is
// carried in (computed once at scan time) so the summary does not re-walk the
// tree and cannot disagree with tabs.jsonl.
type TabTree struct {
	TabIndex  int
	Tree      *tree.Node
	NodeCount int
}

// Input is everything Write needs. Now, BuildInfo and Runtime are inputs rather
// than read inside Write, so a test can produce a byte-exact bundle.
type Input struct {
	Options     config.Config
	Tabs        []TabRecord
	Mounts      []MountRecord
	DebugPage   string
	Trees       []TabTree
	IncludeTree bool
	BuildInfo   string
	Runtime     string
	Now         time.Time
}

type file struct {
	name string
	data []byte
}

// DefaultName returns the bundle filename the TUI uses, stamped in UTC.
func DefaultName(now time.Time) string {
	return "ncdu-next-debuginfo-" + now.UTC().Format("20060102T150405") + ".zip"
}

// BuildInfoText renders runtime/debug.ReadBuildInfo() as a stable text block.
// The result is passed in as Input.BuildInfo so Write stays deterministic.
func BuildInfoText() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "(no build info)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "module %s\n", bi.Main.Path)
	fmt.Fprintf(&b, "version %s\n", bi.Main.Version)
	fmt.Fprintf(&b, "go %s\n", bi.GoVersion)
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision", "vcs.time", "vcs.modified":
			fmt.Fprintf(&b, "%s %s\n", s.Key, s.Value)
		}
	}
	return b.String()
}

// Write assembles and writes the bundle to dest. Entries are written in a fixed
// order with a fixed modification time, and the manifest records the entry list
// actually written.
func Write(dest string, in Input) error {
	files, err := buildFiles(in)
	if err != nil {
		return err
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for _, fl := range files {
		hdr := &zip.FileHeader{Name: fl.name, Method: zip.Deflate}
		hdr.Modified = fixedModTime
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			zw.Close() // nolint:errcheck
			return err
		}
		if _, err := w.Write(fl.data); err != nil {
			zw.Close() // nolint:errcheck
			return err
		}
	}
	return zw.Close()
}

// entryNames is the fixed entry order; tree.jsonl is the only optional entry.
func entryNames(includeTree bool) []string {
	names := []string{"README.txt", "manifest.json", "debug-tab.txt", "tabs.jsonl", "fs.jsonl", "tree-summary.json"}
	if includeTree {
		names = append(names, "tree.jsonl")
	}
	return names
}

func buildFiles(in Input) ([]file, error) {
	names := entryNames(in.IncludeTree)

	tabsData, err := jsonlBytes(in.Tabs)
	if err != nil {
		return nil, err
	}
	fsData, err := jsonlBytes(in.Mounts)
	if err != nil {
		return nil, err
	}
	summary, err := summaryJSON(in.Trees)
	if err != nil {
		return nil, err
	}
	manifest, err := manifestJSON(in, names)
	if err != nil {
		return nil, err
	}

	files := []file{
		{"README.txt", []byte(readmeText(in.IncludeTree))},
		{"manifest.json", manifest},
		{"debug-tab.txt", []byte(in.DebugPage)},
		{"tabs.jsonl", tabsData},
		{"fs.jsonl", fsData},
		{"tree-summary.json", summary},
	}
	if in.IncludeTree {
		dump, err := treeJSONL(in.Trees)
		if err != nil {
			return nil, err
		}
		files = append(files, file{"tree.jsonl", dump})
	}
	return files, nil
}

// jsonlBytes marshals each record on its own line. An empty slice yields an
// empty (valid) file.
func jsonlBytes(v any) ([]byte, error) {
	switch recs := v.(type) {
	case []TabRecord:
		var b strings.Builder
		for _, r := range recs {
			line, err := json.Marshal(r)
			if err != nil {
				return nil, err
			}
			b.Write(line)
			b.WriteByte('\n')
		}
		return []byte(b.String()), nil
	case []MountRecord:
		var b strings.Builder
		for _, r := range recs {
			line, err := json.Marshal(r)
			if err != nil {
				return nil, err
			}
			b.Write(line)
			b.WriteByte('\n')
		}
		return []byte(b.String()), nil
	default:
		return nil, fmt.Errorf("debuginfo: unsupported jsonl type %T", v)
	}
}

// TreeEntry is one entry in a tab's top-N by size.
type TreeEntry struct {
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir"`
}

// TreeSummary is one tab's bounded size summary.
type TreeSummary struct {
	TabIndex  int         `json:"tab_index"`
	NodeCount int         `json:"node_count"`
	TotalSize int64       `json:"total_size"`
	Top       []TreeEntry `json:"top"`
}

// topN is how many entries each summary keeps.
const topN = 20

func summaryJSON(trees []TabTree) ([]byte, error) {
	out := make([]TreeSummary, 0, len(trees))
	for _, tt := range trees {
		s := TreeSummary{TabIndex: tt.TabIndex, NodeCount: tt.NodeCount}
		if tt.Tree != nil {
			s.TotalSize = tt.Tree.Size
			s.Top = topEntries(tt.Tree, topN)
		}
		out = append(out, s)
	}
	return json.MarshalIndent(out, "", "  ")
}

// topEntries returns up to n entries of the tree (excluding the root) by size
// descending, breaking ties by path ascending so equal sizes cannot reorder
// between runs. It keeps a bounded best-of-n set during a single walk, so time is
// O(nodes·n) and working memory O(n) rather than materialising and sorting every
// node.
func topEntries(root *tree.Node, n int) []TreeEntry {
	if n <= 0 || root == nil {
		return nil
	}
	top := make([]TreeEntry, 0, n)
	worst := 0
	add := func(e TreeEntry) {
		if len(top) < n {
			top = append(top, e)
			if len(top) == n {
				worst = worstIndex(top)
			}
			return
		}
		if betterEntry(e, top[worst]) {
			top[worst] = e
			worst = worstIndex(top)
		}
	}
	var walk func(*tree.Node)
	walk = func(node *tree.Node) {
		for _, c := range node.Children {
			add(TreeEntry{Path: nodePath(c), Size: c.Size, IsDir: c.IsDir})
			walk(c)
		}
	}
	walk(root)
	sort.Slice(top, func(i, j int) bool { return betterEntry(top[i], top[j]) })
	return top
}

// betterEntry reports whether a ranks before b: larger size, then path ascending.
func betterEntry(a, b TreeEntry) bool {
	if a.Size != b.Size {
		return a.Size > b.Size
	}
	return a.Path < b.Path
}

// worstIndex returns the index of the entry that ranks last under betterEntry.
func worstIndex(ts []TreeEntry) int {
	w := 0
	for i := 1; i < len(ts); i++ {
		if betterEntry(ts[w], ts[i]) {
			w = i
		}
	}
	return w
}

// TreeRecord is one node in the opt-in tree dump.
type TreeRecord struct {
	TabIndex int    `json:"tab_index"`
	Path     string `json:"path"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	AppSize  int64  `json:"app_size"`
	IsDir    bool   `json:"is_dir"`
	Kind     string `json:"kind"`
	Type     string `json:"type"`
	Mtime    int64  `json:"mtime"`
	Attrs    uint32 `json:"attrs"`
}

func treeJSONL(trees []TabTree) ([]byte, error) {
	var b strings.Builder
	for _, tt := range trees {
		if tt.Tree == nil {
			continue
		}
		var walk func(*tree.Node) error
		walk = func(node *tree.Node) error {
			rec := TreeRecord{
				TabIndex: tt.TabIndex,
				Path:     nodePath(node),
				Name:     node.Name,
				Size:     node.Size,
				AppSize:  node.AppSize,
				IsDir:    node.IsDir,
				Kind:     tree.KindOf(node).String(),
				Type:     node.Type,
				Mtime:    node.Mtime,
				Attrs:    node.Attrs,
			}
			line, err := json.Marshal(rec)
			if err != nil {
				return err
			}
			b.Write(line)
			b.WriteByte('\n')
			for _, c := range node.Children {
				if err := walk(c); err != nil {
					return err
				}
			}
			return nil
		}
		if err := walk(tt.Tree); err != nil {
			return nil, err
		}
	}
	return []byte(b.String()), nil
}

func nodePath(n *tree.Node) string {
	if n == nil {
		return ""
	}
	if n.Path != "" {
		return n.Path
	}
	return n.Name
}

// manifest is the bundle's machine-readable description of itself.
type manifest struct {
	BuildInfo   string       `json:"build_info"`
	Runtime     string       `json:"runtime"`
	GoVersion   string       `json:"go_version"`
	GOOS        string       `json:"goos"`
	GOARCH      string       `json:"goarch"`
	NumCPU      int          `json:"num_cpu"`
	GeneratedAt string       `json:"generated_at"`
	Options     optionRecord `json:"options"`
	Entries     []string     `json:"entries"`
}

type optionRecord struct {
	Root         string   `json:"root"`
	Workers      int      `json:"workers"`
	Excludes     []string `json:"excludes"`
	CountLinks   bool     `json:"count_hardlinks"`
	OneFS        bool     `json:"one_file_system"`
	FollowSym    bool     `json:"follow_symlinks"`
	DetectFormat bool     `json:"detect_format"`
	Debug        bool     `json:"debug"`
	Trash        bool     `json:"trash"`
	LogFile      string   `json:"log_file"`
}

func manifestJSON(in Input, names []string) ([]byte, error) {
	m := manifest{
		BuildInfo:   in.BuildInfo,
		Runtime:     in.Runtime,
		GoVersion:   runtime.Version(),
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
		NumCPU:      runtime.NumCPU(),
		GeneratedAt: in.Now.UTC().Format(time.RFC3339),
		Options: optionRecord{
			Root:         in.Options.Root,
			Workers:      in.Options.Workers,
			Excludes:     in.Options.Excludes,
			CountLinks:   in.Options.CountLinks,
			OneFS:        in.Options.OneFS,
			FollowSym:    in.Options.FollowSym,
			DetectFormat: in.Options.DetectFormat,
			Debug:        in.Options.Debug,
			Trash:        in.Options.Trash,
			LogFile:      in.Options.LogFile,
		},
		Entries: append([]string(nil), names...),
	}
	return json.MarshalIndent(m, "", "  ")
}

// readmeText explains the bundle. It states the privacy boundary explicitly.
func readmeText(includeTree bool) string {
	treeNote := "This bundle does NOT contain the full file list (the --debuginfo-tree switch was not set)."
	if includeTree {
		treeNote = "This bundle DOES contain the full file list in tree.jsonl (the tree switch was set)."
	}
	return "ncdu-next debuginfo bundle\n" +
		"============================\n\n" +
		"One artifact to attach to a bug report.\n\n" +
		"Entries:\n" +
		"  README.txt         this file\n" +
		"  manifest.json      build metadata, GOOS/GOARCH, the effective options,\n" +
		"                     and the list of entries actually written\n" +
		"  debug-tab.txt      the full two-zone Debug page, verbatim\n" +
		"  tabs.jsonl         one object per tab: status, root, counts, filter/search state\n" +
		"  fs.jsonl           one object per filesystem mount across all tabs\n" +
		"  tree-summary.json  per tab: node count, total size, and the top 20 entries\n" +
		"  tree.jsonl         the full tree (opt-in only, one node per line)\n\n" +
		"Privacy: this bundle contains ABSOLUTE PATHS. " + treeNote + "\n"
}
