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
	Forward(target ActorRef)     //重定向
}
