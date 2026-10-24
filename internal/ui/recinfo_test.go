package ui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math/bits"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/1ttyb1tty/ncdu-next/internal/filter"
	"github.com/1ttyb1tty/ncdu-next/internal/scan"
	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

// keyMsg builds a key message the way the existing UI tests do.
func keyMsg(s string) tea.KeyPressMsg { return tea.KeyPressMsg{Text: s} }

// guardNames is the registry of guard tests this file is expected to define. It
// is asserted by TestGuardsPresent so deleting a guard is itself a failure.
var guardNames = []string{
	"TestRenderedFieldsEqualReflected",
	"TestRecordFieldTags",
	"TestDocCoverage",
	"TestSizesReconstruct",
	"TestBoolFieldsListed",
	"TestEnumSliceMatchesConstBlock",
	"TestEnumMeaningComplete",
	"TestBitValuesComputed",
	"TestTablesViewLiveCount",
	"TestPlatformLabel",
	"TestWindowsNotApplicable",
	"TestDebugTabRendersRecords",
}

// parseRecordFieldSets parses recordsView output into record-name -> set of
// field names, independently of the code that produced it.
func parseRecordFieldSets(t *testing.T, text string) map[string]map[string]bool {
	t.Helper()
	out := map[string]map[string]bool{}
	cur := ""
	for _, ln := range strings.Split(text, "\n") {
		if !strings.HasPrefix(ln, " ") && strings.Contains(ln, "  (") && strings.Contains(ln, "B + variable") {
			cur = ln[:strings.Index(ln, "  (")]
			out[cur] = map[string]bool{}
			continue
		}
		if cur == "" {
			continue
		}
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" || !strings.HasPrefix(ln, "  ") {
			cur = ""
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) == 0 || fields[0] == "field" {
			continue
		}
		out[cur][fields[0]] = true
	}
	return out
}

func TestRenderedFieldsEqualReflected(t *testing.T) {
	sets := parseRecordFieldSets(t, recordsView())
	for _, spec := range recordSpecs {
		want := map[string]bool{}
		for i := 0; i < spec.typ.NumField(); i++ {
			want[spec.typ.Field(i).Name] = true
		}
		got := sets[spec.name]
		if got == nil {
			t.Fatalf("no rendered block for %q", spec.name)
		}
		if len(got) != len(want) {
			t.Fatalf("%s: rendered %d fields, reflected %d (%v vs %v)", spec.name, len(got), len(want), got, want)
		}
		for k := range want {
			if !got[k] {
				t.Errorf("%s: reflected field %q missing from render", spec.name, k)
			}
		}
		for k := range got {
			if !want[k] {
				t.Errorf("%s: rendered field %q not in struct", spec.name, k)
			}
		}
	}
}

func TestRecordFieldTags(t *testing.T) {
	for _, spec := range recordSpecs {
		for i := 0; i < spec.typ.NumField(); i++ {
			f := spec.typ.Field(i)
			if f.Tag.Get("doc") == "" {
				t.Errorf("%s.%s has no `doc` tag", spec.typ, f.Name)
			}
		}
	}
}

// packageStructs returns the set of "<pkg>.<Type>" for every struct type
// declared in the package at dir (test files excluded).
func packageStructs(t *testing.T, dir, pkg string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", dir, err)
	}
	out := map[string]bool{}
	for _, p := range pkgs {
		for _, f := range p.Files {
			ast.Inspect(f, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSpec)
				if !ok {
					return true
				}
				if _, ok := ts.Type.(*ast.StructType); ok {
					out[pkg+"."+ts.Name.Name] = true
				}
				return true
			})
		}
	}
	return out
}

func shortPkg(t reflect.Type) string {
	p := t.PkgPath()
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func TestDocCoverage(t *testing.T) {
	dirs := []struct{ dir, pkg string }{
		{"../tree", "tree"},
		{"../scan", "scan"},
		{"../config", "config"},
	}
	for _, d := range dirs {
		for name := range packageStructs(t, d.dir, d.pkg) {
			if documentedTypes[name] || typesNotRecords[name] != "" {
				continue
			}
			t.Errorf("struct %s is neither documented nor allowlisted: add it to recordSpecs/documentedTypes, or to typesNotRecords with a reason", name)
		}
	}
	// Every recordSpec type must be declared in documentedTypes.
	rendered := map[string]bool{}
	for _, spec := range recordSpecs {
		key := shortPkg(spec.typ) + "." + spec.typ.Name()
		if !documentedTypes[key] {
			t.Errorf("recordSpec %s -> %s not in documentedTypes", spec.name, key)
		}
		rendered[key] = true
	}
	// Reverse direction: every documentedTypes key must actually be rendered,
	// and must not also be allowlisted as not-a-record.
	for key := range documentedTypes {
		if !rendered[key] {
			t.Errorf("documentedTypes lists %s but no recordSpec renders it", key)
		}
		if typesNotRecords[key] != "" {
			t.Errorf("%s is both documented and listed as not-a-record", key)
		}
	}
}

func TestSizesReconstruct(t *testing.T) {
	text := recordsView()
	for _, spec := range recordSpecs {
		tp := spec.typ
		if !strings.Contains(text, fmt.Sprintf("fixed %d B", tp.Size())) {
			t.Errorf("recordsView missing fixed size %d for %s", tp.Size(), tp.String())
		}
		prev := -1
		for i := 0; i < tp.NumField(); i++ {
			f := tp.Field(i)
			if int(f.Offset) < prev {
				t.Errorf("%s.%s offset %d < previous %d", tp, f.Name, f.Offset, prev)
			}
			prev = int(f.Offset)
			if int(f.Offset)+int(f.Type.Size()) > int(tp.Size()) {
				t.Errorf("%s.%s overruns struct size %d", tp, f.Name, tp.Size())
			}
		}
	}
}

func TestBoolFieldsListed(t *testing.T) {
	view := flagsView()
	for _, rec := range flagRecordTypes {
		for i := 0; i < rec.typ.NumField(); i++ {
			f := rec.typ.Field(i)
			if f.Type.Kind() != reflect.Bool {
				continue
			}
			if !strings.Contains(view, f.Name) {
				t.Errorf("bool field %s.%s not listed in flagsView", rec.name, f.Name)
			}
		}
	}
}

// constStringValues returns the set of string literal values in a const block of
// the named type, parsed from source.
func constStringValues(t *testing.T, file, typeName string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	out := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || vs.Type == nil {
			return true
		}
		id, ok := vs.Type.(*ast.Ident)
		if !ok || id.Name != typeName {
			return true
		}
		for _, v := range vs.Values {
			if bl, ok := v.(*ast.BasicLit); ok && bl.Kind == token.STRING {
				if s, err := strconv.Unquote(bl.Value); err == nil {
					out[s] = true
				}
			}
		}
		return true
	})
	return out
}

func assertSliceMatchesConsts(t *testing.T, label string, slice, consts map[string]bool) {
	t.Helper()
	if len(slice) == 0 || len(consts) == 0 {
		t.Fatalf("%s: empty slice (%d) or const block (%d)", label, len(slice), len(consts))
	}
	for v := range consts {
		if !slice[v] {
			t.Errorf("%s: constant value %q missing from slice", label, v)
		}
	}
	for v := range slice {
		if !consts[v] {
			t.Errorf("%s: slice value %q not in const block", label, v)
		}
	}
}

func TestEnumSliceMatchesConstBlock(t *testing.T) {
	statuses := map[string]bool{}
	for _, s := range scanStatusValues {
		statuses[string(s)] = true
	}
	assertSliceMatchesConsts(t, "ScanStatus", statuses, constStringValues(t, "tab.go", "ScanStatus"))

	kinds := map[string]bool{}
	for _, k := range tabKindValues {
		kinds[string(k)] = true
	}
	assertSliceMatchesConsts(t, "tabKind", kinds, constStringValues(t, "tab.go", "tabKind"))
}

func TestEnumMeaningComplete(t *testing.T) {
	view := enumsView()
	// Derive the value set from the canonical slices enumsView renders from, so
	// a newly added enum value is checked rather than silently skipped.
	var vals []string
	for _, s := range scanStatusValues {
		vals = append(vals, string(s))
	}
	for _, k := range tabKindValues {
		vals = append(vals, string(k))
	}
	for _, m := range filter.Modes() {
		vals = append(vals, m.String())
	}
	for _, k := range tree.KindValues() {
		vals = append(vals, k.String())
	}
	if len(vals) == 0 {
		t.Fatal("no enum values to check")
	}
	for _, v := range vals {
		prefix := "    " + v + " "
		found := false
		for _, ln := range strings.Split(view, "\n") {
			if strings.HasPrefix(ln, prefix) {
				if strings.TrimSpace(strings.TrimPrefix(ln, prefix)) == "" {
					t.Errorf("enum value %q has no meaning", v)
				}
				found = true
				break
			}
		}
		if !found {
			t.Errorf("enum value %q missing from enumsView", v)
		}
	}
}

func TestBitValuesComputed(t *testing.T) {
	view := flagsView()
	sym := uint32(os.ModeSymlink)
	dir := uint32(os.ModeDir)
	if !strings.Contains(view, fmt.Sprintf("1<<%d", bits.TrailingZeros32(sym))) {
		t.Errorf("flagsView missing computed symlink bit shift 1<<%d", bits.TrailingZeros32(sym))
	}
	if !strings.Contains(view, fmt.Sprintf("%#08x", sym)) {
		t.Errorf("flagsView missing computed symlink value %#08x", sym)
	}
	if !strings.Contains(view, fmt.Sprintf("1<<%d", bits.TrailingZeros32(dir))) {
		t.Errorf("flagsView missing computed dir bit shift 1<<%d", bits.TrailingZeros32(dir))
	}
	// No hard-coded shift literals for these bits in the source.
	src, err := os.ReadFile("recinfo.go")
	if err != nil {
		t.Fatalf("read recinfo.go: %v", err)
	}
	for _, lit := range []string{"1<<27", "1<<31"} {
		if strings.Contains(string(src), lit) {
			t.Errorf("recinfo.go contains hard-coded literal %q; compute it instead", lit)
		}
	}
}

func TestTablesViewLiveCount(t *testing.T) {
	n, sample := scan.TypeTableStats(5)
	if n == 0 {
		t.Fatal("fileTypes table is empty")
	}
	if len(sample) == 0 {
		t.Fatal("expected non-empty sample")
	}
	view := tablesView()
	if !strings.Contains(view, fmt.Sprintf("len=%-4d", n)) {
		t.Errorf("tablesView missing live count len=%d", n)
	}
}

func TestPlatformLabel(t *testing.T) {
	view := recordsView()
	if !strings.Contains(view, runtime.GOOS) || !strings.Contains(view, runtime.GOARCH) {
		t.Errorf("recordsView missing platform label %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func TestWindowsNotApplicable(t *testing.T) {
	win := strings.Join(bitDocs("windows"), "\n")
	if !strings.Contains(win, "n/a on windows") {
		t.Error("windows bit docs must mark dev/ino as n/a on windows")
	}
	linux := strings.Join(bitDocs("linux"), "\n")
	if strings.Contains(linux, "n/a on windows") {
		t.Error("non-windows bit docs must not claim n/a on windows")
	}
}

func TestGuardsPresent(t *testing.T) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, self, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", filepath.Base(self), err)
	}
	present := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if fd, ok := n.(*ast.FuncDecl); ok {
			present[fd.Name.Name] = true
		}
		return true
	})
	for _, name := range guardNames {
		if !present[name] {
			t.Errorf("guard test %s is missing", name)
		}
	}
	if len(recordSpecs) == 0 {
		t.Error("recordSpecs is empty")
	}
	if len(scanStatusValues) == 0 || len(tabKindValues) == 0 {
		t.Error("enum value slices are empty")
	}
}

func TestDebugTabRendersRecords(t *testing.T) {
	root := &tree.Node{Name: "root", IsDir: true, Path: "/root"}
	m := New("/root", root, true, scan.Options{})
	m2, _ := m.Update(keyMsg("D"))
	mm := m2.(Model)
	// The page is longer than one screen and is reached through a viewport, so
	// assert on the assembled page lines rather than the rendered first screen.
	text := strings.Join(debugPage(debugSections(mm.tabs, mm.inspect)), "\n")
	for _, want := range []string{
		"datatypes", "file record", "dir record", "options record",
		"config record", "flags", "tables", "enums",
		"tabs", "inspected tab", "runtime", "totals",
		"tab zone", "program zone",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("debug page missing %q", want)
		}
	}
}
