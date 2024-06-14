package actor

import (
	"fmt"
	"runtime"
	"strings"
)

func StackTrace() string {
	var trace string
	for i := 0; i < 10; i++ {
		pc, file, line, ok := runtime.Caller(i)
		if ok {
			p := strings.Index(file, "/src/")
			if p != -1 {
				file = file[p+len("/src/"):]
			}
			trace += fmt.Sprintf("frame %d:[func:%s,file:%s,line:%d]\n", i, runtime.FuncForPC(pc).Name(), file, line)
		} else {
			break
		}
	}
	return trace
}
