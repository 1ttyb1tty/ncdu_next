# Scan progress, skip/error/size statistics

## Goal

Show live progress during every scan (startup, rescans, headless) and persist the
statistics — including *why* entries were skipped and, where measurable, how much was left
out — in the saved scan result, the Debug tab and the debuginfo bundle. Default I/O profile
is unchanged: measuring skipped subtrees is opt-in.

Today the only signal is the tab-bar `[scanning]` token (`internal/ui/tab.go:805`), the
startup scan runs before the TUI exists (`cmd/ncdu-next/main.go:95` vs `:159`), and the
unused `scan.Options.Progress` callback (`internal/scan/walker.go:22`) reads `files` outside
the mutex (`walker.go:471-476`, `:498-503`) — a latent race. Every skip is silent.

> **DEFERRED — REVISIT SOON.** Implementing the real ncdu array-envelope export (and the
> `dev`/`ino` retention it needs) is deliberately not in this plan. It is the **next** planned
> topic after this one, not a "someday": see *Follow-ups* at the end. Keep the `ScanStats` DTO
> container-agnostic so that work is a move, not a rewrite.

## Decisions

1. **Scope**: progress for the startup scan and rescans. The initial scan moves into the
   TUI; headless `--export` / `--debuginfo` keep the synchronous scan and `exit 1`.
2. **No percentage of the tree** — a walk has no total. Every share is of what has been seen.
3. **Skip classes** (no record produced): `excluded`, `cross_device`, `symlink_cycle`.
4. **Error causes** (records carrying `Err`): `dir_unreadable`, `entry_stat_failed`,
   `symlink_broken`.
5. **Size-suppressed** (record exists, size 0 by policy — never counted as a skip):
   `symlink_stub` (`FollowSym` off), `hardlink_dedup` (`CountLinks` on).
6. **Orthogonal tallies, not a partition**: a dir failing `ReadDir` counts in `Dirs` *and*
   `ErrDirUnreadable`. `DirsNotDescended` totals directories whose contents were not walked.
7. **Recursive accounting of skipped dirs, opt-in** (`--measure-skipped`), with an explicit
   measurement state per outcome; aggregates including an unknown contributor are lower bounds.
8. **Every figure shows `X / Y (p%)`** — numerator, denominator and derived percentage, with
   the denominator tracked and displayed. No display thresholds; the only guard is that a
   non-zero count never renders as `0%`.
9. **Completeness tags, four derived states**: `TOT` / `LO` / `UNK` / `N-A`. Global
   completeness is announced once per block; per-figure tags mark the figure-specific state.
   Tags are presentation only and never enter the saved JSON.
10. **Detail level is toggleable** (`v`), and caps the width-based auto-degradation rather
    than fighting it.
11. **Persistence**: live, plus four artifacts — the Debug tab, the debuginfo bundle, and
    **the `--export` file** (a top-level `scan` object, round-trip lossless).
12. **Startup failure**: interactive → tab `StatusError` (retry with `s`); headless → `exit 1`.
13. **`symlink_cycle` is not-applicable, never missing**: the subtree is already counted at
    the first path that reached it (`walker.go:405-413`); counting its bytes here would double
    count. Contributes 0, excluded from `DirsNotDescended`.

## Data model — `internal/scan/stats.go` (new)

Flat `ScanStats`, all `int64` plus `LastPath string`, `StartedAt time.Time`,
`SkippedBytesComplete bool`. Flat by design: nested structs would each need a
`typesNotRecords` entry (`TestDocCoverage`, `internal/ui/recinfo_test.go:148-181`).

```
Seen              // dirents examined; partition base
Files, Dirs, Bytes// records; Bytes = apparent total of records (post-policy)
SkipExcluded, SkipCrossDevice, SkipSymlinkCycle
ErrEntryStat, ErrSymlinkBroken
ErrDirUnreadable                 // subset of Dirs
SuppressSymlinkStub, SuppressHardlinkDedup   // subsets of Files
DirsNotDescended                 // excluded dirs + cross-device dirs + unreadable dirs
SkippedDirsMeasured, SkippedDirsUnmeasured, SkippedDirsUnmeasurable
SkippedObjects, SkippedBytes     // measured absent subtrees; NOT added to Files/Dirs/Bytes
SkippedBytesComplete
LastPath, StartedAt
```

Composite denominators are accessors, so there is one definition (methods, not fields, so no
extra allowlist entry):

```go
func (s ScanStats) ObjectsKnown() int64 { return s.Seen + s.SkippedObjects }
func (s ScanStats) BytesKnown() int64   { return s.Bytes + s.SkippedBytes }
```

Invariants (one test each):
- `Seen == Files + Dirs + SkipExcluded + SkipCrossDevice + SkipSymlinkCycle + ErrEntryStat + ErrSymlinkBroken` (exact partition).
- `DirsNotDescended == SkippedDirsMeasured + SkippedDirsUnmeasured + SkippedDirsUnmeasurable`.
- `Bytes == Σ recorded file Size` (post-policy) over the finished tree.
- With `--measure-skipped` off: `SkippedObjects == SkippedBytes == SkippedDirsMeasured == 0`.

Three distinct non-measurements, kept apart (machine rule: absent / unknown /
not-applicable are different answers):
- **not-applicable** — `symlink_cycle`; counted in `SkipSymlinkCycle`, never in byte totals.
- **unavailable** — `ErrDirUnreadable` / `entry_stat_failed`: never readable
  (`walker.go:342-348`, `:367-372`); counts known, contents not.
- **not measured** — `excluded` / `cross_device` dirs with the flag off: available in
  principle, deliberately not read.

Recinfo: add `"scan.ScanStats"` to `typesNotRecords` with a reason (transient scan output,
like the existing `"scan.FSReport"` entry, `internal/ui/recinfo.go:73`). Add `doc` tags to
all fields regardless.

## Export shape (`internal/export`)

**Constraint discovered while planning:** there is no ncdu 2.x contract to protect.
`json.go:10-23` writes a plain JSON *object* (`{name, asize, dsize, type, kind, …items}`),
whereas ncdu 2.x's export is a nested *array* envelope `[major, minor, {metadata}, rootdir…]`
— `ncdu -f` cannot read this output. The README and man page claim "ncdu 2.x JSON"; that
claim must be corrected (see Docs), and implementing the real envelope is out of scope.

- `Entry` gains `Scan *ScanStats \`json:"scan,omitempty"\``, populated on the root only, so
  the key appears once at the top level and a scan without stats serialises exactly as today.
- `export.ScanStats` is an **explicit DTO** with its own json tags and an
  `export.FromScanStats(scan.ScanStats) ScanStats` mapper — the same shape as the existing
  `export.FromNode(tree.Node) Entry` (`json.go:25-45`). Explicit tags keep the persisted
  names a stable contract instead of binding them to an in-memory struct. It is deliberately
  **container-agnostic**: it is embedded under `scan` on the root entry today, but if the real
  ncdu array envelope is built later, the same DTO moves into ncdu's metadata element (the
  third array element) unchanged — no counter, tag or UI change. That is what keeps the
  envelope decision cheap to defer.
- The DTO carries the **raw counters**, including the denominators (`seen`, `dirs`, `files`,
  `bytes`) so a consumer can compute shares itself, plus `skipped_bytes_complete`. It carries
  **no tag strings** (`TOT`/`LO`/…) — those are presentation (Decision 9).
- Timestamps are **params, not read inside the writer** (`started_at` RFC3339 UTC,
  `elapsed_ms`), mirroring `debuginfo.Input.Now`/`fixedModTime`, so a bundle is reproducible;
  tests assert non-zero rather than exact.
- Round-trip is **lossless**: new `export.ReadResult(path) (*tree.Node, *ScanStats, error)`
  returns both; `Read` becomes a thin wrapper that discards the stats, so existing callers and
  tests (`json_test.go`) are untouched. The stats must **not** live on `tree.Node` — its field
  order is load-bearing (176 B packed vs 192 B naive) and it is the Debug record reference.
- API is additive: today's `Write`/`Read` keep working with no stats; a new entry point
  carries them. The headless path fills them from `Walker.Stats()` at no extra cost.

## Fractions, tags and detail levels

`frac(n, d, unit) string` in `internal/ui` (pure, table-tested): `X / Y (p%)`, plus `pctOnly`
for narrow widths. Rules: integer arithmetic, one decimal; `d == 0` renders `X / 0` with `-`
for the percentage; a non-zero `n` below 0.1% renders `<0.1%`, never `0%`.

Each category is paired with the denominator that makes its share meaningful — directories
are a minority of dirents, so one denominator for everything would understate the dir-scoped
figures by roughly the dir:entry ratio:

| Group | Numerator | `Y` | Example |
|---|---|---|---|
| skip total, `excluded`, `cross_device`, `symlink_cycle` | each | `Seen` | `4 / 12403 (0.03%)` |
| `err_entry_stat`, `err_symlink_broken` | each | `Seen` | `1 / 12403 (0.01%)` |
| `err_dir_unreadable`, `dirs_not_descended` | each | `Dirs` | `1 / 678 (0.1%)` |
| `symlink_stub`, `hardlink_dedup` | each | `Files` | `10 / 11995 (0.1%)` |
| `skipped_objects` | `SkippedObjects` | `ObjectsKnown()` | `41000 / 53403 (76.8%)` |
| absent bytes | `SkippedBytes` | `BytesKnown()` | `4.2 GiB / 128 GiB (3.3%)` |

**Tags** are derived, never stored. `scan.Measure` with `MeasureTotal` / `MeasureLowerBound` /
`MeasureUnknown` / `MeasureNotApplicable` and `MeasureValues()`, registered in `enumsView`
plus the two enum guards (`TestEnumSliceMatchesConstBlock` against `../scan/stats.go`,
`TestEnumMeaningComplete`), so the vocabulary cannot drift from the renderer.

Semantics that must be stated in the docs:
- **`TOT` means complete *within its stated scope*, which is `Seen`** — not "the whole tree".
  Global completeness is announced once per block (`scan partial: 4 entries skipped, 1 dir
  unreadable`), and that heading is what qualifies every `TOT` beneath it.
- Derivation: absent objects/bytes are `N-A` when no directory was skipped, `UNK` when no
  skipped dir was measured, `LO` when any skipped dir is unmeasured, unmeasurable, or
  `!SkippedBytesComplete`, else `TOT`. `symlink_cycle` is `N-A`. Every other figure is `TOT`.
- Live, only the absent-subtree group is tagged (its partiality is by *content*); the plain
  counters carry no tag because their only partiality is by *time*, announced by the heading.
  Final, every row is tagged.
- Bytes read `>= 4.2 GiB / >= 128 GiB` when the state is `LO`, since both numerator and
  denominator then exclude an unknown amount.

**Detail levels** (one `Model` field, cycled with `v`, default 1, seeded by
`--progress-detail` / TOML `progress_detail`):

| Level | Shows |
|---|---|
| 0 compact | `seen`, skip total + `%`, error total + `%`, `last:` path |
| 1 standard | + per-class breakdown brackets, `not descended`, `suppressed` |
| 2 full | + absent/measured detail, state counts and tags |

The effective level is `min(manual level, what fits)`: the existing width-based drop order
(`tab.go:428-432` is the precedent) applies afterwards, and the toggle never forces content
past the panel width. Drop order: `last:` path, per-class breakdown, error causes,
`not descended`, `suppressed`, then the percentages via `pctOnly`. Never drop a numerator,
its denominator, or a tag. The Debug tab's `scan stats` section always renders level 2,
matching the existing split where the header carries only a token (`fsview.go:20-23`).

## Ordered tasks

1. **`internal/scan/stats.go`** — `ScanStats`, `Measure` + `MeasureValues()`, the two
   accessors, the state-derivation helper.
2. **`internal/scan/walker.go`** — `Stats()`; instrument the sites below; accumulate under the
   existing `mu`; **remove `Options.Progress`** and its race; add `Options.MeasureSkipped` and
   the measurement walk.
3. **`internal/export`** — `ScanStats` DTO + `FromScanStats`; `Entry.Scan` (`omitempty`);
   `ReadResult` with `Read` delegating; a stats-carrying write entry point that leaves
   `Write`/`Read` and `json_test.go` working.
4. **`internal/config/config.go`** — `MeasureSkipped`, `ProgressDetail` (doc tags, since
   `config.Config` is a `recordSpecs` entry and `TestRecordFieldTags` checks every field);
   `--measure-skipped`, `--progress-detail`; merge in `main.go`.
5. **`internal/ui/recinfo.go`** — allowlist `scan.ScanStats`; register `Measure` in
   `enumsView`; extend both enum guards.
6. **`internal/ui`** — `frac` / `pctOnly` / tag derivation; `statsview.go`
   (`scanStatsSection` / `scanStatsLines`) wired into `debugSections` (`tab.go:1029`);
   `Tab.walker` + `Tab.stats` + stats polling; `progressTick`; `Model` detail level + `v` key;
   footer and help text; `Model.WithWarn`; `NewScanning` + `Init`.
7. **`internal/ui/debuginfo.go`** + **`internal/debuginfo`** — nested `scan_stats` on
   `TabRecord`, bundle `README.txt`; zip entry list (`bundle.go:137-143`) unchanged.
8. **`cmd/ncdu-next/main.go`** — split headless vs interactive; interactive builds a pending
   model and lets `Init` scan; headless passes `Walker.Stats()` into the export.
9. **Docs** — `README.md` and `docs/ncdu-next.1`, including the `ncdu 2.x JSON` correction.
10. **Tests** (below), then the gate.

### Walker instrumentation (`internal/scan/walker.go`)

| Site | Effect |
|---|---|
| `Scan` entry | `StartedAt`; `Seen` for the root |
| regular-file root `:298-302` | `Files++`, `Bytes=size`, `LastPath` |
| dir child added `:381-385` | `Dirs++`, `Seen++`, `LastPath` |
| file record, symlink target `:465-473` | `Files++`, `Seen++`, `Bytes+=size`, `LastPath` |
| file record, regular `:492-500` | `Files++`, `Seen++`, `Bytes+=size`, `LastPath` |
| `isExcluded` `:363-365` | `SkipExcluded++`, `Seen++`; if `e.IsDir()` → `DirsNotDescended++` + measurement |
| `OneFS` cross-device `:376-380` | `SkipCrossDevice++`, `Seen++`, `DirsNotDescended++` + measurement |
| repeat symlink target `:405-413` | `SkipSymlinkCycle++`, `Seen++` (never in byte totals) |
| `os.ReadDir` error `:342-348` | `ErrDirUnreadable++`, `DirsNotDescended++`, `SkippedDirsUnmeasurable++`, `LastPath` |
| `e.Info()` error `:367-372` | `ErrEntryStat++`, `Seen++`, `LastPath` |
| symlink target `EvalSymlinks`/`Stat` error `:414-427` | `ErrSymlinkBroken++`, `Seen++`, `LastPath` |
| symlink stub, `FollowSym` off `:394-403` | `SuppressSymlinkStub++` |
| hardlink repeat forced to 0 `:453-464` | `SuppressHardlinkDedup++` |

`e.IsDir()` is read only on the excluded path (a `DirEntry` check).

### Measurement walk (`--measure-skipped`)

- Applies to `excluded` and `cross_device` **directories** only; inline in the walk using the
  existing worker semaphore; unbounded depth, matching the scan.
- **Honours `OneFS`** (does not cross a device the scan would not have); **does not apply
  `Excludes`** inside (else a nested exclusion is counted twice — in the parent's total and as
  its own skip); creates **no records**, touching no `Seen`/`Files`/`Dirs`/`Bytes`; sums
  `fi.Size()` (apparent, matching `Bytes`).
- Outcomes: success → `SkippedDirsMeasured++`, `SkippedObjects += n`, `SkippedBytes += b`; read
  failure → `SkippedDirsUnmeasurable++`, `SkippedBytesComplete = false`; flag off →
  `SkippedDirsUnmeasured++`.

### Startup path (`cmd/ncdu-next/main.go`)

- **headless** (`cfg.Export != "" || cfg.DebugInfo != ""`): scan synchronously as today, but
  pass `Walker.Stats()` into the export. Keeps `exit 1` on failure.
- **interactive**: no pre-TUI scan. `ui.NewScanning(root, cfg.Debug, baseOptions(cfg))
  .WithConfig(cfg).WithWarn(logger.Warn)`; `Model.Init()` returns `startScan` when created
  pending. `NewScanning` sets one tab's `rootPath`, leaves `root`/`cur` nil, `pendingScan =
  true` (mutating the shared `*Tab`, so the first frame reads `StatusScanning`).
  `NewWithReport` stays for the headless constructors.
- The interactive `--one-file-system` fail-open warning would otherwise vanish inside the TUI
  (today's warn at `main.go:103-108`); surface it in the panel via `WithWarn`.

## Tests

`internal/scan`: the four invariants plus `ObjectsKnown`/`BytesKnown`; skips — `excluded` via
`Options.Excludes`, `cross_device` by stubbing `deviceOfEntryFn` (`fs.go:48-51`),
`symlink_cycle` via `FollowSym` + a re-visited target; errors — unreadable dir (`chmod 000`,
**unix build tag**; Windows cannot deny this way) and a dangling symlink with `FollowSym`;
size-suppressed — both counters bump **while the record still exists with `Size=0`**;
measurement — the measured/unmeasured/unmeasurable partition, the incomplete flag when a
nested dir is unreadable, all fields zero with the flag off, and the tree identical with the
flag on and off; `Stats()` called from a goroutine during a large scan (replaces
`TestScanProgress`; `TestScanCancel` stays).

`internal/export`: round-trip preserves the stats (`ReadResult`); `Read` still returns the
tree and its existing tests pass unchanged; an entry written without stats serialises
**byte-identically** to today (assert on the marshalled bytes, `omitempty`); the DTO field
names are pinned by a golden-key assertion so a rename cannot silently change the format; a
partial/unreadable export never fabricates a `TOT`-implying `skipped_bytes_complete = true`.

`internal/ui`: `frac`/`pctOnly` table test (`X / Y (p%)` per table row, `X / 0` with `-`,
`<0.1%` but never `0%`); tag-derivation table test covering all four states, including
`TOT`-is-scope-relative and the block-level global-completeness heading; `v` cycles 0→1→2→0
and the width drop order still applies at level 2; Debug `scan stats` renders level 2
regardless of the model's level; scanning body with no tree; header summary with a tree; tick
stops after `StatusDone` and ignores a stale `gen`; `NewScanning` + `Init` returns a cmd;
OneFS warning via `WithWarn` and in the panel; footer/help lists `v`.

`internal/debuginfo`: nested `scan_stats` asserted in `bundle_test.go` and documented in the
bundle `README.txt`.

## Docs

`README.md`: progress + the skip/error/size-suppressed table; the `X / Y (p%)` convention;
the four tags and the rule that `TOT` means "complete within `Seen`", qualified by the block's
global-completeness heading; the three non-measurement states; the lower-bound rule for
bytes; that `Bytes`/`SkippedBytes` are post-policy apparent totals and never match `du`/`df`;
`--measure-skipped` and `--progress-detail` in the flags table with their costs; the `v` key;
that the startup scan is now cancellable with `c`; and the `scan` object in `--export`.
**Correct the `ncdu 2.x JSON` claim** here and in `docs/ncdu-next.1`: describe the actual
custom object shape, and state plainly that the ncdu array-envelope format is not
implemented, so `ncdu -f` cannot read it.

## Validation

```
powershell -ExecutionPolicy Bypass -File scripts/check.ps1
```

`gofmt -l .`, `go vet ./...`, `go build ./...`, `go test -count=1 -race ./...`; success line
`check OK`. Manual smoke: progress moves on a large tree; a rescan keeps the old tree visible;
`v` cycles and narrows/widens without exceeding the panel; `--measure-skipped` on a tree with
an excluded large directory reports a tagged figure and the same tree without the flag reports
`UNK`; `--export` round-trips through `ReadResult` with the stats intact.

## Risks

- **Windows**: `fileKey` returns `""` (`walker_windows.go:10`), so `symlink_cycle` is
  structurally 0 and there is no cycle guard; the class cannot be exercised there, and the
  cross-device test must stub `deviceOfEntryFn`.
- The unreadable-directory test is unix-only.
- `--measure-skipped` re-reads exactly what the user asked to skip; it can be slow (usually the
  reason for the exclusion) and can itself fail, which is why the state is recorded.
- The self-sleeping tick keeps one goroutine per in-flight scan and must not re-arm for a
  finished, cancelled, superseded (`gen`) or closed tab.
- Removing `Options.Progress` touches a `recordSpecs`-documented struct; regenerate the Debug
  `records` output expectations rather than hand-editing them.
- The new `scan` key changes the export **shape**; consumers that iterate keys rather than
  fields will see it. `omitempty` keeps the no-stats output byte-identical, which the export
  test pins.

## Out of scope

- **Implementing the real ncdu array-envelope export format.** Only the docs change here
  (above). It is deferred because it is a breaking change to a surface `AGENTS.md` calls a
  contract, and because it has an unmet data dependency: ncdu carries per-entry inode/device
  (how it re-detects hardlinks on import) while `tree.Node` retains neither (`node.go:17-34`)
  — the walker computes that identity transiently for dedup and `--one-file-system` only. So
  it needs a decision on retaining `dev`/`ino` (new fields against the 176-vs-192-byte packing
  and the record-reference guards, or a scan-side side table) before any envelope work.
- Percentage/ETA of the whole tree (pre-pass); df-style device-usage bytes for mount
  boundaries (`fs_darwin.go:26` already reads `Statfs_t` fields it discards; Linux/Windows
  would need new readers); `reparse_point` (Windows junctions/volume mount points);
  sparse-vs-allocated size; alternate data streams.

## Follow-ups — revisit soon, in this order

1. **[NEXT] ncdu envelope + `dev`/`ino` retention.** Decide how to retain per-entry
   device/inode (new `tree.Node` fields against the 176-vs-192-byte packing and the
   record-reference guards, or a scan-side side table), then implement
   `[major, minor, metadata, rootdir…]` and move the `ScanStats` DTO into ncdu's metadata
   element — the DTO is already portable by design (see "Export shape"). Anything that
   consumes or documents `--export` should be treated as changing here.
2. Verify the exact required ncdu field set against ncdu's format spec at that point; it was
   not verified from this repository.
