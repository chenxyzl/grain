package grain

import (
	"google.golang.org/protobuf/proto"
)

// GetSystem system
func (x *actorIdWrapper) GetSystem() ISystem { return x.system }

// Tell wraps system.tell: fire-and-forget send to this actor.
func (x *actorIdWrapper) Tell(msg proto.Message) {
	x.GetSystem().getSender().tell(x, msg)
}
