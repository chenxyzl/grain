package examples

import (
	"github.com/timandy/routine"
	"testing"
)

func BenchmarkRoutineGet(b *testing.B) {
	var threadLocal = routine.NewInheritableThreadLocal[string]()
	// 假设你已经有了一个etcd客户端cli
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		threadLocal.Set("hello world")
	}
}
func BenchmarkRoutineSet(b *testing.B) {
	var threadLocal = routine.NewInheritableThreadLocal[string]()
	threadLocal.Set("hello world")
	// 假设你已经有了一个etcd客户端cli
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		threadLocal.Get()
	}
}
