package helper

import (
	"log/slog"

	"gopkg.in/natefinch/lumberjack.v2"
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
	logger := slog.New(slog.NewJSONHandler(r, &slog.HandlerOptions{AddSource: true, Level: slog.LevelInfo, ReplaceAttr: nil}))
	slog.SetDefault(logger)
}
