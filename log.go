package grain

import (
	"io"
	"log/slog"
	"os"
	"path"

	"gopkg.in/natefinch/lumberjack.v2"
)

// NewLogger builds the framework's standard logger — a rotating file (lumberjack) tee'd to
// stdout, text format, source trimmed to the base filename — WITHOUT touching any global.
// Pair it with WithConfigLogger to avoid InitLog's ordering rule entirely.
func NewLogger(name string, level slog.Level) *slog.Logger {
	const maxSize = 100
	r := &lumberjack.Logger{
		Filename:   name,
		LocalTime:  true,
		MaxSize:    maxSize,              //M
		MaxAge:     30,                   //Day
		MaxBackups: 100 * 1024 / maxSize, //Max 100G = 100 * 1024 / maxSize
		Compress:   false,
	}
	ar := io.MultiWriter(r, os.Stdout)
	return slog.New(slog.NewTextHandler(ar, &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.SourceKey {
				s := a.Value.Any().(*slog.Source)
				s.File = path.Base(s.File)
				s.Function = ""
			}
			return a
		}}))
}

// InitLog installs NewLogger(name, level) as the process-wide slog default, so it also affects
// every other slog user in the process, dependencies included.
//
// ⚠️ ORDER MATTERS: it must be called BEFORE system.Start(), which is where the system reads
// the global to build its own logger. Called after, the system and every actor keep logging
// through the default that was in place at Start() — easy to miss, because the caller's own
// slog lines do switch over. WithConfigLogger avoids both problems.
func InitLog(name string, level slog.Level) {
	slog.SetDefault(NewLogger(name, level))
}
