package safemap_test

import (
	"github.com/chenxyzl/grain/utils/al/safemap"
	"github.com/chenxyzl/grain/uuid"
	"strconv"
	"sync/atomic"
	"testing"
)

const parallelism = 100

var (
	maxIdx        int64 = 10000
	idx           int64 = 0
	testMap             = safemap.NewM[string, string]()
	testStringMap       = safemap.NewStringC[string]()
	testIntMap          = safemap.NewIntC[int, string]()
)

func init() {
	uuid.Init(1)
	for i := int64(0); i < maxIdx; i++ {
		sv := strconv.Itoa(int(uuid.Generate()))
		testMap.Set(sv, sv)
		testStringMap.Set(sv, sv)
		testIntMap.Set(int(i), sv)
	}
}

func BenchmarkSafeMap1(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			testMap.Get(strconv.Itoa(int(v)))
		}
	})
}
func BenchmarkSafeMap2(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			testMap.Set(strconv.Itoa(int(v)), strconv.Itoa(int(v)))
		}
	})
}
func BenchmarkSafeMap3(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			testMap.Get(strconv.Itoa(int(v)))
			testMap.Set(strconv.Itoa(int(v)), strconv.Itoa(int(v)))
		}
	})
}
func BenchmarkSafeMap4(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			testStringMap.Get(strconv.Itoa(int(v)))
		}
	})
}
func BenchmarkSafeMap5(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			testStringMap.Set(strconv.Itoa(int(v)), strconv.Itoa(int(v)))
		}
	})
}
func BenchmarkSafeMap6(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			testStringMap.Get(strconv.Itoa(int(v)))
			testStringMap.Set(strconv.Itoa(int(v)), strconv.Itoa(int(v)))
		}
	})
}
func BenchmarkSafeMap7(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			testIntMap.Get(int(v))
		}
	})
}
func BenchmarkSafeMap8(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			testIntMap.Set(int(v), strconv.Itoa(int(v)))
		}
	})
}
func BenchmarkSafeMap9(b *testing.B) {
	b.ResetTimer()
	// 限制并发数
	b.SetParallelism(parallelism)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			v := atomic.AddInt64(&idx, 1) % maxIdx
			testIntMap.Get(int(v))
			testIntMap.Set(int(v), strconv.Itoa(int(v)))
		}
	})
}
