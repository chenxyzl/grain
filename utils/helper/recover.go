package helper

import (
	"log/slog"
	"runtime/debug"
)

func Recover(logger ...*slog.Logger) {
	err := recover()
	if err != nil {
		stackTrace := debug.Stack()
		if len(logger) > 0 {
			logger[0].Error("panic recover", "err", err, "stackTrace", stackTrace)
		} else {
			slog.Error("panic recover", "err", err, "stackTrace", stackTrace)
		}
	}
}

func RecoverInfo(info string, logger ...*slog.Logger) {
	if info == "" {
		Recover()
	} else {
		err := recover()
		if err != nil {
			stackTrace := debug.Stack()
			if len(logger) > 0 {
				logger[0].Error("panic recover", "info", info, "err", err, "stackTrace", stackTrace)
			} else {
				slog.Error("panic recover", "info", info, "err", err, "stackTrace", stackTrace)
			}
		}
	}
}

func RecoverFunc(pc func(err any), logger ...*slog.Logger) {
	if pc == nil {
		Recover()
	} else {
		err := recover()
		if err != nil {
			stackTrace := debug.Stack()
			if len(logger) > 0 {
				logger[0].Error("panic recover", "err", err, "stackTrace", stackTrace)
			} else {
				slog.Error("panic recover", "err", err, "stackTrace", stackTrace)
			}
			pc(err)
		}
	}
}
