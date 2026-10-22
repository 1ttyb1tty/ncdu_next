package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Root       string   `toml:"root"`
	Workers    int      `toml:"workers"`
	CountLinks bool     `toml:"count_hardlinks"`
	OneFS      bool     `toml:"one_file_system"`
	Excludes   []string `toml:"excludes"`
	FollowSym  bool     `toml:"follow_symlinks"`
	Trash      bool     `toml:"trash"`
	Verbose    bool     `toml:"verbose"`
	LogFile    string   `toml:"log_file"`
	Export     string   `toml:"export"`
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
