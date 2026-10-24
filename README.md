# ncdu-next

Modern disk-usage analyzer (ncdu clone) in Go with a bubbletea TUI, parallel
bounded-worker scanning, hardlink/symlink/cross-filesystem policies, exclusion
patterns, JSON export/import, and optional OS trash.

## User guide — pick your category

### 1. Casual end user ("just show me what's big")
Run on the current directory:

```
ncdu-next
```

Or on another path:

```
ncdu-next /some/dir
```

TUI keys:
- `↑/↓` or `k/j` — select entry
- `Enter`/`l` — open directory
- `Backspace`/`h` — go up one level
- `e` / `w` — toggle the wide view (simple `name size` ↔ `name size kind attrs type encoding lines mtime`)
- `u` — cycle size units: bytes → IEC binary (KiB/MiB/…) → SI decimal (kB/MB/…)
- `/` — live filter the current directory (Tab toggles deep/recursive scope)
- `f` — toggle deep (recursive) scope on the committed filter
- `g` — full-tree search → flat results list (Enter jumps, Esc closes)
- `Esc` — clear filter / close search
- `q` / `Ctrl-C` — quit

Use `Enter` to descend into directories and watch the highlighted bar move. Size is
shown in bytes by default (press `u` to cycle units); sort is by size descending on
scan completion. Rows are colored by broad category (kind), derived from the file's
extension.

### 1a. Typed categorization, timestamps and views
Every scanned entry carries a specific `type` id (the extension, e.g. `vdi`,
`png`, `go`) and a broad `kind` (e.g. `disk-image`, `image`, `source`). The wide
view (`e`/`w`) adds the kind, Windows file attributes, type, encoding/line-ending,
and modification time (`YYYY-MM-DD HH:MM`, UTC). On a terminal narrower than about
120 columns each wide record spills across two lines instead of being cut off.

Timestamps are read from the filesystem at scan time: `mtime` on every platform,
plus `ctime`/`atime` where the OS exposes them (Linux), and `FILE_ATTRIBUTE_*`
on Windows. All are read-only metadata; ncdu-next never changes them.

### 1b. Filesystem token and the Debug tab
The navigation header shows the filesystem of the directory currently being
viewed, for example `[NTFS]` or `[ext4]`. `[fs:?]` means the type could not be
read; the token is omitted when there is nothing to report (for example when the
scan root is a regular file) or before a scan has completed. The full state,
including the reason a type is unknown, lives in the Debug tab.

The Debug tab (`D`, requires `--debug`) has two zones:

- **tab** — every tab, with the *inspected* tab marked, that tab's detail
  (status, root, counts, filter/search state), and its `filesystem` section
  (root mount, mount table, reason when unknown). `D` inspects the tab you were
  viewing; pressing `D` again does not change it.
- **program** — runtime counters, cross-tab totals, and the record reference
  (datatypes, records, flags, tables, enums).

The page is longer than a screen, so it scrolls:

- `Up`/`k`, `Down`/`j` — scroll one line
- `PgUp`/`PgDn` — scroll one page
- `Home`/`End` — first/last line
- `[` / `]` — previous/next section
- `E` — write a debuginfo bundle (below) into the working directory
- `q`, `1-8`, `Ctrl+Tab`, `x`, `D` — the usual tab keys; other navigation keys
  (`/`, `g`, `s`, …) are ignored while the Debug tab is focused.

### 1c. Debuginfo bundle
`--debuginfo FILE` writes one zip for a bug report and exits (it can be combined
with `--export`, and does not need `--debug`). It contains `README.txt`,
`manifest.json` (build metadata, platform, effective options, entry list),
`debug-tab.txt` (the whole Debug page), `tabs.jsonl`, `fs.jsonl`, and
`tree-summary.json` (per tab: counts plus the top 20 entries). The full file list
is included only with `--debuginfo-tree` (`tree.jsonl`). Pressing `E` on the
Debug tab writes the same bounded bundle without the tree.

The bundle contains absolute paths; treat it as sensitive. `--export` remains the
data path for tooling.

### 2. Power user ("control what is scanned")
Common flags:

| Flag | Meaning |
|------|---------|
| `--path DIR` | root directory to scan (default `.`) |
| `--workers N` | parallel stat/readdir workers (default 4) |
| `--exclude PATTERN` | skip names matching glob pattern (repeatable) |
| `--one-file-system` | do not cross mount points (Linux, macOS and Windows) |
| `--count-hardlinks` | count every hardlink's size separately (default counts once) |
| `--follow-symlinks` | follow symlinks (cycle guarded) |
| `--trash` | move deleted files to OS trash instead of permanent delete |
| `--export FILE` | write ncdu 2.x JSON and exit (headless) |
| `--debuginfo FILE` | write a debuginfo zip bundle and exit (headless) |
| `--debuginfo-tree` | include the full file list in the debuginfo bundle |
| `--config FILE` | TOML config file (flags override) |
| `-v` / `--verbose` | debug logging to stderr |

Example:

```
ncdu-next --path ~/logs --exclude '*.tmp' --one-file-system --export report.json
```

Config file (TOML) at `~/.config/ncdu-next/config.toml` (or OS equivalent):

```toml
workers = 8
exclude = ["*.tmp", "node_modules"]
one_file_system = true
trash = true
log_file = "/var/log/ncdu-next.log"
```

### 3. Search and filtering (any user)
Filtering is view-level and never changes the scan: `--exclude` decides what is
scanned, filtering decides what is shown. A query is one line of space-separated
clauses combined with AND; a leading `!` negates the whole query.

| Clause | Matches |
|--------|---------|
| `text` | case-insensitive substring on the name |
| `*.go` | glob (any term with `*`, `?`, or `[`) |
| `re:^a.*z$` | regular expression on the name |
| `type:image` | broad kind prefix (`image`, `office`, `disk`, `source`, …) |
| `type:vdi` | specific type id prefix (the extension id, e.g. `vdi`, `png`, `go`) |
| `type:.go` | file extension |
| `size:>10M`, `size:<=500K`, `size:1K..2K` | size threshold (K/M/G/T, 1024-based) |
| `case:` | make matching case-sensitive |

- `/` filters the current directory live; `Tab` inside the prompt toggles deep
  (recursive) scope, which hides non-matching rows but keeps their ancestors
  navigable. A matching directory shows its whole subtree.
- `g` searches the whole tree and lists matching paths; `Enter` jumps to one.
- `Esc` clears the filter; `f` toggles deep scope on a committed filter.
- `--export` always writes the full tree; the view filter does not affect it.

### 4. Operator / CI ("script it, log it, headless")
- `--export FILE` produces a machine-readable ncdu 2.x JSON tree; exit code 0 on
  success, non-zero on scan/export failure.
- `--log-file FILE` appends slog text logs alongside stderr.
- `--config` keeps CI flags out of the command line; flags still override config.
- Ctrl-C aborts mid-scan leaving a usable partial tree (pause/resume not yet
  implemented).
- JSON import (`internal/export.Read`) round-trips export output for audits.

### 5. Developer / contributor ("build, test, extend")
Requires Go 1.23+ (module targets go 1.26).

```
go build ./...          # compile
go vet ./...            # static checks
go test -race ./...     # race-safe tests
gofmt -w .              # format
```

Obligatory checks (the gate CI runs):
- `make check` (Unix/CI) — build + vet + gofmt + `go test -race ./...`.
- `powershell -ExecutionPolicy Bypass -File scripts/check.ps1` (Windows; `-Fast`
  for the focused subset).
- Enable branch protection so the `ci` workflow is a **required** status check.

Module: `github.com/1ttyb1tty/ncdu-next`. Packages:
- `internal/scan` — parallel walker, hardlink dedup, symlink/cross-fs policies,
  filesystem/mount detection
- `internal/tree` — node model (kind enum, timestamps, Windows attrs), sort, delete
  + ancestor recompute
- `internal/filter` — search/filter query engine (`/`, `f`, `g`)
- `internal/export` — ncdu 2.x JSON import/export
- `internal/debuginfo` — the debuginfo zip bundle writer
- `internal/ui` — bubbletea TUI model; the Debug tab (`D`) documents the record
  layout, sizes, flags, bits, and enums, derived from `reflect` + field doc tags
- `internal/config` — flags + TOML config
- `internal/logging` — slog setup

## License

See repository. ncdu is a separate project; ncdu-next is a reimplementation.
