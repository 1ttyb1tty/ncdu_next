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
- `q` / `Ctrl-C` — quit

Use `Enter` to descend into directories and watch the highlighted bar move. Size
is shown in bytes; sort is by size descending on scan completion.

### 2. Power user ("control what is scanned")
Common flags:

| Flag | Meaning |
|------|---------|
| `--path DIR` | root directory to scan (default `.`) |
| `--workers N` | parallel stat/readdir workers (default 4) |
| `--exclude PATTERN` | skip names matching glob pattern (repeatable) |
| `--one-file-system` | do not cross mount points |
| `--count-hardlinks` | count every hardlink's size separately (default counts once) |
| `--follow-symlinks` | follow symlinks (cycle guarded) |
| `--trash` | move deleted files to OS trash instead of permanent delete |
| `--export FILE` | write ncdu 2.x JSON and exit (headless) |
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

### 3. Operator / CI ("script it, log it, headless")
- `--export FILE` produces a machine-readable ncdu 2.x JSON tree; exit code 0 on
  success, non-zero on scan/export failure.
- `--log-file FILE` appends slog text logs alongside stderr.
- `--config` keeps CI flags out of the command line; flags still override config.
- Ctrl-C aborts mid-scan leaving a usable partial tree (pause/resume not yet
  implemented).
- JSON import (`internal/export.Read`) round-trips export output for audits.

### 4. Developer / contributor ("build, test, extend")
Requires Go 1.23+.

```
go build ./...          # compile
go vet ./...            # static checks
go test -race ./...     # race-safe tests
gofmt -w .              # format
```

Module: `github.com/1ttyb1tty/ncdu-next`. Packages:
- `internal/scan` — parallel walker, hardlink dedup, symlink/cross-fs policies
- `internal/tree` — node model, sort, delete + ancestor recompute
- `internal/export` — ncdu 2.x JSON import/export
- `internal/ui` — bubbletea TUI model
- `internal/config` — flags + TOML config
- `internal/logging` — slog setup

## License

See repository. ncdu is a separate project; ncdu-next is a reimplementation.
