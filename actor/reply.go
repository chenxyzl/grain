package actor

import (
	"context"
	"github.com/chenxyzl/grain/actor/uuid"
	"github.com/chenxyzl/grain/utils/fun"
	"google.golang.org/protobuf/proto"
	"strconv"
	"time"
)

type replyProcessor[T proto.Message] struct {
	system  *System
	self    *ActorRef
	result  chan T
	timeout time.Duration
}

func newReplayProcessor[T proto.Message](system *System, timeout time.Duration) *replyProcessor[T] {
	return &replyProcessor[T]{system: system, self: NewActorRef(system.clusterProvider.SelfAddr(), "reply/"+strconv.Itoa(int(uuid.Generate()))), timeout: timeout}
}

// add to mail box
func (x *replyProcessor[T]) receive(ctx IContext) {
	x.Receiver(ctx)
}

func (x *replyProcessor[T]) Receiver(ctx IContext) {
	x.result <- ctx.Message().(T)
}
func (x *replyProcessor[T]) Result() (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), x.timeout)
	defer func() {
		cancel()
		x.system.registry.Remove(x.self)
	}()

	select {
	case resp := <-x.result:
		return resp, nil
	case <-ctx.Done():
		return fun.Zero[T](), ctx.Err()
	}
}
