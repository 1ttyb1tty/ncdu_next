package filter

import (
	"testing"

	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

func node(name string, isDir bool, size int64, typ string) *tree.Node {
	return &tree.Node{Name: name, IsDir: isDir, Size: size, Type: typ}
}

func TestEmptyQueryMatchesAll(t *testing.T) {
	q, err := Parse("   ")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if q.Active() {
		t.Fatal("empty query should be inactive")
	}
	if !q.Match(node("anything", false, 1, "")) {
		t.Fatal("empty query must match every node")
	}
}

func TestSubstringCaseInsensitive(t *testing.T) {
	q, err := Parse("LOG")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !q.Match(node("app.log", false, 1, "")) {
		t.Fatal("case-insensitive substring should match app.log")
	}
	if q.Match(node("app.txt", false, 1, "")) {
		t.Fatal("substring should not match app.txt")
	}
}

func TestSubstringCaseSensitive(t *testing.T) {
	q, err := Parse("case: LOG")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if q.Match(node("app.log", false, 1, "")) {
		t.Fatal("case-sensitive query must not match app.log")
	}
	if !q.Match(node("app.LOG", false, 1, "")) {
		t.Fatal("case-sensitive query should match app.LOG")
	}
}

func TestGlobAutoDetect(t *testing.T) {
	q, err := Parse("*.go")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(q.Terms) != 1 || q.Terms[0].mode != ModeGlob {
		t.Fatalf("expected single glob term, got %+v", q.Terms)
	}
	if !q.Match(node("main.go", false, 1, "")) {
		t.Fatal("glob should match main.go")
	}
	if q.Match(node("main.ts", false, 1, "")) {
		t.Fatal("glob should not match main.ts")
	}
}

func TestRegexAndInvalid(t *testing.T) {
	q, err := Parse("re:^a.*z$")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !q.Match(node("a--z", false, 1, "")) {
		t.Fatal("regex should match a--z")
	}
	if q.Match(node("b--z", false, 1, "")) {
		t.Fatal("regex should not match b--z")
	}
	if _, err := Parse("re:(["); err == nil {
		t.Fatal("invalid regex must error")
	}
	if _, err := Parse("["); err == nil {
		t.Fatal("invalid glob must error")
	}
	if _, err := Parse("size:>oops"); err == nil {
		t.Fatal("invalid size must error")
	}
}

func TestTypeClauses(t *testing.T) {
	cat, err := Parse("type:image")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !cat.Match(node("pic.png", false, 1, "png")) {
		t.Fatal("type:image should match the image kind")
	}
	if cat.Match(node("doc.pdf", false, 1, "pdf")) {
		t.Fatal("type:image should not match an office document")
	}

	// A specific Type id also matches by prefix.
	id, err := Parse("type:vdi")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !id.Match(node("disk.vdi", false, 1, "vdi")) {
		t.Fatal("type:vdi should match the vdi type id")
	}
	if id.Match(node("disk.qcow2", false, 1, "qcow2")) {
		t.Fatal("type:vdi should not match qcow2")
	}

	ext, err := Parse("type:.go")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !ext.Match(node("MAIN.GO", false, 1, "")) {
		t.Fatal("type:.go should match extension case-insensitively")
	}
	if ext.Match(node("main.ts", false, 1, "")) {
		t.Fatal("type:.go should not match main.ts")
	}
}

func TestSizeClauses(t *testing.T) {
	gt, err := Parse("size:>10M")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if gt.Match(node("a", false, 10<<20, "")) {
		t.Fatal("size:>10M must exclude exactly 10M")
	}
	if !gt.Match(node("a", false, (10<<20)+1, "")) {
		t.Fatal("size:>10M must include 10M+1")
	}

	le, err := Parse("size:<=500K")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !le.Match(node("a", false, 500<<10, "")) {
		t.Fatal("size:<=500K must include 500K")
	}
	if le.Match(node("a", false, (500<<10)+1, "")) {
		t.Fatal("size:<=500K must exclude 500K+1")
	}

	rng, err := Parse("size:1K..2K")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !rng.Match(node("a", false, 1024, "")) || !rng.Match(node("a", false, 2048, "")) {
		t.Fatal("range must include its bounds")
	}
	if rng.Match(node("a", false, 4096, "")) {
		t.Fatal("range must exclude out-of-range")
	}
}

func TestCombinedAndNegation(t *testing.T) {
	q, err := Parse("type:image size:>1M logo")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !q.Match(node("logo.png", false, 2<<20, "png")) {
		t.Fatal("combined query should match all clauses")
	}
	if q.Match(node("logo.png", false, 1<<20, "png")) {
		t.Fatal("combined query must fail when the size clause fails")
	}
	if q.Match(node("logo.png", false, 2<<20, "pdf")) {
		t.Fatal("combined query must fail when the type clause fails")
	}

	neg, err := Parse("!logo")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if neg.Match(node("logo.png", false, 1, "")) {
		t.Fatal("negated query must reject a match")
	}
	if !neg.Match(node("other.png", false, 1, "")) {
		t.Fatal("negated query must accept a non-match")
	}
}

func TestVisibleSetAncestorsAndSubtree(t *testing.T) {
	root := node("root", true, 0, "")
	alpha := node("alpha", true, 0, "")
	one := node("one.txt", false, 1, "")
	beta := node("beta", true, 0, "")
	gamma := node("gamma", true, 0, "")
	note := node("note.md", false, 1, "")
	zeta := node("zeta.bin", false, 1, "")
	root.AddChild(alpha)
	alpha.AddChild(one)
	root.AddChild(beta)
	beta.AddChild(gamma)
	gamma.AddChild(note)
	root.AddChild(zeta)

	// A matched directory includes its whole subtree.
	q, _ := Parse("gamma")
	vis := q.VisibleSet(root)
	for _, n := range []*tree.Node{root, beta, gamma, note} {
		if !vis[n] {
			t.Fatalf("%q should be visible for query gamma", n.Name)
		}
	}
	for _, n := range []*tree.Node{alpha, one, zeta} {
		if vis[n] {
			t.Fatalf("%q should be hidden for query gamma", n.Name)
		}
	}

	// A matched leaf keeps only its ancestors.
	q2, _ := Parse("one")
	vis2 := q2.VisibleSet(root)
	for _, n := range []*tree.Node{root, alpha, one} {
		if !vis2[n] {
			t.Fatalf("%q should be visible for query one", n.Name)
		}
	}
	if vis2[beta] || vis2[gamma] || vis2[note] || vis2[zeta] {
		t.Fatal("query one should hide unrelated nodes")
	}

	// An inactive query marks everything.
	q3, _ := Parse("")
	vis3 := q3.VisibleSet(root)
	for _, n := range []*tree.Node{root, alpha, one, beta, gamma, note, zeta} {
		if !vis3[n] {
			t.Fatalf("%q should be visible for an empty query", n.Name)
		}
	}
}

func TestSizeEmptyFileQueries(t *testing.T) {
	for _, q := range []string{"size:0", "size:<=0", "size:<1"} {
		f, err := Parse(q)
		if err != nil {
			t.Fatalf("Parse(%q): %v", q, err)
		}
		if !f.Match(node("empty", false, 0, "")) {
			t.Errorf("%q should match a 0-byte entry", q)
		}
		if f.Match(node("nonempty", false, 1, "")) {
			t.Errorf("%q must not match a 1-byte entry", q)
		}
	}
}

func TestVisibleSetNegatedDeepMasksMatch(t *testing.T) {
	root := node("root", true, 0, "")
	gamma := node("gamma", true, 0, "")
	note := node("note.md", false, 1, "")
	zeta := node("zeta.bin", false, 1, "")
	root.AddChild(gamma)
	gamma.AddChild(note)
	root.AddChild(zeta)

	q, _ := Parse("!*.md")
	vis := q.VisibleSet(root)
	if vis[note] {
		t.Fatal("negated deep query must hide the excluded file (whole-tree reveal)")
	}
	for _, n := range []*tree.Node{root, gamma, zeta} {
		if !vis[n] {
			t.Fatalf("%q should remain visible for !*.md", n.Name)
		}
	}
}

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		err  bool
	}{
		{"0", 0, false},
		{"512", 512, false},
		{"1K", 1 << 10, false},
		{"1.5M", 3 << 19, false},
		{"2G", 2 << 30, false},
		{"10B", 10, false},
		{"", 0, true},
		{"abc", 0, true},
		{"-1M", 0, true},
	}
	for _, c := range cases {
		got, err := ParseSize(c.in)
		if c.err {
			if err == nil {
				t.Errorf("ParseSize(%q) expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSize(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestModesCoverDeclared(t *testing.T) {
	ms := Modes()
	if len(ms) != 3 {
		t.Fatalf("expected 3 modes, got %d", len(ms))
	}
	for i, m := range ms {
		if int(m) != i {
			t.Fatalf("modes out of order at %d: %v", i, m)
		}
		if m.String() == "unknown" {
			t.Fatalf("mode %v has no name", m)
		}
	}
}
