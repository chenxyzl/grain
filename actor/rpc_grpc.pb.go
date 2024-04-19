// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.3.0
// - protoc             v3.5.1
// source: rpc.proto

package actor

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

const (
	Remoting_Listen_FullMethodName = "/actor.Remoting/Listen"
)

// RemotingClient is the client API for Remoting service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type RemotingClient interface {
	Listen(ctx context.Context, opts ...grpc.CallOption) (Remoting_ListenClient, error)
}

type remotingClient struct {
	cc grpc.ClientConnInterface
}

func NewRemotingClient(cc grpc.ClientConnInterface) RemotingClient {
	return &remotingClient{cc}
}

func (c *remotingClient) Listen(ctx context.Context, opts ...grpc.CallOption) (Remoting_ListenClient, error) {
	stream, err := c.cc.NewStream(ctx, &Remoting_ServiceDesc.Streams[0], Remoting_Listen_FullMethodName, opts...)
	if err != nil {
		return nil, err
	}
	x := &remotingListenClient{stream}
	return x, nil
}

type Remoting_ListenClient interface {
	Send(*Envelope) error
	Recv() (*Envelope, error)
	grpc.ClientStream
}

type remotingListenClient struct {
	grpc.ClientStream
}

func (x *remotingListenClient) Send(m *Envelope) error {
	return x.ClientStream.SendMsg(m)
}

func (x *remotingListenClient) Recv() (*Envelope, error) {
	m := new(Envelope)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// RemotingServer is the server API for Remoting service.
// All implementations must embed UnimplementedRemotingServer
// for forward compatibility
type RemotingServer interface {
	Listen(Remoting_ListenServer) error
	mustEmbedUnimplementedRemotingServer()
}

// UnimplementedRemotingServer must be embedded to have forward compatible implementations.
type UnimplementedRemotingServer struct {
}

func (UnimplementedRemotingServer) Listen(Remoting_ListenServer) error {
	return status.Errorf(codes.Unimplemented, "method Listen not implemented")
}
func (UnimplementedRemotingServer) mustEmbedUnimplementedRemotingServer() {}

// UnsafeRemotingServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to RemotingServer will
// result in compilation errors.
type UnsafeRemotingServer interface {
	mustEmbedUnimplementedRemotingServer()
}

func RegisterRemotingServer(s grpc.ServiceRegistrar, srv RemotingServer) {
	s.RegisterService(&Remoting_ServiceDesc, srv)
}

func _Remoting_Listen_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(RemotingServer).Listen(&remotingListenServer{stream})
}

type Remoting_ListenServer interface {
	Send(*Envelope) error
	Recv() (*Envelope, error)
	grpc.ServerStream
}

type remotingListenServer struct {
	grpc.ServerStream
}

func (x *remotingListenServer) Send(m *Envelope) error {
	return x.ServerStream.SendMsg(m)
}

func (x *remotingListenServer) Recv() (*Envelope, error) {
	m := new(Envelope)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// Remoting_ServiceDesc is the grpc.ServiceDesc for Remoting service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Remoting_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "actor.Remoting",
	HandlerType: (*RemotingServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Listen",
			Handler:       _Remoting_Listen_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "rpc.proto",
}
