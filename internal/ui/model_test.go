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
	m := New("/root", root, false)
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
	m := New("/root", root, false)

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
	m := New("/root1", root1, false)

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
	m := New("/root1", root1, false)

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

func TestDebugTabOpenAndSwitch(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	m := New("/root", root, true)

	m2, _ := m.Update(tea.KeyPressMsg{Text: "D"})
	mm := m2.(Model)
	if len(mm.tabs) != 2 {
		t.Fatalf("expected 2 tabs after opening debug, got %d", len(mm.tabs))
	}
	if mm.activeTab().kind != tabKindDebug {
		t.Fatalf("expected active tab to be debug, got %v", mm.activeTab().kind)
	}
	v := mm.View()
	if !strings.Contains(v.Content, "DEBUG - ncdu-next") {
		t.Fatalf("debug view missing header, got: %s", v.Content)
	}
	if !strings.Contains(v.Content, "goroutines") {
		t.Fatalf("debug view missing runtime stats, got: %s", v.Content)
	}

	m3, _ := mm.Update(tea.KeyPressMsg{Text: "D"})
	mm3 := m3.(Model)
	if len(mm3.tabs) != 2 {
		t.Fatalf("expected still 2 tabs, got %d", len(mm3.tabs))
	}
	if mm3.activeTab().kind != tabKindDebug {
		t.Fatalf("expected active tab still debug")
	}

	m4, _ := mm3.Update(tea.KeyPressMsg{Text: "1"})
	mm4 := m4.(Model)
	if mm4.activeTab().kind != tabKindNav {
		t.Fatalf("expected active tab nav after '1', got %v", mm4.activeTab().kind)
	}
}

func TestDebugTabDisabled(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	m := New("/root", root, false)

	m2, _ := m.Update(tea.KeyPressMsg{Text: "D"})
	mm := m2.(Model)
	if len(mm.tabs) != 1 {
		t.Fatalf("debug disabled should not add a tab, got %d", len(mm.tabs))
	}
	if mm.activeTab().kind != tabKindNav {
		t.Fatalf("expected nav tab, got %v", mm.activeTab().kind)
	}
}
