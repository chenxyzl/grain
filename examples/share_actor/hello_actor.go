package share_actor

import (
	"examples/testpb"
	"fmt"

	"github.com/chenxyzl/grain"
	"google.golang.org/protobuf/proto"
)

type HelloActor struct{ grain.BaseActor }

func (x *HelloActor) Started() { x.Logger().Info("Started") }
func (x *HelloActor) PreStop() { x.Logger().Info("PreStop") }
func (x *HelloActor) Receive(context grain.Context) {
	switch msg := context.Message().(type) {
	case *testpb.HelloAsk: //ask-reply
		x.Logger().Info("recv ask", "message", context.Message())
		context.Reply(&testpb.HelloReply{Name: "reply hello to " + context.Sender().GetName()})
	case *testpb.Hello: //tell
		x.Logger().Info("recv tell", "message", context.Message())
	default:
		panic(fmt.Sprintf("not register msg type, msgType:%v, msg:%v", proto.MessageName(msg), msg))
	}
}
