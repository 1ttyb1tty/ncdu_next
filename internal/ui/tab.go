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
	expanded     bool
	status       ScanStatus
	detectFormat bool
	errMsg       string
	kind         tabKind
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
	}
}

// goUp navigates to the parent directory.
func (t *Tab) goUp() {
	if t.cur != nil && t.cur.Parent != nil {
		t.cur = t.cur.Parent
		t.sel = 0
	}
}

// toggleExpanded switches between minimal and expanded column view.
func (t *Tab) toggleExpanded() {
	t.expanded = !t.expanded
}

// cancelScan cancels any in-flight scan on this tab.
func (t *Tab) cancelScan() {
	if t.cancel != nil {
		t.cancel()
	}
}

// view renders the tab's navigation content.
func (t *Tab) view() string {
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

	// Path header bar with background
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#2A2A2A")).
		Padding(0, 1)
	s.WriteString(headerStyle.Render(" " + t.cur.Path + " "))
	s.WriteString("\n\n")

	// File list panel with an ASCII-only border (non-ASCII/ambiguous-width
	// box-drawing characters cause cursor-column drift in the cell-based
	// renderer, corrupting redraws).
	asciiBorder := lipgloss.Border{
		Top:         "-",
		Bottom:      "-",
		Left:        "|",
		Right:       "|",
		TopLeft:     "+",
		TopRight:    "+",
		BottomLeft:  "+",
		BottomRight: "+",
	}
	borderStyle := lipgloss.NewStyle().
		Border(asciiBorder).
		BorderForeground(lipgloss.Color("#4A4A4A")).
		Padding(0, 1)

	var rows []string
	for i, c := range t.children() {
		mark := "  "
		if i == t.sel {
			mark = "> "
		}

		// Type-based color (hex RGB — works with ANSI256/TrueColor profiles)
		typeColor := lipgloss.Color("#808080") // default gray
		switch {
		case strings.HasPrefix(c.Type, "vbox"):
			typeColor = lipgloss.Color("#0000FF") // blue
		case strings.HasPrefix(c.Type, "vz"):
			typeColor = lipgloss.Color("#FF00FF") // magenta
		case strings.HasPrefix(c.Type, "xen"):
			typeColor = lipgloss.Color("#00FFFF") // cyan
		case strings.HasPrefix(c.Type, "parallels"):
			typeColor = lipgloss.Color("#FF00FF") // light magenta
		case strings.HasPrefix(c.Type, "utm"):
			typeColor = lipgloss.Color("#00FF00") // green
		case strings.HasPrefix(c.Type, "container"):
			typeColor = lipgloss.Color("#FFFF00") // yellow
		case strings.HasPrefix(c.Type, "memory"):
			typeColor = lipgloss.Color("#FF0000") // red
		case strings.HasPrefix(c.Type, "doc"):
			typeColor = lipgloss.Color("#FF6347") // light red
		case strings.HasPrefix(c.Type, "image"):
			typeColor = lipgloss.Color("#90EE90") // light green
		case strings.HasPrefix(c.Type, "audio"):
			typeColor = lipgloss.Color("#FFFF00") // light yellow
		case strings.HasPrefix(c.Type, "video"):
			typeColor = lipgloss.Color("#ADD8E6") // light blue
		case strings.HasPrefix(c.Type, "text"):
			typeColor = lipgloss.Color("#FFFFFF") // white
		case strings.HasPrefix(c.Type, "source"):
			typeColor = lipgloss.Color("#E0FFFF") // light cyan
		case strings.HasPrefix(c.Type, "markup"):
			typeColor = lipgloss.Color("#FFFFFF") // bright white
		}

		rowStyle := lipgloss.NewStyle().Foreground(typeColor)
		if i == t.sel {
			rowStyle = rowStyle.Background(lipgloss.Color("#2A2A2A")).Bold(true)
		}

		if t.expanded {
			rowContent := fmt.Sprintf("%s  %-40s  %12d  %s  %s  %s",
				mark,
				c.Name,
				c.Size,
				c.Type,
				c.Encoding,
				c.LineEnding,
			)
			rows = append(rows, rowStyle.Render(rowContent))
		} else {
			rowContent := fmt.Sprintf("%s  %-40s  %12d",
				mark,
				c.Name,
				c.Size,
			)
			rows = append(rows, rowStyle.Render(rowContent))
		}
	}

	panelContent := strings.Join(rows, "\n")
	if len(rows) == 0 {
		panelContent = "(empty)"
	}
	s.WriteString(borderStyle.Render(panelContent))

	return s.String()
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

// scanResultMsg carries the result of an asynchronous scan.
type scanResultMsg struct {
	tabIndex int
	node     *tree.Node
	err      error
}

// scanCmd runs a scan asynchronously and returns the result message.
func scanCmd(tab *Tab) tea.Cmd {
	return func() tea.Msg {
		if tab.rootPath == "" {
			return scanResultMsg{tabIndex: tab.index, node: nil, err: nil}
		}
		w := scan.New(scan.Options{
			Root:         tab.rootPath,
			Workers:      4,
			DetectFormat: tab.detectFormat,
			Ctx:          tab.ctx,
		})
		node, err := w.Scan()
		return scanResultMsg{tabIndex: tab.index, node: node, err: err}
	}
}

// startScan begins a scan on the tab, cancelling any in-flight scan first.
func (t *Tab) startScan() tea.Cmd {
	t.cancelScan()
	// Recreate context since the old one may be cancelled.
	ctx, cancel := context.WithCancel(context.Background())
	t.ctx = ctx
	t.cancel = cancel
	t.status = StatusScanning
	t.errMsg = ""
	return scanCmd(t)
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
		nodeCount := 0
		if tab.root != nil {
			nodeCount = countNodes(tab.root)
		}
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
