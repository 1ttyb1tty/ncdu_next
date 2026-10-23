package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

// Model is the multi-tab container: tabs, active index, and prompt state.
type Model struct {
	tabs         []*Tab
	active       int
	prompt       bool
	promptInput  string
	quit         bool
	debugEnabled bool
	width        int
}

// New creates a model with one tab rooted at the given path (tab 1 at startup).
func New(rootPath string, root *tree.Node, debugEnabled bool) Model {
	m := Model{
		tabs:         []*Tab{newTab(0)},
		active:       0,
		debugEnabled: debugEnabled,
	}
	t := m.tabs[0]
	t.rootPath = rootPath
	t.root = root
	t.cur = root
	t.status = StatusDone
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
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(msg)
	case scanResultMsg:
		return m.updateScanResult(msg)
	}
	return m, nil
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Prompt mode: collecting root path input.
	if m.prompt {
		switch msg.String() {
		case "enter":
			if m.promptInput == "" {
				// Empty input: keep current root path
				m.prompt = false
				m.promptInput = ""
				return m, m.activeTab().startScan()
			}
			m.activeTab().rootPath = m.promptInput
			m.prompt = false
			m.promptInput = ""
			// Auto-start scan after entering root path
			return m, m.activeTab().startScan()
			case "esc":
				m.prompt = false
				m.promptInput = ""
				return m, nil
			case "backspace":
				if len(m.promptInput) > 0 {
					m.promptInput = m.promptInput[:len(m.promptInput)-1]
				}
				return m, nil
			default:
				r := []rune(msg.String())
				if len(r) == 1 && r[0] >= 32 && r[0] < 127 {
					m.promptInput += string(r[0])
				}
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
		m.promptInput = ""
		return m, nil

	// Debug tab (read-only), only when enabled via --debug
	case "D":
		if !m.debugEnabled {
			return m, nil
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
		if m.active >= len(m.tabs) {
			m.active = len(m.tabs) - 1
		}
		return m, nil
	case "r":
		m.prompt = true
		// Start with current root path, but don't show "." (current dir)
		if m.activeTab().rootPath == "." {
			m.promptInput = ""
		} else {
			m.promptInput = m.activeTab().rootPath
		}
		return m, nil

	// Per-tab keys delegated to active tab
	case "s":
		return m, m.activeTab().startScan()
	case "d":
		t := m.activeTab()
		t.detectFormat = !t.detectFormat
		if t.rootPath != "" {
			return m, t.startScan()
		}
		return m, nil
	case "e":
		m.activeTab().toggleExpanded()
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

func (m Model) updateScanResult(msg scanResultMsg) (tea.Model, tea.Cmd) {
	if msg.tabIndex < 0 || msg.tabIndex >= len(m.tabs) {
		return m, nil
	}
	t := m.tabs[msg.tabIndex]
	if msg.err != nil {
		t.status = StatusError
		t.errMsg = msg.err.Error()
		t.root = nil
		t.cur = nil
	} else if msg.node != nil {
		t.root = msg.node
		t.cur = msg.node
		t.sel = 0
		t.status = StatusDone
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
		s += "root path: " + m.promptInput + "\n(Enter to confirm, Esc to cancel)\n\n"
	}

	// Active tab content
	t := m.activeTab()
	if t.kind == tabKindDebug {
		s += debugView(m.tabs)
	} else {
		s += t.view(m.width)
	}

	// Footer
	s += "\n"
	if m.prompt {
		s += "1-8 switch tab  Ctrl+Tab cycle  t new tab  x close  r edit root  q quit"
	} else if m.debugEnabled {
		s += "1-8 switch tab  Ctrl+Tab cycle  t new tab  x close  r root  s scan  d detect-fmt  e toggle  D debug  up/down nav  enter open  backspace up  c cancel  q quit"
	} else {
		s += "1-8 switch tab  Ctrl+Tab cycle  t new tab  x close  r root  s scan  d detect-fmt  e toggle  up/down nav  enter open  backspace up  c cancel  q quit"
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
