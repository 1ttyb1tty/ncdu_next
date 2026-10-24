package scan

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

type Options struct {
	Root         string                         `doc:"root directory to scan; normalized (C: -> C:\\) before use"`
	Workers      int                            `doc:"parallel stat/readdir workers; concurrency only, no effect on records"`
	CountLinks   bool                           `doc:"when true, repeat hardlinks get Size=0 after the first sighting"`
	OneFS        bool                           `doc:"when true, directories on another device are skipped entirely (no record)"`
	FollowSym    bool                           `doc:"when true, symlinks become their target's record; otherwise a Size=0 stub"`
	Excludes     []string                       `doc:"glob patterns skipped by name before a record is created"`
	DetectFormat bool                           `doc:"when true, populates Encoding and LineEnding on text-like files"`
	Progress     func(files int64, path string) `doc:"progress callback; no effect on records"`
	Ctx          context.Context                `doc:"cancellation; no effect on records"`
}

type Walker struct {
	opts  Options
	sem   chan struct{}
	table []FSInfo
	fs    *FSReport
}

func New(o Options) *Walker {
	if o.Workers <= 0 {
		o.Workers = 4
	}
	return &Walker{opts: o, sem: make(chan struct{}, o.Workers)}
}

// FSReport returns the filesystem summary the last Scan produced, or nil when no
// scan has run. A fresh Walker is used per scan, so the returned pointer is not
// aliased after the scan.
func (w *Walker) FSReport() *FSReport { return w.fs }

func joinKey(dev, ino uint64) string {
	return itoa(dev) + ":" + itoa(ino)
}

func itoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func isExcluded(name string, excludes []string) bool {
	for _, e := range excludes {
		if e == "" {
			continue
		}
		if matched, _ := filepath.Match(e, name); matched {
			return true
		}
	}
	return false
}

// TypeTableStats reports the size of the extension->type table and up to n
// sample keys in sorted order. It exists so the Debug tab can show a live count
// without exporting the map itself.
func TypeTableStats(n int) (int, []string) {
	keys := make([]string, 0, len(fileTypes))
	for k := range fileTypes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if n < 0 {
		n = 0
	}
	if n > len(keys) {
		n = len(keys)
	}
	return len(fileTypes), keys[:n]
}

// fileTypes maps lowercased file extensions to a human-readable type/category.
// Extension heuristics are the fast default path; magic-byte signature
// detection (--detect-format) is the optional secondary fallback.
var fileTypes = map[string]string{
	// VirtualBox
	".vdi":  "vdi",
	".vmdk": "vmdk",
	".vhd":  "vhd",
	".ova":  "ova",
	".ovf":  "ovf",
	".vbox": "vbox",
	// Virtuozzo
	".vzv":   "vzv",
	".vzvhd": "vzvhd",
	".ploop": "ploop",
	// Xen / XCP-ng
	".xva": "xva",
	// Parallels
	".hdd":  "hdd",
	".hdds": "hdds",
	".pvm":  "pvm",
	// UTM / generic
	".qcow2": "qcow2",
	".raw":   "raw",
	".vhdx":  "vhdx",
	// Container images
	".tar": "tar",
	".oci": "oci",
	".iso": "iso",
	// Memory / state saves
	".vmem": "vmem",
	".vmss": "vmss",
	".vmsn": "vmsn",
	".dmp":  "dmp",
	".mem":  "mem",
	// Office / documents
	".doc":  "doc",
	".docx": "docx",
	".xls":  "xls",
	".xlsx": "xlsx",
	".ppt":  "ppt",
	".pptx": "pptx",
	".odt":  "odt",
	".ods":  "ods",
	".odp":  "odp",
	".pdf":  "pdf",
	".rtf":  "rtf",
	".epub": "epub",
	// Images
	".jpg":  "jpg",
	".jpeg": "jpg",
	".png":  "png",
	".gif":  "gif",
	".webp": "webp",
	".bmp":  "bmp",
	".tiff": "tiff",
	".tif":  "tiff",
	".ico":  "ico",
	".heic": "heic",
	".heif": "heic",
	// Audio
	".mp3":  "mp3",
	".wav":  "wav",
	".ogg":  "ogg",
	".flac": "flac",
	".m4a":  "m4a",
	".aac":  "aac",
	".wma":  "wma",
	".opus": "opus",
	".midi": "midi",
	// Video
	".mp4":  "mp4",
	".mkv":  "mkv",
	".avi":  "avi",
	".mov":  "mov",
	".webm": "webm",
	".wmv":  "wmv",
	".flv":  "flv",
	".m4v":  "m4v",
	".mpeg": "mpeg",
	".mpg":  "mpeg",
	// Text
	".txt":  "txt",
	".log":  "log",
	".csv":  "csv",
	".tsv":  "tsv",
	".ini":  "ini",
	".cfg":  "cfg",
	".conf": "conf",
	// Programming
	".go":    "go",
	".py":    "py",
	".js":    "js",
	".ts":    "ts",
	".jsx":   "jsx",
	".tsx":   "tsx",
	".rs":    "rs",
	".c":     "c",
	".cpp":   "cpp",
	".cc":    "cc",
	".cxx":   "cxx",
	".h":     "h",
	".hpp":   "hpp",
	".java":  "java",
	".kt":    "kt",
	".swift": "swift",
	".rb":    "rb",
	".php":   "php",
	".cs":    "cs",
	".sh":    "sh",
	".bash":  "bash",
	".zsh":   "zsh",
	".fish":  "fish",
	".ps1":   "ps1",
	".lua":   "lua",
	".r":     "r",
	".dart":  "dart",
	".scala": "scala",
	".sql":   "sql",
	// Markup
	".html":     "html",
	".htm":      "html",
	".css":      "css",
	".xml":      "xml",
	".yaml":     "yaml",
	".yml":      "yml",
	".json":     "json",
	".toml":     "toml",
	".md":       "md",
	".markdown": "md",
	".rst":      "rst",
	".tex":      "tex",
}

func detectFileType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if t, ok := fileTypes[ext]; ok {
		return t
	}
	return ""
}

func isTextType(fileType string) bool {
	switch tree.KindForType(fileType) {
	case tree.KindText, tree.KindSource, tree.KindMarkup:
		return true
	default:
		return false
	}
}

// classifyType maps a specific Type id to its broad Kind. A real file whose
// extension matched nothing has an empty Type and is KindOther; the zero value
// (KindUnknown) is reserved for entries classification never ran for.
func classifyType(fileType string) tree.Kind {
	if fileType == "" {
		return tree.KindOther
	}
	return tree.KindForType(fileType)
}

// setNodeMeta copies platform timestamps and attributes from a FileInfo onto a
// node. Missing values are left zero (see tree.Node field docs).
func setNodeMeta(n *tree.Node, fi os.FileInfo) {
	if n == nil || fi == nil {
		return
	}
	n.Mtime, n.Ctime, n.Atime, n.Attrs = fileMeta(fi)
}

// assignType classifies a file node: the Type id from the extension table
// (optionally refined under --detect-format), its derived Kind, and the text
// encoding/line-ending when the type is text-like.
func (w *Walker) assignType(n *tree.Node, path string) {
	ft, enc, le := w.fileProps(path)
	n.Type = ft
	n.Kind = classifyType(ft)
	n.Encoding = enc
	n.LineEnding = le
}

// fileProps classifies a file by extension and, when --detect-format is
// enabled, optionally refines the type and detects encoding/line-ending.
func (w *Walker) fileProps(path string) (fileType, encoding, lineEnding string) {
	fileType = detectFileType(path)
	if !w.opts.DetectFormat {
		return fileType, "", ""
	}
	fileType = RefineType(path, fileType)
	if isTextType(fileType) {
		encoding, lineEnding = RefineText(path, fileType)
	}
	return
}

func (w *Walker) Scan() (*tree.Node, error) {
	root, err := filepath.Abs(w.opts.Root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		w.fs = &FSReport{Root: FSInfo{Avail: FSNotApplicable, Reason: "scan root is a regular file"}}
		n := &tree.Node{Name: filepath.Base(root), Path: root, IsDir: false, Size: info.Size(), AppSize: info.Size()}
		w.assignType(n, root)
		setNodeMeta(n, info)
		return n, nil
	}
	// Filesystem detection must never fail a scan: every error degrades to a
	// reported reason, and --one-file-system then fails open (descends) with a
	// single warn from the caller.
	table, err := osMountTableFn()
	if err != nil {
		w.fs = &FSReport{Root: FSInfo{Avail: FSUnavailable, Reason: err.Error()}}
	} else {
		w.table = table
		rootFS, ok := fsFor(root, table)
		if !ok {
			rootFS = FSInfo{Avail: FSUnavailable, Reason: "scan root not covered by the OS mount table"}
		}
		w.fs = &FSReport{Root: rootFS, Mounts: mountsUnder(root, table)}
	}
	rootDev := rootDevice(info, root, w.table)
	rootNode := &tree.Node{Name: filepath.Base(root), Path: root, IsDir: true}
	setNodeMeta(rootNode, info)
	var mu sync.Mutex
	var files int64
	var scanErr error
	var wg sync.WaitGroup
	visited := make(map[string]struct{})
	links := make(map[string]struct{})

	var process func(dir string, node *tree.Node, dev uint64)
	process = func(dir string, node *tree.Node, dev uint64) {
		if w.opts.Ctx != nil {
			select {
			case <-w.opts.Ctx.Done():
				mu.Lock()
				if scanErr == nil {
					scanErr = w.opts.Ctx.Err()
				}
				mu.Unlock()
				return
			default:
			}
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			mu.Lock()
			node.Err = err
			mu.Unlock()
			return
		}
		for _, e := range entries {
			if w.opts.Ctx != nil {
				select {
				case <-w.opts.Ctx.Done():
					mu.Lock()
					if scanErr == nil {
						scanErr = w.opts.Ctx.Err()
					}
					mu.Unlock()
					return
				default:
				}
			}
			name := e.Name()
			if isExcluded(name, w.opts.Excludes) {
				continue
			}
			p := filepath.Join(dir, name)
			fi, err := e.Info()
			if err != nil {
				mu.Lock()
				node.AddChild(&tree.Node{Name: name, Path: p, Err: err})
				mu.Unlock()
				continue
			}
			if fi.IsDir() {
				childDev := dev
				if w.opts.OneFS {
					if ddev, dok := deviceOfEntryFn(fi, p, w.table); dok && ddev != dev {
						continue
					}
				}
				child := &tree.Node{Name: name, Path: p, IsDir: true}
				setNodeMeta(child, fi)
				mu.Lock()
				node.AddChild(child)
				mu.Unlock()
				wg.Add(1)
				go func(d string, n *tree.Node, ddev uint64) {
					w.sem <- struct{}{}
					defer func() { <-w.sem; wg.Done() }()
					process(d, n, ddev)
				}(p, child, childDev)
				continue
			}
			if fi.Mode()&os.ModeSymlink != 0 {
				if !w.opts.FollowSym {
					child := &tree.Node{Name: name, Path: p, IsDir: false, Size: 0, AppSize: 0}
					w.assignType(child, p)
					setNodeMeta(child, fi)
					mu.Lock()
					node.AddChild(child)
					mu.Unlock()
					continue
				}
				key := fileKey(fi)
				mu.Lock()
				if key != "" {
					if _, seen := visited[key]; seen {
						mu.Unlock()
						continue
					}
					visited[key] = struct{}{}
				}
				mu.Unlock()
				target, err := filepath.EvalSymlinks(p)
				if err != nil {
					mu.Lock()
					node.AddChild(&tree.Node{Name: name, Path: p, Err: err})
					mu.Unlock()
					continue
				}
				tfi, err := os.Stat(target)
				if err != nil {
					mu.Lock()
					node.AddChild(&tree.Node{Name: name, Path: p, Err: err})
					mu.Unlock()
					continue
				}
				if tfi.IsDir() {
					child := &tree.Node{Name: name, Path: p, IsDir: true}
					setNodeMeta(child, tfi)
					mu.Lock()
					node.AddChild(child)
					mu.Unlock()
					wg.Add(1)
					go func(d string, n *tree.Node) {
						w.sem <- struct{}{}
						defer func() { <-w.sem; wg.Done() }()
						process(d, n, dev)
					}(target, child)
					continue
				}
				size := tfi.Size()
				if w.opts.CountLinks {
					key := fileKey(tfi)
					mu.Lock()
					if key != "" {
						if _, seen := links[key]; seen {
							size = 0
						} else {
							links[key] = struct{}{}
						}
					}
					mu.Unlock()
				}
				child := &tree.Node{Name: name, Path: p, IsDir: false, Size: size, AppSize: size}
				w.assignType(child, p)
				setNodeMeta(child, tfi)
				mu.Lock()
				node.AddChild(child)
				mu.Unlock()
				mu.Lock()
				files++
				mu.Unlock()
				if w.opts.Progress != nil {
					w.opts.Progress(files, p)
				}
				continue
			}
			size := fi.Size()
			if w.opts.CountLinks {
				key := fileKey(fi)
				mu.Lock()
				if key != "" {
					if _, seen := links[key]; seen {
						size = 0
					} else {
						links[key] = struct{}{}
					}
				}
				mu.Unlock()
			}
			child := &tree.Node{Name: name, Path: p, IsDir: false, Size: size, AppSize: size}
			w.assignType(child, p)
			setNodeMeta(child, fi)
			mu.Lock()
			node.AddChild(child)
			mu.Unlock()
			mu.Lock()
			files++
			mu.Unlock()
			if w.opts.Progress != nil {
				w.opts.Progress(files, p)
			}
		}
	}
	process(root, rootNode, rootDev)
	wg.Wait()
	rootNode.Recompute()
	return rootNode, scanErr
}

// rootDevice returns the scan root's --one-file-system identity, through the
// same source the OneFS check uses for children so the two cannot disagree.
func rootDevice(info os.FileInfo, root string, table []FSInfo) uint64 {
	d, _ := deviceOfEntryFn(info, root, table)
	return d
}
