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
	m := New("/root", root)
	m2, _ := m.Update(tea.KeyPressMsg{Text: "down"})
	_ = m2
	mm := m2.(Model)
	m3, _ := mm.Update(tea.KeyPressMsg{Text: "enter"})
	mm2 := m3.(Model)
	if mm2.activeTab().cur != child {
		t.Fatalf("not navigated")
	}
	mm3, _ := mm2.Update(tea.KeyPressMsg{Text: "backspace"})
	if mm3.(Model).activeTab().cur != root {
		t.Fatalf("not up")
	}
	v := mm3.(Model).View()
	if v.Content == "" {
		t.Fatal("empty view")
	}
	mm4, _ := mm3.(Model).Update(tea.KeyPressMsg{Text: "q"})
	_ = mm4
}

func TestModelToggleType(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	child := &tree.Node{Name: "disk.vdi", IsDir: false, Size: 100, Type: "vbox-disk"}
	root.AddChild(child)
	m := New("/root", root)

	if m.activeTab().expanded {
		t.Fatal("expanded should start false")
	}
	v := m.View()
	if containsType(v.Content) {
		t.Fatal("minimal view should not show type")
	}

	m2, _ := m.Update(tea.KeyPressMsg{Text: "e"})
	mm := m2.(Model)
	if !mm.activeTab().expanded {
		t.Fatal("expanded should be true after e")
	}
	v2 := mm.View()
	if !containsType(v2.Content) {
		t.Fatal("expanded view should show type")
	}

	m3, _ := mm.Update(tea.KeyPressMsg{Text: "e"})
	mm2 := m3.(Model)
	if mm2.activeTab().expanded {
		t.Fatal("expanded should be false after second e")
	}
}

func TestModelTabSwitch(t *testing.T) {
	root1 := &tree.Node{Name: "root1", IsDir: true, Path: "/root1"}
	m := New("/root1", root1)

	// Add second tab (opens root prompt)
	m2, _ := m.Update(tea.KeyPressMsg{Text: "t"})
	mm := m2.(Model)
	if len(mm.tabs) != 2 {
		t.Fatalf("expected 2 tabs, got %d", len(mm.tabs))
	}
	if mm.active != 1 {
		t.Fatalf("expected active tab 1 (index), got %d", mm.active)
	}
	// Dismiss the prompt before switching tabs
	mm2, _ := mm.Update(tea.KeyPressMsg{Text: "esc"})
	mm = mm2.(Model)

	// Switch back to tab 1 using "1"
	m3, _ := mm.Update(tea.KeyPressMsg{Text: "1"})
	mm3 := m3.(Model)
	if mm3.active != 0 {
		t.Fatalf("expected active tab 0 after '1', got %d", mm3.active)
	}
}

func TestModelCloseTab(t *testing.T) {
	root1 := &tree.Node{Name: "root1", IsDir: true, Path: "/root1"}
	m := New("/root1", root1)

	// Add second tab (opens root prompt)
	m2, _ := m.Update(tea.KeyPressMsg{Text: "t"})
	mm := m2.(Model)
	if len(mm.tabs) != 2 {
		t.Fatalf("expected 2 tabs, got %d", len(mm.tabs))
	}

	// Dismiss the prompt before closing
	mm2, _ := mm.Update(tea.KeyPressMsg{Text: "esc"})
	mm = mm2.(Model)

	// Close tab
	m3, _ := mm.Update(tea.KeyPressMsg{Text: "x"})
	mm3 := m3.(Model)
	if len(mm3.tabs) != 1 {
		t.Fatalf("expected 1 tab after close, got %d", len(mm3.tabs))
	}
}

func containsType(s string) bool {
	return strings.Contains(s, "vbox-disk")
}
