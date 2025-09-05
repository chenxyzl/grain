package ghelper

import (
	"fmt"
	"runtime"
	"strings"
)

func StackTrace() string {
	var trace string
	for i := 1; i < 20; i++ {
		pc, file, line, ok := runtime.Caller(i)
		if ok {
			p := strings.Index(file, "/src/")
			if p != -1 {
				file = file[p+len("/src/"):]
			}
			trace += fmt.Sprintf("\nframe %d:[file:%s,line:%d,func:%s]", i, file, line, runtime.FuncForPC(pc).Name())
		} else {
			break
		}
	}
	return trace
}
