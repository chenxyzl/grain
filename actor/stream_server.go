package actor

import (
	"github.com/chenxyzl/grain/utils/helper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"io"
	"log/slog"
	"net"
	"time"
)

type rpcService struct {
	senderFunc Sender
	//
	logger *slog.Logger
	addr   string
	gs     *grpc.Server
}

func newRpcServer(senderFunc Sender) *rpcService {
	return &rpcService{senderFunc: senderFunc}
}

func (x *rpcService) Listen(server Remoting_ListenServer) error {
	defer helper.Recover()
	//save to
	for {
		msg, err := server.Recv()
		switch {
		case err == io.EOF:
			x.Logger().Info("connection closed 1")
			return nil
		case status.Code(err) == codes.Canceled:
			x.Logger().Info("connection closed 2")
			return nil
		case status.Code(err) > 0:
			x.Logger().Info("connection closed 3", "cod", status.Code(err))
		case err != nil:
			x.Logger().Error("read failed, close connection", "err", err)
			return err
		default:
			//do something left
		}
		if msg.GetTarget().GetAddress() != x.addr {
			x.Logger().Warn("target address is not match", "target_address", msg.GetTarget().GetAddress(), "self_address", x.addr)
			continue
		}
		//get proc
		x.sendToLocal(msg)
	}
}

func (x *rpcService) mustEmbedUnimplementedRemotingServer() {
	x.Logger().Info("mustEmbedUnimplementedRemotingServer")
}

func (x *rpcService) Start() error {
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		return err
	}
	x.addr = lis.Addr().String()
	x.logger = slog.With("rpcService", x.addr)
	x.gs = grpc.NewServer()
	RegisterRemotingServer(x.gs, x)
	go func() {
		if err = x.gs.Serve(lis); err != nil && err != io.EOF {
			panic(err)
		}
	}()
	return nil
}

func (x *rpcService) Stop() error {
	if x.gs != nil {
		c := make(chan bool, 1)
		go func() {
			x.gs.GracefulStop()
			c <- true
		}()

		select {
		case <-c:
			x.Logger().Info("Stopped Proto.Actor server")
		case <-time.After(time.Second * 10):
			x.gs.Stop()
			x.Logger().Info("Stopped Proto.Actor server", "err", "timeout")
		}
	}
	return nil
}

func (x *rpcService) Addr() string {
	return x.addr
}

func (x *rpcService) Logger() *slog.Logger {
	return x.logger
}

func (x *rpcService) sendToLocal(envelope *Envelope) {
	typ, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(envelope.MsgName))
	if err != nil {
		x.Logger().Error("sendToLocal, unregister msg type", "actor", envelope.GetTarget(), "msgName", envelope.GetMsgName(), "err", err)
		return
	}
	bodyMsg := typ.New().Interface().(proto.Message)
	err = proto.Unmarshal(envelope.Content, bodyMsg)
	if err != nil {
		x.Logger().Error("sendToLocal, msg unmarshal err", "actor", envelope.GetTarget(), "msgName", envelope.GetMsgName(), "err", err)
		return
	}
	//build ctx
	x.senderFunc(envelope.GetTarget(), bodyMsg, envelope.GetSender(), envelope.GetMsgSnId())
}
