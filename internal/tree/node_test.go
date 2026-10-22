package tree

import (
	"testing"
)

func TestRecompute(t *testing.T) {
	root := &Node{Name: "root", IsDir: true}
	child := &Node{Name: "f", IsDir: false, Size: 10, AppSize: 10}
	root.AddChild(child)
	root.Recompute()
	if root.Size != 10 {
		t.Fatalf("got %d", root.Size)
	}
}

func TestSortBySize(t *testing.T) {
	root := &Node{Name: "root", IsDir: true}
	root.AddChild(&Node{Name: "a", IsDir: false, Size: 5})
	root.AddChild(&Node{Name: "b", IsDir: false, Size: 10})
	SortBySize(root, true)
	if root.Children[0].Name != "b" {
		t.Fatalf("sort wrong")
	}
}

func TestSortByName(t *testing.T) {
	root := &Node{Name: "root", IsDir: true}
	root.AddChild(&Node{Name: "c", IsDir: false, Size: 5})
	root.AddChild(&Node{Name: "a", IsDir: false, Size: 10})
	root.AddChild(&Node{Name: "b", IsDir: false, Size: 7})
	SortByName(root)
	if root.Children[0].Name != "a" || root.Children[2].Name != "c" {
		t.Fatalf("sort wrong")
	}
}
