package scan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

type Options struct {
	Root         string
	Workers      int
	CountLinks   bool
	OneFS        bool
	FollowSym    bool
	Excludes     []string
	DetectFormat bool
	Progress     func(files int64, path string)
	Ctx          context.Context
}

type Walker struct {
	opts Options
	sem  chan struct{}
}

func New(o Options) *Walker {
	if o.Workers <= 0 {
		o.Workers = 4
	}
	return &Walker{opts: o, sem: make(chan struct{}, o.Workers)}
}

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

// fileTypes maps lowercased file extensions to a human-readable type/category.
// Extension heuristics are the fast default path; magic-byte signature
// detection (--detect-format) is the optional secondary fallback.
var fileTypes = map[string]string{
	// VirtualBox
	".vdi":  "vbox-disk",
	".vmdk": "vbox-disk",
	".vhd":  "vbox-disk",
	".ova":  "vbox-ova",
	".ovf":  "vbox-ova",
	".vbox": "vbox-vm",
	// Virtuozzo
	".vzv":   "vz-disk",
	".vzvhd": "vz-disk",
	".ploop": "vz-disk",
	// Xen / XCP-ng
	".xva": "xen-disk",
	// Parallels
	".hdd":  "parallels-disk",
	".hdds": "parallels-disk",
	".pvm":  "parallels-vm",
	// UTM / generic
	".qcow2": "utm-disk",
	".raw":   "utm-disk",
	".vhdx":  "utm-disk",
	// Container images
	".tar": "container-image",
	".oci": "container-image",
	".iso": "container-image",
	// Memory / state saves
	".vmem": "memory-state",
	".vmss": "memory-state",
	".vmsn": "memory-state",
	".dmp":  "memory-state",
	".mem":  "memory-state",
}

func detectFileType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if t, ok := fileTypes[ext]; ok {
		return t
	}
	return ""
}

// fileType classifies a file by extension, optionally refined by magic-byte
// signature detection when --detect-format is enabled.
func (w *Walker) fileType(path string) string {
	t := detectFileType(path)
	if w.opts.DetectFormat {
		t = RefineType(path, t)
	}
	return t
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
		n := &tree.Node{Name: filepath.Base(root), Path: root, IsDir: false, Size: info.Size(), AppSize: info.Size(), Type: w.fileType(root)}
		return n, nil
	}
	rootDev := rootDevice(info)
	rootNode := &tree.Node{Name: filepath.Base(root), Path: root, IsDir: true}
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
					if ddev, dok := deviceOf(fi); dok && ddev != dev {
						continue
					}
				}
				child := &tree.Node{Name: name, Path: p, IsDir: true}
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
				child := &tree.Node{Name: name, Path: p, IsDir: false, Size: size, AppSize: size, Type: w.fileType(p)}
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
			child := &tree.Node{Name: name, Path: p, IsDir: false, Size: size, AppSize: size, Type: w.fileType(p)}
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

func rootDevice(info os.FileInfo) uint64 {
	d, _ := deviceOf(info)
	return d
}
