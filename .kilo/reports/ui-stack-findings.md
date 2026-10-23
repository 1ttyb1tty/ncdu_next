# UI Stack Findings — ncdu-next TUI

Date: 2026-10-23
Component: `cmd/ncdu-next`, `internal/ui/`

## bubbletea architecture

**bubbletea v2.1.0 provides NO UI widgets out of the box.** It is only the Elm-style event loop: `Model → Update → View`, plus input handling, window resize, and alternate screen management.

All UI elements (tab bar, row highlighting, type colors, separators, borders) must be hand-composed inside `View()` using **lipgloss** (`charm.land/lipgloss/v2`) for styling (colors, borders, padding, margins, width/height).

**Mental model:**
- `Model` holds app state (tabs, current node, selection, expand flag, detect-format flag)
- `Update(msg)` handles keypresses → returns new `Model` + optional `Cmd`
- `View()` renders the model as a styled string → bubbletea prints it to the terminal each frame
- Pre-built widgets (text input, spinner, list, table, file picker, help) live in the separate **bubbles** project, not bubbletea

## bubbletea v2.1.0 bug: alt screen escape sequence never written

**Symptom:** TUI renders only the footer line; tab bar and content area are missing; terminal stays in main buffer.

**Root cause:** In `internal/ui/cursed_renderer.go`:
- `start()` returns early when `lastView == nil` (first render), so `enableAltScreen(s, true, true)` with `write=true` is never called.
- `flush()` detects `shouldUpdateAltScreen` on first render but calls `enableAltScreen(s, view.AltScreen, false)` with `write=false`, so `\x1b[?1049h` is never written to the terminal.

**Result:** Renderer internal state thinks alt screen is active, but the terminal never switches to it.

**Workaround (committed as `4811779`):** Manually write `\x1b[?1049h` before `tea.NewProgram(...).Run()` and `\x1b[?1049l` after it returns (deferred). Do NOT set `View.AltScreen = true` to avoid double-handling.

**Required prerequisite:** Enable `ENABLE_VIRTUAL_TERMINAL_PROCESSING` on `STDOUT` via `golang.org/x/sys/windows` `SetConsoleMode` before starting bubbletea, otherwise ANSI sequences (including alt screen) are not interpreted on Windows consoles.

## Windows terminal specifics

- `term.IsTerminal(os.Stdout)` returns `false` on Windows PowerShell/CMD — bubbletea does not treat stdout as a TTY by default.
- `uv.OpenTTY()` succeeds and returns real TTY handles, but bubbletea's `Run()` uses `os.Stdout`/`os.Stdin` directly unless configured otherwise.
- Console mode `0x3` (ENABLE_PROCESSED_OUTPUT | ENABLE_WRAP_AT_EOL_OUTPUT) → add `0x4` (ENABLE_VIRTUAL_TERMINAL_PROCESSING) → `0x7`.

## Related decisions (v1 scope)

- Delete (OS trash / `.ncdu_nx` folder) → v2
- Resource stats status bar (mem/CPU/IOPS) → v2/v2.5/v3; logical IOPS issued by our process only, never system-wide
- `n`/`N` (scan-in-new-tab, detailed scan-in-new-tab) → v2; v1 uses `t` → `r` → `s`
- Configurable extension→type map → v2/v2.5/v3

## Files

- `cmd/ncdu-next/main.go` — VT mode enable + manual alt screen enter/exit
- `internal/ui/model.go` — multi-tab Model, View with tab bar + separator
- `internal/ui/tab.go` — Tab struct, view rendering with lipgloss styling, scan commands
- `internal/ui/tab_test.go` — tab lifecycle, status transitions, cancel, prompt, cycling
