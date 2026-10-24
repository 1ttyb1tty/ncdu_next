package ui

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/1ttyb1tty/ncdu-next/internal/filter"
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

// scanStatusValues is the canonical enum order, kept beside the constants so the
// Debug tab's reference and the AST-guard test share one source of truth.
var scanStatusValues = []ScanStatus{StatusIdle, StatusScanning, StatusDone, StatusError}

// tabKind distinguishes a navigation tab from a read-only debug tab.
type tabKind string

const (
	tabKindNav   tabKind = "nav"
	tabKindDebug tabKind = "debug"
)

// tabKindValues is the canonical tabKind order, kept beside the constants.
var tabKindValues = []tabKind{tabKindNav, tabKindDebug}

// UnitMode selects how sizes are rendered. It cycles bytes -> IEC binary ->
// SI decimal with the `u` key; all modes are integer-only (no floats).
type UnitMode uint8

const (
	UnitBytes UnitMode = iota
	UnitBinary
	UnitDecimal
)

// unitModeValues is the canonical cycle/order used by the toggle and the tests.
var unitModeValues = []UnitMode{UnitBytes, UnitBinary, UnitDecimal}

// unitLabel is the column-header label above the size column for a mode.
func unitLabel(m UnitMode) string {
	switch m {
	case UnitBinary:
		return "KiB/MiB/GiB"
	case UnitDecimal:
		return "kB/MB/GB/TB"
	default:
		return "bytes"
	}
}

// nextUnit returns the next mode in the cycle.
func nextUnit(m UnitMode) UnitMode {
	for i, v := range unitModeValues {
		if v == m {
			return unitModeValues[(i+1)%len(unitModeValues)]
		}
	}
	return UnitBytes
}

// maxTabs is the maximum number of concurrent navigation/scan tabs.
const maxTabs = 8

// wideViewMinWidth is the panel width below which the wide view spills each
// record across two lines instead of one.
const wideViewMinWidth = 120

// Tab is an independent navigation/scan session: root path, scan result,
// navigation state, and per-tab options.
type Tab struct {
	index        int
	rootPath     string
	root         *tree.Node
	cur          *tree.Node
	fs           *scan.FSReport
	sel          int
	scroll       int
	expanded     bool
	unit         UnitMode
	status       ScanStatus
	detectFormat bool
	errMsg       string
	kind         tabKind
	nodeCount    int
	scanGen      int
	ctx          context.Context
	cancel       context.CancelFunc

	// Search/filter state (view-level; never mutates the tree).
	q             *filter.Query
	filterRaw     string
	deep          bool
	filterErr     string
	visible       map[*tree.Node]bool
	searchActive  bool
	searchRaw     string
	searchResults []*tree.Node
	searchSel     int
	searchScroll  int
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

// rawChildren returns the current node's children, unfiltered.
func (t *Tab) rawChildren() []*tree.Node {
	n := t.cur
	if n == nil {
		n = t.root
	}
	if n == nil || !n.IsDir {
		return nil
	}
	return n.Children
}

// children returns the children of the current node (or root's children),
// filtered by the active query. With no active query it returns the unfiltered
// slice unchanged (no allocation), so the default view is byte-for-byte the same.
func (t *Tab) children() []*tree.Node {
	ch := t.rawChildren()
	if t.q == nil || !t.q.Active() {
		return ch
	}
	out := make([]*tree.Node, 0, len(ch))
	if t.deep {
		for _, c := range ch {
			if t.visible[c] {
				out = append(out, c)
			}
		}
		return out
	}
	for _, c := range ch {
		if t.q.Match(c) {
			out = append(out, c)
		}
	}
	return out
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

// applyFilter parses raw and installs it as the active filter. On a parse error
// the previous query is retained and the error is kept for display.
func (t *Tab) applyFilter(raw string, deep bool) {
	q, err := filter.Parse(raw)
	if err != nil {
		t.filterErr = err.Error()
		return
	}
	t.filterErr = ""
	t.q = q
	t.filterRaw = raw
	t.deep = deep
	t.rebuildVisible()
	t.clampSelection()
}

// previewFilter applies a live query only for the shallow (local) scope; the
// deep scope is rebuilt on commit, so typing does not walk the whole tree.
func (t *Tab) previewFilter(raw string) {
	if t.deep {
		return
	}
	t.applyFilter(raw, false)
}

// toggleDeep flips the recursive scope and recomputes visibility.
func (t *Tab) toggleDeep() {
	if t.q == nil {
		return
	}
	t.deep = !t.deep
	t.rebuildVisible()
	t.clampSelection()
}

// rebuildVisible recomputes the deep-scope visible set (nil for shallow scope).
func (t *Tab) rebuildVisible() {
	if t.deep && t.q != nil && t.q.Active() && t.root != nil {
		t.visible = t.q.VisibleSet(t.root)
	} else {
		t.visible = nil
	}
}

// clearFilter removes the active filter, restoring the unfiltered view.
func (t *Tab) clearFilter() {
	t.q = nil
	t.filterRaw = ""
	t.deep = false
	t.filterErr = ""
	t.visible = nil
	t.clampSelection()
}

// clampSelection keeps the row cursor within the currently visible children.
func (t *Tab) clampSelection() {
	n := len(t.children())
	if n == 0 {
		t.sel = 0
		return
	}
	if t.sel >= n {
		t.sel = n - 1
	}
	if t.sel < 0 {
		t.sel = 0
	}
}

// buildSearch runs the query across the whole tree and enters the results view.
func (t *Tab) buildSearch(raw string) {
	q, err := filter.Parse(raw)
	if err != nil {
		t.filterErr = err.Error()
		return
	}
	t.filterErr = ""
	t.searchRaw = raw
	t.searchResults = t.searchResults[:0]
	if t.root != nil {
		var walk func(*tree.Node)
		walk = func(n *tree.Node) {
			if q.Active() && q.Match(n) {
				t.searchResults = append(t.searchResults, n)
			}
			for _, c := range n.Children {
				walk(c)
			}
		}
		walk(t.root)
	}
	t.searchSel = 0
	t.searchScroll = 0
	t.searchActive = true
}

// closeSearch exits the search-results view.
func (t *Tab) closeSearch() {
	t.searchActive = false
	t.searchResults = nil
	t.searchSel = 0
	t.searchScroll = 0
}

// jumpToResult navigates to the selected search result and exits search mode.
func (t *Tab) jumpToResult() {
	if t.searchSel < 0 || t.searchSel >= len(t.searchResults) {
		return
	}
	n := t.searchResults[t.searchSel]
	if n.IsDir {
		t.cur = n
		t.sel = 0
	} else if n.Parent != nil {
		t.cur = n.Parent
		t.sel = 0
		// The row cursor indexes the *visible* (filtered) children, so resolve
		// against children(): if the target is hidden, clampSelection below
		// keeps the cursor in range.
		for i, c := range t.children() {
			if c == n {
				t.sel = i
				break
			}
		}
	}
	t.scroll = 0
	t.closeSearch()
	t.clampSelection()
}

// searchUp and searchDown move the results cursor.
func (t *Tab) searchUp() {
	if t.searchSel > 0 {
		t.searchSel--
	}
}

func (t *Tab) searchDown() {
	if t.searchSel < len(t.searchResults)-1 {
		t.searchSel++
	}
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
	if t.searchActive {
		return t.searchView(width, height)
	}
	// panelWidth/nameWidth/maxRows come from the shared metrics helper so the
	// nav view and the search view cannot drift apart on chrome height or
	// column widths.
	panelWidth, maxRows, nameWidth := panelMetrics(width, height, t.expanded)
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

	// Filtered children are computed once and reused for the header count and
	// the body rows (avoids a second filter pass + allocation per frame).
	ch := t.children()

	// Path header bar: single line, padded to the exact panel width so the
	// background fills precisely and cannot bleed onto adjacent cells. The
	// active filter (scope, query, shown/total) is appended when present.
	headerText := " " + sanitize(t.cur.Path) + " "
	if tok := t.fsToken(); tok != "" {
		headerText += tok + " "
	}
	if t.q != nil && t.q.Active() {
		scope := "filter"
		if t.deep {
			scope = "deep"
		}
		headerText += fmt.Sprintf("[%s: %s - %d/%d] ", scope, sanitize(t.filterRaw), len(ch), len(t.rawChildren()))
	}
	if t.filterErr != "" {
		headerText += "(bad query: " + sanitize(t.filterErr) + ") "
	}
	header := padRight(truncateWidth(headerText, panelWidth), panelWidth)
	s.WriteString(headerStyle.Render(header))
	s.WriteString("\n")

	// Horizontal separators drawn manually at the exact panel width.
	sep := sepStyle.Render(strings.Repeat("-", panelWidth))

	// Wide view may spill each record across two lines on a narrow panel, so the
	// number of records that fit is the body height divided by lines/record.
	spill := t.expanded && panelWidth < wideViewMinWidth
	linesPerRecord := 1
	if spill {
		linesPerRecord = 2
	}
	maxRecords := maxRows / linesPerRecord
	if maxRecords < 1 {
		maxRecords = 1
	}

	// Keep the selection within the visible window.
	if t.sel < t.scroll {
		t.scroll = t.sel
	}
	if t.sel >= t.scroll+maxRecords {
		t.scroll = t.sel - maxRecords + 1
	}
	if t.scroll < 0 {
		t.scroll = 0
	}
	end := t.scroll + maxRecords
	if end > len(ch) {
		end = len(ch)
	}

	// Column-label row; the size header reflects the current unit mode.
	labels := labelStyle.Render(padRight(truncateWidth(t.columnLabels(nameWidth, spill), panelWidth), panelWidth))

	rows := make([]string, 0, maxRows)
	for i := t.scroll; i < end; i++ {
		c := ch[i]
		mark := "  "
		if i == t.sel {
			mark = "> "
		}

		rowStyle := styleForNode(c)
		if i == t.sel {
			rowStyle = rowStyle.Background(lipgloss.Color("#2A2A2A")).Bold(true)
		}

		name := padRight(truncateWidth(sanitize(c.Name), nameWidth), nameWidth)
		line1, line2, two := t.recordLines(c, mark, name, spill)
		rows = append(rows, rowStyle.Render(padRight(line1, panelWidth)))
		if two {
			rows = append(rows, rowStyle.Render(padRight(line2, panelWidth)))
		}
	}

	if len(rows) == 0 {
		msg := "(empty)"
		if t.q != nil && t.q.Active() {
			msg = "(no matches)"
		}
		rows = append(rows, padRight(msg, panelWidth))
	}
	s.WriteString(sep)
	s.WriteString("\n")
	s.WriteString(labels)
	s.WriteString("\n")
	s.WriteString(strings.Join(rows, "\n"))
	s.WriteString("\n")
	s.WriteString(sep)

	return s.String()
}

// columnLabels renders the header row above the data columns for the current
// view/unit mode. In spill mode only the first line of the wide header is shown.
func (t *Tab) columnLabels(nameWidth int, spill bool) string {
	if !t.expanded {
		return fmt.Sprintf("%s %s %12s", "  ", padRight("name", nameWidth), unitLabel(t.unit))
	}
	if spill {
		return fmt.Sprintf("%s %s %12s %s", "  ", padRight("name", nameWidth), unitLabel(t.unit), padRight("kind", 13))
	}
	return fmt.Sprintf("%s %s %12s %s %s %s %s %s %s",
		"  ", padRight("name", nameWidth), unitLabel(t.unit),
		padRight("kind", 13), padRight("attrs", 6), padRight("type", 10),
		padRight("enc", 6), padRight("lines", 6), padRight("mtime", 16))
}

// recordLines renders one record body as either a single wide/simple line or, in
// spill mode, two lines: name/size/kind/attrs then type/encoding/lines/mtime. It
// returns the second line only when two is true, so the common single-line case
// allocates no slice (unlike a []string return, which escapes to the heap).
func (t *Tab) recordLines(c *tree.Node, mark, name string, spill bool) (line1, line2 string, two bool) {
	size := t.sizeText(c.Size)
	if !t.expanded {
		return fmt.Sprintf("%s %s %12s", mark, name, size), "", false
	}
	kind := padRight(truncateWidth(kindName(c), 13), 13)
	attrs := padRight(truncateWidth(scan.AttrLetters(c.Attrs), 6), 6)
	if spill {
		line1 = fmt.Sprintf("%s %s %12s %s %s", mark, name, size, kind, attrs)
		line2 = fmt.Sprintf("   %s %s %s %s",
			padRight(truncateWidth(c.Type, 10), 10),
			padRight(truncateWidth(c.Encoding, 6), 6),
			padRight(truncateWidth(c.LineEnding, 6), 6),
			formatTime(c.Mtime))
		return line1, line2, true
	}
	line1 = fmt.Sprintf("%s %s %12s %s %s %s %s %s %s",
		mark, name, size, kind, attrs,
		padRight(truncateWidth(c.Type, 10), 10),
		padRight(truncateWidth(c.Encoding, 6), 6),
		padRight(truncateWidth(c.LineEnding, 6), 6),
		formatTime(c.Mtime))
	return line1, "", false
}

// searchView renders the flat, full-tree search-results list.
func (t *Tab) searchView(width, height int) string {
	panelWidth, maxRows, nameWidth := panelMetrics(width, height, false)

	headerText := fmt.Sprintf(" search: %s (%d matches) ", sanitize(t.searchRaw), len(t.searchResults))
	header := padRight(truncateWidth(headerText, panelWidth), panelWidth)
	sep := sepStyle.Render(strings.Repeat("-", panelWidth))

	labelContent := fmt.Sprintf("%s %s %12s", "  ", padRight("path", nameWidth), unitLabel(t.unit))
	labels := labelStyle.Render(padRight(labelContent, panelWidth))

	if t.searchSel < t.searchScroll {
		t.searchScroll = t.searchSel
	}
	if t.searchSel >= t.searchScroll+maxRows {
		t.searchScroll = t.searchSel - maxRows + 1
	}
	if t.searchScroll < 0 {
		t.searchScroll = 0
	}
	end := t.searchScroll + maxRows
	if end > len(t.searchResults) {
		end = len(t.searchResults)
	}

	rows := make([]string, 0, maxRows)
	for i := t.searchScroll; i < end; i++ {
		c := t.searchResults[i]
		mark := "  "
		if i == t.searchSel {
			mark = "> "
		}
		rowStyle := styleForNode(c)
		if i == t.searchSel {
			rowStyle = rowStyle.Background(lipgloss.Color("#2A2A2A")).Bold(true)
		}
		p := c.Path
		if p == "" {
			p = c.Name
		}
		p = sanitize(p)
		rowContent := fmt.Sprintf("%s %s %12s", mark, padRight(truncateWidth(p, nameWidth), nameWidth), t.sizeText(c.Size))
		rows = append(rows, rowStyle.Render(padRight(rowContent, panelWidth)))
	}
	if len(rows) == 0 {
		rows = append(rows, padRight("(no matches)", panelWidth))
	}

	var s strings.Builder
	s.WriteString(headerStyle.Render(header))
	s.WriteString("\n")
	s.WriteString(sep)
	s.WriteString("\n")
	s.WriteString(labels)
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
	labelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080")).Bold(true)
	styleDefault = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))
	// kindStyles maps a broad Kind to its row color. Keyed by Kind (derived from
	// Type) so a new extension inherits its category color automatically.
	kindStyles = map[tree.Kind]lipgloss.Style{
		tree.KindDiskImage:      lipgloss.NewStyle().Foreground(lipgloss.Color("#0000FF")),
		tree.KindContainerImage: lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")),
		tree.KindMemoryState:    lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")),
		tree.KindOfficeDoc:      lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6347")),
		tree.KindImage:          lipgloss.NewStyle().Foreground(lipgloss.Color("#90EE90")),
		tree.KindAudio:          lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")),
		tree.KindVideo:          lipgloss.NewStyle().Foreground(lipgloss.Color("#ADD8E6")),
		tree.KindText:           lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")),
		tree.KindSource:         lipgloss.NewStyle().Foreground(lipgloss.Color("#E0FFFF")),
		tree.KindMarkup:         lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")),
		tree.KindOther:          styleDefault,
	}
	// kindStyleOrder is the fixed enumeration order for the Debug tab.
	kindStyleOrder = tree.KindValues()
)

// kindName returns the node's broad-category name via the shared resolver.
func kindName(n *tree.Node) string {
	return tree.KindOf(n).String()
}

// styleForKind returns the precomputed style for a broad Kind, defaulting to gray.
func styleForKind(k tree.Kind) lipgloss.Style {
	if s, ok := kindStyles[k]; ok {
		return s
	}
	return styleDefault
}

// styleForNode returns the precomputed row style for a node via the shared Kind
// resolver.
func styleForNode(n *tree.Node) lipgloss.Style {
	return styleForKind(tree.KindOf(n))
}

// panelMetrics returns the shared list-view layout metrics (panel width, body
// row count, name column width) so the navigation view and the search-results
// view cannot drift apart on chrome height or column widths.
func panelMetrics(width, height int, wide bool) (panelWidth, maxRows, nameWidth int) {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	panelWidth = width - 2
	if panelWidth < 30 {
		panelWidth = 30
	}
	// Total height minus the tab bar, the path header, the column-label row,
	// the two separators, the footer and blank lines.
	maxRows = height - 8
	if maxRows < 1 {
		maxRows = 1
	}
	// Name column width leaves room for the fixed columns: simple view has only
	// the size column left of the name; wide reserves kind/attrs/type/enc/lines/
	// mtime when it fits on one line, or the shorter first-line set when it spills.
	switch {
	case !wide:
		nameWidth = panelWidth - 16
	case panelWidth < wideViewMinWidth:
		nameWidth = panelWidth - 37
	default:
		nameWidth = panelWidth - 79
	}
	if nameWidth < 10 {
		nameWidth = 10
	}
	return panelWidth, maxRows, nameWidth
}

// sanitize replaces ASCII control bytes (and DEL) with '?' so filenames and
// paths read from the filesystem cannot inject terminal escape sequences into
// the rendered frame.
func sanitize(s string) string {
	clean := true
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			b.WriteRune('?')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
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

// humanSize formats a byte count in 1024-based (IEC) units, integer-only:
// KiB, MiB, GiB, TiB, PiB, EiB. Values below 1 KiB are a plain byte count and
// larger values are rounded to the nearest whole unit (round half up).
func humanSize(n int64) string {
	return formatSize(n, UnitBinary)
}

// formatSize renders a byte count in the given unit mode. All modes are
// integer-only: UnitBytes is a plain count; UnitBinary uses 1024-based IEC
// suffixes; UnitDecimal uses 1000-based SI suffixes.
func formatSize(n int64, mode UnitMode) string {
	if n < 0 {
		n = 0
	}
	switch mode {
	case UnitBinary:
		return scaleSize(n, 1024, []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"})
	case UnitDecimal:
		return scaleSize(n, 1000, []string{"kB", "MB", "GB", "TB", "PB", "EB"})
	default:
		return strconv.FormatInt(n, 10)
	}
}

// scaleSize divides n by successive powers of base, picking the largest unit
// that keeps the value >= 1, and rounds half up to a whole unit. Values below
// one unit are a plain byte count.
func scaleSize(n, base int64, units []string) string {
	if n < base {
		return strconv.FormatInt(n, 10)
	}
	div := base
	i := 0
	for i+1 < len(units) && n/div >= base {
		div *= base
		i++
	}
	v := (n + div/2) / div
	return strconv.FormatInt(v, 10) + " " + units[i]
}

// formatTime renders a Unix-epoch-nanosecond timestamp as "YYYY-MM-DD HH:MM" in
// UTC. UTC keeps the rendered value and its tests independent of the host
// timezone. A zero value (unread) renders as "-".
func formatTime(ns int64) string {
	if ns == 0 {
		return "-"
	}
	return time.Unix(0, ns).UTC().Format("2006-01-02 15:04")
}

// sizeText renders a node size according to the tab's unit mode.
func (t *Tab) sizeText(n int64) string {
	return formatSize(n, t.unit)
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
	fs   *scan.FSReport
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
		return scanResultMsg{tab: tab, gen: gen, node: node, fs: w.FSReport(), err: err}
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

// debugSection is one titled block of the Debug page. Zone groups sections into
// the "tab" zone (about one inspected tab) and the "program" zone (about the
// process as a whole, including the record reference). It is ui-local, so
// TestDocCoverage (which walks tree/scan/config only) ignores it.
type debugSection struct {
	Zone  string
	Title string
	Lines []string
}

// debugBodyHeight is the number of page lines the Debug view can show at the
// given terminal height, after the pinned chrome (tab bar, separators, title,
// zone bar, footer).
func debugBodyHeight(height int) int {
	if height <= 0 {
		height = 24
	}
	h := height - 7
	if h < 1 {
		h = 1
	}
	return h
}

// nodeRecSize is the in-memory size of one tree.Node record.
func nodeRecSize() uint64 { return uint64(reflect.TypeOf(tree.Node{}).Size()) }

// resolveInspect returns the tab whose detail the Debug page shows, validated
// against the live tab list so closing it cannot leave a dangling pointer; it
// falls back to the first navigation tab, never to the Debug tab itself.
func resolveInspect(tabs []*Tab, inspect *Tab) *Tab {
	if inspect != nil {
		for _, t := range tabs {
			if t == inspect {
				return t
			}
		}
	}
	for _, t := range tabs {
		if t.kind == tabKindNav {
			return t
		}
	}
	return nil
}

// splitLines splits a renderer's text into lines, dropping one trailing newline.
func splitLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// bodyAfterHeading drops the renderer's own heading line (its first line) so the
// page can supply the heading as the section Title instead.
func bodyAfterHeading(s string) []string {
	lines := splitLines(s)
	if len(lines) > 0 {
		lines = lines[1:]
	}
	return lines
}

// tabsTableLines renders the all-tabs table with the inspected row marked.
func tabsTableLines(tabs []*Tab, inspect *Tab) []string {
	lines := []string{fmt.Sprintf("  %-4s %-6s %-10s %-6s %-10s %s", "#", "kind", "status", "nodes", "est/mem", "root")}
	rec := nodeRecSize()
	for _, tab := range tabs {
		marker := "  "
		if tab == inspect {
			marker = "> "
		}
		root := tab.rootPath
		if len(root) > 40 {
			root = "..." + root[len(root)-37:]
		}
		statusStr := string(tab.status)
		if tab.status == StatusIdle && tab.root == nil {
			statusStr = "empty"
		}
		est := bytesHuman(rec * uint64(tab.nodeCount))
		lines = append(lines, fmt.Sprintf("%s%-4d %-6s %-10s %-6d %-10s %s", marker, tab.displayNumber(), tab.kind, statusStr, tab.nodeCount, est, sanitize(root)))
	}
	if len(tabs) == 0 {
		lines = append(lines, "  (no tabs)")
	}
	return lines
}

// inspectDetailLines renders the per-tab detail block for one tab.
func inspectDetailLines(t *Tab) []string {
	if t == nil {
		return []string{"  (no inspected tab)"}
	}
	lines := []string{
		fmt.Sprintf("  tab          %d (%s)", t.displayNumber(), t.kind),
		fmt.Sprintf("  status       %s", t.status),
		fmt.Sprintf("  root path    %s", sanitize(t.rootPath)),
		fmt.Sprintf("  node count   %d", t.nodeCount),
		fmt.Sprintf("  est/mem      %s", bytesHuman(nodeRecSize()*uint64(t.nodeCount))),
		fmt.Sprintf("  unit mode    %s", unitLabel(t.unit)),
		fmt.Sprintf("  expanded     %v", t.expanded),
		fmt.Sprintf("  detect fmt   %v", t.detectFormat),
	}
	if t.q != nil && t.q.Active() {
		scope := "local"
		if t.deep {
			scope = "deep"
		}
		lines = append(lines, fmt.Sprintf("  filter       %s %q", scope, sanitize(t.filterRaw)))
	} else {
		lines = append(lines, "  filter       (none)")
	}
	if t.filterErr != "" {
		lines = append(lines, "  filter err   "+sanitize(t.filterErr))
	}
	if t.searchActive {
		lines = append(lines, fmt.Sprintf("  search       %q (%d results)", sanitize(t.searchRaw), len(t.searchResults)))
	} else {
		lines = append(lines, "  search       (inactive)")
	}
	return lines
}

// totalsLines renders the cross-tab totals.
func totalsLines(tabs []*Tab) []string {
	total := 0
	for _, t := range tabs {
		total += t.nodeCount
	}
	rec := nodeRecSize()
	return []string{
		fmt.Sprintf("  tabs         %d", len(tabs)),
		fmt.Sprintf("  nodes        %d", total),
		fmt.Sprintf("  est/mem      %s", bytesHuman(rec*uint64(total))),
		fmt.Sprintf("  (est/mem = %d B/record * nodes; excludes string/slice payloads)", rec),
	}
}

// runtimeLines renders the process runtime block.
func runtimeLines() []string {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return []string{
		fmt.Sprintf("  alloc        %s", bytesHuman(ms.Alloc)),
		fmt.Sprintf("  total_alloc  %s", bytesHuman(ms.TotalAlloc)),
		fmt.Sprintf("  sys          %s", bytesHuman(ms.Sys)),
		fmt.Sprintf("  mallocs      %s", formatUint(ms.Mallocs)),
		fmt.Sprintf("  frees        %s", formatUint(ms.Frees)),
		fmt.Sprintf("  gc_cycles    %s", formatUint(uint64(ms.NumGC))),
		fmt.Sprintf("  goroutines   %s", formatUint(uint64(runtime.NumGoroutine()))),
		fmt.Sprintf("  cgo_calls    %s", formatUint(uint64(runtime.NumCgoCall()))),
		fmt.Sprintf("  num_cpu      %s", formatUint(uint64(runtime.NumCPU()))),
	}
}

// debugSections builds the whole Debug page, in display order: the `tab` zone
// (all tabs, one inspected tab's detail) first so pressing D shows it without
// scrolling, then the `program` zone (runtime, totals, record reference).
func debugSections(tabs []*Tab, inspect *Tab) []debugSection {
	ins := resolveInspect(tabs, inspect)
	secs := []debugSection{
		{Zone: "tab", Title: "tabs", Lines: tabsTableLines(tabs, ins)},
		{Zone: "tab", Title: "inspected tab", Lines: inspectDetailLines(ins)},
	}
	secs = append(secs, filesystemSections(ins)...)
	secs = append(secs,
		debugSection{Zone: "program", Title: "runtime", Lines: runtimeLines()},
		debugSection{Zone: "program", Title: "totals", Lines: totalsLines(tabs)},
		debugSection{Zone: "program", Title: fmt.Sprintf("datatypes [%s/%s]", runtime.GOOS, runtime.GOARCH), Lines: bodyAfterHeading(datatypesView())},
		debugSection{Zone: "program", Title: fmt.Sprintf("records [%s/%s]", runtime.GOOS, runtime.GOARCH), Lines: bodyAfterHeading(recordsBody())},
		debugSection{Zone: "program", Title: "flags", Lines: bodyAfterHeading(flagsView())},
		debugSection{Zone: "program", Title: "tables", Lines: bodyAfterHeading(tablesView())},
		debugSection{Zone: "program", Title: "enums", Lines: bodyAfterHeading(enumsView())},
	)
	return secs
}

// debugLayout flattens the sections into page lines and records each section
// heading's line offset from the same pass, so the [/] jumps cannot disagree with
// what is rendered. A zone heading is emitted when the zone changes.
func debugLayout(secs []debugSection) (lines []string, offsets []int) {
	lastZone := ""
	for i, sec := range secs {
		if i > 0 {
			lines = append(lines, "")
		}
		if sec.Zone != lastZone {
			lines = append(lines, "== "+sec.Zone+" zone ==")
			lastZone = sec.Zone
		}
		offsets = append(offsets, len(lines))
		lines = append(lines, sec.Title)
		lines = append(lines, sec.Lines...)
	}
	if len(lines) == 0 {
		lines = append(lines, "(no debug data)")
	}
	return lines, offsets
}

// debugPage flattens the sections into the page lines, one blank line between
// section bodies so a heading is visually distinct.
func debugPage(secs []debugSection) []string {
	lines, _ := debugLayout(secs)
	return lines
}

// sectionOffsets returns the page line offset of each section heading, for the
// [/] jumps.
func sectionOffsets(secs []debugSection) []int {
	_, offs := debugLayout(secs)
	return offs
}

// debugViewport returns the visible window of the page for a given line offset
// and body height, clamped to the page bounds.
func debugViewport(lines []string, offset, height int) []string {
	if len(lines) == 0 || height <= 0 {
		return nil
	}
	if offset < 0 {
		offset = 0
	}
	if offset > len(lines)-1 {
		offset = len(lines) - 1
	}
	end := offset + height
	if end > len(lines) {
		end = len(lines)
	}
	return lines[offset:end]
}

// clampOffset keeps a page line offset within [0, n-1].
func clampOffset(offset, n int) int {
	if n <= 0 {
		return 0
	}
	if offset < 0 {
		return 0
	}
	if offset > n-1 {
		return n - 1
	}
	return offset
}

// zoneAt returns the zone name of the section containing the page line offset.
func zoneAt(secs []debugSection, offset int) string {
	offs := sectionOffsets(secs)
	zone := ""
	for i, o := range offs {
		if offset >= o {
			zone = secs[i].Zone
		}
	}
	return zone
}

// debugZoneBar is the pinned bar naming the inspected tab and the current zone
// and position within the page. ins must already be resolved against the live
// tab list.
func debugZoneBar(ins *Tab, secs []debugSection, offset, total int) string {
	tabPart := "tab -"
	if ins != nil {
		tabPart = fmt.Sprintf("tab %d (%s)", ins.displayNumber(), sanitize(ins.rootPath))
	}
	zone := zoneAt(secs, offset)
	if zone == "" {
		zone = "tab"
	}
	return fmt.Sprintf(" %s | %s  %d/%d ", tabPart, zone, offset+1, total)
}

// debugView renders the Debug tab: the pinned header and zone/position bar, then
// the visible window of the assembled page.
func debugView(tabs []*Tab, inspect *Tab, offset, width, height int) string {
	ins := resolveInspect(tabs, inspect)
	secs := debugSections(tabs, ins)
	lines := debugPage(secs)
	offset = clampOffset(offset, len(lines))

	panelWidth := width - 2
	if panelWidth < 30 {
		panelWidth = 30
	}

	bar := debugZoneBar(ins, secs, offset, len(lines))

	var s strings.Builder
	s.WriteString(lipgloss.NewStyle().Bold(true).Render("DEBUG - ncdu-next"))
	s.WriteString("\n")
	s.WriteString(truncateWidth(bar, panelWidth))
	s.WriteString("\n")
	for _, ln := range debugViewport(lines, offset, debugBodyHeight(height)) {
		s.WriteString(truncateWidth(ln, panelWidth))
		s.WriteString("\n")
	}
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
