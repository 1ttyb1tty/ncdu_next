# Plan: Debug tab — derived record layout, flags/bits, and type/enum reference

**Goal:** Extend the read-only Debug tab (`D`) so it documents the in-memory data
model of ncdu-next: the per-**file** and per-**directory** record (`tree.Node`),
the **input records** (`scan.Options`, `config.Config`), the code-defined
**tables/maps** (`fileTypes`, style maps), the **boolean flags and bit
constants** that shape records, and the **enums** (`ScanStatus`, `tabKind`,
`filter.Mode`, the `Type` vocabulary) — each field with its datatype, byte size
(fixed, or a `len`-dependent min → max), and a one-line explanation.

**The driving constraint (user-raised): these structures change often, so the
Debug screen must not rot.** The design therefore splits every datum into two
classes and makes each class cheap to keep correct:

| Class | Examples | How it stays correct | Manual cost per change |
|-------|----------|----------------------|------------------------|
| **Derived** (structure) | field names, types, offsets, padding, fixed struct size, header sizes, pointer size, enum value numbers, bit values, `len()` of tables, list of `bool` fields | Computed at render time from `reflect` + runtime constants. Never written down. | **Zero** |
| **Co-located** (prose) | what a field means, what a flag toggles, what a bit gates | Lives as a struct **tag** on the field it describes; a test fails if any field lacks one | One tag next to the field you already edited |

The one thing that cannot be mechanically derived — the English sentence — is
placed where the developer is already editing, and its absence is a failing test
and a visible `(undocumented)` marker in the running UI. Nothing else needs
touching when a struct changes.

## Decisions (resolved)

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Layout/sizes | Computed at render time from `reflect` (`Field(i).Name/Type/Offset/Size`, `Type.Size()`) | Field add/remove/rename/reorder updates the screen automatically; a hand-written size table is the maintenance the user flagged |
| Field prose | **Struct tags**, e.g. ``Name string `doc:"base name of the entry" file:"set" dir:"set"` ``, read via `reflect` `Tag.Get` | Doc lives at the edit site, so it moves with the field; removes the second registry that would go stale; tags cost no per-instance memory |
| Missing prose | Test fails; renderer prints `(undocumented)` | Drift is loud in CI *and* visible in the TUI, not silent |
| Stale prose | Not mechanically detectable (an edited sentence can describe the old behavior); co-location is the mitigation | Stated honestly rather than overclaimed |
| Boolean flags | Discovered by `reflect` (`Kind()==Bool`), explained by the same `doc` tag; listed in a `flags` table | A bool is a 1-byte field and needs its own row, not just a type entry |
| Bit constants | Rendered from runtime values (`uint32(os.ModeSymlink)`, `bits.TrailingZeros32`, `unsafe`-free), never a literal | A hard-coded `1<<n` is exactly what rots; computed values cannot |
| Enums | Canonical value slice per enum + an AST test that parses the const block and asserts the slice matches | Go cannot enumerate consts at runtime; the AST test fails the moment a constant is added without updating the slice |
| Tables/maps | `len()` and a sample computed at runtime via small accessors (e.g. `scan.TypeTableStats()`); one prose line each | Counts cannot go stale; contents are code-defined and only their meaning needs wording |
| Platform differences | Sizes labelled `GOOS/GOARCH`; Windows `fileKey`/`deviceOf` stubs shown as `n/a on windows` | 32/64-bit and pointer size change every number; "absent" is not "disabled" |
| Records shown | `tree.Node` in `file` and `dir` modes, plus `scan.Options` and `config.Config` | Flags/bits are fields of these records; all four use the tag+reflect mechanism |
| Update scope | Debug tab + inert `doc` tags; no behavior change to tree/scan/export | Purely informational |

## What the sections show

### `datatypes` — legend
Type sizes from `reflect.TypeOf` on sentinels, labelled with platform:
`bool` 1 B (0x00/0x01); `int`/`int64`/`uint64` 8 B; `string` 16 B header +
content; `*Node` 8 B (64-bit); `[]*Node`/`[]string` 24 B header + `len*elem`;
`error` 16 B interface; `func(...)` 8 B; `context.Context` 16 B interface.

### `records` — four record types, all reflect-driven
For each record, one row per field: `field | type | size | offset | role | doc`.
`size` is fixed for scalars, and `header B + len*elem` (with a min→max note) for
`string`/slice/pointer. `doc` comes from the field's tag.

- **file record / dir record** (`tree.Node`): role column marks `file` vs `dir`
  meaning (`IsDir` `false`/`true`; `Children` `nil`/array; `Parent` `set`/
  `set|nil`; `Size` `bytes`/`subtree`). Fixed total read from `reflect`
  (≈152 B on amd64 — computed, not asserted as text).
- **options record** (`scan.Options`): `Root`, `Workers`, `CountLinks`,
  `OneFS`, `FollowSym`, `Excludes`, `DetectFormat`, `Progress`, `Ctx`, with docs
  noting which fields change node records (flags) and which are scan-only.
- **config record** (`config.Config`): CLI/TOML input; docs mark fields that do
  **not** reach records (`trash`, `verbose`, `debug`, `log_file`, `export`).

### `flags` — booleans and bits
- Boolean fields, discovered by `reflect`, grouped by owning record, with their
  `doc` tag and default: `tree.Node.IsDir`; the `scan.Options`/`config.Config`
  flags.
- Bit constants, **values computed at runtime** and prose from a small doc list:
  - `os.ModeSymlink` — symlink bit (computed: `1<<27`, `0x08000000`).
  - `os.ModeDir` / `FileInfo.IsDir()` — directory bit (`1<<31`, `0x80000000`).
  - `os.FileMode` type word — high bits classify the entry; low 9 bits are
    permissions (not stored on the record).
  - `syscall.Stat_t.Dev` (`uint64`) — compared for `--one-file-system`, part of
    the hardlink key.
  - `syscall.Stat_t.Ino` (`uint64`) — with `Dev` forms the `"dev:ino"` dedup key.
  - Availability: `fileKey`→`""`, `deviceOf`→`(0,false)` on **windows**, so dedup
    and one-file-system are `n/a on windows`.

### `tables` — code-defined maps
Runtime `len()` and a few sample entries, one prose line each:
`scan` `fileTypes` (extension→type; drives the `Type` field), the `Type` prefix
vocabulary, and `ui` `typeStyles`/`typePrefixes` (render colors). Transient scan
maps (`visited`, `links`) are noted as ephemeral and not enumerated.

### `enums` — enum-style vocabularies
`ScanStatus` (`idle|scanning|done|error`), `tabKind` (`nav|debug`),
`filter.Mode` (`substring|glob|regex`), and the `Type` prefix list — each
rendered from its canonical slice. Also live `nodeCount` and an **estimated**
record-memory figure (`reflect size × nodeCount`), labelled as excluding string/
slice payloads.

## Maintenance model (the point of this plan)

| Change a developer makes | What they must also do | What catches them if they don't |
|--------------------------|------------------------|-------------------------------|
| Add/remove/reorder a field | Add/keep a `doc` tag on the field | `TestRecordFieldTags` (reflect), `(undocumented)` in the UI |
| Change a field's type | Nothing (sizes/offsets are derived) | — |
| Add an enum constant | Append to the value slice | `TestEnumSliceMatchesConstBlock` (AST) |
| Add an entry to `fileTypes` | Nothing (count is runtime `len`) | — |
| Change a flag's meaning | Edit the `doc` tag | *Nothing automatic* — co-location only (residual gap, stated) |
| Add a new record struct to document | Add a row to the `records` driver list | `TestRecordsCoverKnownStructs` |

### Guard design rules (how the guards themselves are validated)

A guard that cannot fail, or that fails for the wrong reason, is worse than no
guard. Every guard below must satisfy:

1. **Fail when the guarded thing is removed.** Each guard is proven by a
   deliberate tamper that lands *on the guarded quantity* (add an untagged
   field, drop an enum value, rename a table), with the check confirmed non-zero
   before the tamper is reverted — a passing guard and a no-op tamper look
   identical from the outside.
2. **Set-equality, both directions.** "Every field has a doc tag" is necessary
   but not sufficient; the guard asserts the documented set, the rendered row
   set, and the reflected field set are **equal** — no missing field, no extra
   doc, no duplicated row.
3. **Validate the output, not the intent.** Tests parse `recordsView()`'s text
   back into rows and compare that row set to `reflect` independently. A
   rendering bug that skips a field therefore fails, even when tagging is
   perfect.
4. **Reconstruct, don't restate.** The size check sums field slots with their
   alignment and asserts it equals `reflect.Type.Size()`, so a misuse of the
   reflect API surfaces as a mismatch rather than a plausible-looking number.
5. **Tie behavioral prose to behavior tests.** Where a `doc` tag makes a
   *checkable* claim (e.g. `CountLinks` "repeat hardlinks get `Size=0`"), a
   scan-level test asserts that behavior, so the sentence is guarded rather
   than merely written. Unverifiable prose is the stated residual.
6. **Independent expectation.** Where feasible, the expected value is
   recomputed from a second source (constants from `os`/`syscall`, counts from
   `len`, sizes from `reflect`) rather than copied from the view.

## Gates (obligatory checks)

The guards above are only as good as the mechanism that runs them. Three tiers,
weakest → strongest; only tiers 2 and 3 are actual obligations. The repo today
has **no** Makefile, CI workflow, or hooks (only `go.mod`/`go.sum`), so this is
net-new infrastructure. The `Makefile`/CI location is repo-wide and may be
captured in a separate infrastructure plan; this section specifies what the
target must contain for the debug schema and why.

**Tier 1 — local convenience (feedback, not a gate).**
- `make check-fast`: `gofmt -l .` (fail if output non-empty) + `go vet ./...` +
  the focused tests (`go test ./internal/ui/... -run 'Record|Enum|Flag|Table'`).
- Optionally wired as a pre-commit/pre-push hook (`.pre-commit-config.yaml` or
  `lefthook.yml`).
- **Honest:** hooks are bypassed with `--no-verify`; treat as fast feedback only.

**Tier 2 — CI required check (the mandate).**
- `make check`: `go build ./...` + `go vet ./...` + `gofmt -l .` (fail on any
  output) + `go test -count=1 -race ./...`, all with asserted exit codes — no
  `|| true`, no `continue-on-error` on the check step.
- `.github/workflows/ci.yml` runs `scripts/check.ps1` on `windows-latest`. The
  binary imports `golang.org/x/sys/windows` without a build-tag split, so a
  Linux runner would fail to build (this was caught in review).
- Windows has no `make` (verified on this machine), so `scripts/check.ps1`
  (`-Fast` for tier 1) is the executed gate and the Makefile mirrors it for
  Unix/CI parity. Both run whole packages for the fast tier rather than a test
  name regex, which could silently exclude a guard.
- The default branch marks that workflow **required** (branch protection). That
  setting lives in GitHub, not in the repo, so it is recorded as a checklist item
  here and in `README.md` — it cannot be version-controlled from this plan.

**Tier 3 — structural (makes omission impossible, not merely detected).**
- `TestDocCoverage` (dependency-free): walk the documented packages with
  `go/parser` and assert **every declared struct** in `internal/tree`,
  `internal/scan`, `internal/config` is either registered in the `records` driver
  list or listed in an explicit `allowlist` map with a reason. Adding a struct
  without a decision fails CI.
- `TestEnumSliceMatchesConstBlock` and `TestRecordsCoverKnownStructs` (already
  specified) make field/enum omission obligatory.
- Optional codegen path: `go generate ./...` + `git diff --exit-code` for
  generated enum tables — stronger, but adds a `golang.org/x/tools` dependency;
  **deferred** (dependency-free AST tests are the chosen v1).

**Tier 4 — gate the gates.**
- `TestGuardsPresent`: asserts each named guard test exists (via a small registry
  in the test file) and that the driver list and enum slices are non-empty, so
  deleting a guard is itself a failure.
- Prove the CI target itself: break one field tag, run `make check` from the repo
  root, confirm it exits non-zero, then revert — a target that passes a broken
  tree is a false gate.

**Residual (stated):**
- Branch protection is a repository setting a human must enable; no file in this
  repo can force it.
- Hooks are bypassable; only CI is authoritative.
- A required check that does not actually fail (allow-failure, wrong test glob,
  wrong working directory) is a false gate; the tamper step above is what proves
  the target, not the presence of the workflow file.

## Concrete integration

**Struct tags** (inert, no runtime size cost) added co-located:
- `internal/tree/node.go`: `doc:"..."` (and `file:`/`dir:` role tags on the
  mode-dependent fields) on each `Node` field.
- `internal/scan/walker.go`: `doc:"..."` on each `Options` field.
- `internal/config/config.go`: `doc:"..."` on each `Config` field.

**`internal/ui/recinfo.go` (new)**
```go
var records = []struct {
    Name string
    Type reflect.Type // tree.Node{}, scan.Options{}, config.Config{}
    Modes, bool        // show file/dir role columns
}{ /* ... */ }

func recordsView() string // datatypes + all records, reflect + tags
func flagsView() string   // bool fields (reflect) + computed bit values
func tablesView() string  // len()+samples via scan.TypeTableStats(), ui slices
func enumsView() string   // canonical enum slices + counts/estimate
```
- Reflection walks `reflect.TypeOf(rec.Type)` and reads `Tag.Get("doc")`.
- Bit values printed via `fmt.Sprintf("%#08x", uint32(os.ModeSymlink))` and
  `math/bits.TrailingZeros32` — computed, not literal.
- Enum slices: `scanStatusValues`, `tabKindValues` beside their const blocks;
  `filterModeValues` exported from `internal/filter`.

**`internal/scan`**: add `TypeTableStats() (count int, sample []string)` (reads
`fileTypes`) so the table section reports a live count without exporting the map.

**`internal/ui/tab.go`**: `debugView` appends `recordsView()`, `flagsView()`,
`tablesView()`, `enumsView()`; add `D` to footer/man-page if absent.

**`internal/tree`, scan behavior, `internal/export`**: unchanged apart from tags.

## Scope boundaries

**In scope:**
- Tags on `tree.Node`, `scan.Options`, `config.Config`
- `internal/ui/recinfo.go` (new): record driver list, reflect/tag helpers, the
  four view funcs, enum slices
- `internal/scan`: `TypeTableStats()` accessor
- `internal/filter`: export `filterModeValues`
- `internal/ui/recinfo_test.go` (new) + the guard tests, plus `TestDocCoverage`
  and `TestGuardsPresent`
- `Makefile` (`check`, `check-fast`) and `.github/workflows/ci.yml` (net-new; the
  required-check/branch-protection step is a documented manual setting)
- `README.md` / `docs/ncdu-next.1`: Debug tab is the model reference, and the
  branch-protection requirement

**Out of scope (deferred):**
- `go:generate`/`stringer` enum codegen (AST test is sufficient for v1; adopt if
  enums proliferate) → v2
- Live heap/pprof, exact string/slice payload bytes, `--debug-dump`, per-OS
  `Stat_t` layouts, `export.Entry` side-by-side → v2

## Failure modes

- New field without a `doc` tag → `TestRecordFieldTags` fails and
  `TestRenderedFieldsEqualReflected` fails; UI shows `(undocumented)` (loud in CI
  and at runtime)
- A field tagged but not rendered (rendering bug) → `TestRenderedFieldsEqualReflected`
  fails on the row-set mismatch, so tagging alone cannot mask it
- New enum constant not in its slice, or a slice entry not in the const block →
  `TestEnumSliceMatchesConstBlock` fails in the corresponding direction
- 32-bit build → sizes/offsets come from `reflect` at runtime, so the table is
  right, not wrong-but-plausible (`TestSizesReconstruct` recomputes from the API)
- Windows → `Dev`/`Ino` rows read `n/a on windows`, never a misleading `0`;
  asserted by an injected-platform test, not only a manual check
- Emptied `fileTypes` → `tablesView` shows `0` (runtime `len`), and
  `TypeTableStats` is covered by a test
- **Residual, stated:** a `doc` sentence can become semantically outdated while
  still present. This is narrowed — checkable claims are pinned by
  `TestFlagClaimsHold` — but prose that encodes no testable behavior (e.g.
  wording of `Root` normalization) remains unguarded, and no test can catch it.
  That residue is the honest limit of "fully validate any change" here.

## Rollback

Additive: one new file + one accessor + inert tags + four view calls. Removing
`recinfo.go`, the accessor, and the `debugView` calls restores the current Debug
tab and leaves tags harmless. No behavior change.

## Validation plan

Gate (all must be green before merge):
1. `go build ./...`, `go vet ./...`, `gofmt -l .` clean
2. `go test -count=1 -race ./...` all pass

Correctness — structure is fully and precisely reflected (set equality):
3. `TestRenderedFieldsEqualReflected`: parse `recordsView()` output into rows and
   assert, per record, `rendered == reflect` field set **exactly** (no missing,
   no extra, no duplicate); repeat for all four records
4. `TestRecordFieldTags`: every field's `doc` tag non-empty; `TestRecordsNoStaleTags`
   asserts the tag set has no entry for a nonexistent field (tags on live fields
   cannot be stale, this guards a separate doc list if one is introduced)
5. `TestRecordsCoverKnownStructs`: the driver list names every struct we intend
   to document (fails when a record is added to the codebase but not the list)
6. `TestSizesReconstruct`: sum of field slots with alignment equals
   `reflect.Type.Size()`; offsets strictly monotonic and equal
   `Field(i).Offset`; padding after `IsDir` is present (8-byte alignment)

Completeness — nothing silently omitted:
7. `TestBoolFieldsListed`: every `bool` field of every record appears in
   `flagsView()` (set-equality both directions)
8. `TestEnumSliceMatchesConstBlock`: AST-parse each enum's const block; the
   slice equals the constant set exactly — no constant unlisted, no extra entry
9. `TestEnumMeaningComplete`: every enum value has a non-empty meaning string
10. `TestBitValuesComputed`: `flagsView()` values equal `uint32(os.ModeSymlink)`
    / `uint32(os.ModeDir)` (independent source), and no `1<<n` literal exists in
    `recinfo.go` for these bits
11. `TestTablesViewLiveCount`: `tablesView()` count equals `len(fileTypes)` via
    `TypeTableStats()`, with non-empty sample entries

Behavioral prose is guarded:
12. `TestFlagClaimsHold`: for each flag whose `doc` states a checkable effect
    (at least `CountLinks`→repeat hardlink `Size=0`, `FollowSym`→symlink stub
    `Size=0`, `OneFS`→cross-device dir skipped, `DetectFormat`→
    `Encoding`/`LineEnding` populated), run the matching scan test and assert
    the claim. Unverifiable prose (e.g. `Root`'s normalization wording) is listed
    in the plan as *not* guarded.

Platform:
13. `TestPlatformLabel`: the rendered label contains `runtime.GOOS`/`GOARCH`
14. `TestWindowsNotApplicable`: with an injected platform value (no build tags),
    the `Dev`/`Ino` rows render `n/a on windows`; on the current host the rows
    render real computed values

Tamper proof (each guard must fail for the *right* reason — do, verify, revert):
15. Add a `tree.Node` field with no tag → `TestRecordFieldTags` exits non-zero
    and names the field; read the mutated struct back to confirm the field
    landed where intended. (`TestRenderedFieldsEqualReflected` is *not* expected
    to fail for a missing tag — the field still renders, as `(undocumented)`;
    that guard instead fails when a field is dropped from or added to the
    rendered rows, e.g. a rendering bug.)
16. Delete one entry from `scanStatusValues` → `TestEnumSliceMatchesConstBlock`
    exits non-zero
17. Add a `fileTypes` entry → `TestTablesViewLiveCount` still passes (proves the
    count is derived, not pinned)
18. Introduce a literal `1<<27` in `recinfo.go` → `TestBitValuesComputed`'s
    no-literal assertion exits non-zero

UI:
19. `TestDebugTabRendersRecords`: `D` shows all eight section headers
    (`datatypes`, `file record`, `dir record`, `options record`,
    `config record`, `flags`, `tables`, `enums`)
20. Manual: `ncdu-next --debug`, press `D`, confirm sections, computed bit
    values, the `n/a on windows` note, and the live table count

## Ordered task list

1. Add `doc`/`file`/`dir` tags to `tree.Node` (`internal/tree/node.go`),
   `scan.Options` (`internal/scan/walker.go`), `config.Config`
   (`internal/config/config.go`)
2. Add `TypeTableStats()` to `internal/scan`; export `filterModeValues` from
   `internal/filter`
3. Create `internal/ui/recinfo.go`: record driver list, reflect/tag helpers, the
   bit/flag docs, `recordsView`, `flagsView`, `tablesView`, `enumsView`, enum
   slices
4. Call the four view funcs from `debugView` in `internal/ui/tab.go`
5. Add `internal/ui/recinfo_test.go` covering validation points 3–14 (structure
   set-equality, size reconstruction, completeness, behavioral claims, platform),
   plus `TestDocCoverage` (Tier 3) and `TestGuardsPresent` (Tier 4)
6. Run the tamper checks 15–18 by hand: mutate, confirm the named guard exits
   non-zero, read the mutated value back to confirm it landed in the guarded
   quantity, then revert
7. Add `Makefile` with `check` (Tier 2) and `check-fast` (Tier 1); add
   `.github/workflows/ci.yml` running `make check`
8. Prove the gate: break one field tag, run `make check` from the repo root,
   confirm non-zero exit, revert
9. Update `README.md` and `docs/ncdu-next.1` (model reference + branch-protection
   checklist), and record the required-status-check setting
10. Run `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test -count=1 -race ./...`
11. Manual smoke test per validation point 20

## References

- Existing Debug tab: `internal/ui/tab.go` (`debugView`, `bytesHuman`, `formatUint`)
- Records: `internal/tree/node.go` (`Node`), `internal/scan/walker.go`
  (`Options`), `internal/config/config.go` (`Config`)
- Bits tested by the walker: `fi.Mode()&os.ModeSymlink`, `fi.IsDir()`
  (`internal/scan/walker.go`); `fileKey`/`deviceOf`, `Stat_t.Dev`/`Ino`
  (`internal/scan/walker_unix.go`, `stat_unix.go`, `walker_windows.go`)
- Values verified this session: `os.ModeDir = 1<<31 (0x80000000)`,
  `os.ModeSymlink = 1<<27 (0x08000000)` (measured via a `go run` scratch program)
- Tables: `internal/scan/walker.go` (`fileTypes`), `internal/ui/tab.go`
  (`typeStyles`, `typePrefixes`)
- Enums: `internal/ui/tab.go` (`ScanStatus`, `tabKind`)
- Search/filter plan (for `filter.Mode`): `.kilo/plans/1792781827212-search-filter-plan.md`
- Plan format reference: `.kilo/plans/1792710217521-vm-scan-plan.md`
