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

func TestKindOf(t *testing.T) {
	if got := KindOf(&Node{Type: "png"}); got != KindImage {
		t.Errorf("KindOf(type png) = %v, want image", got)
	}
	if got := KindOf(&Node{Kind: KindOther}); got != KindOther {
		t.Errorf("KindOf(set KindOther) = %v, want other", got)
	}
	if got := KindOf(&Node{}); got != KindUnknown {
		t.Errorf("KindOf(zero node) = %v, want unknown", got)
	}
	if got := KindOf(nil); got != KindUnknown {
		t.Errorf("KindOf(nil) = %v, want unknown", got)
	}
}

func TestKindValuesHaveNames(t *testing.T) {
	seen := map[Kind]bool{}
	for _, k := range KindValues() {
		if seen[k] {
			t.Errorf("duplicate Kind value %d in KindValues", k)
		}
		seen[k] = true
		if k != KindUnknown && k.String() == "unknown" {
			t.Errorf("Kind %d has no name", k)
		}
	}
}
