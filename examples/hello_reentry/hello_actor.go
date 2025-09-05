package main

import (
	"examples/testpb"
	"fmt"
	"time"

	"github.com/chenxyzl/grain"
	"google.golang.org/protobuf/proto"
)

var (
	helloActorA grain.ActorRef
	helloActorB grain.ActorRef
)

type HelloActorA struct{ grain.BaseActor }

func (x *HelloActorA) Started() {
	x.Logger().Info("Started1")
	reply := x.Request(helloActorB, &testpb.HelloRequestA2B{Name: "hello a2b"})
	x.Logger().Info("HelloActorA get reply", "reply", reply)
}
func (x *HelloActorA) PreStop() { x.Logger().Info("PreStop1") }
func (x *HelloActorA) Receive(context grain.Context) {
	switch msg := context.Message().(type) {
	case *testpb.HelloRequestB2A:
		x.Logger().Info("HelloActorA received HelloRequestB2A")
		context.Reply(&testpb.HelloReplyB2A{Name: "HelloReplyB2A"})
		time.Sleep(time.Second * 1)
	case *testpb.Hello: //tell
		x.Logger().Info("HelloActorA recv tell", "message", context.Message())
	default:
		panic(fmt.Sprintf("not register msg type, msgType:%v, msg:%v", proto.MessageName(msg), msg))
	}
}

type HelloActorB struct{ grain.BaseActor }

func (x *HelloActorB) Started() { x.Logger().Info("Started2") }
func (x *HelloActorB) PreStop() { x.Logger().Info("PreStop2") }
func (x *HelloActorB) Receive(context grain.Context) {
	switch msg := context.Message().(type) {
	case *testpb.HelloRequestA2B: //request-reply
		x.Logger().Info("HelloActorB received HelloRequestA2B")
		reply := x.Request(helloActorA, &testpb.HelloRequestB2A{Name: "HelloRequestB2A"})
		x.Logger().Info("HelloActorB get reply", "reply", reply)
		context.Reply(&testpb.HelloReplyA2B{Name: "HelloReplyA2B"})
		time.Sleep(time.Second * 1)
	case *testpb.Hello: //tell
		x.Logger().Info("HelloActorB recv tell", "message", context.Message())
	default:
		panic(fmt.Sprintf("not register msg type, msgType:%v, msg:%v", proto.MessageName(msg), msg))
	}
}
