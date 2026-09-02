package grain

import (
	"google.golang.org/protobuf/proto"
)

func (x *actorIdWrapper) GetSystem() ISystem { return x.system }

// Tell is a fire-and-forget send to this actor.
func (x *actorIdWrapper) Tell(msg proto.Message) {
	x.GetSystem().getSender().tell(x, msg)
}
