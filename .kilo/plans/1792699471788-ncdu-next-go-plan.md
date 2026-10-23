# ncdu-next: modernized disk-usage analyzer (Go)

## Goal
Reimplement ncdu as a modern Go TUI application: feature parity first, Windows-supported, parallel metadata scanning via bounded worker pool, bubbletea v2 UI.

## Decisions (resolved)
- Language: Go; module `github.com/1ttyb1tty/ncdu-next`; Go 1.23+.
- TUI: bubbletea v2 (Elm/MVU) + lipgloss v2; classic ncdu keymap default.
- Concurrency: bounded goroutine pool (errgroup SetLimit, ~handful workers) for `stat`/`readdir`; no async FS I/O.
- Windows: first-class (Go cross-compile + bubbletea).
- Architecture: single binary (no client/server).
- Scope v1: ncdu parity (scan, browse, sort, delete, exclude, hardlinks, cross-fs, JSON import/export, shell-out). Modern extras deferred to v2.
- v2 extras: fuzzy search, dedup, persistent cache, graphs, remote scan, self-resource monitoring (RSS/CPU/goroutines/throughput).
- JSON format: ncdu 2.x for import/export.
- Symlinks: do not follow by default; `--follow-symlinks` opts in with cycle guard.
- Cross-filesystem: cross by default; `--one-file-system` restricts.
- Hardlinks: count size once per (dev,ino) unless `--count-hardlinks`.
- Multiple roots: each in its own tab (tabbed interface).
- Deletion: configurable permanent vs OS trash (flag/config), with confirm prompt.
- Config: flags + optional TOML config file at standard OS location; flags override.
- Default sort: size descending on scan completion.
- Progress/cancellation: live file count + current path; Ctrl-C aborts with partial tree; pause/resume key; background scan with UI always responsive + cancel key.
- Memory v1: small fields packed into typed uint8 enums + bit flags (Go has no native enum/bitfield; use typed constants). Paths remain full strings in v1.
- Path representation: deferred to v1.5; requires discussion before implementation.
- Logging: slog text, flexible destinations (stderr and/or file), warn default, `-v` debug.

## Affected boundaries / modules
- `cmd/ncdu-next/` entrypoint
- `internal/scan/` — parallel walker, node tree, size aggregation, hardlink handling
- `internal/tree/` — node model, sort/filter, deletion, tabbed root set
- `internal/ui/` — bubbletea model/update/view, keymap, help, tabs
- `internal/export/` — ncdu 2.x JSON import/export
- `internal/config/` — flags, excludes, TOML config

## Ordered tasks
1. Scaffold module: `go.mod` (module path, Go 1.23), directory layout, Makefile/justfile, golangci-lint config.
2. Define node model in `internal/tree/` (path, size, count, mtime, dev/inode, children, parent, sort state).
3. Implement scanner in `internal/scan/`: bounded worker pool, directory walk, hardlink dedup, symlink policy, cross-fs policy, permission-error handling, progress channel, cancellation.
4. Wire progress/cancellation into scan API (context-based cancel + pause).
5. Implement deletion in `internal/tree/` (permanent + trash, confirm, ancestor recompute).
6. Implement export in `internal/export/` (ncdu 2.x JSON, golden tests).
7. Build TUI in `internal/ui/`: bubbletea model, navigation, sort keys, delete, help, tabs for multiple roots, progress view.
8. Implement config loader (flags + TOML) in `internal/config/`.
9. Add logging (slog) with `-v` and destination flags.
10. Tests: walker fixtures (symlinks, hardlinks, permission-denied), export golden, race detector, TUI smoke.
11. Lint/typecheck: `go vet`, `gofmt`, `golangci-lint`; manual TUI smoke on Windows + Linux.
12. Memory: pack small fields into typed enums / bit flags — **v1**
    - `tree.Node.Type` → `nodeType uint8` with `String()` reverse lookup
    - `tree.Node.Encoding` → `uint8` code (v1 stub returns "")
    - `tree.Node.LineEnding` → `uint8` code (v1 stub returns "")
    - `tree.Node.IsDir` → bit in `flags uint8`
    - `tree.Node.Err` → `*os.PathError` or error-code uint8
    - UI enums `ScanStatus`, `tabKind` → `uint8`
    - `scan` dedup maps: `uint64` packed key instead of `joinKey` string (Unix)
    - JSON export/import mapping layers for packed codes
13. **v1.5 (high priority, needs discussion — do not implement yet)** Path representation
    - Current: every node stores full absolute `Path`; root prefix duplicated in every descendant.
    - Options to discuss: store `Name` only + rebuild via `Parent`; intern path segments; store root + relative segments.
    - Impact: path strings are ~100+ MB on a full C: tree; this is the dominant memory cost.
    - Decision gate: confirm rebuild-via-`Parent` is fast enough for deep trees, or whether interning is needed.

## Data flow
1. CLI flags/config → root(s) + options.
2. Walker dispatches directories to bounded pool; each worker returns child entries; main goroutine merges into tree (single writer, deterministic ordering).
3. Progress stream to UI; scan runs in background, UI always responsive.
4. On completion, UI model receives root; user navigates/sorts/deletes across tabs.
5. Delete mutates tree in place and recomputes ancestors.
6. Export serializes tree to ncdu 2.x JSON.

## Failure modes
- Permission errors: skip + record in status, don't abort scan.
- Symlink cycles: visited (dev,inode) set; skip.
- Hardlinks: count size once per (dev,ino) unless `--count-hardlinks`.
- Very large trees: stream progress; avoid holding full path strings (store relative + root).
- Deletion failures: report per-item, keep tree consistent.
- Pause/cancel mid-scan: partial tree remains usable.

## Validation
- Unit tests for walker (fixtures: nested dirs, symlinks, hardlinks, permission-denied).
- Golden tests for JSON export vs ncdu 2.x output.
- Race detector (`go test -race`) on scan + UI integration.
- Manual TUI smoke on Windows + Linux terminals.
- `go vet`, `gofmt`, `golangci-lint`.

## Rollout / migration
- n/a: new project, no existing installs or data to migrate.

## Open questions (non-blocking)
- Exact worker count default (suggest CPU count, capped).
- TOML library choice (BurntSushi/toml vs pelletier/go-toml).
- Trash library choice per OS.
- v1.5 path task: relative-only storage + `Parent` rebuild vs path interning — performance vs memory trade-off unresolved.