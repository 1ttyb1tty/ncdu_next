package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/1ttyb1tty/ncdu-next/internal/config"
	"github.com/1ttyb1tty/ncdu-next/internal/debuginfo"
	"github.com/1ttyb1tty/ncdu-next/internal/export"
	"github.com/1ttyb1tty/ncdu-next/internal/logging"
	"github.com/1ttyb1tty/ncdu-next/internal/scan"
	"github.com/1ttyb1tty/ncdu-next/internal/ui"
	"golang.org/x/sys/windows"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	config.ApplyFlags(&cfg)
	cfgPath := ""
	flag.StringVar(&cfgPath, "config", "", "config file path")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: ncdu-next [options] [path]\n\nOptions:\n")
		flag.VisitAll(func(f *flag.Flag) {
			fmt.Fprintf(os.Stderr, "  --%s\t%s (default: %v)\n", f.Name, f.DefValue, f.DefValue)
		})
		fmt.Fprintf(os.Stderr, "\nTUI keys: q/Ctrl-C quit, up/k down/j select, Enter/l open, Backspace/h up, e/w wide view, u units, d detect-format, s scan, r root, t new tab, x close tab, 1-8/Ctrl+Tab switch tab, c cancel scan, D debug tab (--debug), E debuginfo bundle on the debug tab\n")
		fmt.Fprintf(os.Stderr, "See docs/ncdu-next.1 and README.md for details.\n")
	}
	flag.Parse()
	if cfgPath != "" {
		c2, err := config.Load(cfgPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		// flags override: re-apply only provided? Simpler: merge non-zero
		if c2.Root != "" {
			cfg.Root = c2.Root
		}
		if c2.Workers > 0 {
			cfg.Workers = c2.Workers
		}
		cfg.CountLinks = cfg.CountLinks || c2.CountLinks
		cfg.OneFS = cfg.OneFS || c2.OneFS
		cfg.FollowSym = cfg.FollowSym || c2.FollowSym
		cfg.Trash = cfg.Trash || c2.Trash
		cfg.Verbose = cfg.Verbose || c2.Verbose
		if cfg.LogFile == "" {
			cfg.LogFile = c2.LogFile
		}
		if cfg.Export == "" {
			cfg.Export = c2.Export
		}
		cfg.Excludes = append(cfg.Excludes, c2.Excludes...)
		cfg.DetectFormat = cfg.DetectFormat || c2.DetectFormat
		cfg.Debug = cfg.Debug || c2.Debug
		if cfg.DebugInfo == "" {
			cfg.DebugInfo = c2.DebugInfo
		}
		cfg.DebugInfoTree = cfg.DebugInfoTree || c2.DebugInfoTree
	}

	logger, closer, err := logging.Setup(cfg.Verbose, cfg.LogFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer closer()

	// Normalize Windows drive-only paths (e.g. "C:" or "c:") to "C:\"
	// so os.ReadDir/filepath.Abs work correctly.
	root := cfg.Root
	if runtime.GOOS == "windows" {
		if len(root) == 2 && root[1] == ':' {
			root = root + "\\"
		} else if len(root) == 1 && root[0] >= 'A' && root[0] <= 'Z' {
			root = string(root[0]) + ":\\"
		} else if len(root) == 1 && root[0] >= 'a' && root[0] <= 'z' {
			root = strings.ToUpper(string(root[0])) + ":\\"
		}
	}

	w := scan.New(scan.Options{Root: root, Workers: cfg.Workers, CountLinks: cfg.CountLinks, OneFS: cfg.OneFS, Excludes: cfg.Excludes, DetectFormat: cfg.DetectFormat})
	node, err := w.Scan()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// --one-file-system fails open when the filesystem identity is unknown: warn
	// once, at the root, rather than per directory.
	if cfg.OneFS {
		if fs := w.FSReport(); fs != nil && fs.Root.Avail != scan.FSAvailable {
			logger.Warn("--one-file-system: filesystem identity unavailable; crossing mount points",
				"reason", fs.Root.Reason)
		}
	}
	if cfg.Export != "" {
		if err := export.Write(cfg.Export, node); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	// Headless bundle: like --export, write and exit. The two may be combined.
	if cfg.DebugInfo != "" {
		m := ui.NewWithReport(root, node, w.FSReport(), cfg.Debug, baseOptions(cfg)).WithConfig(cfg)
		in := m.DebugInfoInput(cfg.DebugInfoTree, debuginfo.BuildInfoText(), time.Now())
		if err := debuginfo.Write(cfg.DebugInfo, in); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if cfg.DebugInfoTree && cfg.DebugInfo == "" {
		// A silently ignored switch looks like success; say so instead.
		logger.Warn("--debuginfo-tree has no effect without --debuginfo")
	}
	if cfg.Export != "" || cfg.DebugInfo != "" {
		return
	}

	// Enable virtual terminal processing on Windows so ANSI escape
	// sequences (including alt screen) work in the console.
	if runtime.GOOS == "windows" {
		stdout, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
		if err == nil {
			var mode uint32
			if windows.GetConsoleMode(stdout, &mode) == nil {
				mode |= windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
				windows.SetConsoleMode(stdout, mode) // nolint:errcheck
			}
		}
	}

	// Detect color profile, but on Windows the console may not be
	// recognized as a TTY (term.IsTerminal returns false), which would
	// force NoTTY and strip all colors. Since we enabled VT processing
	// above, ANSI sequences work — upgrade to at least ANSI256 so colors
	// render. On non-Windows or when detection already yields color, keep
	// the detected profile.
	cp := colorprofile.Detect(os.Stdout, os.Environ())
	if runtime.GOOS == "windows" && (cp == colorprofile.NoTTY || cp == colorprofile.Ascii) {
		cp = colorprofile.ANSI256
	}

	// Base scan options for TUI-triggered rescans: same config as the initial
	// scan, minus Root (per-tab) and DetectFormat (per-tab). Root is the
	// normalized value so rescans don't fall back to the raw "C:" form.
	p := tea.NewProgram(ui.NewWithReport(root, node, w.FSReport(), cfg.Debug, baseOptions(cfg)).WithConfig(cfg), tea.WithColorProfile(cp))

	if _, err := runTUI(p); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// baseOptions returns the scan options used for TUI-triggered rescans: the same
// config as the initial scan, minus Root and DetectFormat (both per-tab).
func baseOptions(cfg config.Config) scan.Options {
	return scan.Options{
		Workers:    cfg.Workers,
		CountLinks: cfg.CountLinks,
		OneFS:      cfg.OneFS,
		FollowSym:  cfg.FollowSym,
		Excludes:   cfg.Excludes,
	}
}

// runTUI enters the alternate screen buffer, runs the program, and always
// restores the terminal state. bubbletea v2.1.0's alt-screen write path passes
// write=false, so the entry sequence is written here; the exit is written by
// the deferred restore (and os.Exit is kept out of this function so the defer
// always runs, including on a panic).
func runTUI(p *tea.Program) (tea.Model, error) {
	fmt.Print("\x1b[?1049h")
	defer fmt.Print("\x1b[?1049l")
	return p.Run()
}
