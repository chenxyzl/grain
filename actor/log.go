package actor

import (
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"log/slog"
	"os"
	"path"
)

func InitLog(name string, level slog.Level) {
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
	logger := slog.New(slog.NewTextHandler(ar, &slog.HandlerOptions{
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
	slog.SetDefault(logger)
}
