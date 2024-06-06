package helper

import (
	"log/slog"
)

func Recover(pfs ...func(e any, trace string)) {
	e := recover()
	if e != nil {
		stackTrace := StackTrace()
		if len(pfs) > 0 {
			for _, pf := range pfs {
				pf(e, stackTrace)
			}
		} else {
			slog.Error("panic recover", "err", e, "stackTrace", stackTrace)
		}
	}
}
