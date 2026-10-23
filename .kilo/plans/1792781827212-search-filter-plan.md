# Plan: search and filtering

**Goal:** Add interactive search and filtering to the ncdu-next TUI over an
already-scanned tree. Three user-facing capabilities share one query engine:
(1) a **live filter** of the current directory's rows as you type, (2) a
**recursive filter / tree pruning** that hides non-matching rows while keeping
ancestors navigable, and (3) a **full-tree search** that lists matching paths as
a flat, jumpable result list. A query can match on name (substring), name
(glob/regex), type/extension/category, and size threshold — combined in one
input line.

This is view-level filtering of the *post-scan* tree. It does not change what is
scanned (that remains `scan.Options.Excludes` / `--exclude`, applied during the
walk). The two coexist.

## Decisions (resolved)

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Query engine location | New `internal/filter` package | One predicate reused by all three capabilities; `ui` imports `filter`, `filter` imports `tree` (no cycle) |
| Match targets | name substring, name glob/regex, type/extension/category, size threshold | User-selected; one line, space-separated clauses |
| Query syntax | Default = case-insensitive substring on `Name`. `*`/`?`/`[` present → glob (`filepath.Match`). `re:` prefix → regex. `type:<cat>` / `type:.ext` clause. `size:>10M`, `size:<=500K`, `size:10M..1G` clause. `!` prefix negates a clause. Free text (no prefix) = substring terms, all must match (AND). | Single-input, no modal grammar editor; reuses existing `humanSize` units |
| Filter visual semantics | Hide rows; ancestors of a match stay visible | User-selected |
| Match-inside-directory rule | If a **directory itself** matches, its **entire subtree** is shown; otherwise only matches and their ancestors are shown | Prevents navigating into a matched dir and seeing an empty list (dead-end). **Revisit in v2** — see Revisit section |
| Live filter scope | Current directory only (local), re-evaluated per frame | O(children) per frame, no tree walk while typing |
| Recursive filter scope | Whole tree; visibility precomputed once per commit into `visible map[*tree.Node]bool` | O(n) once per commit, not per keystroke; `children()` then O(1) lookups |
| Applying pruning | Non-destructive: keep `filter` state, filter in `Tab.children()`; clear filter to restore | Mutating the tree would break `Recompute`, `Delete`, and undo |
| Sizes under pruning | Keep true `Node.Size`; show a filter badge + shown/total match count in the header. Do **not** silently recompute sizes to the filtered subset | A silently changed byte count breaks the "size is bytes on disk" contract; the renderer has no `%` column today anyway |
| Filter persistence | Per tab (each `Tab` owns its query state); survives tab switches; cleared by `Esc` in nav mode or `Esc` in the prompt | Matches per-tab isolation of scans/navigation |
| Full-tree search presentation | A mode inside the nav tab (`searchActive`): flat `path  size` list, own cursor/scroll; `Enter` jumps, `Esc` closes | Reuses the existing predicate and row renderer; no new tab kind |
| Jump target from search results | Directory → set `cur` to it; file → set `cur` to its parent and select the child in the visible list | Lands the user where the match lives |
| Export interplay | `--export` always writes the **full** tree; the view filter never affects export | Export is an audit artifact; filtering is presentation |
| New key bindings | `/` filter prompt (live, local); `Tab` **inside** the prompt toggles deep (recursive); `f` toggles deep on a committed filter; `g` full-tree search results; `Esc` in nav mode clears filter + search | `f`/`g` are unused; `Esc` is currently a no-op outside prompts; `n`/`N` left free for the vm-scan plan. **Revisit in v2** — see Revisit section |
| Prompt state | Keep `m.prompt bool` for the root prompt; add `m.promptKind` (`root`/`filter`/`search`) and reuse `promptInput` | Avoids churning `TestRootPromptHandling`, which reads `m.prompt` |
| Non-interactive CLI filter (`--filter`/`--find` with `--export`) | **Deferred** (v2) | Not selected in scope; natural follow-on once the engine exists |

## Query engine (`internal/filter`)

```go
type Mode int // ModeSubstring, ModeGlob, ModeRegex

type Query struct {
    Raw      string        // original input, for the header badge
    Name     string        // compiled substring terms joined (substring mode)
    re       *regexp.Regexp // when ModeRegex
    glob     string        // when ModeGlob
    Mode     Mode
    TypeCat  string        // "image", "source", ... (Type prefix) or "" 
    TypeExt  string        // ".go", ... (filepath.Ext match) or ""
    Min, Max int64         // inclusive size bounds; Max==0 means unbounded
    Negate   bool          // leading '!'
}

func Parse(input string) (*Query, error) // compiles regex, parses size/type clauses
func (q *Query) Match(n *tree.Node) bool   // node-level predicate
func (q *Query) String() string            // display badge
```

- `Match`: name test AND type test AND size test (each omitted when unset);
  `Negate` inverts the whole predicate.
- Name test: substring (`strings.Contains`, lowercased unless `case:`),
  glob (`filepath.Match`), or regex (`re.MatchString`).
- Type test: `TypeCat != ""` → `strings.HasPrefix(n.Type, TypeCat)`;
  `TypeExt != ""` → `strings.EqualFold(filepath.Ext(n.Name), TypeExt)`.
- Size test: `n.Size` (files) / subtree total (dirs) within `[Min,Max]`.
- Size parsing reuses the K/M/G/T/P/E (1024-based) convention of `humanSize`
  in `internal/ui/tab.go`; a new `filter.ParseSize` mirrors it in reverse.
- Invalid regex → `Parse` returns an error; the UI shows it inline and does not
  apply the query.

### Visibility (recursive scope)

```go
// VisibleSet returns the set of nodes to display for a recursive query:
// every match, every ancestor of a match, and the full subtree of any
// matching directory.
func VisibleSet(root *tree.Node, q *Query) map[*tree.Node]bool
```

Single post-order walk: a node is visible if it matches (and if it is a dir,
its whole subtree is added), or if any child is visible. Result cached on the
tab and rebuilt only when the query or scope changes.

## Concrete integration

**`internal/ui/tab.go`**
- `Tab` gains: `q *filter.Query`, `filterRaw string`, `deep bool`,
  `filterErr string`, `visible map[*tree.Node]bool`, `searchActive bool`,
  `searchResults []*tree.Node`, `searchSel, searchScroll int`.
- `children()` returns `n.Children` filtered by the active query:
  - local scope → `q.Match` on each child;
  - deep scope → membership in `t.visible`.
  When no query is active, returns `n.Children` unchanged (no allocation).
- `selectUp/Down`, `enter`, and scroll math already call `children()`, so they
  pick up filtering for free; add an empty-filtered-list guard so `enter` and
  selection cannot panic.
- `applyFilter(raw string, deep bool)`: `filter.Parse`; on success set `q`,
  `filterRaw`, `deep`, and `visible = filter.VisibleSet(root, q)` when deep;
  on error set `filterErr` and leave the previous query in force.
- `clearFilter()`: reset all filter fields.
- `buildSearch(raw string)`: parse, walk the tree collecting `Match` nodes into
  `searchResults`, set `searchActive`, reset cursor.
- `view()`: when `searchActive`, render the results list instead of the current
  directory; otherwise add a filter badge to the path header
  (e.g. `[filter: *.go — 12 shown / 340]`).
- Footer hints updated with `/ filter`, `f deep`, `g search`, `Esc clear`.

**`internal/ui/model.go`**
- Add `promptKind` (`promptRoot`, `promptFilter`, `promptSearch`) alongside the
  existing `prompt bool`; `r` sets `promptRoot`, `/` sets `promptFilter`.
- In the prompt branch: printable runes append to `promptInput`; `backspace`
  deletes; `enter` commits (`root`→start scan as today, `filter`→
  `applyFilter`, `search`→`buildSearch`); `esc` cancels (and for filter/search,
  clears the active query); `tab` (filter prompt only) toggles `deep` and
  re-applies live.
- Live preview: on each keystroke in the filter prompt, call `applyFilter` on
  the current `promptInput` with the current `deep` (local scope is O(children),
  cheap; deep scope rebuilds the visible set on `Tab`/`Enter`, not per keystroke).
- Nav-mode keys: `f` toggles `deep` and re-applies the committed query; `g`
  opens the search prompt (or, when results are already shown, re-opens);
  `esc` clears filter + search.
- In `searchActive`: `up`/`down` move `searchSel`; `enter` jumps (dir → `cur`;
  file → parent + select); `esc` exits search.

**`internal/tree/node.go`** — unchanged (visitor walks `Children` directly).

**`internal/scan/walker.go`** — unchanged; `--exclude` stays pre-scan.

**`internal/config` + `cmd/ncdu-next/main.go`** — unchanged in v1 (no new flag).

## Scope boundaries

**In scope:**
- `internal/filter/` (new): `Query`, `Parse`, `Match`, `VisibleSet`, `ParseSize`, `String`
- `internal/ui/tab.go`: filter/search state, filtered `children()`, search
  results view, header badge, footer hints
- `internal/ui/model.go`: `promptKind`, `/` `f` `g` `Esc`, live preview, search
  navigation
- Tests: `internal/filter/filter_test.go`, additions to `internal/ui/*_test.go`
- `README.md` §2/§4 and `docs/ncdu-next.1`: document the new keys and syntax

**Out of scope (deferred):**
- Non-interactive `--filter`/`--find` with `--export` → v2
- Persisting queries in TOML config → v2
- Recomputing directory sizes to the filtered subset, and any `%` column → v2
- Filtering during scan / pruning the tree before export → v2
- Fuzzy/subsequence matching, saved searches, `--exclude`-style regex upgrade → v2

## Revisit / reevaluate (v2 tasks)

These two choices are **accepted for v1** but explicitly flagged for
reevaluation in v2 — they are provisional, not settled. Each is a
self-contained v2 task with a stated trigger and an acceptance check.

### R1. Key bindings (`/` `f` `g` `Esc`)
- **Why provisional:** `f`/`g` were chosen as "unused today". The vm-scan plan
  reserves `n`/`N`, and v2 features (delete dialog, view modes, CLI
  `--filter`) will consume more of the key space. The binding may need to move.
- **Reevaluate when:** any v2 feature needs `f`, `g`, or `Esc`, or on user
  feedback that the set is not discoverable.
- **v2 task:** audit all keys across `internal/ui` + planned v2 features,
  settle a single key map, update the footer hints, `README.md` §2, and
  `docs/ncdu-next.1` together, and add a test asserting the full key map.
- **Acceptance:** no two actions share a key; footer/README/man page agree with
  the code; a key-map test fails if any binding is removed.

### R2. Matching a directory reveals its whole subtree
- **Why provisional:** "matched dir ⇒ show entire subtree" is a UX guess to
  avoid dead-ends. The alternative (show only matching descendants) is
  defensible and may be preferred; it also changes `VisibleSet` cost (a match on
  a high-level dir currently pulls in a large subtree).
- **Reevaluate when:** users report that a matched dir shows too much, or that
  `VisibleSet` walk cost is dominated by matched-dir subtrees on large trees.
- **v2 task:** make the rule a toggle (`deep:subtree` vs `deep:leaves-only`),
  or pick one after usability testing; cover both branches in
  `internal/filter` tests.
- **Acceptance:** both semantics have a test; the chosen default is documented
  in `README.md` and the man page; `VisibleSet` cost is measured, not assumed.

## Failure modes

- Invalid regex / malformed `size:` clause → `filterErr` shown in the prompt;
  previous query retained; app not affected
- Filter matches nothing → list renders `(no matches)` and the badge shows
  `0 shown`; `enter` is a no-op
- Deep filter on a very large tree → one O(n) walk per commit; acceptable since
  it is not per-frame. If it proves slow, debounce commit (note for
  implementation, not a v1 blocker)
- Search result node no longer present after a rescan → `buildSearch` runs on
  the current `root`; a scan that replaces `root` clears `searchActive`
- Selection index beyond the filtered list (filter applied while a row is
  selected) → clamp `sel`/`searchSel` after every filter change

## Rollback

Additive and inert by default: with no query active, `children()` returns the
unfiltered slice and the renderer is byte-for-byte the current output. The only
edits to existing behavior are the footer hint text and the new `Esc` binding
in nav mode (previously a no-op). Removing `internal/filter` and the new `Tab`
fields restores the current program.

## Validation plan

1. `go build ./...` compiles
2. `go vet ./...` clean
3. `gofmt -l .` clean
4. `go test -count=1 -race ./...` all pass
5. `internal/filter` tests: substring / glob auto-detect / `re:` regex / invalid
   regex error / `type:image` / `type:.go` / `size:>10M` / combined AND /
   `!` negation / `VisibleSet` keeps ancestors and includes a matched dir's
   subtree / `ParseSize` boundaries
6. `internal/ui` tests: `/` enters filter prompt and live-filters rows; `Esc`
   clears; `f` toggles deep and recomputes; `g` builds a results list; `enter`
   in results jumps to a dir and to a file's parent; invalid regex shows
   `filterErr` and keeps the prior list; empty result renders `(no matches)`;
   no-filter output is unchanged (regression guard)
7. Manual: `ncdu-next` on a mixed tree — `/` then type, watch rows prune and the
   badge count; `Tab` for deep; `f` to toggle; `g` search and jump; `Esc` to
   clear; confirm `--export` is unaffected by any active filter

## Ordered task list

1. Create `internal/filter/filter.go`: `Query`, `Mode`, `Parse`, `Match`,
   `ParseSize`, `String`, and `VisibleSet`
2. Add `internal/filter/filter_test.go` covering the cases in §5
3. Extend `internal/ui/tab.go`: filter/search fields, filtered `children()`,
   `applyFilter`/`clearFilter`/`buildSearch`, header badge, results view,
   footer hints, selection clamping
4. Extend `internal/ui/model.go`: `promptKind`, `/` `f` `g` `Esc`, live preview,
   in-prompt `Tab` scope toggle, search navigation
5. Add/adjust `internal/ui` tests for the new keys and views
6. Update `README.md` (§2 TUI keys, §4 packages) and `docs/ncdu-next.1`
7. Run `go build ./...`, `go vet ./...`, `gofmt -l .`,
   `go test -count=1 -race ./...`
8. Manual smoke test per §7

## References

- Existing TUI: `internal/ui/model.go` (`Model`, `Update`, `View`),
  `internal/ui/tab.go` (`Tab.children`, `selectUp/Down`, `enter`, `view`)
- Tree model: `internal/tree/node.go` (`Node.Children`, `Node.Size`, `Delete`)
- Pre-scan excludes: `internal/scan/walker.go` (`isExcluded`)
- Size formatting to mirror: `internal/ui/tab.go` (`humanSize`, `sizeText`)
- Plan format reference: `.kilo/plans/1792710217521-vm-scan-plan.md`
- Prior UI findings: `.kilo/reports/ui-stack-findings.md`
