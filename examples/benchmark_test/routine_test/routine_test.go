package routine_test

import (
	"runtime"
	"testing"

	"github.com/timandy/routine"
)

func BenchmarkRoutineGet(b *testing.B) {
	var threadLocal = routine.NewInheritableThreadLocal[string]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		threadLocal.Set("hello world")
	}
}
func BenchmarkRoutineSet(b *testing.B) {
	var threadLocal = routine.NewInheritableThreadLocal[string]()
	threadLocal.Set("hello world")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		threadLocal.Get()
	}
}
func BenchmarkGosched(b *testing.B) {
	for i := 0; i < b.N; i++ {
		runtime.Gosched()
	}
}

func BenchmarkGetId(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		routine.Goid()
	}
}
