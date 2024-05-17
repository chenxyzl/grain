package helper

import (
	"log/slog"
)

func Recover(logger ...*slog.Logger) {
	err := recover()
	if err != nil {
		stackTrace := StackTrace()
		if len(logger) > 0 {
			logger[0].Error("panic recover",
				"err", err,
				"stackTrace", stackTrace)
		} else {
			slog.Error("panic recover",
				"err", err,
				"stackTrace", stackTrace)
		}
	}
}

// RecoverInfo recover with a msg, use msgProvider instead string for performance
func RecoverInfo(msgProvider func() string, logger ...*slog.Logger) {
	if msgProvider == nil {
		Recover(logger...)
	} else {
		err := recover()
		if err != nil {
			stackTrace := StackTrace()
			if len(logger) > 0 {
				logger[0].Error("panic recover",
					"msg", msgProvider(),
					"err", err,
					"stackTrace", stackTrace)
			} else {
				slog.Error("panic recover",
					"msg", msgProvider(),
					"err", err,
					"stackTrace", stackTrace)
			}
		}
	}
}

func RecoverFunc(pc func(err any), logger ...*slog.Logger) {
	if pc == nil {
		Recover(logger...)
	} else {
		err := recover()
		if err != nil {
			stackTrace := StackTrace()
			if len(logger) > 0 {
				logger[0].Error("panic recover",
					"err", err,
					"stackTrace", stackTrace)
			} else {
				slog.Error("panic recover",
					"err", err,
					"stackTrace", stackTrace)
			}
			pc(err)
		}
	}
}
