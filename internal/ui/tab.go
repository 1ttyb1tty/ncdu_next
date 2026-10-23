package ui

import (
	"context"
	"fmt"
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
	ctx          context.Context
	cancel       context.CancelFunc
}

// newTab creates an idle tab with its own cancellable context.
func newTab(index int) *Tab {
	ctx, cancel := context.WithCancel(context.Background())
	return &Tab{
		index:  index,
		status: StatusIdle,
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
			s.WriteString("no root set — press r to enter a path, then s to scan")
		} else {
			s.WriteString("no data")
		}
		return s.String()
	}
	s.WriteString(lipgloss.NewStyle().Bold(true).Render(t.cur.Path))
	s.WriteString("\n")
	for i, c := range t.children() {
		mark := "  "
		if i == t.sel {
			mark = "> "
		}
		if t.expanded {
			s.WriteString(fmt.Sprintf("%s%-40s %12d %s %s %s\n", mark, c.Name, c.Size, c.Type, c.Encoding, c.LineEnding))
		} else {
			s.WriteString(fmt.Sprintf("%s%-40s %12d\n", mark, c.Name, c.Size))
		}
	}
	return s.String()
}

// tabBarEntry renders the tab's entry in the tab bar, e.g. "1*[scanning]".
func (t *Tab) tabBarEntry(active bool) string {
	statusStr := ""
	switch t.status {
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
