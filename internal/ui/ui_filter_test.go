package ui

import (
	"strings"
	"testing"

	"github.com/1ttyb1tty/ncdu-next/internal/scan"
	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

// filterTree builds a small tree used by the filter/search tests.
func filterTree() *tree.Node {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	root.AddChild(&tree.Node{Name: "a.txt", IsDir: false, Path: "/root/a.txt", Size: 10, Type: "text-plain"})
	root.AddChild(&tree.Node{Name: "b.log", IsDir: false, Path: "/root/b.log", Size: 20, Type: "text-log"})
	sub := &tree.Node{Name: "sub", IsDir: true, Path: "/root/sub"}
	sub.AddChild(&tree.Node{Name: "note.md", IsDir: false, Path: "/root/sub/note.md", Size: 30, Type: "markup-md"})
	root.AddChild(sub)
	return root
}

func typeString(m Model, s string) Model {
	for _, r := range s {
		m2, _ := m.Update(keyMsg(string(r)))
		m = m2.(Model)
	}
	return m
}

func TestFilterPromptLiveFiltersRows(t *testing.T) {
	m := New("/root", filterTree(), false, scan.Options{})

	m2, _ := m.Update(keyMsg("/"))
	m = m2.(Model)
	if !m.prompt || m.promptKind != promptFilter {
		t.Fatal("expected filter prompt after /")
	}
	m = typeString(m, "log")
	v := m.View().Content
	if !strings.Contains(v, "b.log") {
		t.Fatalf("live filter should show b.log, got:\n%s", v)
	}
	if strings.Contains(v, "a.txt") {
		t.Fatalf("live filter should hide a.txt, got:\n%s", v)
	}

	// Enter commits the filter (prompt closes, rows stay filtered).
	m3, _ := m.Update(keyMsg("enter"))
	m = m3.(Model)
	if m.prompt {
		t.Fatal("prompt should close after enter")
	}
	if !strings.Contains(m.View().Content, "b.log") {
		t.Fatal("committed filter should keep b.log visible")
	}

	// Esc in nav mode clears the filter.
	m4, _ := m.Update(keyMsg("esc"))
	m = m4.(Model)
	if !strings.Contains(m.View().Content, "a.txt") {
		t.Fatal("esc should clear the filter and restore a.txt")
	}
}

func TestFilterEscInPromptClears(t *testing.T) {
	m := New("/root", filterTree(), false, scan.Options{})
	m2, _ := m.Update(keyMsg("/"))
	m = typeString(m2.(Model), "log")
	m3, _ := m.Update(keyMsg("esc"))
	m = m3.(Model)
	if m.prompt {
		t.Fatal("prompt should close on esc")
	}
	if m.activeTab().q != nil && m.activeTab().q.Active() {
		t.Fatal("filter should be cleared on esc")
	}
	if !strings.Contains(m.View().Content, "a.txt") {
		t.Fatal("cleared filter should restore a.txt")
	}
}

func TestFilterDeepScope(t *testing.T) {
	m := New("/root", filterTree(), false, scan.Options{})

	// Enter a query and toggle deep scope via Tab inside the prompt.
	m2, _ := m.Update(keyMsg("/"))
	m = typeString(m2.(Model), "note")
	m3, _ := m.Update(keyMsg("tab"))
	m = m3.(Model)
	if !m.activeTab().deep {
		t.Fatal("tab should enable deep scope")
	}
	m4, _ := m.Update(keyMsg("enter"))
	m = m4.(Model)

	v := m.View().Content
	if !strings.Contains(v, "[deep:") {
		t.Fatalf("header should show deep scope, got:\n%s", v)
	}
	if !strings.Contains(v, "sub") {
		t.Fatal("deep filter should keep the ancestor directory sub visible")
	}
	if strings.Contains(v, "a.txt") {
		t.Fatal("deep filter should hide non-matching a.txt")
	}
}

func TestFilterLocalNoMatch(t *testing.T) {
	m := New("/root", filterTree(), false, scan.Options{})
	m2, _ := m.Update(keyMsg("/"))
	m = typeString(m2.(Model), "nomatch")
	m3, _ := m.Update(keyMsg("enter"))
	m = m3.(Model)
	if !strings.Contains(m.View().Content, "(no matches)") {
		t.Fatal("a non-matching local filter should render (no matches)")
	}
}

func TestInvalidQueryKeepsPrevious(t *testing.T) {
	m := New("/root", filterTree(), false, scan.Options{})
	tb := m.activeTab()

	// A valid filter first.
	tb.applyFilter("a", false)
	if !strings.Contains(m.View().Content, "a.txt") {
		t.Fatal("valid filter should show a.txt")
	}

	// An invalid query must set filterErr and retain the previous query.
	tb.applyFilter("re:(", false)
	if tb.filterErr == "" {
		t.Fatal("invalid regex should set filterErr")
	}
	if !strings.Contains(m.View().Content, "a.txt") {
		t.Fatal("invalid query must retain the previous (valid) filter")
	}
}

func TestBadQueryMarker(t *testing.T) {
	m := New("/root", filterTree(), false, scan.Options{})
	m2, _ := m.Update(keyMsg("/"))
	m = typeString(m2.(Model), "re:(")
	if m.activeTab().filterErr == "" {
		t.Fatal("typing an invalid regex should set filterErr")
	}
	if !strings.Contains(m.View().Content, "bad query") {
		t.Fatalf("header should flag the bad query, got:\n%s", m.View().Content)
	}
}

func TestSearchBuildAndJump(t *testing.T) {
	m := New("/root", filterTree(), false, scan.Options{})

	m2, _ := m.Update(keyMsg("g"))
	m = typeString(m2.(Model), "note")
	m3, _ := m.Update(keyMsg("enter"))
	m = m3.(Model)
	if !m.activeTab().searchActive {
		t.Fatal("enter in the search prompt should open the results view")
	}
	v := m.View().Content
	if !strings.Contains(v, "search:") || !strings.Contains(v, "note.md") {
		t.Fatalf("search view should list note.md, got:\n%s", v)
	}

	// Enter jumps to the result's parent and exits search.
	m4, _ := m.Update(keyMsg("enter"))
	m = m4.(Model)
	if m.activeTab().searchActive {
		t.Fatal("jump should exit search mode")
	}
	if m.activeTab().cur == nil || m.activeTab().cur.Name != "sub" {
		t.Fatalf("jump should navigate to the parent dir, got %v", m.activeTab().cur)
	}
}

func TestSearchEscCloses(t *testing.T) {
	m := New("/root", filterTree(), false, scan.Options{})
	m2, _ := m.Update(keyMsg("g"))
	m = typeString(m2.(Model), "note")
	m3, _ := m.Update(keyMsg("enter"))
	m = m3.(Model)
	m4, _ := m.Update(keyMsg("esc"))
	m = m4.(Model)
	if m.activeTab().searchActive {
		t.Fatal("esc should close the search view")
	}
}

func TestFilterPromptTabWithInvalidQueryKeepsScope(t *testing.T) {
	m := New("/root", filterTree(), false, scan.Options{})
	m.activeTab().applyFilter("a", false)

	m2, _ := m.Update(keyMsg("/"))
	m = typeString(m2.(Model), "[") // promptInput becomes "a[" -> invalid glob
	if m.activeTab().filterErr == "" {
		t.Fatal("expected a parse error for the invalid query")
	}
	m3, _ := m.Update(keyMsg("tab"))
	m = m3.(Model)
	if m.activeTab().deep {
		t.Fatal("deep must not flip when the query fails to parse")
	}
	if !strings.Contains(m.View().Content, "a.txt") {
		t.Fatalf("the previous valid filter should still apply, got:\n%s", m.View().Content)
	}
}

func TestJumpToVisibleResultUnderFilter(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	root.AddChild(&tree.Node{Name: "x.txt", IsDir: false, Path: "/root/x.txt", Size: 1})
	root.AddChild(&tree.Node{Name: "target.log", IsDir: false, Path: "/root/target.log", Size: 2})
	root.AddChild(&tree.Node{Name: "y.txt", IsDir: false, Path: "/root/y.txt", Size: 3})
	m := New("/root", root, false, scan.Options{})

	// Filter hides everything but target.log.
	m.activeTab().applyFilter("log", false)

	m2, _ := m.Update(keyMsg("g"))
	m = typeString(m2.(Model), "target.log")
	m3, _ := m.Update(keyMsg("enter")) // run search
	m = m3.(Model)
	m4, _ := m.Update(keyMsg("enter")) // jump
	m = m4.(Model)

	if m.activeTab().cur != root {
		t.Fatalf("expected to land in root, got %v", m.activeTab().cur)
	}
	// The cursor indexes the visible (filtered) list: target.log is row 0 there.
	if m.activeTab().sel != 0 {
		t.Fatalf("expected cursor on filtered row 0, got %d", m.activeTab().sel)
	}
	if !strings.Contains(m.View().Content, ">  target.log") {
		t.Fatalf("cursor should mark target.log, got:\n%s", m.View().Content)
	}
}

func TestSanitizeControlBytes(t *testing.T) {
	if got := sanitize("plain"); got != "plain" {
		t.Fatalf("clean string altered: %q", got)
	}
	got := sanitize("a\x1b[2Jb")
	if strings.ContainsRune(got, 0x1b) {
		t.Fatalf("ESC not stripped: %q", got)
	}
	if !strings.Contains(got, "a?[2Jb") {
		t.Fatalf("unexpected sanitize result: %q", got)
	}
}

func TestViewSanitizesFileName(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	root.AddChild(&tree.Node{Name: "evil\x1b[2Jname", IsDir: false, Path: "/root/evil\x1b[2Jname", Size: 1})
	m := New("/root", root, false, scan.Options{})
	if !strings.Contains(m.View().Content, "evil?[2Jname") {
		t.Fatal("control bytes in a filename must be replaced before rendering")
	}
}
