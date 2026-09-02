package grain

import (
	"github.com/chenxyzl/grain/remote"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func (x *system) RecvEnvelope(envelope *remote.Envelope) {
	// Entry point for data from another node, so treat it as untrusted: nil-safe getters only,
	// and reject nil rather than panic on the grpc handler goroutine, which has no recover.
	if envelope == nil {
		x.Logger().Error("recvEnvelope, nil envelope")
		return
	}
	typ, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(envelope.GetMsgName()))
	if err != nil {
		x.Logger().Error("recvEnvelope, unregister msg type", "actor", envelope.GetTarget(), "msgName", envelope.GetMsgName(), "err", err)
		return
	}
	bodyMsg := typ.New().Interface()
	err = proto.Unmarshal(envelope.GetContent(), bodyMsg)
	if err != nil {
		x.Logger().Error("recvEnvelope, msg unmarshal err", "actor", envelope.GetTarget(), "msgName", envelope.GetMsgName(), "err", err)
		return
	}
	// A target is mandatory; without one there is nothing to route to
	if envelope.GetTarget() == "" {
		x.Logger().Error("recvEnvelope, empty target", "msgName", envelope.GetMsgName())
		return
	}
	// A sender is optional, and "" must stay nil, or `ctx.Sender() != nil` is a false positive
	var sender ActorRef
	if s := envelope.GetSender(); s != "" {
		sender = newActorRefFromAID(s, x)
	}
	var target = newActorRefFromAID(envelope.GetTarget(), x)
	x.tellWithSender(target, bodyMsg, sender, envelope.GetMsgSnId())
}
