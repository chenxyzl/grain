package grain

import (
	"context"
	"errors"
	"io"

	remote2 "github.com/chenxyzl/grain/remote"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

//send msg to remote rpcServer
//1.init rpcClient
//2.

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
		// runtime failure (bad address / dial setup): log and stop self instead
		// of panicking. A later send re-spawns the write stream actor.
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
	go func() {
		for {
			unknownMsg, err := stream.Recv()
			switch {
			case errors.Is(err, io.EOF):
				x.Logger().Info("streamWriteActor closed 1", "address", x.address)
			case status.Code(err) == codes.Canceled || status.Code(err) == codes.OK || status.Code(err) == codes.Unavailable:
				x.Logger().Info("streamWriteActor closed 2", "address", x.address, "cod", status.Code(err))
			case status.Code(err) > 0:
				x.Logger().Warn("streamWriteActor closed 3", "address", x.address, "cod", status.Code(err))
			case err != nil:
				x.Logger().Warn("streamWriteActor lost connection with err", "address", x.address, "error", err)
			default:
				x.Logger().Warn("streamWriteActor got a msg form remote, but this stream only for write", "address", x.address, "msg", unknownMsg)
			}
			//only can send, not allowed Recv
			x.GetSystem().Poison(x.Self())
			return
		}
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
	//
	msg := ctx.Message()
	msgName := string(msg.ProtoReflect().Descriptor().FullName())
	//marshal
	content, err := proto.Marshal(msg)
	if err != nil {
		x.Logger().Error("proto marshal err", "from", ctx.Sender(), "target", ctx.Target(), "msgName", msgName, "msg", msg, "err", err)
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
	x.sendToRemote(envelope)
}

func (x *streamWriteActor) sendToRemote(msg *remote2.Envelope) {
	err := x.remote.Send(msg)
	if err != nil {
		_ = x.conn.Close()
		x.GetSystem().Poison(x.Self())
		x.Logger().Error("send envelope err,", "err", err)
	}
}
