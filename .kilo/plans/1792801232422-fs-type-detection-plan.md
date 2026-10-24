# Plan: filesystem detection, a two-zone scrollable Debug tab, and a debuginfo bundle

**Goal.** Four deliverables across three phases: Phase 1 = 1; Phase 2 = 2 and 3; Phase 3 = 4.
Phases 2 and 3 both build on Phase 1 (the sectioned page) and on each other (Phase 3's
`fs.jsonl` serialises Phase 2's report). Each phase is independently reviewable and
revertible, and Phase 3 can be deferred without touching Phases 1–2.

1. **Debug tab structure** — restructure the existing `D` singleton into a labelled `tab`
   zone and a `program` zone (one *inspected* tab), and make the whole page reachable with
   a line viewport. Changes no scan behaviour.
2. **Filesystem display** — a compact `[NTFS]` / `[ext4]` token in the navigation header
   for the directory currently being viewed, plus a `filesystem` section inside the
   Debug tab's `tab` zone (root FS, device identity, per-mount table, and the reason when
   the type is unknown).
3. **Behavior fix** — `--one-file-system` currently does nothing on Windows, because
   `deviceOf` returns `(0,false)` there (`internal/scan/walker_windows.go:12`) and the
   walker only skips when a device differs (`internal/scan/walker.go:355`). Volume
   identity from the OS mount table makes the flag real there. The README (`README.md:58`)
   and man page (`docs/ncdu-next.1:23`) already advertise the flag unconditionally, so
   today's docs are wrong on Windows and become right afterwards.
4. **Debuginfo bundle** — one zip a user or CI can attach to a bug report: what the
   binary is, what it was asked to do, and what each tab holds, with the full tree only
   behind an explicit switch.

**Constraints confirmed against the code**

- `tree.Node.Type` already means *file format id* (extension-derived, `node.go:27`).
  The filesystem name must use a different field (`FSType`).
- Per-node filesystem typing is rejected: syscall work per path for a value that is
  constant across a subtree. Detection is per **mount**.
- CI runs on `windows-latest` only (`.github/workflows/ci.yml:11`); the gate is
  `scripts/check.ps1` (gofmt, `go vet`, `go build`, `go test -count=1 -race ./...`).
  Linux-only code cannot be exercised in CI, so its parsing logic must live in a
  build-tag-free file that Windows CI *can* test, and the tagged readers must sit behind
  stubbable seams.
- A build-tag-free file **cannot** import `golang.org/x/sys/unix` (build-constrained to
  Unix; it fails to compile on Windows). So the shared parser cannot decode a Linux
  `dev_t`, and `Device` must be an **opaque identity**, not a platform argument.
- `TestDocCoverage` (`internal/ui/recinfo_test.go:148`) AST-parses **every** struct in
  `internal/tree`, `internal/scan`, `internal/config` — **ignoring build tags** — and
  fails unless the struct is a `recordSpecs` row or is allowlisted with a reason.
  `_test.go` files are excluded (`recinfo_test.go:116-118`), so test-local fixtures need
  no registry entry. `internal/ui` and `internal/debuginfo` are *not* walked.
- `constStringValues` (`recinfo_test.go:221`) collects only constants whose `ValueSpec`
  names its type explicitly, so **every `FSAvail` constant must repeat the type** the way
  `ScanStatus` does in `tab.go:23-28`. Using `= "x"` shorthand for a later constant makes
  the guard fail.
- `golang.org/x/sys` is an **indirect** dependency (`go.mod:26`); importing
  `golang.org/x/sys/windows` directly requires `go mod tidy`. `archive/zip` and
  `encoding/json` are stdlib, so the bundle adds no dependency.
- `ui.New` has 30 test call sites (`model_test.go`, `tab_test.go`, `ui_filter_test.go`,
  `recinfo_test.go:408`), so the initial report must not change its signature.
- `logging.Setup` returns a `*slog.Logger` that `main.go:74` currently discards
  (`_ = logger`).
- There is **no version string** in this repo (`main.go` has no `--version`, `go.mod`
  carries none), so build metadata must come from `runtime/debug.ReadBuildInfo()`
  (module version, VCS revision/time, Go version) rather than an invented constant.
- `--export FILE` already writes ncdu 2.x JSON and exits (`main.go:95-101`). The bundle
  must reference it, not silently duplicate it.
- **The Debug tab has no scrolling and runs in the alternate screen** (`model.go:393`),
  which has no scrollback. Counting the section rows in the code gives roughly 130 lines
  at one tab (≈14 `datatypes` + ≈44 `records` + ≈19 `flags` + ≈5 `tables` + ≈26 `enums` +
  ≈10 `runtime` + ≈4 `tabs` + chrome) — an estimate from the row counts, not a
  measurement. `records` (~44) and `enums` (~30) individually exceed a 24-row terminal,
  so a section-per-screen selector would not fix reachability; only a line viewport does.
- **`m.active` cannot name the inspected tab.** `D` sets `m.active` to the Debug tab's
  index (`model.go:200`, `:208`), so any per-tab detail must track its own inspected tab.
- **While the Debug tab is focused, nav-only keys still fire**: `updateKey`
  (`model.go:161-285`) has no `tabKindDebug` branch, so `/`, `g`, `f`, `s`, `d`, `u`, `e`
  act on it (`g` even sets `searchActive` on a nil-root tab, invisible in `View()`).
- `syscall.Statfs_t` (hence `Fstypename`/`Fsid`) is not layout-compatible across darwin
  and the BSDs, so one reader cannot be shared by them.

## Decisions (resolved)

| Decision | Choice | Consequence |
|---|---|---|
| Debug structure | One `D` singleton, two labelled zones: `tab` (summary + **one inspected tab**'s detail) then `program` (runtime, cross-tab totals, reference sections) | Per-tab payload is on the first screen; no per-tab Debug tabs |
| Inspected tab | Tracked in `Model` as a `*Tab` (pointer, so `x` reindexing cannot dangle it), set when `D` is pressed **from a nav tab**; validated against the live list at render | `D` pressed while already on the Debug tab leaves it unchanged |
| Reachability | Line **viewport** over the assembled page: `up`/`down`/`j`/`k`, `pgup`/`pgdn`, `home`/`end`, `[`/`]` section jumps; pinned header + zone/position bar | Reachable at any terminal size; mirrors the existing `t.scroll`/`maxRows` pattern |
| Debug key ownership | The Debug tab claims its own keys; nav-only keys are ignored while it is focused | Documented behavior change (a fix): `/`, `g`, `s`, `d`, `u`, `e` no longer act on it |
| Phase split | Phase 1 = Debug structure; Phase 2 = FS detection/display; Phase 3 = debuginfo bundle | Phase 3 is a pure addition to an already-restructured page |
| Carriage | Sidecar `*scan.FSReport` the walker accumulates, exposed via `w.FSReport()` | No `tree.Node` field, no `Scan()` signature change, no per-node memory |
| Mount list source | OS mount table: Linux `/proc/self/mountinfo`, darwin `getfsstat`, Windows volume enumeration | Catches bind mounts and subvolumes |
| Platform coverage | `linux`, `darwin`, `windows` implemented; every other Unix-like GOOS → `fs_other.go` → `unavailable` with a reason | BSDs are **not** claimed (their `Statfs_t` layouts differ and cannot be compiled here) |
| darwin reader | Ships, **best-effort**: compiles via cross-build, no runtime test exists here | Isolated behind `osMountTable` |
| `Device` field | **Opaque identity**: Linux `major<<32 \| minor`, Windows volume serial, darwin folded `Fsid` | No `x/sys/unix` import in shared code; never compared across platforms |
| Three states | `FSAvail` string enum `available` \| `unavailable` \| `not-applicable` | Reuses the existing string-enum guards and `enumsView` |
| Header tokens | `[NTFS]` available, `[fs:?]` unavailable, omitted when not applicable or when there is no report | Full state + reason live in the Debug section |
| `FSReport` shape | `{Root FSInfo; Mounts []FSInfo}`, pointer; `nil` = "no report yet" | No `Complete` bool; no `flagRecordTypes` churn |
| OneFS identity | Unix unchanged (`Stat_t.Dev`); Windows resolves the path against the mount table | No Linux/darwin regression risk |
| OneFS when identity is unknown | **Fail open** (descend) + one warn log | Never silently hide data |
| Initial report into the UI | New `ui.NewWithReport(...)`; `New(...)` delegates with `nil` | Zero churn in the 30 existing test call sites |
| Not-applicable trigger | The scan root is a regular file (documented policy) | One testable, honest trigger |
| Bundle contents | Bounded by default: `README.txt`, `manifest.json`, `debug-tab.txt`, `tabs.jsonl`, `fs.jsonl`, `tree-summary.json`; whole tree only with `--debuginfo-tree` | No unbounded output; `--export` stays the data path |
| Bundle trigger | `--debuginfo FILE` writes and exits (like `--export`, combinable with it, no `--debug` needed); `E` on the Debug tab writes a default-named bundle in the cwd | Headless for CI, live for support |
| Bundle shape | New `internal/debuginfo` (stdlib only) owns the record types + `Write`; `ui` maps its `Tab`s into them; `ui` therefore depends on `debuginfo`, never the reverse | One writer shared by the CLI and the TUI; `debuginfo` is trivially testable |
| Bundle determinism | `Now`, build info, and the runtime text are inputs to `Write`; fixed zip `Modified`; top-N sorted size desc then **path asc** | A byte-exact test is possible; equal sizes cannot reorder between runs |

## Phase 1 — Debug tab structure (no scan changes)

**Page builder (testable, returns every line).** Build the page from an explicit ordered
section list rather than by sniffing heading text, so section offsets are correct by
construction:

```go
// debugSection is ui-local, so TestDocCoverage (which walks tree/scan/config only) ignores it.
type debugSection struct {
    Zone  string   // "tab" | "program"
    Title string   // heading, e.g. "filesystem", "records"
    Lines []string // body, excluding the heading
}

func debugSections(tabs []*Tab, inspect *Tab) []debugSection // the WHOLE page, in display order
func debugPage(secs []debugSection) []string                 // flattened lines
func sectionOffsets(secs []debugSection) []int               // line offsets of headings, for [/]
func debugViewport(lines []string, offset, height int) []string // the visible window
```

`View()` keeps the pinned chrome (tab bar, separators, `DEBUG - ncdu-next`, the zone bar
`tab <n> (<root>) | program  <offset>/<total>`, footer) and renders
`debugViewport(debugPage(secs), t.scroll, bodyHeight)` where `bodyHeight` is derived the
way `panelMetrics` derives `maxRows` (`tab.go:647`). `t.scroll` is the debug tab's line
offset, clamped to `[0, len(lines)-1]` every render.

**Zone order: `tab` first, then `program`.** The zone you pressed `D` for is then on
screen without scrolling (the current code has the same property: the `tabs` table sits at
~line 12, above the reference). The reference zone is ~110 lines and is reached by
scrolling or `]`.

**Zone contents.**

- `tab` (first): the existing all-tabs table with the **inspected** row marked, then the
  inspected tab's detail — status, root path, node count, est/mem, unit mode, expanded,
  detectFormat, filter scope + raw + error, search active + raw + result count — and, in
  Phase 2, the `filesystem` section.
- `program` (second): the existing `runtime` block, a new cross-tab totals line (summed
  node counts and summed est/mem), then `datatypes`/`records`/`flags`/`tables`/`enums`
  (unchanged renderers).
- A `(no tabs)` / `(no scan report)` line for every empty case, so the page is never blank
  and tests can assert a marker.

**Keys** — add a `tabKindDebug` branch to `updateKey` beside the search-results branch
(`model.go:144-159`), handling `up`/`k`, `down`/`j`, `pgup`, `pgdown`, `home`, `end`, `[`,
`]`, and retaining `q`, `ctrl+c`, `1-8`, `ctrl+tab`, `t`, `x`, `D`. Everything else is
ignored while the Debug tab is focused. Confirm the v2 key strings for `pgup`/`pgdown`
against `tea.KeyPressMsg.String()` when implementing rather than assuming. `x` closing the
inspected tab must not leave a dangling pointer: validate `inspect` against the live
`tabs` at render time and fall back to the first nav tab.

**`TestDebugTabRendersRecords` must be rewritten.** It asserts section headings in
`View().Content` (`recinfo_test.go:406-419`); under a viewport only the first screen is
rendered, so those headings would be absent and the test would fail. Assert on
`debugPage(...)` lines instead. The function name is in `guardNames`
(`recinfo_test.go:29-42`), so keep it; put new debug tests in `tab_test.go` so
`guardNames` needs no addition.

## Phase 2 — Filesystem detection and display

### Data model (`internal/scan/fs.go`, no build tag)

```go
// FSAvail is the three-state availability of a filesystem's type. Every constant
// names its type explicitly (the AST guard in internal/ui reads them by type).
type FSAvail string

const (
    FSAvailable     FSAvail = "available"      // OS named the filesystem
    FSUnavailable   FSAvail = "unavailable"    // read failed, path not covered, or unimplemented here
    FSNotApplicable FSAvail = "not-applicable" // nothing to read: scan root is a regular file
)

func FSAvailValues() []FSAvail // canonical order, beside the const block

type FSInfo struct {
    MountPoint string  `doc:"mount point / volume root of this filesystem"`
    FSType     string  `doc:"OS-reported filesystem name (NTFS, ext4, apfs); empty unless Avail is available"`
    Avail      FSAvail `doc:"three-state availability: available | unavailable | not-applicable"`
    Device     uint64  `doc:"opaque OS identity for this filesystem (Linux major<<32|minor, Windows volume serial, darwin Fsid); 0 when the OS reports none; never compared across platforms"`
    Reason     string  `doc:"why the type is unavailable or not applicable; empty when available"`
}

// FSReport is the per-scan filesystem summary, exposed via Walker.FSReport().
type FSReport struct {
    Root   FSInfo   // the scan root's filesystem (or its unavailability reason)
    Mounts []FSInfo // mount points strictly inside the root
}
```

Exactly **two** new structs in `scan`:

- `scan.FSInfo` → `recordSpecs` row `{"fs record", reflect.TypeOf(scan.FSInfo{}), modeNone}` + `documentedTypes["scan.FSInfo"] = true`. `modeNone` needs no `file:`/`dir:` role tags; all five fields need `doc` tags. No bool field, so `flagsView`/`TestBoolFieldsListed` are unaffected.
- `scan.FSReport` → `typesNotRecords["scan.FSReport"]` = `"per-scan filesystem summary (Root + Mounts) exposed by Walker.FSReport(); transient, like scan.Walker"`.

Mount-table rows reuse `FSInfo` (`Avail = FSAvailable`, `Reason = ""`), so no
`mountEntry` struct is introduced.

### Platform seams (build-tag-free, stubbable on a Windows-only CI)

```go
var (
    osMountTableFn  = osMountTable   // platform mount-table reader
    deviceOfEntryFn = deviceOfEntry  // platform --one-file-system identity
)

func parseMountinfo(r io.Reader) ([]FSInfo, error)      // pure /proc/self/mountinfo parser
func mountsUnder(root string, table []FSInfo) []FSInfo  // filter to strictly-inside mount points
func fsFor(path string, table []FSInfo) (FSInfo, bool)  // longest-prefix match
func FSLookup(fs *FSReport, path string) (FSInfo, bool) // exported: longest prefix in Mounts, else Root
```

- `parseMountinfo` handles the `-` separator, optional fields, and `\040` escapes; `Device = uint64(major)<<32 | uint64(minor)` (opaque, documented).
- `pathPrefix(mountPoint, path string) bool` is the single path predicate ("path is at or under mountPoint"), **separator-aware** (`/a/bc` is not under `/a/b`), case-insensitive only on Windows. `mountsUnder` and `fsFor` both use it; nothing else reimplements path semantics.
- `FSLookup` wraps `fsFor` over `Mounts` then `Root`, returning `(FSInfo{}, false)` for a nil report, so the UI never reimplements path semantics.

| File | Build tag | Provides |
|---|---|---|
| `fs_linux.go` | `linux` | `osMountTable()` reads `/proc/self/mountinfo` → `parseMountinfo`; `pathPrefix` case-sensitive |
| `fs_darwin.go` | `darwin` | `osMountTable()` via `syscall.Getfsstat` (two-pass: nil buffer for the count, then `MNT_NOWAIT`) → `Statfs_t{Fstypename, Fsid, Mntonname}`; `Device` folds `Fsid`; `pathPrefix` case-sensitive |
| `fs_windows.go` | `windows` | two-phase enumeration (below); `pathPrefix` case-insensitive |
| `fs_other.go` | `!linux && !darwin && !windows` | `osMountTable()` returns `errUnsupported` → `FSUnavailable` with that reason; `pathPrefix` case-sensitive. Unix-like GOOS only: the package already requires `unix \|\| windows` (no `deviceOf` elsewhere), so `!unix && !windows` stays unsupported rather than a new hole |

`deviceOfEntry(fi os.FileInfo, path string, table []FSInfo) (uint64, bool)` sits next to
the existing `deviceOf`: `walker_unix.go` (`unix`) returns `deviceOf(fi)`, ignoring
`path`/`table`, so **Unix behavior is byte-for-byte unchanged**; `walker_windows.go`
returns `fsFor(path, table)`.

**Windows two-phase enumeration** (avoids blocking on unrelated, possibly disconnected
network mappings): (1) `FindFirstVolumeW`/`FindNextVolumeW` +
`GetVolumePathNamesForVolumeNameW` (two-pass for the buffer length) → volume mount points,
including drive letters **and** mounted folders — local metadata only; (2)
`GetVolumeInformationW` (FS name + serial) **only** for volumes at or under the root. A
path not covered by that table (e.g. a UNC root) yields `Root.Avail = FSUnavailable` with
a reason, and OneFS fails open there.

### Walker integration (`internal/scan/walker.go`)

- `Walker` gains `table []FSInfo` and `fs *FSReport`; new `func (w *Walker) FSReport() *FSReport` (a fresh Walker per scan, so the pointer is not aliased after the scan).
- In `Scan()` (after `filepath.Abs`, so the filter sees an absolute root): file root → `Root = FSInfo{Avail: FSNotApplicable, Reason: "scan root is a regular file"}` with no table read; directory root → `table, err := osMountTableFn()`, and on error `Root = FSInfo{Avail: FSUnavailable, Reason: err.Error()}` with `Mounts` nil (**the scan still succeeds**); on success → `Root, _ = fsFor(root, table)`, `Mounts = mountsUnder(root, table)` (excluding the root's own entry).
- `rootDevice` becomes `rootDevice(info, root, w.table)` and goes through `deviceOfEntryFn`, so root and children use one identity source. The OneFS check at `walker.go:355` becomes `deviceOfEntryFn(fi, p, w.table)`; still only called when `w.opts.OneFS` is set.
- No change to `Options`, so no `config`/`Options` doc-tag churn.

### UI integration

- `scanResultMsg` gains `fs *scan.FSReport`; `scanCmd` (`tab.go:830`) fills it from `w.FSReport()` (the empty-root early return at `:833` leaves it nil).
- `Tab` gains `fs *scan.FSReport`, set on success in `updateScanResult` and cleared with `t.root` in the `msg.err != nil` branch (`model.go:309-316`). Cancellation keeps the prior tree **and** its report (early return at `model.go:304`).
- **Initial report:** `ui.NewWithReport(rootPath, root, fs, debugEnabled, base)`, with `New(...)` delegating with `fs = nil`; `main.go:138` switches to `NewWithReport(root, node, w.FSReport(), cfg.Debug, baseOpts)`.
- Header (`tab.go:404`): append `t.fsToken()` after the sanitized path. `t.fs == nil` → no token; `scan.FSLookup(t.fs, t.cur.Path)` → `FSAvailable` → `[<FSType>]`, `FSUnavailable` → `[fs:?]`, `FSNotApplicable` → none. Render the name through `sanitize` + `truncateWidth` (fixed budget, e.g. 12 columns); the existing `padRight(truncateWidth(headerText, panelWidth), panelWidth)` stays.
- `filesystem` section, inside Phase 1's `tab` zone: root mount point + FS type (or state + reason) + mount count, then the inline mount list (`mount point | FSType | avail | device`). Sanitize and truncate mount points and `Reason` the way the tabs table truncates `root` (`tab.go:883-886`).
- `recinfo.go`: the two registry entries, an `fsAvail` block in `enumsView`, and the corrected `bitDocs` sentence.
- **Stale prose fix (required):** `bitDocs` (`recinfo.go:199-214`) currently says one-file-system is `n/a on windows`; after this change only **hardlink dedup** (`fileKey`) is n/a there. Update the sentence and `TestWindowsNotApplicable` (`recinfo_test.go:365`) together, asserting the new claim so the old one cannot return.
- `main.go`: when `cfg.OneFS` is set and `w.FSReport().Root.Avail != scan.FSAvailable`, log at warn through the logger from `logging.Setup` (`main.go:68-74`): `--one-file-system: filesystem identity unavailable (<reason>); crossing mount points`. Per-directory unknowns are not logged individually.

## Phase 3 — debuginfo bundle

**Purpose.** One artifact for a bug report: identity, inputs, and per-tab state by
default; the tree only on request. It complements `--export` rather than replacing it.

**Archive contents** (`archive/zip`, fixed entry order):

| Entry | Format | Contents |
|---|---|---|
| `README.txt` | text | what the bundle is, what each entry holds, and the privacy note: it contains absolute paths, and the full file list only with the tree switch |
| `manifest.json` | JSON | `runtime/debug.ReadBuildInfo()` (module path/version, VCS revision + time when stamped, Go version), GOOS/GOARCH, `runtime.NumCPU`, generated-at (UTC), the effective options (workers, excludes, count-links, one-file-system, follow-symlinks, detect-format, debug, trash, log-file path), and the entry list actually written |
| `debug-tab.txt` | text | the full two-zone page from Phase 1, so the model reference and the inspected tab's state are both verbatim |
| `tabs.jsonl` | JSONL | one object per tab, in tab order: index, kind, status, root, nodeCount, estMem, unit, expanded, detectFormat, filter raw + scope, search active + raw + result count, and that tab's FS report |
| `fs.jsonl` | JSONL | one object per mount across all tabs (tab index, mount point, fstype, avail, device, reason) |
| `tree-summary.json` | JSON | per tab: nodeCount, totalSize, and the top 20 entries by size (tie-break size desc, then path asc) |
| `tree.jsonl` | JSONL | **opt-in only** (`--debuginfo-tree`): one node per line — path, name, size, appSize, isDir, kind, type, mtime, attrs — depth-first in display order |

**Package.** New `internal/debuginfo`, stdlib only. It must not import `internal/ui`; the
page text and the records are inputs, so the CLI and the TUI share one writer:

```go
type TabRecord struct { /* index, kind, status, root, nodeCount, estMem, ... */ }
type MountRecord struct { /* tabIndex, mountPoint, fstype, avail, device, reason */ }

type Input struct {
    Options    config.Config  // effective options, recorded verbatim
    Tabs       []TabRecord
    Mounts     []MountRecord
    DebugPage  string         // already-rendered text
    Tree       *tree.Node     // nil unless the tree switch is set
    BuildInfo  string         // captured by the caller
    Runtime    string         // the runtime block, captured by the caller
    Now        time.Time      // injected so a test can pin it
}

func Write(path string, in Input) error
```

`ui` owns the mapping from its `Tab`s to `debuginfo.TabRecord`/`MountRecord` (only `ui`
knows the `Tab` internals) via e.g. `func (m Model) DebugInfoInput(tree *tree.Node, now time.Time) debuginfo.Input`, plus an exported accessor for the page text so the headless
path does not need a running TUI. `ui` therefore depends on `debuginfo`, never the
reverse.

**Determinism.** `Now`, `BuildInfo`, and `Runtime` are inputs rather than read inside
`Write`, so a test can produce a byte-exact zip. Every zip entry gets one fixed
`Modified`, entries are written in a fixed order, and `tree-summary.json` sorts by size
desc then **path asc** — never via `tree.SortBySize`, which is not stable for equal sizes
so equal-sized entries could otherwise reorder between runs. Records go through
`encoding/json` structs (not maps), so key order cannot vary.

**Trigger.**

- `--debuginfo FILE` writes the bundle and exits, exactly like `--export FILE`; the two
  may be combined, and `--debuginfo` does not require `--debug`.
- `--debuginfo-tree` (bool) adds `tree.jsonl`; without `--debuginfo` it is ignored with a
  warn rather than a silent no-op.
- On the Debug tab, `E` writes `ncdu-next-debuginfo-<UTC yyyymmddThhmmss>.zip` in the
  working directory and reports the path in the zone bar. It runs as a `tea.Cmd` (the
  `scanCmd` pattern), never inline in `Update`, so a large dump cannot stall rendering;
  completion arrives as a message that sets a transient status. The key never includes the
  tree (bounded by design); only the flag can.

**Privacy and size.** Default bundle is bounded (no tree), but still contains absolute
paths via `debug-tab.txt`, `tabs.jsonl`, and `tree-summary.json`'s top-N; `README.txt`
states this. Redaction is out of scope.

**Docs:** `README.md` (the zones and their keys, the header token, `--one-file-system` on
Windows, and a short "debuginfo bundle" subsection) and `docs/ncdu-next.1` (the Debug keys,
the two new flags, `e`/`w`, `D`).

## Failure modes

- **`TestDebugTabRendersRecords` under a viewport** → headings are no longer in `View().Content`, so it fails unless rewritten to assert on `debugPage` lines; keep the name (it is in `guardNames`).
- **Inspected tab = the Debug tab** → reading `m.active` yields the Debug tab's own empty state. Capture `inspect` in the `D` handler *before* focusing the Debug tab, and never overwrite it when `D` is pressed while already there.
- **Dangling inspected pointer** → `x` closing that tab; validate against the live `tabs` at render and fall back.
- **Debug offset out of range** after the page shrinks (the inspected tab is closed) → clamp `t.scroll` to `[0, len(lines)-1]` every render.
- Missing registry decision for a new struct → `TestDocCoverage` fails. Fixed by the two Phase 2 entries (`internal/debuginfo` is not walked, so its records need none).
- `FSAvail` constants written with the `= "x"` shorthand → `constStringValues` misses them and `TestEnumSliceMatchesConstBlock` fails with a confusing message. Write each with an explicit `FSAvail` type.
- New enum added to `enumsView` but not to `TestEnumMeaningComplete`'s value list → the enum is silently unguarded. Both must change together.
- **Naive path prefix** (`strings.HasPrefix`) makes `/a/bc` look like it is under `/a/b`, mis-attributing a directory's filesystem and mis-filtering the mount list. `pathPrefix` compares at a separator boundary; a test covers exactly this pair.
- Stale bit prose → `bitDocs` claims OneFS is n/a on Windows after it works; text and test change together.
- Windows enumeration failing or slow → never fatal: `Root.Avail = FSUnavailable` with the reason, OneFS falls back to its old no-op behavior (fail-open), one warn log.
- Windows path semantics → case-insensitive comparison on cleaned paths, the `C:\` trailing-separator edge, mounted-folder volume paths; a UNC root renders `unavailable`, never a wrong name.
- Test seams + `-race` → `osMountTableFn`/`deviceOfEntryFn` are package vars; a test sets one **before** `Scan()` and restores it **after** it returns (goroutines are joined by then), and never calls `t.Parallel()`.
- darwin `getfsstat` reader compiles but is never executed here → it must not be described as verified in a commit message, README, or doc comment.
- Display safety → the FS name, mount points, and `Reason` are external data; render them through `sanitize`/`truncateWidth` and the fixed-width Debug formats.
- **Non-deterministic bundle** → `runtime.MemStats`, `time.Now`, and Go map iteration all leak in if read inside `Write`; a byte-exact test fails until `Now`/`Runtime` are inputs, the top-N tie-break is explicit, and JSON goes through structs.
- **New `Config` fields only reach the program two ways.** `ApplyFlags` registers the flag, but `main.go`'s `--config` branch (`main.go:46-65`) merges fields one by one, so a field missing from that block is silently ignored when set from the config file — the flag appears to work while the file setting does nothing.
- **Bundle writer on the UI thread** → a large tree dump inline in `Update` freezes the view; it must be a `tea.Cmd` with a completion message.
- **`--debuginfo-tree` without `--debuginfo`** → silently ignored would be a no-op that looks like success; warn.
- **Nice, not silent, failures** → an unwritable path returns an error and a non-zero exit, exactly like `--export` (`main.go:95-101`), rather than producing a partial bundle.

## Out of scope

- Any `export.Entry` / `ncdu 2.x` JSON field.
- Per-node filesystem attribution on `tree.Node`.
- Per-tab Debug tabs, and a `--no-fs` flag or config key.
- Filesystem free/total/used space.
- Fixing `deviceOf`-based **hardlink dedup** on Windows (`fileKey` stays `""`).
- BSD (freebsd/netbsd/openbsd) readers: they fall through to `fs_other.go` and report `unavailable`.
- Search/highlight inside the Debug page (section jumps only).
- Bundle redaction/anonymisation, remote upload or telemetry, and including the `--log-file` contents (the manifest only records its path).

## Ordered task list

### Phase 1 — Debug tab structure (no scan changes)

1. `internal/ui/tab.go`: introduce `debugSection` and split the page into `debugSections(tabs, inspect)`, `debugPage(secs)`, `sectionOffsets(secs)`, `debugViewport(lines, offset, height)`; add the `tab`/`program` zone headings, cross-tab totals, the inspected-marked tabs row, the per-tab detail block, and the pinned zone/position bar.
2. `internal/ui/model.go`: `Model.inspect *Tab` (set in the `D` handler when leaving a nav tab), pass it plus `m.width`/`m.height` into the debug render, and add the `tabKindDebug` key branch (`up`/`k`, `down`/`j`, `pgup`, `pgdown`, `home`, `end`, `[`, `]`; keep `q`/`ctrl+c`/`1-8`/`ctrl+tab`/`t`/`x`/`D`; ignore nav-only keys).
3. `internal/ui/recinfo_test.go`: rewrite `TestDebugTabRendersRecords` to assert on `debugPage(...)` (keep the name).
4. `internal/ui/tab_test.go`: viewport tests — offset clamping, section jumps land on headings, the inspected tab defaults to the tab `D` was pressed from and is never the Debug tab, closing the inspected tab falls back.
5. Manual: `ncdu-next --debug`, press `D`, confirm both zones, scroll/jump to the last section on a small terminal, and that `/`, `g`, `s`, `d` no longer act on the Debug tab.

### Phase 2 — Filesystem detection and display

6. `internal/scan/fs.go`: `FSAvail` (+ values + `FSAvailValues`), `FSInfo`, `FSReport`, `parseMountinfo`, `pathPrefix` contract, `mountsUnder`, `fsFor`, `FSLookup`, and the `osMountTableFn`/`deviceOfEntryFn` seams.
7. `internal/scan/fs_linux.go`, `fs_darwin.go`, `fs_windows.go`, `fs_other.go`: `osMountTable` + `pathPrefix` (darwin tag is `darwin` only).
8. `internal/scan/walker_unix.go` / `walker_windows.go`: `deviceOfEntry` next to `deviceOf`.
9. `internal/scan/walker.go`: `table`/`fs` fields, `FSReport()`, populate the report in `Scan()`, `rootDevice` + OneFS check through `deviceOfEntryFn`.
10. `go mod tidy` (promotes `golang.org/x/sys` to direct for the Windows reader).
11. `internal/ui/tab.go`: `scanResultMsg.fs`, `Tab.fs`, `fsToken()`, header token, and the `filesystem` section inside the `tab` zone.
12. `internal/ui/model.go`: `NewWithReport` (+ `New` delegating), carry `fs` through `updateScanResult`.
13. `cmd/ncdu-next/main.go`: pass `w.FSReport()` to `NewWithReport`; warn on OneFS with an unavailable identity.
14. `internal/ui/recinfo.go`: `fs record` row, `documentedTypes`, `typesNotRecords`, `fsAvail` enums block, corrected `bitDocs`.
15. `internal/ui/recinfo_test.go`: extend `TestEnumSliceMatchesConstBlock` (parse `../scan/fs.go`, block `FSAvail`) and `TestEnumMeaningComplete`; update `TestWindowsNotApplicable`.
16. `internal/scan/fs_test.go`: `parseMountinfo` fixture (with `\040` and a bind-style duplicate), `pathPrefix` boundary cases, `mountsUnder`, `fsFor` longest-prefix, `FSLookup`, the three availability states as three separate tests (stubbing `osMountTableFn`), a stubbed reader error, and `TestOneFileSystemDeviceChange` (stub `deviceOfEntryFn`).
17. `internal/ui/tab_test.go`: header token for available / unavailable / not-applicable / nil report, and that the token follows the **current** directory across two synthetic mounts.

### Phase 3 — debuginfo bundle

18. `internal/debuginfo/bundle.go`: record types, `Input`, `Write` (fixed entry order, fixed zip `Modified`, struct-based JSON, top-N tie-break), and `README.txt` generation.
19. `internal/ui/debuginfo.go`: `Model.DebugInfoInput(tree, now)` mapping tabs + FS reports, and an exported page-text accessor for the headless path.
20. `internal/ui/model.go`: the `E` key on the Debug tab as a `tea.Cmd` writing the default-named bundle, with a completion message + transient zone-bar status.
21. `cmd/ncdu-next/main.go` + `internal/config/config.go`: `--debuginfo FILE` (write and exit, combinable with `--export`) and `--debuginfo-tree`, each with a `doc` tag (guarded by `TestRecordFieldTags`), registered in `ApplyFlags`, **and added to the explicit `--config` merge block** (`main.go:46-65`). `DebugInfoTree` being a bool also lands in the Debug `flags` view automatically via `flagRecordTypes`/`TestBoolFieldsListed`.
22. `internal/debuginfo/bundle_test.go`: byte-exact determinism with pinned `Now`/`BuildInfo`/`Runtime`; default entry set has no `tree.jsonl`; the tree switch adds it; every `.jsonl` line parses; every entry's `Modified` is the fixed epoch; `tree-summary` tie-break ordering.
23. `internal/ui/tab_test.go` or `internal/debuginfo`: `TestDebugInfoTabRecordFidelity` — a synthetic tab's record carries the same values the page shows for it.

### All phases

24. Docs: `README.md`, `docs/ncdu-next.1`.
25. Gates: `scripts/check.ps1`, plus `GOOS=linux go build ./internal/...` and `GOOS=darwin go build ./internal/...`.

## Validation plan

1. `scripts/check.ps1` → `check OK`.
2. `GOOS=linux go build ./internal/...` and `GOOS=darwin go build ./internal/...` succeed.
3. `parseMountinfo` checked against a hand-written fixture whose expected mount points, fstypes, and packed device ids are computed independently in the test.
4. Three separate tests for `available` / `unavailable` / `not-applicable`; each fails if the states are collapsed into one.
5. `pathPrefix` boundary test: `/a/b` is a prefix of `/a/b/c` and **not** of `/a/bc`.
6. `TestOneFileSystemDeviceChange`: device change → child skipped; unchanged → descended; `(0,false)` → fail-open descends.
7. Header test: a synthetic report with two mounts shows the child's FS after navigating into it, and a nil report shows no token.
8. Debug tests: `debugPage` contains both zone headings and every section; `debugViewport` shows the right window for a given offset and clamps at both ends; `[`/`]` land on heading offsets; the inspected tab is the tab `D` was pressed from, never the Debug tab.
9. Bundle determinism: two `Write` calls with the same pinned `Input` produce byte-identical zips; the entry list matches the manifest's recorded list; the tamper check is to replace the top-N tie-break with a size-only sort and confirm the ordering test fails.
10. Bundle contents: default set has no `tree.jsonl`; `--debuginfo-tree` adds it; with an empty tree the bundle is still valid and `README.txt`/`manifest.json` are present.
11. Tamper checks — each guard must fail for the right reason (mutate, confirm a non-zero exit, read the mutated value back, revert): add an `FSInfo` field with no `doc` tag → `TestRecordFieldTags`; drop an `FSAvailValues` entry → `TestEnumSliceMatchesConstBlock`; write one `FSAvail` constant with the `= "x"` shorthand → the same guard; empty a meaning in `enumsView` → `TestEnumMeaningComplete`.
12. Windows smoke (CI + manual): a scan resolves `Root.Avail == FSAvailable` with a non-empty `FSType` (assert non-empty only), and `--one-file-system` stops at a mounted-folder boundary while the default scan crosses it.
13. Linux manual: `/proc/self/mountinfo` names the root's type and a nested mount, and the header token changes across that boundary.
14. Manual: `--one-file-system` against a path whose identity is unavailable → warn log, `[fs:?]` token, Debug reason.
15. Manual: `--debuginfo out.zip` and `--debuginfo out.zip --debuginfo-tree`, then unzip, confirm the entries, that `tabs.jsonl` parses, and the size difference; press `E` on the Debug tab and confirm the bundle path appears in the zone bar without the view stalling.

## Residual (stated, not hidden)

- The darwin reader **ships but is best-effort**: it compiles and cannot be executed by this repo's CI or on this machine. Isolated behind `osMountTable`.
- Linux `osMountTable()` end-to-end is unexercised in CI; `parseMountinfo` is tested, the read is not.
- Windows volume enumeration runs on Windows CI, but mounted-folder and UNC behaviour is a manual check.
- BSDs are intentionally unavailable; adding them needs per-OS `Statfs_t` readers and a way to compile each.
- `bitDocs` remains prose: the corrected sentence is asserted for the presence of the new claim, not its full meaning.
- The Debug tab's cross-tab totals and `tree-summary.json`'s top-N are O(nodes); they are recomputed per render / per write and are not cached.
- A bundle is only as trustworthy as its inputs: `BuildInfo` is `(devel)` for an untagged local build, and the VCS fields are absent unless the binary was built inside a repository, so the manifest can legitimately say less than a user expects.

## Post-implementation addendum (2026-10-24)

This plan was written before implementation; this section records where the
shipped code ended up differing from it, plus the work that was added along the
way. The original text above is left as written, so the deltas stay visible.

### Commits

- `71031b9` Phase 1 — Debug viewport, zones, dedicated keys.
- `d6366ce` Phase 2 — filesystem detection; `--one-file-system` on Windows.
- `c26af02` Phase 3 — debuginfo bundle and `--debuginfo` flags.
- `214e104`, `7d323ee` — review findings 1–3 and 4–9 (see below).
- `d190714` — build-artifact convention (below; beyond the plan).
- Version identity work (below) — pending commit at the time of writing.

### Deviations from this plan (and why)

- **`debuginfo.Input` carries `Trees []TabTree` + `IncludeTree`, not a single
  `Tree *tree.Node`.** The plan's `tree-summary.json` is per tab, so one tree
  field could not express it. Each `TabTree` also carries `NodeCount`.
- **`osMountTable` takes the absolute root** (`osMountTable(root string)`), so the
  Windows reader can limit device-backed `GetVolumeInformationW` queries to
  relevant volumes. This closes plan-phase-2's stated intent ("only for volumes at
  or under the root"), which the root-less seam could not honour.
- **Windows relevance is "covers the root OR sits under it"** (`relevantMountPoints`).
  The ancestor case is required for `fsFor` to resolve the root; a nested-only
  filter would have made the root's own filesystem unresolvable.
- **`TabTree.NodeCount` is carried in from the scan**, and `debuginfo.countNodes`
  was removed, so `tabs.jsonl` and `tree-summary.json` cannot disagree on a tab's
  node count. The plan did not specify where that number came from.
- **The Debug page is assembled once per render** (`debugPageState` /
  `buildDebugPage`) instead of being rebuilt in `View`, in the zone bar, and again
  per scroll key. `debugLayout` is the single source for both the rendered lines
  and the section-jump offsets.
- **`internal/scan/walker_unix.go` now uses `devOf`/`inoOf`** so
  `GOOS=darwin go build ./internal/...` compiles; the plan (and `AGENTS.md`) had
  recorded darwin as an out-of-scope break.

### Review findings and fixes (not in the plan)

1–3 (`214e104`): a failed Windows volume read is `FSUnavailable`, not
`FSAvailable` with a blank type; a followed symlink's target is descended under the
*target's* device so `--one-file-system` no longer silently skips its
subdirectories; the OneFS warning fires only on `FSUnavailable`, not
`FSNotApplicable`. 4–9 (`7d323ee`): root-scoped volume queries; bounded top-N
selection; single page assembly per render; the duplicate `countNodes` removed;
`fsStateText` uses the enum's own values; the dead Windows `deviceOf` stub removed.

### Work beyond the plan: identity and artifact

- **Build artifact convention** (`d190714`): `scripts/build.ps1` and a `make
  artifact` target build `bin/ncdu-next.exe` and fail when the embedded
  `vcs.revision` differs from HEAD; documented in `AGENTS.md`.
- **Self-describing identity**: `internal/debuginfo.Build` / `ReadBuild`, plus
  `--version` and `--version --json`, a startup log stamp, a Debug-tab `build`
  section, and a CI step that re-runs `scripts/build.ps1` so the built artifact
  must name the commit it came from.

### Limitations that changed

- The top-N summary is still recomputed per write, but now keeps a bounded
  best-of-N set (O(nodes·n) time, O(n) memory) instead of materialising every node.
- The Debug page is assembled once per render, but a scroll or section-jump key
  still assembles it once in the key handler.
- `manifest.json`'s `build_info` is present even when the binary is unstamped, and
  now says so explicitly (`revision (no VCS metadata)`) rather than showing a
  misleading partial stamp.

## References

- Debug view and keys: `internal/ui/tab.go:858` (`debugView`), `:883-886` (root truncation), `:647` (`panelMetrics`); `internal/ui/model.go:144-159` (search-results key branch), `:194-210` (`D`), `:309-316`, `:335-395` (`View`, `AltScreen`)
- Walker + OneFS check: `internal/scan/walker.go:296`, `:307`, `:355`, `:480`
- Platform `deviceOf`/`fileKey`/`fileMeta`: `internal/scan/walker_unix.go`, `walker_windows.go`, `stat_unix.go`, `meta_linux.go`, `meta_unix.go`
- Existing headless output (the model for `--debuginfo`): `cmd/ncdu-next/main.go:95-101`; `internal/export/json.go`
- Config fields that need `doc` tags: `internal/config/config.go:12-25`
- Debug guards and registries: `internal/ui/recinfo.go` (`recordSpecs`, `documentedTypes`, `typesNotRecords`, `flagRecordTypes`, `bitDocs`, `recordsView`, `enumsView`), `internal/ui/recinfo_test.go:29` (`guardNames`), `:100`, `:116`, `:148`, `:204`, `:221`, `:267`, `:365`, `:406`
- Header/layout and constructors: `internal/ui/tab.go:377-533`, `scanCmd` `:830`; `internal/ui/model.go:41`, `:304`
- Gate and entrypoint: `scripts/check.ps1`, `.github/workflows/ci.yml`, `cmd/ncdu-next/main.go:68-90`, `:138`
- Dependency state: `go.mod:26` (`golang.org/x/sys` indirect)
- Repo conventions: `AGENTS.md` (two-table invariants, absent/unknown/n-a, Debug record guards, EOL pinning)
