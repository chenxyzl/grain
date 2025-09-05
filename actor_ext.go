package grain

import (
	"time"

	"google.golang.org/protobuf/proto"
)

// GetSystem system
func (x *actorIdWrapper) GetSystem() ISystem {
	if x == nil {
		return nil
	}
	return x.system
}

func (x *actorIdWrapper) getNextSnId() uint64 {
	return x.GetSystem().getGenRequestId().genRequestId()
}
func (x *actorIdWrapper) getSender() iSender {
	return x.GetSystem().getSender()
}
func (x *actorIdWrapper) getRegister() iRegister {
	return x.GetSystem().getRegistry()
}
func (x *actorIdWrapper) getConfigRequestTimeout() time.Duration {
	return x.GetSystem().getConfig().requestTimeout
}

func (x *actorIdWrapper) Send(msg proto.Message) {
	x.getSender().tell(x, msg)
}
func (x *actorIdWrapper) NoReentryRequest(target ActorRef, req proto.Message) proto.Message {
	ret, err := NoReentryRequest[proto.Message](target, req)
	if err != nil {
		panic(err)
	}
	return ret
}
func (x *actorIdWrapper) NoReentryRequestE(target ActorRef, req proto.Message) (proto.Message, error) {
	return NoReentryRequest[proto.Message](target, req)
}
