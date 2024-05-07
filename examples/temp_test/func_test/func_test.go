package func_test

import (
	"reflect"
	"runtime"
	"sync/atomic"
	"testing"
)

type s struct {
	v int
}

func (x *s) Fun1(i int) int {
	return x.v + i
}

func BenchmarkFunc1(b *testing.B) {
	b.ResetTimer()
	a := 0
	x := &s{v: 1}
	for i := range b.N {
		a += x.Fun1(i)
	}
	_ = a
}
func BenchmarkFunc2(b *testing.B) {
	x := &s{v: 1}
	v := reflect.ValueOf(x.Fun1)
	b.ResetTimer()
	a := 0
	for i := range b.N {
		a1 := v.Call([]reflect.Value{reflect.ValueOf(i)})
		a += a1[0].Interface().(int)
	}
	_ = a
}
func BenchmarkFunc3(b *testing.B) {
	x := &s{v: 1}
	v := reflect.ValueOf(x.Fun1)
	f := v.Interface().(func(int) int)
	b.ResetTimer()
	a := 0
	for i := range b.N {
		a += f(i)
	}
	_ = a
}
func TestFun1(t *testing.T) {
	x := &s{v: 100}
	v := reflect.ValueOf(x.Fun1)
	f := v.Interface().(func(int) int)
	a := f(1)
	println(a)
}

func testGoChan() {
	for _, v := range c {
		go func() {
			for v1 := range v {
				_ = v1
			}
		}()
		//fmt.Println(v)
	}
}

var idx int64
var maxIdx int64 = 10000
var c []chan int

func init() {
	for i := int64(0); i < maxIdx; i++ {
		c = append(c, make(chan int, 1024))
	}

	n := runtime.NumCPU()
	runtime.GOMAXPROCS(n * 2)
}

func BenchmarkChan(b *testing.B) {
	go testGoChan()
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(100)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			_ = v
			c[0] <- int(v)
		}
	})
}
func BenchmarkChanMore(b *testing.B) {
	go testGoChan()
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(100)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			_ = v
			c[v] <- int(v)
		}
	})
}
