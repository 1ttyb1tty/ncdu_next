package ui

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/1ttyb1tty/ncdu-next/internal/scan"
	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

// ScanStatus tracks the current/last operation status of a tab.
type ScanStatus string

const (
	StatusIdle     ScanStatus = "idle"
	StatusScanning ScanStatus = "scanning"
	StatusDone     ScanStatus = "done"
	StatusError    ScanStatus = "error"
)

// tabKind distinguishes a navigation tab from a read-only debug tab.
type tabKind string

const (
	tabKindNav   tabKind = "nav"
	tabKindDebug tabKind = "debug"
)

// maxTabs is the maximum number of concurrent navigation/scan tabs.
const maxTabs = 8

// Tab is an independent navigation/scan session: root path, scan result,
// navigation state, and per-tab options.
type Tab struct {
	index        int
	rootPath     string
	root         *tree.Node
	cur          *tree.Node
	sel          int
	scroll       int
	expanded     bool
	status       ScanStatus
	detectFormat bool
	errMsg       string
	kind         tabKind
	nodeCount    int
	scanGen      int
	ctx          context.Context
	cancel       context.CancelFunc
}

// newTab creates an idle nav tab with its own cancellable context.
func newTab(index int) *Tab {
	ctx, cancel := context.WithCancel(context.Background())
	return &Tab{
		index:  index,
		status: StatusIdle,
		kind:   tabKindNav,
		ctx:    ctx,
		cancel: cancel,
	}
}

// newDebugTab creates a read-only debug tab showing runtime and per-tab stats.
func newDebugTab(index int) *Tab {
	ctx, cancel := context.WithCancel(context.Background())
	return &Tab{
		index:  index,
		status: StatusDone,
		kind:   tabKindDebug,
		ctx:    ctx,
		cancel: cancel,
	}
}

// displayNumber returns the 1-based tab number shown in the tab bar.
func (t *Tab) displayNumber() int { return t.index + 1 }

// children returns the children of the current node (or root's children).
func (t *Tab) children() []*tree.Node {
	n := t.cur
	if n == nil {
		n = t.root
	}
	if n == nil || !n.IsDir {
		return nil
	}
	return n.Children
}

// selectUp moves the selection up by one.
func (t *Tab) selectUp() {
	if t.sel > 0 {
		t.sel--
	}
}

// selectDown moves the selection down by one.
func (t *Tab) selectDown() {
	ch := t.children()
	if t.cur != nil && t.sel < len(ch)-1 {
		t.sel++
	}
}

// enter opens the selected directory (if it is a directory).
func (t *Tab) enter() {
	ch := t.children()
	if t.cur != nil && t.sel < len(ch) && ch[t.sel].IsDir {
		t.cur = ch[t.sel]
		t.sel = 0
		t.scroll = 0
	}
}

// goUp navigates to the parent directory.
func (t *Tab) goUp() {
	if t.cur != nil && t.cur.Parent != nil {
		t.cur = t.cur.Parent
		t.sel = 0
		t.scroll = 0
	}
}

// toggleExpanded switches between minimal and expanded column view.
func (t *Tab) toggleExpanded() {
	t.expanded = !t.expanded
}

// cancelScan cancels any in-flight scan on this tab and invalidates its
// result, so a late-returning cancelled scan cannot overwrite tab state.
func (t *Tab) cancelScan() {
	if t.cancel != nil {
		t.cancel()
	}
	t.scanGen++
}

// view renders the tab's navigation content at the given terminal width.
func (t *Tab) view(width, height int) string {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	panelWidth := width - 2
	if panelWidth < 30 {
		panelWidth = 30
	}
	var s strings.Builder
	if t.cur == nil {
		if t.rootPath == "" {
			s.WriteString("no root set - press r to enter a path, then s to scan")
		} else if t.status == StatusError {
			s.WriteString("error: " + t.errMsg)
		} else {
			s.WriteString("no data")
		}
		return s.String()
	}

	// Path header bar: single line, padded to the exact panel width so the
	// background fills precisely and cannot bleed onto adjacent cells.
	header := padRight(truncateWidth(" "+t.cur.Path+" ", panelWidth), panelWidth)
	s.WriteString(headerStyle.Render(header))
	s.WriteString("\n")

	// Horizontal separators drawn manually at the exact panel width.
	sep := sepStyle.Render(strings.Repeat("-", panelWidth))

	nameWidth := panelWidth - 16
	if t.expanded {
		nameWidth = panelWidth - 53
	}
	if nameWidth < 10 {
		nameWidth = 10
	}

	// Only the visible slice of children is rendered: total height minus the
	// tab bar, the two list separators, the footer and blank lines.
	maxRows := height - 7
	if maxRows < 1 {
		maxRows = 1
	}
	ch := t.children()

	// Keep the selection within the visible window.
	if t.sel < t.scroll {
		t.scroll = t.sel
	}
	if t.sel >= t.scroll+maxRows {
		t.scroll = t.sel - maxRows + 1
	}
	if t.scroll < 0 {
		t.scroll = 0
	}
	end := t.scroll + maxRows
	if end > len(ch) {
		end = len(ch)
	}

	rows := make([]string, 0, maxRows)
	for i := t.scroll; i < end; i++ {
		c := ch[i]
		mark := "  "
		if i == t.sel {
			mark = "> "
		}

		rowStyle := styleForType(c.Type)
		if i == t.sel {
			rowStyle = rowStyle.Background(lipgloss.Color("#2A2A2A")).Bold(true)
		}

		name := padRight(truncateWidth(c.Name, nameWidth), nameWidth)
		var rowContent string
		if t.expanded {
			rowContent = fmt.Sprintf("%s %s %12d %s %s %s",
				mark,
				name,
				c.Size,
				padRight(truncateWidth(c.Type, 16), 16),
				padRight(truncateWidth(c.Encoding, 9), 9),
				padRight(truncateWidth(c.LineEnding, 8), 8),
			)
		} else {
			rowContent = fmt.Sprintf("%s %s %12d", mark, name, c.Size)
		}
		// Pad to the exact panel width so the selection background fills
		// precisely and never bleeds onto adjacent cells.
		rowContent = padRight(rowContent, panelWidth)
		rows = append(rows, rowStyle.Render(rowContent))
	}

	if len(rows) == 0 {
		rows = append(rows, padRight("(empty)", panelWidth))
	}
	s.WriteString(sep)
	s.WriteString("\n")
	s.WriteString(strings.Join(rows, "\n"))
	s.WriteString("\n")
	s.WriteString(sep)

	return s.String()
}

// Precomputed styles, built once, so the render loop does not allocate a new
// style per row per frame.
var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#2A2A2A"))
	sepStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#4A4A4A"))
	styleDefault = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))
	typeStyles   = map[string]lipgloss.Style{
		"vbox":      lipgloss.NewStyle().Foreground(lipgloss.Color("#0000FF")),
		"vz":        lipgloss.NewStyle().Foreground(lipgloss.Color("#FF00FF")),
		"xen":       lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF")),
		"parallels": lipgloss.NewStyle().Foreground(lipgloss.Color("#FF00FF")),
		"utm":       lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")),
		"container": lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")),
		"memory":    lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")),
		"doc":       lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6347")),
		"image":     lipgloss.NewStyle().Foreground(lipgloss.Color("#90EE90")),
		"audio":     lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")),
		"video":     lipgloss.NewStyle().Foreground(lipgloss.Color("#ADD8E6")),
		"text":      lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")),
		"source":    lipgloss.NewStyle().Foreground(lipgloss.Color("#E0FFFF")),
		"markup":    lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")),
	}
	typePrefixes = []string{
		"vbox", "vz", "xen", "parallels", "utm", "container",
		"memory", "doc", "image", "audio", "video", "text", "source", "markup",
	}
)

// styleForType returns the precomputed style for a file type, defaulting to
// gray. Prefixes are checked in a fixed order for deterministic results.
func styleForType(t string) lipgloss.Style {
	for _, p := range typePrefixes {
		if strings.HasPrefix(t, p) {
			return typeStyles[p]
		}
	}
	return styleDefault
}

// padRight pads s on the right with spaces to display width w.
func padRight(s string, w int) string {
	cur := lipgloss.Width(s)
	if cur >= w {
		return s
	}
	return s + strings.Repeat(" ", w-cur)
}

// truncateWidth shortens s to at most display width w, appending "..." when
// it was cut. Width is measured with lipgloss so wide runes are handled.
func truncateWidth(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	if w <= 3 {
		return strings.Repeat(".", w)
	}
	var b strings.Builder
	cur := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if cur+rw > w-3 {
			break
		}
		b.WriteRune(r)
		cur += rw
	}
	return b.String() + "..."
}

// tabBarEntry renders the tab's entry in the tab bar, e.g. "1*[scanning]".
func (t *Tab) tabBarEntry(active bool) string {
	statusStr := ""
	switch t.status {
	case StatusIdle:
		statusStr = "[idle]"
	case StatusScanning:
		statusStr = "[scanning]"
	case StatusDone:
		statusStr = "[done]"
	case StatusError:
		statusStr = "[error]"
	}
	marker := " "
	if active {
		marker = "*"
	}
	if t.kind == tabKindDebug {
		return fmt.Sprintf("%d%s[debug]", t.displayNumber(), marker)
	}
	return fmt.Sprintf("%d%s%s", t.displayNumber(), marker, statusStr)
}

// scanResultMsg carries the result of an asynchronous scan. It references the
// originating tab by pointer (not slice position) and carries the scan
// generation so stale/cancelled results cannot be applied.
type scanResultMsg struct {
	tab  *Tab
	gen  int
	node *tree.Node
	err  error
}

// scanCmd runs a scan asynchronously and returns the result message. base
// carries the configured scan options (workers, excludes, hardlink/one-fs/
// symlink policy); only Root, DetectFormat and Ctx are per-tab.
func scanCmd(tab *Tab, base scan.Options, gen int) tea.Cmd {
	return func() tea.Msg {
		if tab.rootPath == "" {
			return scanResultMsg{tab: tab, gen: gen}
		}
		o := base
		o.Root = tab.rootPath
		o.DetectFormat = tab.detectFormat
		o.Ctx = tab.ctx
		w := scan.New(o)
		node, err := w.Scan()
		return scanResultMsg{tab: tab, gen: gen, node: node, err: err}
	}
}

// startScan begins a scan on the tab, cancelling any in-flight scan first.
func (t *Tab) startScan(base scan.Options) tea.Cmd {
	t.cancelScan()
	// Recreate context since the old one may be cancelled.
	ctx, cancel := context.WithCancel(context.Background())
	t.ctx = ctx
	t.cancel = cancel
	t.status = StatusScanning
	t.errMsg = ""
	return scanCmd(t, base, t.scanGen)
}

// debugView renders the read-only debug tab: runtime stats and per-tab breakdown.
func debugView(tabs []*Tab) string {
	var s strings.Builder
	s.WriteString(lipgloss.NewStyle().Bold(true).Render("DEBUG - ncdu-next"))
	s.WriteString("\n\n")

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	s.WriteString("runtime:\n")
	s.WriteString(fmt.Sprintf("  alloc        %s\n", bytesHuman(ms.Alloc)))
	s.WriteString(fmt.Sprintf("  total_alloc  %s\n", bytesHuman(ms.TotalAlloc)))
	s.WriteString(fmt.Sprintf("  sys          %s\n", bytesHuman(ms.Sys)))
	s.WriteString(fmt.Sprintf("  mallocs      %s\n", formatUint(ms.Mallocs)))
	s.WriteString(fmt.Sprintf("  frees        %s\n", formatUint(ms.Frees)))
	s.WriteString(fmt.Sprintf("  gc_cycles    %s\n", formatUint(uint64(ms.NumGC))))
	s.WriteString(fmt.Sprintf("  goroutines   %s\n", formatUint(uint64(runtime.NumGoroutine()))))
	s.WriteString(fmt.Sprintf("  cgo_calls    %s\n", formatUint(uint64(runtime.NumCgoCall()))))
	s.WriteString(fmt.Sprintf("  num_cpu      %s\n", formatUint(uint64(runtime.NumCPU()))))
	s.WriteString("\n")

	s.WriteString("tabs:\n")
	s.WriteString(fmt.Sprintf("  %-4s %-6s %-10s %-6s %s\n", "#", "kind", "status", "nodes", "root"))
	for _, tab := range tabs {
		nodeCount := tab.nodeCount
		root := tab.rootPath
		if len(root) > 40 {
			root = "..." + root[len(root)-37:]
		}
		statusStr := string(tab.status)
		if tab.status == StatusIdle && tab.root == nil {
			statusStr = "empty"
		}
		s.WriteString(fmt.Sprintf("  %-4d %-6s %-10s %-6d %s\n", tab.displayNumber(), tab.kind, statusStr, nodeCount, root))
	}
	s.WriteString("\n")
	s.WriteString("press q or esc to return, 1-8 to switch tab")
	return s.String()
}

// countNodes recursively counts nodes in a tree.
func countNodes(n *tree.Node) int {
	if n == nil {
		return 0
	}
	total := 1
	for _, c := range n.Children {
		total += countNodes(c)
	}
	return total
}

// bytesHuman formats bytes into a human readable string.
func bytesHuman(b uint64) string {
	const unit = 1024
	if b < unit {
		return strconv.FormatUint(b, 10) + " B"
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	pre := "KMGTPE"[exp]
	return strconv.FormatFloat(float64(b)/float64(div), 'f', 1, 64) + " " + string(pre) + "iB"
}

// formatUint formats a uint64 as a decimal string.
func formatUint(n uint64) string {
	return strconv.FormatUint(n, 10)
}
