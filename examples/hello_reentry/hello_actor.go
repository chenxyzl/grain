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

// Started deliberately does NOT Ask. Reentrancy is off during Started() —
// yieldTurn skips the successor-drainer handoff while life == lifeStarting, so no
// handler runs against half-initialized state — which also means the actor cannot
// answer incoming requests there. Ask therefore refuses outright from Started()
// and returns message.CodeAskNotRunning. See docs/reentrancy.md §九.
//
// The a->b->a cycle is therefore kicked off from a normal handler below.
func (x *HelloActorA) Started() {
	x.Logger().Info("Started1")
}
func (x *HelloActorA) PreStop() { x.Logger().Info("PreStop1") }
func (x *HelloActorA) Receive(context grain.Context) {
	switch msg := context.Message().(type) {
	case *testpb.HelloAskB2A:
		x.Logger().Info("HelloActorA received HelloAskB2A")
		context.Reply(&testpb.HelloReplyB2A{Name: "HelloReplyB2A"})
		time.Sleep(time.Second * 1)
	case *testpb.Hello: //tell — kicks off the a->b->a cycle from a normal handler
		x.Logger().Info("HelloActorA recv tell", "message", context.Message())
		reply, err := x.Ask[*testpb.HelloReplyA2B](helloActorB, &testpb.HelloAskA2B{Name: "hello a2b"})
		if err != nil {
			x.Logger().Error("HelloActorA ask err", "err", err)
			return
		}
		x.Logger().Info("HelloActorA get reply", "reply", reply)
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
		reply, err := x.Ask[*testpb.HelloReplyB2A](helloActorA, &testpb.HelloAskB2A{Name: "HelloAskB2A"})
		if err != nil {
			x.Logger().Error("HelloActorB ask err", "err", err)
			return
		}
		x.Logger().Info("HelloActorB get reply", "reply", reply)
		context.Reply(&testpb.HelloReplyA2B{Name: "HelloReplyA2B"})
		time.Sleep(time.Second * 1)
	case *testpb.Hello: //tell
		x.Logger().Info("HelloActorB recv tell", "message", context.Message())
	default:
		panic(fmt.Sprintf("not register msg type, msgType:%v, msg:%v", proto.MessageName(msg), msg))
	}
}
