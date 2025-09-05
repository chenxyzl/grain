package grain

import (
	"github.com/chenxyzl/grain/remote"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func (x *system) RecvEnvelope(envelope *remote.Envelope) {
	typ, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(envelope.MsgName))
	if err != nil {
		x.Logger().Error("recvEnvelope, unregister msg type", "actor", envelope.GetTarget(), "msgName", envelope.GetMsgName(), "err", err)
		return
	}
	bodyMsg := typ.New().Interface().(proto.Message)
	err = proto.Unmarshal(envelope.Content, bodyMsg)
	if err != nil {
		x.Logger().Error("recvEnvelope, msg unmarshal err", "actor", envelope.GetTarget(), "msgName", envelope.GetMsgName(), "err", err)
		return
	}
	//build ctx
	x.tellWithSender(newActorRefFromAID(envelope.GetTarget(), x), bodyMsg, newActorRefFromAID(envelope.GetSender(), x), envelope.GetMsgSnId())
}
