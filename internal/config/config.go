package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Root          string   `toml:"root" doc:"root directory; maps to scan.Options.Root"`
	Workers       int      `toml:"workers" doc:"scan concurrency; maps to scan.Options.Workers"`
	CountLinks    bool     `toml:"count_hardlinks" doc:"maps to scan.Options.CountLinks; repeat hardlinks get Size=0"`
	OneFS         bool     `toml:"one_file_system" doc:"maps to scan.Options.OneFS; cross-device dirs are skipped"`
	Excludes      []string `toml:"excludes" doc:"maps to scan.Options.Excludes; names skipped before a record"`
	FollowSym     bool     `toml:"follow_symlinks" doc:"maps to scan.Options.FollowSym; symlinks become target records"`
	Trash         bool     `toml:"trash" doc:"not in records: delete destination only"`
	Verbose       bool     `toml:"verbose" doc:"not in records: logging only"`
	LogFile       string   `toml:"log_file" doc:"not in records: logging only"`
	Export        string   `toml:"export" doc:"not in records: JSON export path only"`
	DetectFormat  bool     `toml:"detect_format" doc:"maps to scan.Options.DetectFormat; populates Encoding/LineEnding"`
	Debug         bool     `toml:"debug" doc:"not in records: enables the in-program debug tab"`
	DebugInfo     string   `toml:"debuginfo" doc:"not in records: write a debuginfo bundle to this path and exit"`
	DebugInfoTree bool     `toml:"debuginfo_tree" doc:"not in records: include the full tree in the debuginfo bundle"`
}

func DefaultPath() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "ncdu-next", "config.toml")
	}
	return "config.toml"
}

func Load(path string) (Config, error) {
	var c Config
	if path == "" {
		path = DefaultPath()
	}
	if _, err := os.Stat(path); err != nil {
		return c, nil
	}
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return c, fmt.Errorf("config: %w", err)
	}
	return c, nil
}

func ApplyFlags(c *Config) {
	flag.StringVar(&c.Root, "path", valueOr(c.Root, "."), "root path")
	flag.IntVar(&c.Workers, "workers", valueOrInt(c.Workers, 4), "workers")
	flag.BoolVar(&c.CountLinks, "count-hardlinks", c.CountLinks, "count hardlinks")
	flag.BoolVar(&c.OneFS, "one-file-system", c.OneFS, "stay on one filesystem")
	flag.BoolVar(&c.FollowSym, "follow-symlinks", c.FollowSym, "follow symlinks")
	flag.BoolVar(&c.Trash, "trash", c.Trash, "use OS trash")
	flag.BoolVar(&c.Verbose, "v", c.Verbose, "debug logging")
	flag.StringVar(&c.LogFile, "log-file", c.LogFile, "log file")
	flag.StringVar(&c.Export, "export", c.Export, "export JSON path")
	flag.BoolVar(&c.DetectFormat, "detect-format", c.DetectFormat, "enable magic-byte format detection")
	flag.BoolVar(&c.Debug, "debug", c.Debug, "enable in-program debug tab")
	flag.StringVar(&c.DebugInfo, "debuginfo", c.DebugInfo, "write a debuginfo bundle and exit")
	flag.BoolVar(&c.DebugInfoTree, "debuginfo-tree", c.DebugInfoTree, "include the full tree in the debuginfo bundle")
}

func valueOr(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
func valueOrInt(n, d int) int {
	if n <= 0 {
		return d
	}
	return n
}
