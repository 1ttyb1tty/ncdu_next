package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanProgress(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("world"), 0644)
	calls := 0
	var last string
	w := New(Options{Root: dir, Workers: 2, Progress: func(files int64, path string) {
		calls++
		last = path
	}})
	n, err := w.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls %d, want 2", calls)
	}
	if last == "" {
		t.Fatal("expected a path")
	}
	_ = n
}

func TestScanCancel(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5000; i++ {
		os.WriteFile(filepath.Join(dir, itoa2(i)+".txt"), []byte("x"), 0644)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(2 * time.Millisecond)
		cancel()
	}()
	w := New(Options{Root: dir, Workers: 8, Ctx: ctx})
	_, err := w.Scan()
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

func itoa2(i int) string {
	if i == 0 {
		return "0"
	}
	var b [16]byte
	k := len(b)
	for i > 0 {
		k--
		b[k] = byte('0' + i%10)
		i /= 10
	}
	return string(b[k:])
}
