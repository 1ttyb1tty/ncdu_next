# ncdu-next — repository conventions

`github.com/1ttyb1tty/ncdu-next`: a Go (1.26) disk-usage TUI on bubbletea/lipgloss v2.
The machine-wide rules in `~/.config/kilo/AGENTS.md` also apply; this file holds only
what is specific to this repository.

## Verification gate

There is no `make` on this machine. Run the PowerShell gate from the repo root:

    powershell -ExecutionPolicy Bypass -File scripts/check.ps1

Full gate: `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test -count=1 -race ./...`;
the success line is `check OK`. `-Fast` skips build and race (focused tests
`go test ./internal/filter/ ./internal/ui/`, success line `check-fast OK`) but still runs
`gofmt -l .`. Run the full gate with `-race` before finishing a code change.

`.gitattributes` pins `* text=auto eol=lf`. Do not remove it: without it, a Windows
checkout with `core.autocrlf=true` materializes CRLF and `gofmt -l .` fails on every file
even though the committed blobs are LF.

## Build artifact

`bin/ncdu-next.exe` is the executable people run and report on, and `bin/` is
gitignored. Nothing used to produce it: `make build` and the gate are compile-only
(`go build ./...`), so the artifact on disk went stale silently and a fixed bug
still reproduced in the binary. Rebuild it as the last step of any code change:

    powershell -ExecutionPolicy Bypass -File scripts/build.ps1

(the `artifact` make target is the same build on Unix). The binary stamps the
revision it was built from, so staleness is checkable without file times:

    go version -m bin/ncdu-next.exe   # vcs.revision must equal `git rev-parse HEAD`

The script prints the embedded revision and exits non-zero if it differs from
HEAD; `vcs.modified=true` means the tree was dirty at build time. The verification
gate does not write the binary.

The binary also states its own identity, from `debug.ReadBuildInfo` (so it needs
no config and no VCS checkout, and reports `(no VCS metadata)` when unstamped):
`--version` prints module version, embedded revision, toolchain and platform;
`--version --json` prints the same as JSON for scripts; the Debug tab's `build`
section shows it; and the debuginfo `manifest.json` carries it. CI re-runs
`scripts/build.ps1` so the built artifact must name the commit it came from.

## Portability check

`GOOS=linux go build ./internal/...` is the useful cross-check. `GOOS=linux go build ./...`
fails because `cmd/ncdu-next/main.go` imports `golang.org/x/sys/windows` unconditionally,
and `internal/scan/walker_unix.go` does not build on darwin (`s.Dev` is int32 there and is
passed to a uint64 helper). Neither is in scope here.

## Model invariants (load-bearing)

- **Two tables must agree.** `internal/scan/walker.go` `fileTypes` maps an extension to a
  specific Type id (`vdi`, `png`, `go`, ...); `internal/tree/node.go` `typeKind` maps that
  id to a broad `Kind`. Add a new format in both, or `TestFileTypesHaveKind` (scan) fails.
  The tree side is covered by `TestKindOf` and `TestKindValuesHaveNames`.
- **`tree.Node` field order is load-bearing for size.** The packed order is 176 B; the naive
  order is 192 B. It is also reflected by the Debug tab's record reference (below), so a
  reorder changes what that tab renders. Access is by field name, so it is otherwise safe.
- **The Debug record reference is test-guarded.** Every `tree.Node` field needs a `doc`
  struct tag plus `file:`/`dir:` role tags, and any new documented struct or enum must be
  registered in `internal/ui/recinfo.go` (`recordSpecs`, `documentedTypes`/`typesNotRecords`,
  the `flags`/`bits`/`enums` views, and a canonical value slice such as `scanStatusValues`).
  Otherwise `TestDocCoverage`, `TestRecordFieldTags`, `TestEnumSliceMatchesConstBlock` or
  `TestEnumMeaningComplete` fails.
- **The export `type` field is a contract.** `internal/export/json.go` writes the specific
  Type id (e.g. `vdi`, `png`), not a category; the value domain changed from the older
  category-prefixed ids, so external consumers of `--export` see different strings.

## Conventions

- Tests run with `-race`; new classification, timestamp, unit or view behavior is expected
  to carry a test in the package it belongs to.
