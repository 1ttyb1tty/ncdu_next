package config

import (
	"os"
	"testing"
)

func FuzzLoad(f *testing.F) {
	f.Add("root = '.'\n")
	tmp := os.TempDir()
	f.Fuzz(func(t *testing.T, data string) {
		p := tmp + "/fuzz.toml"
		os.WriteFile(p, []byte(data), 0644)
		_, _ = Load(p)
	})
}
