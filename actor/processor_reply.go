package actor

import (
	"context"
	"github.com/chenxyzl/grain/actor/uuid"
	"github.com/chenxyzl/grain/utils/fun"
	"google.golang.org/protobuf/proto"
	"log/slog"
	"strconv"
	"time"
)

var _ iProcess = (*processorReply[proto.Message])(nil)

type processorReply[T proto.Message] struct {
	slog    *slog.Logger
	system  *System
	_self   *ActorRef
	result  chan T
	timeout time.Duration
}

func newProcessorReplay[T proto.Message](system *System, timeout time.Duration) *processorReply[T] {
	self := NewActorRef(system.clusterProvider.SelfAddr(), "reply/"+strconv.Itoa(int(uuid.Generate())))
	return &processorReply[T]{
		system:  system,
		_self:   self,
		result:  make(chan T, 1),
		timeout: timeout,
		slog:    slog.With("reply", self),
	}
}

func (x *processorReply[T]) self() *ActorRef     { return x._self }
func (x *processorReply[T]) start() error        { return nil }
func (x *processorReply[T]) stop() error         { return nil }
func (x *processorReply[T]) send(ctx IContext)   { x.result <- ctx.Message().(T); x.invoke(ctx) }
func (x *processorReply[T]) invoke(ctx IContext) {}

func (x *processorReply[T]) Result() (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), x.timeout)
	defer func() {
		cancel()
		x.system.registry.remove(x._self)
	}()

	select {
	case resp := <-x.result:
		return resp, nil
	case <-ctx.Done():
		return fun.Zero[T](), ctx.Err()
	}
}
