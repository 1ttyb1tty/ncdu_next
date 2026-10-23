package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	tea "charm.land/bubbletea/v2"

	"github.com/1ttyb1tty/ncdu-next/internal/config"
	"github.com/1ttyb1tty/ncdu-next/internal/export"
	"github.com/1ttyb1tty/ncdu-next/internal/logging"
	"github.com/1ttyb1tty/ncdu-next/internal/scan"
	"github.com/1ttyb1tty/ncdu-next/internal/ui"
	"golang.org/x/sys/windows"
	"github.com/charmbracelet/colorprofile"
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
		fmt.Fprintf(os.Stderr, "\nTUI keys: q/Ctrl-C quit, ↑/k ↓/j select, Enter/l open, Backspace/h up, e toggle type, d detect-format, s scan, r root, t new tab, x close tab, 1-8/Ctrl+Tab switch tab, c cancel scan\n")
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
	}

	logger, closer, err := logging.Setup(cfg.Verbose, cfg.LogFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer closer()
	_ = logger

	w := scan.New(scan.Options{Root: cfg.Root, Workers: cfg.Workers, CountLinks: cfg.CountLinks, OneFS: cfg.OneFS, Excludes: cfg.Excludes, DetectFormat: cfg.DetectFormat})
	node, err := w.Scan()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if cfg.Export != "" {
		if err := export.Write(cfg.Export, node); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
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

	// Diagnose and force true color profile for rich terminal colors
	cp := colorprofile.Detect(os.Stdout, os.Environ())
	fmt.Fprintf(os.Stderr, "DEBUG: detected color profile=%s\n", cp)
	fmt.Fprintf(os.Stderr, "DEBUG: forcing true color profile\n")

	// Enter the alternate screen buffer before starting bubbletea.
	// bubbletea v2.1.0 has a bug where enableAltScreen is called with
	// write=false from flush(), so the alt screen escape sequence is never
	// actually written to the terminal on first render. We handle alt
	// screen manually here and leave AltScreen off on the view to avoid
	// double-handling.
	fmt.Print("\x1b[?1049h")
	defer fmt.Print("\x1b[?1049l")

	p := tea.NewProgram(ui.New(cfg.Root, node, cfg.Debug), tea.WithColorProfile(colorprofile.TrueColor))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
