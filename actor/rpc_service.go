package actor

import (
	"context"
	share "github.com/chenxyzl/grain/utils/helper"
	"google.golang.org/grpc"
	"io"
	"log/slog"
	"net"
)

type RPCService struct {
}

func (x *RPCService) Listen(server Remoting_ListenServer) error {
	defer share.Recover()
	//save to
	for {
		msg, err := server.Recv()
		if err == io.EOF {
			slog.Info("Remote Close Connection")
		} else {
			slog.Error("Read Failed, Close Connection", "err", err)
		}

		ctx := context.WithValue(context.Background(), "sender", "bar")

		return err
	}
}

func (x *RPCService) mustEmbedUnimplementedRemotingServer() {
	//TODO implement me
	panic("implement me")
}

func (x *RPCService) start() error {
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		return err
	}
	s := grpc.NewServer()
	RegisterRemotingServer(s, x)
	go func() {
		if err = s.Serve(lis); err != nil && err != io.EOF {
			panic(err)
		}
	}()
	return nil
}
