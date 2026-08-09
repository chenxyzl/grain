package grain

import (
	"google.golang.org/protobuf/proto"
)

type Context interface {
	Target() ActorRef            //目标
	Sender() ActorRef            //发送者
	GetMsgSnId() uint64          //消息序列id
	Message() proto.Message      //消息内容
	Reply(message proto.Message) //返回
	Forward(target ActorRef)     //把当前消息原样转发给 target(保留原 sender, target 可直接回复原发送者)
}
