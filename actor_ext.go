package grain

import (
	"google.golang.org/protobuf/proto"
)

// GetSystem system
func (x *actorIdWrapper) GetSystem() ISystem { return x.system }

// Send wrapper system.tell
func (x *actorIdWrapper) Send(msg proto.Message) {
	x.GetSystem().getSender().tell(x, msg)
}
func (x *actorIdWrapper) NoReentryAsk(target ActorRef, req proto.Message) proto.Message {
	ret, err := NoReentryAsk[proto.Message](target, req)
	if err != nil {
		panic(err)
	}
	return ret
}
func (x *actorIdWrapper) NoReentryAskE(target ActorRef, req proto.Message) (proto.Message, error) {
	return NoReentryAsk[proto.Message](target, req)
}
