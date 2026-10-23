# Feature Analysis Report

## Implemented (v1 baseline)
- Parallel scan with bounded workers; tested.
- Tree model, recompute, sort; tested.
- bubbletea v2 UI navigation; tested.
- JSON export validated against FS sums.

## v2 candidates
- Symlink loop detection (safety, high).
- Hardlink dedup (correctness, medium).
- Exclude patterns (usability, low risk).
- Cross-filesystem (correctness, medium).
- Permission errors (robustness, high).
- Deletion mutation + recompute (core parity, high risk).
- JSON import compat (medium).
- File type detection/filter/sort (nice-to-have).
- Cache/graphs/remote (advanced, defer).

## Prioritization
1. Safety features first.
2. Compat next.
3. Analytics last.

## Testing strategy
- Unit walker invariants and tree sums.
- Integration golden export.
- Race scan+UI.
- Manual TUI smoke.

## Metrics
- Coverage targets: scan 90%, tree 95%, export 95%, ui 85%.
- Performance: scan <5s for 100k files.

## Risks
- Windows symlink privileges.
- Large tree memory.
- UI blocking on huge dirs.
## Memory savings

### Current cost model (go 1.26, 64-bit)

Every `tree.Node` stores five strings. A Go string = 16-byte header (ptr+len, inline in the struct) + a separate heap allocation for bytes, rounded up to the allocator's size class. Empty strings (`""`) cost 16 bytes of header with zero data. No small-string optimization; no interning.

`tree.Node` fixed layout ≈ **152 bytes**, of which **80 bytes are just the five string headers** (`Name`, `Path`, `Type`, `Encoding`, `LineEnding`). `Encoding` and `LineEnding` are always `""` in v1 — 32 bytes per node for no information.

### Path duplication dominates (deferred to v1.5 — see plan)

The walker (`internal/scan/walker.go`) stores the **full absolute path** on every node:

```go
p := filepath.Join(dir, name)   // dir = parent's full Path
child := &tree.Node{Name: name, Path: p, ...}
```

The root prefix is copied verbatim into every descendant. There is no interning and no path reconstruction from `Parent`. On a ~700k-node C: tree with ~160-byte average paths, path strings alone are ~112 MB of the ~263 MB live heap. Path representation needs further discussion; do not implement yet. See **v1.5 path task** in the plan.

### Packable fields — do in v1 (enums / bit flags / compact codes)

Go has no `enum` keyword or struct bitfields, but typed integer constants work as enums:

```go
type nodeType uint8
const (
    typeUnknown nodeType = iota
    typeSourceGo
    typeImageJpeg
    typeVboxDisk
    // ...
)

type nodeFlags uint8
const (
    flagIsDir nodeFlags = 1 << iota
    flagHasErr
    flagSymlink
)
```

Each enum is **1 byte**; multiple flags share one byte via `&`, `|`, `^`, `&^`. Strings to pack in v1:

| Field | Current | Packed alternative | Size |
|---|---|---|---|
| `Type` | `string` (header + heap alloc) | `nodeType uint8` | 1 B |
| `Encoding` | `string` (always `""`) | `uint8` code | 1 B |
| `LineEnding` | `string` (always `""`) | `uint8` code | 1 B |
| `IsDir` | `bool` (1 B, padded) | bit in `flags uint8` | shared |
| `Err` | `error` interface (16 B, usually nil) | `*os.PathError` or error-code `uint8` | 0–8 B |

`fileTypes` (`internal/scan/walker.go`) has ~110 entries, fits in `uint8`. UI string enums `ScanStatus` and `tabKind` (`internal/ui/tab.go`) are also packable as `uint8`.

Dedup maps in `internal/scan/walker.go` use `joinKey(dev, ino)` strings as `map[string]struct{}` keys. On Unix this allocates one string per hardlink/symlink entry; a packed `uint64` key `(dev << 32) | ino` removes the string. Windows returns `""` already (`walker_windows.go`).

### Estimated savings (700k-node C: tree, packing only)

| Change | RAM saved | GC objects removed |
|---|---|---|
| `Type` string → `nodeType uint8` | ~10–15 MB | ~700k |
| `Encoding` + `LineEnding` packed/dropped | ~22 MB | ~1.4M |
| `Err` interface → packed code/pointer | ~5–6 MB | up to ~700k |
| `uint64` dedup keys (Unix) | varies | varies |
| **Subtotal** | **~35–45 MB** | **several million** |

Packing reduces GC work directly: each string is a separate heap object with a pointer the GC must scan. Removing ~2M string objects means shorter mark phases, fewer `Mallocs`, lower `TotalAlloc`, and better cache locality (codes sit inline next to `Size`/`AppSize`).

### Trade-offs

- Conversion: map codes back to display strings in UI/debug (`nodeType.String()` via lookup table) — trivial O(1), but new code to maintain.
- Alignment: order struct fields by size (largest first) or cluster small fields to avoid padding gaps.
- Readability: integer codes need a lookup table in debug/JSON export; add explicit mapping layers.
