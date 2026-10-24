package ui

import (
	"context"
	"fmt"
	"math/bits"
	"os"
	"reflect"
	"runtime"
	"strings"

	"github.com/1ttyb1tty/ncdu-next/internal/config"
	"github.com/1ttyb1tty/ncdu-next/internal/filter"
	"github.com/1ttyb1tty/ncdu-next/internal/scan"
	"github.com/1ttyb1tty/ncdu-next/internal/tree"
)

// This file backs the Debug tab's data-model reference. Structure (names, types,
// offsets, sizes) is derived from reflect at render time; prose lives as `doc`
// struct tags on the fields themselves. Tests in recinfo_test.go enforce that
// every documented field carries a doc tag and that the rendered rows match the
// reflected field set exactly, so the reference cannot silently drift.

type recMode int

const (
	modeNone recMode = iota
	modeFile
	modeDir
)

func (m recMode) String() string {
	switch m {
	case modeFile:
		return "file"
	case modeDir:
		return "dir"
	default:
		return ""
	}
}

type recordSpec struct {
	name string
	typ  reflect.Type
	mode recMode
}

// recordSpecs is the driver list of record types shown in `records`. Adding a
// new documented struct requires a row here (see TestDocCoverage).
var recordSpecs = []recordSpec{
	{"file record", reflect.TypeOf(tree.Node{}), modeFile},
	{"dir record", reflect.TypeOf(tree.Node{}), modeDir},
	{"options record", reflect.TypeOf(scan.Options{}), modeNone},
	{"config record", reflect.TypeOf(config.Config{}), modeNone},
}

// documentedTypes maps "<pkg>.<Type>" for every record covered above. It is used
// by TestDocCoverage to prove no struct in the documented packages is omitted
// without an explicit allowlist decision.
var documentedTypes = map[string]bool{
	"tree.Node":     true,
	"scan.Options":  true,
	"config.Config": true,
}

// typesNotRecords is the explicit allowlist of structs in the documented
// packages that are deliberately not part of the record reference, with a reason.
var typesNotRecords = map[string]string{
	"scan.Walker": "internal scan state (opts/sem); runtime, not a record",
}

// recordsView renders the datatype legend and the four record types.
func recordsView() string {
	var b strings.Builder
	b.WriteString(datatypesView())
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("records [%s/%s]:\n", runtime.GOOS, runtime.GOARCH))
	b.WriteString(fmt.Sprintf("  %-13s %-15s %-16s %-7s %s\n", "field", "type", "size", "offset", "explanation"))
	for _, spec := range recordSpecs {
		renderRecord(&b, spec)
	}
	return b.String()
}

func datatypesView() string {
	strHdr := reflect.TypeOf("").Size()
	sliceHdr := reflect.TypeOf([]*tree.Node{}).Size()
	ptrSz := reflect.TypeOf((*tree.Node)(nil)).Size()
	ifaceSz := reflect.TypeOf((*error)(nil)).Elem().Size()
	fnSz := reflect.TypeOf((func(files int64, path string))(nil)).Size()
	ctxSz := reflect.TypeOf((*context.Context)(nil)).Elem().Size()

	var b strings.Builder
	b.WriteString(fmt.Sprintf("datatypes [%s/%s]:\n", runtime.GOOS, runtime.GOARCH))
	b.WriteString(fmt.Sprintf("  %-16s %-16s %s\n", "type", "size", "meaning"))
	row := func(name, size, mean string) {
		b.WriteString(fmt.Sprintf("  %-16s %-16s %s\n", name, size, mean))
	}
	row("bool", fmt.Sprintf("%d B", reflect.TypeOf(false).Size()), "flag (0x00/0x01); the only bit stored as its own field")
	row("int", fmt.Sprintf("%d B", reflect.TypeOf(int(0)).Size()), "platform-sized count")
	row("int64", fmt.Sprintf("%d B", reflect.TypeOf(int64(0)).Size()), "signed 64-bit byte count")
	row("uint64", fmt.Sprintf("%d B", reflect.TypeOf(uint64(0)).Size()), "unsigned 64-bit bitfield/id")
	row("uint8", fmt.Sprintf("%d B", reflect.TypeOf(uint8(0)).Size()), "byte-sized enum (Kind)")
	row("uint32", fmt.Sprintf("%d B", reflect.TypeOf(uint32(0)).Size()), "32-bit bitmask (Windows FILE_ATTRIBUTE_*)")
	row("string", fmt.Sprintf("%d B header + len", strHdr), "length + data pointer")
	row("*Node", fmt.Sprintf("%d B", ptrSz), "pointer")
	row("[]*Node", fmt.Sprintf("%d B header + len*%d", sliceHdr, ptrSz), "slice header + backing array")
	row("error", fmt.Sprintf("%d B", ifaceSz), "interface (type + data word)")
	row("func(...)", fmt.Sprintf("%d B", fnSz), "function value (closure pointer)")
	row("context.Context", fmt.Sprintf("%d B", ctxSz), "interface")
	return b.String()
}

func renderRecord(b *strings.Builder, spec recordSpec) {
	t := spec.typ
	label := spec.name
	if spec.mode != modeNone {
		label = fmt.Sprintf("%s  (IsDir=%v)", spec.name, spec.mode == modeDir)
	}
	b.WriteString(fmt.Sprintf("%s  (%s)  fixed %d B + variable\n", label, t.String(), t.Size()))
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		doc := docOf(f)
		if spec.mode != modeNone {
			role := f.Tag.Get(spec.mode.String())
			if role == "" {
				role = "n/a"
			}
			doc = fmt.Sprintf("%s  [%s: %s]", doc, spec.mode, role)
		}
		b.WriteString(fmt.Sprintf("  %-13s %-15s %-16s %-7d %s\n", f.Name, f.Type.String(), fieldSizeText(f.Type), f.Offset, doc))
	}
}

// fieldSizeText returns a human size for a field type: a fixed size for scalars,
// and "header + len*elem" for variable-length types.
func fieldSizeText(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return fmt.Sprintf("%d B + len", t.Size())
	case reflect.Slice:
		return fmt.Sprintf("%d B + len*%d", t.Size(), t.Elem().Size())
	case reflect.Bool:
		return "1 B"
	case reflect.Interface:
		return fmt.Sprintf("%d B (iface)", t.Size())
	default:
		return fmt.Sprintf("%d B", t.Size())
	}
}

// docOf returns the field's `doc` tag, or a loud placeholder when absent so a
// missing explanation is visible in the running UI as well as in tests.
func docOf(f reflect.StructField) string {
	if d := f.Tag.Get("doc"); d != "" {
		return d
	}
	return "(undocumented)"
}

// flagRecordTypes lists the records whose boolean fields are explained by the
// `flags` section (deduplicated: tree.Node appears once even though it is shown
// in both file and dir mode).
var flagRecordTypes = []struct {
	name string
	typ  reflect.Type
}{
	{"tree.Node", reflect.TypeOf(tree.Node{})},
	{"scan.Options", reflect.TypeOf(scan.Options{})},
	{"config.Config", reflect.TypeOf(config.Config{})},
}

// flagsView lists boolean fields (discovered via reflect) and the runtime bit
// constants the walker tests.
func flagsView() string {
	var b strings.Builder
	b.WriteString("flags (boolean fields, discovered via reflect):\n")
	b.WriteString(fmt.Sprintf("  %-13s %-14s %-8s %s\n", "field", "record", "default", "effect"))
	for _, rec := range flagRecordTypes {
		t := rec.typ
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Type.Kind() != reflect.Bool {
				continue
			}
			b.WriteString(fmt.Sprintf("  %-13s %-14s %-8s %s\n", f.Name, rec.name, "false", docOf(f)))
		}
	}
	b.WriteString("bits (os.FileMode / syscall; values computed at runtime, not written down):\n")
	for _, line := range bitDocs(runtime.GOOS) {
		b.WriteString("  " + line + "\n")
	}
	return b.String()
}

// bitDocs returns the bit-level notes. It takes the OS explicitly so the
// Windows "n/a" path is testable without a Windows host.
func bitDocs(goos string) []string {
	sym := uint32(os.ModeSymlink)
	dir := uint32(os.ModeDir)
	devIno := "syscall.Stat_t.Dev / .Ino (uint64): device id + inode -> hardlink key \"dev:ino\"; compared for --one-file-system"
	if goos == "windows" {
		devIno += "  [n/a on windows: fileKey=\"\", deviceOf=(0,false)]"
	} else {
		devIno += "  [available on " + goos + "]"
	}
	return []string{
		fmt.Sprintf("os.ModeSymlink = 1<<%d = %#08x  symlink bit; checked before a record is created", bits.TrailingZeros32(sym), sym),
		fmt.Sprintf("os.ModeDir     = 1<<%d = %#08x  directory bit; selects file vs dir mode", bits.TrailingZeros32(dir), dir),
		"os.FileMode type word: high bits classify the entry; low 9 bits are permissions (not stored on the record)",
		devIno,
	}
}

// tablesView reports the code-defined maps with runtime counts.
func tablesView() string {
	n, sample := scan.TypeTableStats(5)
	var b strings.Builder
	b.WriteString("tables (code-defined maps; counts computed at runtime):\n")
	b.WriteString(fmt.Sprintf("  %-22s len=%-4d extension -> Type id; e.g. %s\n", "scan.fileTypes", n, strings.Join(sample, " ")))
	b.WriteString(fmt.Sprintf("  %-22s len=%-4d Kind -> row color\n", "ui.kindStyles", len(kindStyles)))
	b.WriteString(fmt.Sprintf("  %-22s len=%-4d fixed Kind enumeration\n", "ui.kindStyleOrder", len(kindStyleOrder)))
	b.WriteString("  scan.visited/links     transient per-scan dedup maps (not enumerated)\n")
	return b.String()
}

// enumsView renders the enum-style vocabularies from their canonical slices.
func enumsView() string {
	var b strings.Builder
	b.WriteString("enums:\n")

	b.WriteString("  ScanStatus (per-tab scan state):\n")
	statusMean := map[ScanStatus]string{
		StatusIdle:     "no scan yet, or the last scan was cancelled",
		StatusScanning: "scan in flight",
		StatusDone:     "last scan succeeded",
		StatusError:    "last scan failed",
	}
	for _, s := range scanStatusValues {
		b.WriteString(fmt.Sprintf("    %-9s %s\n", string(s), statusMean[s]))
	}

	b.WriteString("  tabKind (tab role):\n")
	kindMean := map[tabKind]string{tabKindNav: "navigable scan tab", tabKindDebug: "read-only debug tab"}
	for _, k := range tabKindValues {
		b.WriteString(fmt.Sprintf("    %-9s %s\n", string(k), kindMean[k]))
	}

	b.WriteString("  filter.Mode (query matcher):\n")
	modeMean := map[filter.Mode]string{
		filter.ModeSubstring: "case-insensitive substring on the name",
		filter.ModeGlob:      "filepath.Match glob on the name",
		filter.ModeRegex:     "regular expression on the name",
	}
	for _, m := range filter.Modes() {
		b.WriteString(fmt.Sprintf("    %-9s %s\n", m.String(), modeMean[m]))
	}

	b.WriteString("  Kind (broad category derived from Type; \"\" Type -> other):\n")
	catMean := map[tree.Kind]string{
		tree.KindUnknown:        "classification not attempted (zero value)",
		tree.KindDiskImage:      "virtual disk / VM image (vdi, qcow2, ...)",
		tree.KindContainerImage: "container/archive image (tar, oci, iso)",
		tree.KindMemoryState:    "memory dump / VM state save (vmem, dmp, ...)",
		tree.KindOfficeDoc:      "office / document format (doc, pdf, epub, ...)",
		tree.KindImage:          "raster image (png, jpg, tiff, ...)",
		tree.KindAudio:          "audio (mp3, flac, opus, ...)",
		tree.KindVideo:          "video (mp4, mkv, webm, ...)",
		tree.KindText:           "plain text / data (txt, log, csv, ...)",
		tree.KindSource:         "programming source (go, py, rs, ...)",
		tree.KindMarkup:         "markup / structured text (html, json, md, ...)",
		tree.KindOther:          "file whose extension matched nothing",
	}
	for _, k := range tree.KindValues() {
		b.WriteString(fmt.Sprintf("    %-15s %s\n", k.String(), catMean[k]))
	}
	return b.String()
}
