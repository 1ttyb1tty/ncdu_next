package tree

import (
	"path/filepath"
	"sort"
)

// Node is the per-entry record for both files and directories. The struct is
// shared; IsDir selects the mode, and the meaning of Children/Parent/Size
// differs by mode. The doc/file/dir struct tags are read by the Debug tab's
// record reference (internal/ui/recinfo.go) and guarded by tests there.
type Node struct {
	Name       string  `doc:"base name of the entry" file:"set" dir:"set"`
	Path       string  `doc:"full path from the scan root" file:"set" dir:"set"`
	IsDir      bool    `doc:"selects directory vs file mode" file:"false" dir:"true"`
	Size       int64   `doc:"bytes on disk; for a directory, the recomputed subtree total" file:"bytes" dir:"subtree"`
	AppSize    int64   `doc:"sum of apparent sizes" file:"bytes" dir:"sum"`
	Type       string  `doc:"enum-like type id (see enums)" file:"set" dir:"set"`
	Encoding   string  `doc:"text encoding, populated under --detect-format" file:"maybe" dir:"maybe"`
	LineEnding string  `doc:"crlf|lf|cr|mixed, populated under --detect-format" file:"maybe" dir:"maybe"`
	Children   []*Node `doc:"child records" file:"nil" dir:"array"`
	Parent     *Node   `doc:"owning directory" file:"set" dir:"set|nil"`
	Err        error   `doc:"stat/read error for this entry" file:"maybe" dir:"maybe"`
}

func (n *Node) AddChild(c *Node) {
	c.Parent = n
	n.Children = append(n.Children, c)
}

func (n *Node) Recompute() {
	if !n.IsDir {
		return
	}
	var total int64
	for _, c := range n.Children {
		c.Recompute()
		total += c.Size
	}
	n.Size = total
	n.AppSize = total
}

func SortBySize(n *Node, desc bool) {
	if n == nil || !n.IsDir {
		return
	}
	sort.Slice(n.Children, func(i, j int) bool {
		if desc {
			return n.Children[i].Size > n.Children[j].Size
		}
		return n.Children[i].Size < n.Children[j].Size
	})
	for _, c := range n.Children {
		SortBySize(c, desc)
	}
}

func SortByName(n *Node) {
	if n == nil || !n.IsDir {
		return
	}
	sort.Slice(n.Children, func(i, j int) bool {
		return n.Children[i].Name < n.Children[j].Name
	})
	for _, c := range n.Children {
		SortByName(c)
	}
}

func (n *Node) Delete() {
	if n.Parent == nil {
		return
	}
	for i, c := range n.Parent.Children {
		if c == n {
			n.Parent.Children = append(n.Parent.Children[:i], n.Parent.Children[i+1:]...)
			break
		}
	}
	cur := n.Parent
	for cur != nil {
		cur.Recompute()
		cur = cur.Parent
	}
}

func BaseName(p string) string { return filepath.Base(p) }
