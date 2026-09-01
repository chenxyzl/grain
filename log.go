package grain

import (
	"io"
	"log/slog"
	"os"
	"path"

	"gopkg.in/natefinch/lumberjack.v2"
)

// NewLogger builds the framework's standard logger — a rotating file (lumberjack) tee'd
// to stdout, text format, source location trimmed to the base filename — and returns it
// WITHOUT touching any global.
//
// Pair it with WithConfigLogger to keep logging entirely explicit:
//
//	system := grain.NewSystem(name, ver, urls,
//	    grain.WithConfigLogger(grain.NewLogger("./game.log", slog.LevelInfo)))
//
// That path has no ordering rules at all, which InitLog cannot offer — see there.
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

// InitLog installs NewLogger(name, level) as the process-wide slog default.
//
// ⚠️ ORDER MATTERS: it mutates a global, and a system reads that global when it builds
// its own logger — in Start() (and again in init(), once the node id is known). So it
// must be called BEFORE system.Start(). Called after, the system and every actor keep
// logging through whatever default was in place at Start(), and this handler is never
// used for framework output. Anything the caller logs directly through slog does switch
// over, which is what makes the mistake easy to miss: your own lines move, the
// framework's do not.
//
// It also affects every other slog user in the process, this framework's dependencies
// included.
//
// To avoid both, hand the logger to the system explicitly and skip the global:
//
//	grain.WithConfigLogger(grain.NewLogger("./game.log", slog.LevelInfo))
func InitLog(name string, level slog.Level) {
	slog.SetDefault(NewLogger(name, level))
}
