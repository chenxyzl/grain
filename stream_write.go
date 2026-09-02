package grain

import (
	"context"
	"errors"
	"io"

	"github.com/chenxyzl/grain/message"
	remote2 "github.com/chenxyzl/grain/remote"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

//send msg to remote rpcServer over one grpc stream

var _ IActor = (*streamWriteActor)(nil)

type streamWriteActor struct {
	BaseActor
	router      ActorRef
	address     string
	dialOptions []grpc.DialOption
	callOptions []grpc.CallOption
	//
	conn   *grpc.ClientConn
	remote remote2.Remoting_ListenClient
}

func newStreamWriterActor(router ActorRef, address string, dialOptions []grpc.DialOption, callOptions []grpc.CallOption) IActor {
	return &streamWriteActor{
		router:      router,
		address:     address,
		dialOptions: dialOptions,
		callOptions: callOptions,
	}
}

func (x *streamWriteActor) Started() {
	conn, err := grpc.NewClient(x.address, x.dialOptions...)
	if err != nil {
		// stop self instead of panicking; a later send re-spawns this actor
		x.Logger().Error("connect to grpc server err, stop self", "addr", x.address, "err", err)
		x.GetSystem().Poison(x.Self())
		return
	}
	stream, err := remote2.NewRemotingClient(conn).Listen(context.Background(), x.callOptions...)
	if err != nil {
		_ = conn.Close()
		x.Logger().Error("listen to grpc server err, stop self", "addr", x.address, "err", err)
		x.GetSystem().Poison(x.Self())
		return
	}
	x.conn = conn
	x.remote = stream
	// One Recv, then stop: the stream is write-only, so any return from Recv is terminal —
	// either the stream died or the peer sent something it never should have.
	go func() {
		unknownMsg, err := stream.Recv()
		switch {
		// err == nil MUST be tested first: status.Code(nil) is codes.OK, so a lower arm would
		// swallow it and log a protocol violation as a routine close.
		case err == nil:
			x.Logger().Warn("streamWriteActor got a msg from remote, but this stream is write-only",
				"address", x.address, "unexpectedMsg", unknownMsg)
		case errors.Is(err, io.EOF):
			x.Logger().Info("streamWriteActor stream closed by peer", "address", x.address)
		case status.Code(err) == codes.Canceled || status.Code(err) == codes.Unavailable:
			x.Logger().Info("streamWriteActor stream closed", "address", x.address,
				"code", status.Code(err))
		default:
			x.Logger().Warn("streamWriteActor stream failed", "address", x.address,
				"code", status.Code(err), "err", err)
		}
		x.GetSystem().Poison(x.Self())
	}()
}

func (x *streamWriteActor) PreStop() {
	if x.remote != nil {
		err := x.remote.CloseSend()
		if err != nil {
			x.Logger().Error("streamWriteActor close grpc send stream err", "error", err)
		}
		x.remote = nil
	}
	if x.conn != nil {
		err := x.conn.Close()
		if err != nil {
			x.Logger().Error("streamWriteActor close grpc conn err", "error", err)
		}
		x.conn = nil
	}
	x.Logger().Info("streamWriteActor Stoped ...")
}

func (x *streamWriteActor) Receive(ctx Context) {
	msg := ctx.Message()
	// The stream may never have opened: Started() self-poisons on a dial/Listen failure, but
	// poison() only sets a flag, so the run loop still drains everything already queued. Without
	// this guard each of those hit a nil x.remote, panicked, and was swallowed by invoke's
	// recover — never sent, never dead-lettered, and any waiting Ask burned its full askTimeout.
	// Listen() does not set WaitForReady, so this is simply what happens when a peer is down.
	if x.remote == nil {
		x.deadLetter(ctx, DeadLetterReasonUnreachable)
		return
	}
	msgName := string(proto.MessageName(msg))
	content, err := proto.Marshal(msg)
	if err != nil {
		x.Logger().Error("proto marshal err", "from", ctx.Sender(), "target", ctx.Target(), "msgName", msgName, "msg", msg, "err", err)
		x.deadLetter(ctx, "proto marshal failed: "+err.Error())
		return
	}
	var sender string
	if ctx.Sender() != nil {
		sender = ctx.Sender().GetId()
	}
	envelope := &remote2.Envelope{
		Header:  nil,
		Sender:  sender, //ctx.GetSender().GetId()//panic if sender is nil
		Target:  ctx.Target().GetId(),
		MsgSnId: ctx.GetMsgSnId(),
		MsgName: msgName,
		Content: content,
	}
	x.sendToRemote(ctx, envelope)
}

// deadLetter reports a message this stream could not carry, failing a waiting Ask at once so it
// does not sit out askTimeout for a peer we already know we cannot reach.
func (x *streamWriteActor) deadLetter(ctx Context, reason string) {
	if s := ctx.Sender(); s != nil && s.isAsk() {
		s.Tell(message.WithErr("send to remote failed: " + reason))
	}
	x.GetSystem().emitDeadLetter(DeadLetter{
		Target:  ctx.Target(),
		Owner:   x.Self(),
		Sender:  ctx.Sender(),
		Message: ctx.Message(),
		MsgSnId: ctx.GetMsgSnId(),
		Reason:  reason,
	})
}

func (x *streamWriteActor) sendToRemote(ctx Context, msg *remote2.Envelope) {
	err := x.remote.Send(msg)
	if err != nil {
		x.Logger().Error("send envelope err,", "err", err)
		x.deadLetter(ctx, DeadLetterReasonUnreachable)
		// nil-checked: Started() can leave conn unset, and PreStop may already have closed it
		if x.conn != nil {
			_ = x.conn.Close()
		}
		x.GetSystem().Poison(x.Self())
	}
}
