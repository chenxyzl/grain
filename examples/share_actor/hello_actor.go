package share_actor

import (
	"examples/testpb"
	"fmt"
	"github.com/chenxyzl/grain/actor"
	"google.golang.org/protobuf/proto"
)

type HelloActor struct{ actor.BaseActor }

func (x *HelloActor) Started() { x.Logger().Info("Started") }
func (x *HelloActor) PreStop() { x.Logger().Info("PreStop") }
func (x *HelloActor) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *testpb.HelloRequest: //request-reply
		x.Logger().Info("recv request", "message", context.Message())
		context.Reply(&testpb.HelloReply{Name: "reply " + msg.Name})
	case *testpb.Hello: //tell
		x.Logger().Info("recv tell", "message", context.Message())
	default:
		panic(fmt.Sprintf("not register msg type, msgType:%v, msg:%v", proto.MessageName(msg), msg))
	}
}
