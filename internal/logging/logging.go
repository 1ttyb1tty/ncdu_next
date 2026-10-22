package logging

import (
	"io"
	"log/slog"
	"os"
)

func Setup(verbose bool, logFile string) (*slog.Logger, func(), error) {
	level := slog.LevelWarn
	if verbose {
		level = slog.LevelDebug
	}
	var w io.Writer = os.Stderr
	closer := func() {}
	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, closer, err
		}
		w = io.MultiWriter(os.Stderr, f)
		closer = func() { f.Close() }
	}
	h := slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})
	return slog.New(h), closer, nil
}
