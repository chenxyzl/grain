package ghelper

import (
	"runtime"
	"strconv"
	"strings"
)

// maxStackFrames bounds a trace so a deep or recursive stack cannot make an unbounded log line.
const maxStackFrames = 32

// StackTrace renders the caller's stack for a log line, marking truncation explicitly. Uses
// runtime.Callers + CallersFrames, not runtime.Caller in a loop: the loop re-walks the stack per
// frame, and FuncForPC(pc).Name() reports the OUTER function for inlined frames.
func StackTrace() string {
	pcs := make([]uintptr, maxStackFrames)
	// skip 2: runtime.Callers itself and StackTrace.
	n := runtime.Callers(2, pcs)
	if n == 0 {
		return ""
	}
	var b strings.Builder
	frames := runtime.CallersFrames(pcs[:n])
	for i := 0; ; i++ {
		frame, more := frames.Next()
		b.WriteString("\nframe ")
		b.WriteString(strconv.Itoa(i))
		b.WriteString(":[file:")
		b.WriteString(trimFilePath(frame.File))
		b.WriteString(",line:")
		b.WriteString(strconv.Itoa(frame.Line))
		b.WriteString(",func:")
		b.WriteString(frame.Function)
		b.WriteString("]")
		if !more {
			break
		}
	}
	if n == maxStackFrames {
		b.WriteString("\n...(truncated)")
	}
	return b.String()
}

// trimFilePath shortens an absolute source path to its last two segments (package dir + file),
// which is what identifies the location. Trimming at "/src/" instead only matches under GOPATH.
func trimFilePath(file string) string {
	if file == "" {
		return "?"
	}
	last := strings.LastIndexByte(file, '/')
	if last <= 0 {
		return file
	}
	prev := strings.LastIndexByte(file[:last], '/')
	if prev < 0 {
		return file
	}
	return file[prev+1:]
}
