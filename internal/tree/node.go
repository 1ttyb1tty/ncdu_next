package tree

import (
	"path/filepath"
	"sort"
)

type Node struct {
	Name       string
	Path       string
	IsDir      bool
	Size       int64
	AppSize    int64
	Type       string
	Encoding   string
	LineEnding string
	Children   []*Node
	Parent     *Node
	Err        error
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
