package helper

import (
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"log/slog"
	"os"
)

func InitLog(name string) {
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
	_ = ar
	logger := slog.New(slog.NewJSONHandler(ar, &slog.HandlerOptions{AddSource: true, Level: slog.LevelInfo, ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == slog.SourceKey {
			s := a.Value.Any().(*slog.Source)
			//s.File = path.Base(s.File)
			s.Function = ""
		}
		return a
	}}))
	slog.SetDefault(logger)
}
