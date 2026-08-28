package ghelper

import (
	"runtime"
	"strconv"
	"strings"
)

// maxStackFrames bounds the trace so a deep or recursive stack cannot produce an
// unbounded log line. Truncation is marked explicitly rather than silently dropped.
const maxStackFrames = 32

// StackTrace renders the caller's stack for a log line.
//
// Uses runtime.Callers + CallersFrames rather than runtime.Caller in a loop: the loop
// re-walked the stack once per frame, built the string with O(n^2) concatenation, and
// FuncForPC(pc).Name() reports the OUTER function for inlined frames, so names were
// sometimes wrong. CallersFrames expands inline frames correctly.
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

// trimFilePath shortens an absolute source path to something readable in a log.
//
// The old code trimmed at "/src/", which only ever matched under GOPATH — in a module
// build paths look like /home/me/proj/pkg/file.go, so project files were logged with
// their full absolute path and the trim was dead code. Keep the last two segments
// (package dir + file), which is what actually identifies the location.
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
