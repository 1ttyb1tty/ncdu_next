package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/1ttyb1tty/ncdu-next/internal/scan"
	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

func TestTabScanLifecycle(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0644)

	tab := newTab(0)
	tab.rootPath = dir

	// startScan sets status to Scanning and returns a cmd
	cmd := tab.startScan(scan.Options{})
	if tab.status != StatusScanning {
		t.Fatalf("status %q, want Scanning right after startScan", tab.status)
	}

	// Run the scan command synchronously
	msg := cmd()
	result, ok := msg.(scanResultMsg)
	if !ok {
		t.Fatalf("expected scanResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("scan error: %v", result.err)
	}
	if result.node == nil {
		t.Fatal("expected non-nil node from scan")
	}

	// Feed result into model -> status transitions Scanning -> Done
	model := Model{tabs: []*Tab{tab}, active: 0}
	m2, _ := model.updateScanResult(result)
	model = m2.(Model)
	if model.tabs[0].status != StatusDone {
		t.Fatalf("status %q, want Done after scan result", model.tabs[0].status)
	}
	if model.tabs[0].root == nil {
		t.Fatal("expected root node to be set after scan")
	}
}

func TestTabScanErrorPath(t *testing.T) {
	tab := newTab(0)
	msg := scanResultMsg{tab: tab, gen: tab.scanGen, node: nil, err: errors.New("boom")}
	model := Model{tabs: []*Tab{tab}, active: 0}
	m2, _ := model.updateScanResult(msg)
	model = m2.(Model)
	if model.tabs[0].status != StatusError {
		t.Fatalf("status %q, want Error", model.tabs[0].status)
	}
	if model.tabs[0].errMsg != "boom" {
		t.Fatalf("errMsg %q, want boom", model.tabs[0].errMsg)
	}
}

func TestTabScanEmptyRoot(t *testing.T) {
	tab := newTab(0)
	tab.rootPath = ""
	cmd := tab.startScan(scan.Options{})
	msg := cmd()
	result, ok := msg.(scanResultMsg)
	if !ok {
		t.Fatalf("expected scanResultMsg, got %T", msg)
	}
	if result.node != nil || result.err != nil {
		t.Fatal("expected nil node and nil err for empty root")
	}
	model := Model{tabs: []*Tab{tab}, active: 0}
	m2, _ := model.updateScanResult(result)
	model = m2.(Model)
	if model.tabs[0].status != StatusIdle {
		t.Fatalf("status %q, want Idle for empty root", model.tabs[0].status)
	}
}

func TestTabCancelScan(t *testing.T) {
	tab := newTab(0)
	tab.status = StatusScanning
	// cancelScan should not panic and should leave status for caller to reset
	tab.cancelScan()
	// No assertion beyond "did not panic"; caller resets status to Idle
}

func TestTabBarEntry(t *testing.T) {
	tab := newTab(0)
	tab.status = StatusScanning
	if got := tab.tabBarEntry(true); got != "1*[scanning]" {
		t.Fatalf("tabBarEntry %q, want 1*[scanning]", got)
	}
	tab.index = 2
	tab.status = StatusDone
	if got := tab.tabBarEntry(false); got != "3 [done]" {
		t.Fatalf("tabBarEntry %q, want 3 [done]", got)
	}
	tab.status = StatusIdle
	if got := tab.tabBarEntry(false); got != "3 [idle]" {
		t.Fatalf("tabBarEntry %q, want 3 [idle]", got)
	}
}

func TestTabDetectFormatToggleViaKey(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	m := New("/root", root, false, scan.Options{})
	tb := m.activeTab()
	if tb.detectFormat {
		t.Fatal("detectFormat should start false")
	}

	// Press 'd' to toggle on -> triggers rescan (root set)
	m2, cmd := m.Update(tea.KeyPressMsg{Text: "d"})
	mm := m2.(Model)
	if !mm.activeTab().detectFormat {
		t.Fatal("detectFormat should be true after d")
	}
	if cmd == nil {
		t.Fatal("expected a scan command after d with root set")
	}

	// Press 'd' again to toggle off -> no rescan needed since detectFormat off
	m3, _ := mm.Update(tea.KeyPressMsg{Text: "d"})
	mm2 := m3.(Model)
	if mm2.activeTab().detectFormat {
		t.Fatal("detectFormat should be false after second d")
	}
}

func TestRootPromptHandling(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	m := New("/root", root, false, scan.Options{})

	// Press 'r' to enter prompt
	m2, _ := m.Update(tea.KeyPressMsg{Text: "r"})
	mm := m2.(Model)
	if !mm.prompt {
		t.Fatal("prompt should be active after r")
	}
	if mm.promptInput != "/root" {
		t.Fatalf("promptInput %q, want /root", mm.promptInput)
	}

	// Type a new path character by character
	for _, ch := range "/tmp/foo" {
		m2, _ = mm.Update(tea.KeyPressMsg{Text: string(ch)})
		mm = m2.(Model)
	}
	if mm.promptInput != "/root/tmp/foo" {
		t.Fatalf("promptInput %q, want /root/tmp/foo", mm.promptInput)
	}

	// Backspace removes last char
	mb, _ := mm.Update(tea.KeyPressMsg{Text: "backspace"})
	mmb := mb.(Model)
	if mmb.promptInput != "/root/tmp/fo" {
		t.Fatalf("promptInput %q, want /root/tmp/fo", mmb.promptInput)
	}

	// Escape cancels
	me, _ := mmb.Update(tea.KeyPressMsg{Text: "esc"})
	mme := me.(Model)
	if mme.prompt {
		t.Fatal("prompt should be inactive after esc")
	}

	// Re-enter prompt and type path again for enter-commit test
	mr, _ := mme.Update(tea.KeyPressMsg{Text: "r"})
	mmr := mr.(Model)
	for _, ch := range "/tmp/foo" {
		mr2, _ := mmr.Update(tea.KeyPressMsg{Text: string(ch)})
		mmr = mr2.(Model)
	}

	// Enter commits
	menter, _ := mmr.Update(tea.KeyPressMsg{Text: "enter"})
	mmEnter := menter.(Model)
	if mmEnter.activeTab().rootPath != "/root/tmp/foo" {
		t.Fatalf("rootPath %q, want /root/tmp/foo", mmEnter.activeTab().rootPath)
	}
	if mmEnter.prompt {
		t.Fatal("prompt should be inactive after enter")
	}
}

func TestNewTabLimit(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	m := New("/root", root, false, scan.Options{})

	// Add tabs up to maxTabs
	for i := 0; i < maxTabs-1; i++ {
		m2, _ := m.Update(tea.KeyPressMsg{Text: "t"})
		m = m2.(Model)
		// dismiss prompt
		m3, _ := m.Update(tea.KeyPressMsg{Text: "esc"})
		m = m3.(Model)
	}
	if len(m.tabs) != maxTabs {
		t.Fatalf("expected %d tabs, got %d", maxTabs, len(m.tabs))
	}

	// One more should be ignored (at max)
	m2, _ := m.Update(tea.KeyPressMsg{Text: "t"})
	m = m2.(Model)
	if len(m.tabs) != maxTabs {
		t.Fatalf("expected %d tabs at max, got %d", maxTabs, len(m.tabs))
	}
}

func TestTabSwitchViaNumberKeys(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	m := New("/root", root, false, scan.Options{})

	m2, _ := m.Update(tea.KeyPressMsg{Text: "t"})
	m = m2.(Model)
	m3, _ := m.Update(tea.KeyPressMsg{Text: "esc"})
	m = m3.(Model)
	if len(m.tabs) != 2 {
		t.Fatalf("expected 2 tabs, got %d", len(m.tabs))
	}

	m4, _ := m.Update(tea.KeyPressMsg{Text: "2"})
	m = m4.(Model)
	if m.active != 1 {
		t.Fatalf("expected active index 1, got %d", m.active)
	}

	m5, _ := m.Update(tea.KeyPressMsg{Text: "1"})
	m = m5.(Model)
	if m.active != 0 {
		t.Fatalf("expected active index 0, got %d", m.active)
	}
}

func TestTabCtrlTabCycling(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	m := New("/root", root, false, scan.Options{})

	// Add two more tabs (dismiss prompts)
	for i := 0; i < 2; i++ {
		m2, _ := m.Update(tea.KeyPressMsg{Text: "t"})
		m = m2.(Model)
		m3, _ := m.Update(tea.KeyPressMsg{Text: "esc"})
		m = m3.(Model)
	}
	if len(m.tabs) != 3 {
		t.Fatalf("expected 3 tabs, got %d", len(m.tabs))
	}

	// Switch to tab 1 first so cycleTab starts from a known index
	m2, _ := m.Update(tea.KeyPressMsg{Text: "1"})
	m = m2.(Model)

	// Directly test cycleTab logic
	c1, _ := m.cycleTab(1)
	if c1.active != 1 {
		t.Fatalf("expected active index 1 after cycleTab(1), got %d", c1.active)
	}
	c2, _ := c1.cycleTab(1)
	if c2.active != 2 {
		t.Fatalf("expected active index 2 after second cycleTab(1), got %d", c2.active)
	}
	c3, _ := c2.cycleTab(1)
	if c3.active != 0 {
		t.Fatalf("expected wrap to index 0 after third cycleTab(1), got %d", c3.active)
	}

	// cycleTab(-1) cycles backward
	c4, _ := c3.cycleTab(-1)
	if c4.active != 2 {
		t.Fatalf("expected active index 2 after cycleTab(-1), got %d", c4.active)
	}
}

func TestTabLastCannotClose(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	m := New("/root", root, false, scan.Options{})

	// Last tab cannot be closed
	m2, _ := m.Update(tea.KeyPressMsg{Text: "x"})
	m = m2.(Model)
	if len(m.tabs) != 1 {
		t.Fatalf("expected 1 tab (last can't close), got %d", len(m.tabs))
	}
}

func TestTabScanViaKey(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0644)

	root := &tree.Node{Name: "root", IsDir: true, Path: dir}
	m := New(dir, root, false, scan.Options{})

	// Press 's' to start scan on tab 1
	m2, cmd := m.Update(tea.KeyPressMsg{Text: "s"})
	m = m2.(Model)
	if m.activeTab().status != StatusScanning {
		t.Fatalf("status %q, want Scanning after s key", m.activeTab().status)
	}
	if cmd == nil {
		t.Fatal("expected a scan command after s key")
	}

	// Run the scan command
	msg := cmd()
	result, ok := msg.(scanResultMsg)
	if !ok {
		t.Fatalf("expected scanResultMsg, got %T", msg)
	}
	m3, _ := m.updateScanResult(result)
	m = m3.(Model)
	if m.activeTab().status != StatusDone {
		t.Fatalf("status %q, want Done after scan completes", m.activeTab().status)
	}
}

func TestTabCancelViaKey(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0644)

	root := &tree.Node{Name: "root", IsDir: true, Path: dir}
	m := New(dir, root, false, scan.Options{})

	// Start a scan
	m2, _ := m.Update(tea.KeyPressMsg{Text: "s"})
	m = m2.(Model)
	if m.activeTab().status != StatusScanning {
		t.Fatalf("status %q, want Scanning after s key", m.activeTab().status)
	}

	// Cancel with 'c'
	m3, _ := m.Update(tea.KeyPressMsg{Text: "c"})
	m = m3.(Model)
	if m.activeTab().status != StatusIdle {
		t.Fatalf("status %q, want Idle after cancel", m.activeTab().status)
	}
}

func TestTabCancelIgnoresLateResult(t *testing.T) {
	tab := newTab(0)
	tab.rootPath = "/x"
	tab.startScan(scan.Options{}) // bumps scanGen, status Scanning
	staleGen := tab.scanGen
	tab.cancelScan() // bumps scanGen again, invalidating the scan
	tab.status = StatusIdle

	// A result from the cancelled scan (stale generation) must be ignored.
	msg := scanResultMsg{tab: tab, gen: staleGen, node: &tree.Node{Name: "late", IsDir: true}}
	model := Model{tabs: []*Tab{tab}, active: 0}
	m2, _ := model.updateScanResult(msg)
	model = m2.(Model)
	if model.tabs[0].status != StatusIdle {
		t.Fatalf("status %q, want Idle (stale result ignored)", model.tabs[0].status)
	}
	if model.tabs[0].root != nil {
		t.Fatal("stale result must not set the tree")
	}
}

func TestTabCloseReindexes(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	m := New("/root", root, false, scan.Options{})

	// Two more tabs (dismiss prompts) -> 3 tabs.
	for i := 0; i < 2; i++ {
		mx, _ := m.Update(tea.KeyPressMsg{Text: "t"})
		m = mx.(Model)
		my, _ := m.Update(tea.KeyPressMsg{Text: "esc"})
		m = my.(Model)
	}
	if len(m.tabs) != 3 {
		t.Fatalf("expected 3 tabs, got %d", len(m.tabs))
	}

	// Select the middle tab, then close it.
	mx, _ := m.Update(tea.KeyPressMsg{Text: "2"})
	m = mx.(Model)
	my, _ := m.Update(tea.KeyPressMsg{Text: "x"})
	m = my.(Model)

	if len(m.tabs) != 2 {
		t.Fatalf("expected 2 tabs after close, got %d", len(m.tabs))
	}
	for i, tb := range m.tabs {
		if tb.index != i {
			t.Fatalf("tab %d has index %d, want %d", i, tb.index, i)
		}
	}
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{512, "512"},
		{1023, "1023"},
		{1024, "1.0K"},
		{1536, "1.5K"},
		{1024 * 1024, "1.0M"},
		{5 * 1024 * 1024 * 1024, "5.0G"},
		{-5, "0"},
	}
	for _, c := range cases {
		if got := humanSize(c.n); got != c.want {
			t.Errorf("humanSize(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestUnitsToggle(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	root.AddChild(&tree.Node{Name: "big.bin", IsDir: false, Size: 5 * 1024 * 1024, Type: "image-png"})
	m := New("/root", root, false, scan.Options{})

	// Default: bytes mode.
	if m.activeTab().human {
		t.Fatal("human should start false")
	}
	v := m.View().Content
	if !strings.Contains(v, "bytes") {
		t.Fatal("byte mode should show a 'bytes' label")
	}
	if !strings.Contains(v, "5242880") {
		t.Fatal("byte mode should show the raw byte count")
	}

	// Toggle to human-friendly.
	m2, _ := m.Update(tea.KeyPressMsg{Text: "u"})
	mm := m2.(Model)
	if !mm.activeTab().human {
		t.Fatal("human should be true after u")
	}
	v2 := mm.View().Content
	if !strings.Contains(v2, "size") {
		t.Fatal("human mode should show a 'size' label")
	}
	if !strings.Contains(v2, "5.0M") {
		t.Fatalf("human mode should show 5.0M, got:\n%s", v2)
	}
}
