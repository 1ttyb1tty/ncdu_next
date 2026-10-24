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

func TestFormatSize(t *testing.T) {
	cases := []struct {
		n    int64
		mode UnitMode
		want string
	}{
		{0, UnitBytes, "0"},
		{512, UnitBytes, "512"},
		{1023, UnitBinary, "1023"},
		{1024, UnitBinary, "1 KiB"},
		{1536, UnitBinary, "2 KiB"},
		{5 * 1024 * 1024, UnitBinary, "5 MiB"},
		{5 * 1024 * 1024 * 1024, UnitBinary, "5 GiB"},
		{-5, UnitBinary, "0"},
		{999, UnitDecimal, "999"},
		{1000, UnitDecimal, "1 kB"},
		{1500, UnitDecimal, "2 kB"},
		{1_000_000, UnitDecimal, "1 MB"},
		{1500 * 1000, UnitDecimal, "2 MB"},
	}
	for _, c := range cases {
		if got := formatSize(c.n, c.mode); got != c.want {
			t.Errorf("formatSize(%d, %v) = %q, want %q", c.n, c.mode, got, c.want)
		}
	}
	// humanSize is the binary-mode alias.
	if got := humanSize(2048); got != "2 KiB" {
		t.Errorf("humanSize(2048) = %q, want 2 KiB", got)
	}
}

func TestUnitsToggle(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	root.AddChild(&tree.Node{Name: "big.bin", IsDir: false, Size: 5 * 1024 * 1024, Type: "png", Kind: tree.KindImage})
	m := New("/root", root, false, scan.Options{})

	// Default: bytes mode.
	if m.activeTab().unit != UnitBytes {
		t.Fatalf("unit should start bytes, got %v", m.activeTab().unit)
	}
	v := m.View().Content
	if !strings.Contains(v, "bytes") {
		t.Fatal("byte mode should show a 'bytes' label")
	}
	if !strings.Contains(v, "5242880") {
		t.Fatal("byte mode should show the raw byte count")
	}

	// bytes -> IEC binary.
	m2, _ := m.Update(tea.KeyPressMsg{Text: "u"})
	mm := m2.(Model)
	if mm.activeTab().unit != UnitBinary {
		t.Fatalf("unit should be binary after u, got %v", mm.activeTab().unit)
	}
	v2 := mm.View().Content
	if !strings.Contains(v2, "KiB/MiB/GiB") {
		t.Fatal("binary mode should show a KiB/MiB/GiB label")
	}
	if !strings.Contains(v2, "5 MiB") {
		t.Fatalf("binary mode should show 5 MiB, got:\n%s", v2)
	}

	// IEC binary -> SI decimal.
	m3, _ := mm.Update(tea.KeyPressMsg{Text: "u"})
	mm2 := m3.(Model)
	if mm2.activeTab().unit != UnitDecimal {
		t.Fatalf("unit should be decimal after second u, got %v", mm2.activeTab().unit)
	}
	v3 := mm2.View().Content
	if !strings.Contains(v3, "kB/MB/GB/TB") {
		t.Fatal("decimal mode should show a kB/MB/GB/TB label")
	}
	if !strings.Contains(v3, "5 MB") {
		t.Fatalf("decimal mode should show 5 MB, got:\n%s", v3)
	}

	// SI decimal -> bytes (cycles).
	m4, _ := mm2.Update(tea.KeyPressMsg{Text: "u"})
	mm3 := m4.(Model)
	if mm3.activeTab().unit != UnitBytes {
		t.Fatalf("unit should cycle back to bytes, got %v", mm3.activeTab().unit)
	}
}

func TestWideViewShowsKindAndMtime(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	root.AddChild(&tree.Node{
		Name: "disk.vdi", IsDir: false, Size: 1024, Type: "vdi", Kind: tree.KindDiskImage,
		Mtime: 1_700_000_000_000_000_000,
	})
	m := New("/root", root, false, scan.Options{})
	m.width, m.height = 200, 24 // wide fits on one line at this width

	m2, _ := m.Update(tea.KeyPressMsg{Text: "w"})
	mm := m2.(Model)
	if !mm.activeTab().expanded {
		t.Fatal("w should enable the wide view")
	}
	// 200 columns: wide fits on one line, so kind and mtime render.
	v := mm.View().Content
	if !strings.Contains(v, "disk-image") {
		t.Fatalf("wide view should show the kind, got:\n%s", v)
	}
	if !strings.Contains(v, "mtime") {
		t.Fatalf("wide view should show the mtime column header, got:\n%s", v)
	}
}

func TestWideViewNarrowSpills(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	root.AddChild(&tree.Node{
		Name: "disk.vdi", IsDir: false, Size: 1024, Type: "vdi", Kind: tree.KindDiskImage,
		Mtime: 1_700_000_000_000_000_000,
	})
	m := New("/root", root, false, scan.Options{})
	m.width, m.height = 90, 24 // panelWidth 88 < wideViewMinWidth
	m2, _ := m.Update(tea.KeyPressMsg{Text: "w"})
	mm := m2.(Model)
	v := mm.View().Content
	// Both lines of the spilled record must appear.
	if !strings.Contains(v, "disk.vdi") || !strings.Contains(v, "disk-image") {
		t.Fatalf("narrow wide view should show name and kind, got:\n%s", v)
	}
	if !strings.Contains(v, "2023-11-14 22:13") {
		t.Fatalf("narrow wide view should show the formatted mtime, got:\n%s", v)
	}
}

// TestPanelMetricsWidthFits pins the column budget: the name column plus the
// fixed column width must equal the panel width exactly, in every view mode, so
// rows neither bleed past the panel edge nor leave a trailing gap.
func TestPanelMetricsWidthFits(t *testing.T) {
	for _, width := range []int{80, 100, 121, 200} {
		panelWidth, _, nameWidth := panelMetrics(width, 24, true)
		fixed := 79
		if panelWidth < wideViewMinWidth {
			fixed = 37
		}
		if nameWidth+fixed != panelWidth {
			t.Errorf("width %d: nameWidth %d + fixed %d != panelWidth %d", width, nameWidth, fixed, panelWidth)
		}
	}
	if panelWidth, _, nameWidth := panelMetrics(80, 24, false); nameWidth+16 != panelWidth {
		t.Errorf("simple view: nameWidth %d + 16 != panelWidth %d", nameWidth, panelWidth)
	}
}

// debugModel returns a model with the Debug tab focused, inspecting the tab D
// was pressed from.
func debugModel(t *testing.T) Model {
	t.Helper()
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	m := New("/root", root, true, scan.Options{})
	m2, _ := m.Update(tea.KeyPressMsg{Text: "D"})
	return m2.(Model)
}

func TestDebugViewportClamps(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e"}
	if got := debugViewport(lines, -5, 2); len(got) != 2 || got[0] != "a" {
		t.Errorf("negative offset should clamp to 0, got %v", got)
	}
	if got := debugViewport(lines, 99, 2); len(got) != 1 || got[0] != "e" {
		t.Errorf("past-end offset should clamp to last line, got %v", got)
	}
	if got := debugViewport(lines, 0, 99); len(got) != len(lines) {
		t.Errorf("tall window should show all lines, got %v", got)
	}
	if got := debugViewport(nil, 0, 5); got != nil {
		t.Errorf("empty page should yield nil, got %v", got)
	}
}

func TestDebugScrollClamps(t *testing.T) {
	m := debugModel(t)
	tb := m.activeTab()
	total := len(debugPage(debugSections(m.tabs, m.inspect)))

	// up at the top stays at the top.
	m.debugScroll(1)
	if tb.scroll != 1 {
		t.Fatalf("scroll %d, want 1", tb.scroll)
	}
	m.debugScroll(-100)
	if tb.scroll != 0 {
		t.Fatalf("scroll %d, want 0 after over-scroll up", tb.scroll)
	}
	m.debugScroll(1 << 30)
	if tb.scroll != clampOffset(1<<30, total) {
		t.Fatalf("scroll %d, want last line %d", tb.scroll, total-1)
	}
	if tb.scroll != total-1 {
		t.Fatalf("scroll %d, want %d", tb.scroll, total-1)
	}

	// Home/end keys route through the same clamp.
	m2, _ := m.Update(tea.KeyPressMsg{Text: "home"})
	m = m2.(Model)
	if m.activeTab().scroll != 0 {
		t.Fatalf("home: scroll %d, want 0", m.activeTab().scroll)
	}
	m3, _ := m.Update(tea.KeyPressMsg{Text: "end"})
	m = m3.(Model)
	if m.activeTab().scroll != total-1 {
		t.Fatalf("end: scroll %d, want %d", m.activeTab().scroll, total-1)
	}
}

func TestDebugSectionJumpsLandOnHeadings(t *testing.T) {
	m := debugModel(t)
	secs := debugSections(m.tabs, m.inspect)
	offs := sectionOffsets(secs)
	if len(offs) < 3 {
		t.Fatalf("expected several sections, got %d", len(offs))
	}
	tb := m.activeTab()
	tb.scroll = 0

	// The first forward jump lands on the first heading (after the zone heading).
	m.debugJumpSection(1)
	if tb.scroll != offs[0] {
		t.Fatalf("first jump: scroll %d, want %d", tb.scroll, offs[0])
	}
	// Forward jumps then land on each successive heading.
	for i := 1; i < len(offs); i++ {
		m.debugJumpSection(1)
		if tb.scroll != offs[i] {
			t.Fatalf("jump %d: scroll %d, want %d", i, tb.scroll, offs[i])
		}
	}
	// Forward past the end stays on the last heading.
	m.debugJumpSection(1)
	if tb.scroll != offs[len(offs)-1] {
		t.Fatalf("over-jump forward: scroll %d, want %d", tb.scroll, offs[len(offs)-1])
	}
	// Backward jumps walk the headings in reverse.
	for i := len(offs) - 2; i >= 0; i-- {
		m.debugJumpSection(-1)
		if tb.scroll != offs[i] {
			t.Fatalf("back-jump %d: scroll %d, want %d", i, tb.scroll, offs[i])
		}
	}
	// Backward past the start stays on the first heading.
	m.debugJumpSection(-1)
	if tb.scroll != offs[0] {
		t.Fatalf("over-jump back: scroll %d, want %d", tb.scroll, offs[0])
	}
}

func TestDebugInspectsKeyTabNotDebugTab(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	m := New("/root", root, true, scan.Options{})
	// Add and focus a second nav tab.
	m2, _ := m.Update(tea.KeyPressMsg{Text: "t"})
	m = m2.(Model)
	m3, _ := m.Update(tea.KeyPressMsg{Text: "esc"})
	m = m3.(Model)
	second := m.activeTab()

	m4, _ := m.Update(tea.KeyPressMsg{Text: "D"})
	m = m4.(Model)
	if m.inspect != second {
		t.Fatal("inspect should be the tab D was pressed from")
	}
	if m.activeTab().kind != tabKindDebug {
		t.Fatal("active tab should be the debug tab")
	}
	if m.activeTab() == m.inspect {
		t.Fatal("inspect must never be the debug tab itself")
	}

	// Pressing D again must not change the inspected tab.
	m5, _ := m.Update(tea.KeyPressMsg{Text: "D"})
	m = m5.(Model)
	if m.inspect != second {
		t.Fatal("inspect must be unchanged by a second D")
	}
}

func TestDebugInspectFallsBackWhenClosed(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	m := New("/root", root, true, scan.Options{})
	// A second nav tab, so a fallback exists after the inspected one is gone.
	m2, _ := m.Update(tea.KeyPressMsg{Text: "t"})
	m = m2.(Model)
	m3, _ := m.Update(tea.KeyPressMsg{Text: "esc"})
	m = m3.(Model)
	closed := m.activeTab()
	m4, _ := m.Update(tea.KeyPressMsg{Text: "D"})
	m = m4.(Model)
	if m.inspect != closed {
		t.Fatal("inspect should be the second nav tab")
	}
	// Simulate the inspected tab being removed from the live list (as closing it
	// would), leaving m.inspect dangling.
	kept := m.tabs[:0:0]
	for _, tb := range m.tabs {
		if tb != closed {
			kept = append(kept, tb)
		}
	}
	m.tabs = kept
	for _, tb := range m.tabs {
		if tb == closed {
			t.Fatal("closed tab still present")
		}
	}
	// Closing a tab reindexes and re-points active; do the same here so the
	// render is exercised in the state the close path would leave.
	for i, tb := range m.tabs {
		tb.index = i
		if tb.kind == tabKindDebug {
			m.active = i
		}
	}
	got := resolveInspect(m.tabs, m.inspect)
	if got == nil {
		t.Fatal("expected a fallback inspected tab")
	}
	if got.kind == tabKindDebug {
		t.Fatal("fallback must never be the debug tab")
	}
	// The page must still render without panicking.
	_ = m.View().Content
}

func TestDebugKeysIgnoreNavOnlyKeys(t *testing.T) {
	m := debugModel(t)
	// `g` used to reach the debug tab and set searchActive on a nil-root tab;
	// now it must be ignored.
	m2, _ := m.Update(tea.KeyPressMsg{Text: "g"})
	mm := m2.(Model)
	if mm.prompt || mm.activeTab().searchActive {
		t.Fatal("nav-only key g must be ignored on the debug tab")
	}
}

func fsTab(fs *scan.FSReport, cur string) *Tab {
	tb := newTab(0)
	tb.fs = fs
	if cur != "" {
		tb.cur = &tree.Node{Name: "c", Path: cur, IsDir: true}
	}
	return tb
}

func TestFsTokenStates(t *testing.T) {
	// available -> the OS-reported name.
	tb := fsTab(&scan.FSReport{Root: scan.FSInfo{MountPoint: "/", FSType: "NTFS", Avail: scan.FSAvailable}}, "/x")
	if got := tb.fsToken(); got != "[NTFS]" {
		t.Errorf("available token = %q, want [NTFS]", got)
	}
	// unavailable -> a visible placeholder, not a wrong name.
	tb = fsTab(&scan.FSReport{Root: scan.FSInfo{Avail: scan.FSUnavailable, Reason: "boom"}}, "/x")
	if got := tb.fsToken(); got != "[fs:?]" {
		t.Errorf("unavailable token = %q, want [fs:?]", got)
	}
	// not-applicable -> omitted.
	tb = fsTab(&scan.FSReport{Root: scan.FSInfo{Avail: scan.FSNotApplicable, Reason: "file"}}, "/x")
	if got := tb.fsToken(); got != "" {
		t.Errorf("not-applicable token = %q, want empty", got)
	}
	// nil report -> omitted.
	if got := fsTab(nil, "/x").fsToken(); got != "" {
		t.Errorf("nil report token = %q, want empty", got)
	}
}

func TestFsTokenFollowsCurrentDirectory(t *testing.T) {
	fs := &scan.FSReport{
		Root:   scan.FSInfo{MountPoint: "/", FSType: "ext4", Avail: scan.FSAvailable},
		Mounts: []scan.FSInfo{{MountPoint: "/data", FSType: "NTFS", Avail: scan.FSAvailable}},
	}
	cases := []struct {
		cur, want string
	}{
		{"/", "[ext4]"},
		{"/etc", "[ext4]"},
		{"/data", "[NTFS]"},
		{"/data/sub", "[NTFS]"},
		{"/database", "[ext4]"}, // not under /data
	}
	for _, c := range cases {
		if got := fsTab(fs, c.cur).fsToken(); got != c.want {
			t.Errorf("cur %q token = %q, want %q", c.cur, got, c.want)
		}
	}
}

func TestFsTokenRendersInHeader(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/"}
	fs := &scan.FSReport{Root: scan.FSInfo{MountPoint: "/", FSType: "NTFS", Avail: scan.FSAvailable}}
	m := NewWithReport("/", root, fs, false, scan.Options{})
	if v := m.View().Content; !strings.Contains(v, "[NTFS]") {
		t.Fatalf("header missing the filesystem token, got:\n%s", v)
	}
}
