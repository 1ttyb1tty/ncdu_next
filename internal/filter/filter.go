// Package filter provides a small query engine for filtering an already-scanned
// tree. It backs the TUI's live filter, deep (tree-pruning) filter, and full-tree
// search. A query is a single input line: free-text name terms (substring or
// glob), re: regex terms, type: clauses, and size: clauses, combined with AND;
// a leading '!' negates the whole query.
package filter

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

// Mode identifies how a name term is matched.
type Mode int

const (
	ModeSubstring Mode = iota
	ModeGlob
	ModeRegex
)

var modeNames = [...]string{"substring", "glob", "regex"}

// String returns the mode's canonical name.
func (m Mode) String() string {
	if int(m) >= 0 && int(m) < len(modeNames) {
		return modeNames[m]
	}
	return "unknown"
}

// Modes returns the canonical enum values in declaration order. It is the single
// source the Debug tab renders and the test asserts against the const block.
func Modes() []Mode { return []Mode{ModeSubstring, ModeGlob, ModeRegex} }

// nameTerm is one free-text/regex clause, matched against a node's name.
type nameTerm struct {
	mode Mode
	val  string
	re   *regexp.Regexp
}

// Query is a parsed filter/search predicate. The zero/empty Query matches
// everything (so a cleared filter is a no-op).
type Query struct {
	Raw           string
	Terms         []nameTerm
	TypeCat       string
	TypeExt       string
	Min, Max      int64
	HasSize       bool
	HasMax        bool
	CaseSensitive bool
	Negate        bool
}

// Active reports whether the query constrains anything (an empty query matches
// all nodes).
func (q *Query) Active() bool {
	if q == nil {
		return false
	}
	return len(q.Terms) > 0 || q.TypeCat != "" || q.TypeExt != "" || q.HasSize
}

// Parse compiles a query from a single input line. An empty input yields a Query
// that matches everything. Invalid regex, glob, or size clauses return an error.
func Parse(input string) (*Query, error) {
	q := &Query{Raw: strings.TrimSpace(input)}
	s := q.Raw
	if s == "" {
		return q, nil
	}
	if strings.HasPrefix(s, "!") {
		q.Negate = true
		s = strings.TrimSpace(s[1:])
		if s == "" {
			return q, nil
		}
	}
	for _, tok := range strings.Fields(s) {
		switch {
		case strings.HasPrefix(tok, "re:"):
			pat := tok[len("re:"):]
			if pat == "" {
				return nil, fmt.Errorf("filter: empty regex")
			}
			q.Terms = append(q.Terms, nameTerm{mode: ModeRegex, val: pat})
		case strings.HasPrefix(tok, "type:"):
			v := tok[len("type:"):]
			if v == "" {
				return nil, fmt.Errorf("filter: empty type clause")
			}
			if strings.HasPrefix(v, ".") {
				q.TypeExt = v
			} else {
				q.TypeCat = v
			}
		case strings.HasPrefix(tok, "size:"):
			min, max, hasMax, err := parseSizeExpr(tok[len("size:"):])
			if err != nil {
				return nil, err
			}
			q.Min, q.Max, q.HasMax = min, max, hasMax
			q.HasSize = true
		case tok == "case:":
			q.CaseSensitive = true
		default:
			if strings.ContainsAny(tok, "*?[") {
				if _, err := filepath.Match(tok, "x"); err != nil {
					return nil, fmt.Errorf("filter: bad glob %q: %w", tok, err)
				}
				q.Terms = append(q.Terms, nameTerm{mode: ModeGlob, val: tok})
			} else {
				q.Terms = append(q.Terms, nameTerm{mode: ModeSubstring, val: tok})
			}
		}
	}
	// Regex terms are compiled after all tokens are read so that a trailing
	// "case:" can still influence case-insensitivity.
	for i := range q.Terms {
		t := &q.Terms[i]
		if t.mode != ModeRegex {
			continue
		}
		pat := t.val
		if !q.CaseSensitive {
			pat = "(?i)" + pat
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			return nil, fmt.Errorf("filter: bad regex %q: %w", t.val, err)
		}
		t.re = re
	}
	return q, nil
}

// Match reports whether a node satisfies the query. A nil or empty query matches
// every node; a negated query inverts its positive result.
func (q *Query) Match(n *tree.Node) bool {
	if q == nil || !q.Active() {
		return true
	}
	if n == nil {
		return false
	}
	ok := q.matchPositive(n)
	if q.Negate {
		return !ok
	}
	return ok
}

func (q *Query) matchPositive(n *tree.Node) bool {
	for _, t := range q.Terms {
		if !t.match(n.Name, q.CaseSensitive) {
			return false
		}
	}
	if q.TypeCat != "" && !strings.HasPrefix(n.Type, q.TypeCat) {
		return false
	}
	if q.TypeExt != "" && !strings.EqualFold(filepath.Ext(n.Name), q.TypeExt) {
		return false
	}
	if q.HasSize {
		if n.Size < q.Min {
			return false
		}
		if q.HasMax && n.Size > q.Max {
			return false
		}
	}
	return true
}

func (t nameTerm) match(name string, caseSensitive bool) bool {
	switch t.mode {
	case ModeRegex:
		return t.re != nil && t.re.MatchString(name)
	case ModeGlob:
		base, pat := name, t.val
		if !caseSensitive {
			base, pat = strings.ToLower(base), strings.ToLower(pat)
		}
		ok, _ := filepath.Match(pat, base)
		return ok
	default:
		base, val := name, t.val
		if !caseSensitive {
			base, val = strings.ToLower(base), strings.ToLower(val)
		}
		return strings.Contains(base, val)
	}
}

// VisibleSet returns the set of nodes to display for a recursive (deep) query:
// every match, every ancestor of a match, and the full subtree of any matching
// directory. The last rule is a deliberate v1 choice (see the plan) so that
// entering a matched directory never shows an empty list.
func (q *Query) VisibleSet(root *tree.Node) map[*tree.Node]bool {
	vis := make(map[*tree.Node]bool)
	if root == nil {
		return vis
	}
	var walk func(n *tree.Node) bool
	walk = func(n *tree.Node) bool {
		self := q.Match(n)
		// A matched directory reveals its whole subtree. This rule is only
		// meaningful for a positive query: under negation it would reveal the
		// entire tree, so negated matches contribute only their own node.
		if self && n.IsDir && !q.Negate {
			markSubtree(n, vis)
			return true
		}
		keep := self
		for _, c := range n.Children {
			if walk(c) {
				keep = true
			}
		}
		if keep {
			vis[n] = true
		}
		return keep
	}
	walk(root)
	return vis
}

func markSubtree(n *tree.Node, vis map[*tree.Node]bool) {
	vis[n] = true
	for _, c := range n.Children {
		markSubtree(c, vis)
	}
}

// ParseSize parses a byte count with an optional 1024-based unit suffix
// (B, K, M, G, T, P, E); fractional values are allowed ("1.5M").
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("filter: empty size")
	}
	mult := int64(1)
	switch s[len(s)-1] {
	case 'b', 'B':
		s = s[:len(s)-1]
	case 'k', 'K':
		mult, s = 1<<10, s[:len(s)-1]
	case 'm', 'M':
		mult, s = 1<<20, s[:len(s)-1]
	case 'g', 'G':
		mult, s = 1<<30, s[:len(s)-1]
	case 't', 'T':
		mult, s = 1<<40, s[:len(s)-1]
	case 'p', 'P':
		mult, s = 1<<50, s[:len(s)-1]
	case 'e', 'E':
		mult, s = 1<<60, s[:len(s)-1]
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("filter: bad size %q", s)
	}
	if f < 0 {
		return 0, fmt.Errorf("filter: negative size")
	}
	return int64(f * float64(mult)), nil
}

// parseSizeExpr parses an operator+size clause body: ">10M", ">=1", "<500K",
// "<=2G", "10M..1G", or a bare "10M" (exact). hasMax distinguishes a real
// upper bound (including 0, so "size:<=0" means empty files) from no upper
// bound at all.
func parseSizeExpr(e string) (min, max int64, hasMax bool, err error) {
	e = strings.TrimSpace(e)
	if e == "" {
		return 0, 0, false, fmt.Errorf("filter: empty size clause")
	}
	switch {
	case strings.HasPrefix(e, ">="):
		v, err := ParseSize(e[2:])
		return v, 0, false, err
	case strings.HasPrefix(e, "<="):
		v, err := ParseSize(e[2:])
		return 0, v, true, err
	case strings.HasPrefix(e, ">"):
		v, err := ParseSize(e[1:])
		return v + 1, 0, false, err
	case strings.HasPrefix(e, "<"):
		v, err := ParseSize(e[1:])
		if err != nil {
			return 0, 0, false, err
		}
		if v < 1 {
			v = 1
		}
		return 0, v - 1, true, nil
	case strings.Contains(e, ".."):
		parts := strings.SplitN(e, "..", 2)
		lo, err := ParseSize(parts[0])
		if err != nil {
			return 0, 0, false, err
		}
		hi, err := ParseSize(parts[1])
		if err != nil {
			return 0, 0, false, err
		}
		return lo, hi, true, nil
	default:
		v, err := ParseSize(e)
		return v, v, true, err
	}
}
