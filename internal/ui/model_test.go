package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

func TestModelNav(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	child := &tree.Node{Name: "sub", IsDir: true, Path: "/root/sub"}
	root.AddChild(child)
	m := New(root)
	m2, _ := m.Update(tea.KeyPressMsg{Text: "down"})
	_ = m2
	m3, _ := m2.(Model).Update(tea.KeyPressMsg{Text: "enter"})
	mm := m3.(Model)
	if mm.cur != child {
		t.Fatalf("not navigated")
	}
	mm2, _ := mm.Update(tea.KeyPressMsg{Text: "backspace"})
	if mm2.(Model).cur != root {
		t.Fatalf("not up")
	}
	v := mm2.(Model).View()
	if v.Content == "" {
		t.Fatal("empty view")
	}
	mm3, cmd := mm2.(Model).Update(tea.KeyPressMsg{Text: "q"})
	_ = mm3
	_ = cmd
}

func TestModelToggleType(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	child := &tree.Node{Name: "disk.vdi", IsDir: false, Size: 100, Type: "vbox-disk"}
	root.AddChild(child)
	m := New(root)

	if m.expanded {
		t.Fatal("expanded should start false")
	}
	v := m.View()
	if containsType(v.Content) {
		t.Fatal("minimal view should not show type")
	}

	m2, _ := m.Update(tea.KeyPressMsg{Text: "e"})
	mm := m2.(Model)
	if !mm.expanded {
		t.Fatal("expanded should be true after e")
	}
	v2 := mm.View()
	if !containsType(v2.Content) {
		t.Fatal("expanded view should show type")
	}

	m3, _ := mm.Update(tea.KeyPressMsg{Text: "e"})
	mm2 := m3.(Model)
	if mm2.expanded {
		t.Fatal("expanded should be false after second e")
	}
}

func containsType(s string) bool {
	return strings.Contains(s, "vbox-disk")
}
