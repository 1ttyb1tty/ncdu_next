package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

type Model struct {
	root *tree.Node
	cur  *tree.Node
	sel  int
}

func New(root *tree.Node) Model { return Model{root: root, cur: root} }

func (m Model) Init() tea.Cmd { return nil }

type keyMsg struct{ key string }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.sel > 0 {
				m.sel--
			}
		case "down", "j":
			if m.cur != nil && m.sel < len(m.cur.Children)-1 {
				m.sel++
			}
		case "enter", "l":
			if m.cur != nil && m.sel < len(m.cur.Children) && m.cur.Children[m.sel].IsDir {
				m.cur = m.cur.Children[m.sel]
				m.sel = 0
			}
		case "backspace", "h":
			if m.cur != nil && m.cur.Parent != nil {
				m.cur = m.cur.Parent
				m.sel = 0
			}
		}
	}
	return m, nil
}

func (m Model) View() tea.View {
	v := tea.NewView("")
	if m.cur == nil {
		v.Content = "no data"
		return v
	}
	s := lipgloss.NewStyle().Bold(true).Render(m.cur.Path) + "\n"
	for i, c := range m.cur.Children {
		mark := "  "
		if i == m.sel {
			mark = "> "
		}
		s += fmt.Sprintf("%s%-40s %12d\n", mark, c.Name, c.Size)
	}
	s += "\nq quit  ↑/↓ select  enter open  backspace up"
	v.Content = s
	return v
}
