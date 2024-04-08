package examples

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	outer "grain/examples/pb"
	"reflect"
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

type RpcFunc[Req, Rep proto.Message] func(req Req) Rep

func ReqTest(req *outer.GetRecommendGroupList_Request) *outer.GetRecommendGroupList_Reply {
	_ = req
	reply := &outer.GetRecommendGroupList_Reply{}
	return reply
}

func TestFun2(t *testing.T) {
	var a RpcFunc[*outer.GetRecommendGroupList_Request, *outer.GetRecommendGroupList_Reply] = ReqTest
	fmt.Println(a)
}
