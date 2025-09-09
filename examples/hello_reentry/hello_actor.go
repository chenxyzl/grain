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
	reply := x.Ask(helloActorB, &testpb.HelloAskA2B{Name: "hello a2b"})
	x.Logger().Info("HelloActorA get reply", "reply", reply)
}
func (x *HelloActorA) PreStop() { x.Logger().Info("PreStop1") }
func (x *HelloActorA) Receive(context grain.Context) {
	switch msg := context.Message().(type) {
	case *testpb.HelloAskB2A:
		x.Logger().Info("HelloActorA received HelloAskB2A")
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
	case *testpb.HelloAskA2B: //ask-reply
		x.Logger().Info("HelloActorB received HelloAskA2B")
		reply := x.Ask(helloActorA, &testpb.HelloAskB2A{Name: "HelloAskB2A"})
		x.Logger().Info("HelloActorB get reply", "reply", reply)
		context.Reply(&testpb.HelloReplyA2B{Name: "HelloReplyA2B"})
		time.Sleep(time.Second * 1)
	case *testpb.Hello: //tell
		x.Logger().Info("HelloActorB recv tell", "message", context.Message())
	default:
		panic(fmt.Sprintf("not register msg type, msgType:%v, msg:%v", proto.MessageName(msg), msg))
	}
}
