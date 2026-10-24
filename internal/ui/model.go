package ui

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/1ttyb1tty/ncdu-next/internal/config"
	"github.com/1ttyb1tty/ncdu-next/internal/scan"
	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

// promptKind distinguishes what an open input prompt is collecting.
type promptKind int

const (
	promptNone promptKind = iota
	promptRoot
	promptFilter
	promptSearch
)

// Model is the multi-tab container: tabs, active index, and prompt state.
type Model struct {
	tabs         []*Tab
	active       int
	inspect      *Tab
	prompt       bool
	promptKind   promptKind
	promptInput  string
	quit         bool
	debugEnabled bool
	width        int
	height       int
	base         scan.Options
	cfg          config.Config
	// debugInfoStatus is a transient message from the E key's bundle write.
	debugInfoStatus string
}

// New creates a model with one tab rooted at the given path (tab 1 at startup),
// with no filesystem report. base carries the configured scan options used for
// TUI-triggered rescans.
func New(rootPath string, root *tree.Node, debugEnabled bool, base scan.Options) Model {
	return NewWithReport(rootPath, root, nil, debugEnabled, base)
}

// NewWithReport is New plus the initial scan's filesystem report; it is the
// constructor the CLI uses so the header token is right before any rescan.
func NewWithReport(rootPath string, root *tree.Node, fs *scan.FSReport, debugEnabled bool, base scan.Options) Model {
	m := Model{
		tabs:         []*Tab{newTab(0)},
		active:       0,
		debugEnabled: debugEnabled,
		base:         base,
	}
	t := m.tabs[0]
	t.rootPath = rootPath
	t.root = root
	t.cur = root
	t.fs = fs
	t.status = StatusDone
	t.nodeCount = countNodes(root)
	return m
}

// WithConfig records the effective options so a Debug-tab bundle can report what
// the program was asked to do. It returns a copy (Model is used by value).
func (m Model) WithConfig(cfg config.Config) Model {
	m.cfg = cfg
	return m
}

// Init returns no initial command.
func (m Model) Init() tea.Cmd { return nil }

func (m Model) activeTab() *Tab {
	return m.tabs[m.active]
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(msg)
	case scanResultMsg:
		return m.updateScanResult(msg)
	case debugInfoMsg:
		if msg.err != nil {
			m.debugInfoStatus = "debuginfo: " + msg.err.Error()
		} else {
			m.debugInfoStatus = "debuginfo: wrote " + msg.path
		}
		return m, nil
	}
	return m, nil
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Prompt mode: collecting input (root path, filter query, or search query).
	if m.prompt {
		switch msg.String() {
		case "enter":
			switch m.promptKind {
			case promptFilter:
				m.activeTab().applyFilter(m.promptInput, m.activeTab().deep)
				m.prompt = false
				m.promptKind = promptNone
				m.promptInput = ""
				return m, nil
			case promptSearch:
				m.activeTab().buildSearch(m.promptInput)
				m.prompt = false
				m.promptKind = promptNone
				m.promptInput = ""
				return m, nil
			default: // promptRoot
				if m.promptInput != "" {
					m.activeTab().rootPath = m.promptInput
				}
				m.prompt = false
				m.promptKind = promptNone
				m.promptInput = ""
				// Auto-start scan after entering root path (empty keeps current).
				return m, m.activeTab().startScan(m.base)
			}
		case "esc":
			if m.promptKind == promptFilter {
				m.activeTab().clearFilter()
			}
			m.prompt = false
			m.promptKind = promptNone
			m.promptInput = ""
			return m, nil
		case "tab":
			// Filter prompt only: toggle deep (recursive) scope. Let the commit
			// path own the flag and its rebuild, so a parse error cannot leave
			// deep set with a stale/nil visible set.
			if m.promptKind == promptFilter {
				t := m.activeTab()
				t.applyFilter(m.promptInput, !t.deep)
			}
			return m, nil
		case "backspace":
			if len(m.promptInput) > 0 {
				m.promptInput = m.promptInput[:len(m.promptInput)-1]
			}
			if m.promptKind == promptFilter {
				m.activeTab().previewFilter(m.promptInput)
			}
			return m, nil
		default:
			r := []rune(msg.String())
			if len(r) == 1 && r[0] >= 32 && r[0] < 127 {
				m.promptInput += string(r[0])
				if m.promptKind == promptFilter {
					m.activeTab().previewFilter(m.promptInput)
				}
			}
			return m, nil
		}
	}

	// Search-results view: cursor navigation and jump.
	if m.activeTab().kind == tabKindNav && m.activeTab().searchActive {
		switch msg.String() {
		case "up", "k":
			m.activeTab().searchUp()
		case "down", "j":
			m.activeTab().searchDown()
		case "enter", "l":
			m.activeTab().jumpToResult()
		case "esc", "g":
			m.activeTab().closeSearch()
		case "q", "ctrl+c":
			m.quit = true
			return m, tea.Quit
		}
		return m, nil
	}

	// Debug tab: it owns its keys. Scrolling and section jumps act on the page;
	// nav-only keys are ignored (they used to act on the Debug tab, e.g. `g`
	// setting searchActive on a nil-root tab); the retained keys fall through to
	// the shared handling below.
	if m.activeTab().kind == tabKindDebug {
		switch msg.String() {
		case "up", "k":
			m.debugScroll(-1)
			return m, nil
		case "down", "j":
			m.debugScroll(1)
			return m, nil
		case "pgup":
			m.debugScroll(-debugBodyHeight(m.height))
			return m, nil
		case "pgdown", "pgdn":
			m.debugScroll(debugBodyHeight(m.height))
			return m, nil
		case "home":
			m.debugScrollTo(0)
			return m, nil
		case "end":
			m.debugScrollTo(1 << 30)
			return m, nil
		case "[":
			m.debugJumpSection(-1)
			return m, nil
		case "]":
			m.debugJumpSection(1)
			return m, nil
		case "E":
			// Write a bounded bundle (never the full tree) off the UI thread.
			m.debugInfoStatus = "debuginfo: writing..."
			return m, m.writeDebugInfoCmd()
		case "q", "ctrl+c",
			"1", "2", "3", "4", "5", "6", "7", "8",
			"ctrl+tab", "ctrl+shift+tab", "t", "x", "D":
			// Retained: handled by the shared switch below.
		default:
			return m, nil
		}
	}

	switch msg.String() {
	case "q", "ctrl+c":
		m.quit = true
		return m, tea.Quit

	// Tab switching
	case "ctrl+tab":
		m, _ = m.cycleTab(1)
		return m, nil
	case "ctrl+shift+tab":
		m, _ = m.cycleTab(-1)
		return m, nil
	case "1", "2", "3", "4", "5", "6", "7", "8":
		n := int(msg.String()[0] - '1')
		if n < len(m.tabs) {
			m.active = n
		}
		return m, nil

	// Tab management
	case "t":
		if len(m.tabs) >= maxTabs {
			return m, nil
		}
		idx := len(m.tabs)
		m.tabs = append(m.tabs, newTab(idx))
		m.active = idx
		m.prompt = true
		m.promptKind = promptRoot
		m.promptInput = ""
		return m, nil

	// Debug tab (read-only), only when enabled via --debug
	case "D":
		if !m.debugEnabled {
			return m, nil
		}
		// Capture the inspected tab before moving focus, and never overwrite it
		// when D is pressed while the Debug tab is already focused.
		if m.activeTab().kind != tabKindDebug {
			m.inspect = m.activeTab()
		}
		for i, tab := range m.tabs {
			if tab.kind == tabKindDebug {
				m.active = i
				return m, nil
			}
		}
		if len(m.tabs) >= maxTabs {
			return m, nil
		}
		idx := len(m.tabs)
		m.tabs = append(m.tabs, newDebugTab(idx))
		m.active = idx
		return m, nil
	case "x":
		if len(m.tabs) <= 1 {
			return m, nil
		}
		m.tabs = append(m.tabs[:m.active], m.tabs[m.active+1:]...)
		// Reindex so tab identity/position stay consistent after a close.
		for i, t := range m.tabs {
			t.index = i
		}
		if m.active >= len(m.tabs) {
			m.active = len(m.tabs) - 1
		}
		return m, nil
	case "r":
		m.prompt = true
		m.promptKind = promptRoot
		// Start with current root path, but don't show "." (current dir)
		if m.activeTab().rootPath == "." {
			m.promptInput = ""
		} else {
			m.promptInput = m.activeTab().rootPath
		}
		return m, nil

	// Search / filtering
	case "/":
		m.prompt = true
		m.promptKind = promptFilter
		m.promptInput = m.activeTab().filterRaw
		return m, nil
	case "f":
		m.activeTab().toggleDeep()
		return m, nil
	case "g":
		m.prompt = true
		m.promptKind = promptSearch
		m.promptInput = ""
		return m, nil
	case "esc":
		m.activeTab().clearFilter()
		return m, nil

	// Per-tab keys delegated to active tab
	case "s":
		return m, m.activeTab().startScan(m.base)
	case "d":
		t := m.activeTab()
		t.detectFormat = !t.detectFormat
		if t.rootPath != "" {
			return m, t.startScan(m.base)
		}
		return m, nil
	case "e", "w":
		m.activeTab().toggleExpanded()
		return m, nil
	case "u":
		m.activeTab().unit = nextUnit(m.activeTab().unit)
		return m, nil
	case "up", "k":
		m.activeTab().selectUp()
		return m, nil
	case "down", "j":
		m.activeTab().selectDown()
		return m, nil
	case "enter", "l":
		m.activeTab().enter()
		return m, nil
	case "backspace", "h":
		m.activeTab().goUp()
		return m, nil
	case "c":
		m.activeTab().cancelScan()
		m.activeTab().status = StatusIdle
		return m, nil
	}
	return m, nil
}

func (m Model) cycleTab(delta int) (Model, tea.Cmd) {
	n := len(m.tabs)
	m.active = (m.active + delta + n) % n
	return m, nil
}

// debugScroll moves the Debug page offset by delta lines, clamped to the page.
func (m Model) debugScroll(delta int) {
	t := m.activeTab()
	st := buildDebugPage(m.tabs, m.inspect)
	t.scroll = clampOffset(t.scroll+delta, len(st.lines))
}

// debugScrollTo sets the Debug page offset, clamped to the page.
func (m Model) debugScrollTo(offset int) {
	t := m.activeTab()
	st := buildDebugPage(m.tabs, m.inspect)
	t.scroll = clampOffset(offset, len(st.lines))
}

// debugJumpSection moves the Debug page offset to the previous/next section
// heading, staying put at the ends.
func (m Model) debugJumpSection(dir int) {
	t := m.activeTab()
	offs := buildDebugPage(m.tabs, m.inspect).offs
	if len(offs) == 0 {
		return
	}
	if dir > 0 {
		for _, o := range offs {
			if o > t.scroll {
				t.scroll = o
				return
			}
		}
		t.scroll = offs[len(offs)-1]
		return
	}
	for i := len(offs) - 1; i >= 0; i-- {
		if offs[i] < t.scroll {
			t.scroll = offs[i]
			return
		}
	}
	t.scroll = offs[0]
}

func (m Model) updateScanResult(msg scanResultMsg) (tea.Model, tea.Cmd) {
	t := msg.tab
	if t == nil {
		return m, nil
	}
	// Ignore results from a superseded or cancelled scan.
	if msg.gen != t.scanGen {
		return m, nil
	}
	if errors.Is(msg.err, context.Canceled) {
		// Cancellation is not an error: return to idle, keep any prior tree.
		t.status = StatusIdle
		return m, nil
	}
	if msg.err != nil {
		t.status = StatusError
		t.errMsg = msg.err.Error()
		t.root = nil
		t.cur = nil
		t.fs = nil
		t.nodeCount = 0
		t.closeSearch()
		t.visible = nil
	} else if msg.node != nil {
		t.root = msg.node
		t.cur = msg.node
		t.fs = msg.fs
		t.sel = 0
		t.scroll = 0
		t.status = StatusDone
		t.nodeCount = countNodes(msg.node)
		// A new tree invalidates any prior search results; a deep filter's
		// visible set must be recomputed against the new tree.
		t.closeSearch()
		t.rebuildVisible()
		t.clampSelection()
	} else {
		t.status = StatusIdle
	}
	return m, nil
}

func (m Model) View() tea.View {
	v := tea.NewView("")
	if m.quit {
		v.Content = ""
		return v
	}

	// Tab bar
	var bar strings.Builder
	for i, t := range m.tabs {
		if i > 0 {
			bar.WriteString("  ")
		}
		bar.WriteString(t.tabBarEntry(i == m.active))
	}
	s := bar.String() + "\n"
	s += lipgloss.NewStyle().Foreground(lipgloss.Color("#808080")).Render(strings.Repeat("-", lipgloss.Width(bar.String()))) + "\n\n"

	// Prompt overlay
	if m.prompt {
		switch m.promptKind {
		case promptFilter:
			scope := "local"
			if m.activeTab().deep {
				scope = "deep"
			}
			s += "filter [" + scope + "]: " + m.promptInput + "\n(Tab scope, Enter apply, Esc clear)\n\n"
		case promptSearch:
			s += "search: " + m.promptInput + "\n(Enter search, Esc cancel)\n\n"
		default:
			s += "root path: " + m.promptInput + "\n(Enter to confirm, Esc to cancel)\n\n"
		}
	}

	// Active tab content
	t := m.activeTab()
	if t.kind == tabKindDebug {
		// Assemble the page once, clamp against it, then render from the same slice.
		st := buildDebugPage(m.tabs, m.inspect)
		t.scroll = clampOffset(t.scroll, len(st.lines))
		s += debugView(st, t.scroll, m.width, m.height, m.debugInfoStatus)
	} else {
		s += t.view(m.width, m.height)
	}

	// Footer
	s += "\n"
	if m.prompt {
		s += "1-8 switch tab  Ctrl+Tab cycle  t new tab  x close  r edit root  q quit"
	} else if t.kind == tabKindDebug {
		s += "up/down scroll  pgup/pgdown page  home/end  [ ] section  E debuginfo  1-8 switch tab  Ctrl+Tab cycle  x close  D debug  q quit"
	} else if m.debugEnabled {
		s += "1-8 switch tab  Ctrl+Tab cycle  t new tab  x close  r root  s scan  d detect-fmt  e/w wide  u units  / filter  f deep  g search  Esc clear  D debug  up/down nav  enter open  backspace up  c cancel  q quit"
	} else {
		s += "1-8 switch tab  Ctrl+Tab cycle  t new tab  x close  r root  s scan  d detect-fmt  e/w wide  u units  / filter  f deep  g search  Esc clear  up/down nav  enter open  backspace up  c cancel  q quit"
	}
	v.Content = s
	// AltScreen=true makes bubbletea's internal frame model match the real
	// terminal state (we enter the alt screen manually in main.go because
	// bubbletea v2.1.0's alt-screen write path passes write=false). Without
	// this, bubbletea sizes frames to content height and erases below as if
	// in the main buffer, corrupting incremental redraws (border/indent
	// glitches on navigation).
	v.AltScreen = true
	return v
}
