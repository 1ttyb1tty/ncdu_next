package scan

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

type Options struct {
	Root       string
	Workers    int
	CountLinks bool
	OneFS      bool
	FollowSym  bool
	Excludes   []string
	// Progress is invoked with the number of files seen and the current path.
	Progress func(files int64, path string)
	// Context enables cancellation; nil means no cancellation.
	Ctx context.Context
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

func fileKey(fi os.FileInfo) string {
	s, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	return joinKey(s.Dev, s.Ino)
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
		n := &tree.Node{Name: filepath.Base(root), Path: root, IsDir: false, Size: info.Size(), AppSize: info.Size()}
		return n, nil
	}
	rootDev := rootDevice(info)
	rootNode := &tree.Node{Name: filepath.Base(root), Path: root, IsDir: true}
	var mu sync.Mutex
	var files int64
	var wg sync.WaitGroup
	visited := make(map[string]struct{})
	links := make(map[string]struct{})

	var process func(dir string, node *tree.Node, dev uint64)
	process = func(dir string, node *tree.Node, dev uint64) {
		if w.opts.Ctx != nil {
			select {
			case <-w.opts.Ctx.Done():
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
					di, ok := fi.Sys().(*syscall.Stat_t)
					if ok && di.Dev != dev {
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
				child := &tree.Node{Name: name, Path: p, IsDir: false, Size: size, AppSize: size}
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
	return rootNode, nil
}

func rootDevice(info os.FileInfo) uint64 {
	s, ok := info.Sys().(*syscall.Stat_t)
	if ok {
		return s.Dev
	}
	return 0
}

// ensure fs import used
var _ = fs.ValidPath
