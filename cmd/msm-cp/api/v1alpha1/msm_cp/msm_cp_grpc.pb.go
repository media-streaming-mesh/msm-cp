// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

package msm_cp

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

// MsmControlPlaneClient is the client API for MsmControlPlane service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type MsmControlPlaneClient interface {
	Send(ctx context.Context, opts ...grpc.CallOption) (MsmControlPlane_SendClient, error)
}

type msmControlPlaneClient struct {
	cc grpc.ClientConnInterface
}

func NewMsmControlPlaneClient(cc grpc.ClientConnInterface) MsmControlPlaneClient {
	return &msmControlPlaneClient{cc}
}

func (c *msmControlPlaneClient) Send(ctx context.Context, opts ...grpc.CallOption) (MsmControlPlane_SendClient, error) {
	stream, err := c.cc.NewStream(ctx, &MsmControlPlane_ServiceDesc.Streams[0], "/msm_cp.MsmControlPlane/Send", opts...)
	if err != nil {
		return nil, err
	}
	x := &msmControlPlaneSendClient{stream}
	return x, nil
}

type MsmControlPlane_SendClient interface {
	Send(*Message) error
	Recv() (*Message, error)
	grpc.ClientStream
}

type msmControlPlaneSendClient struct {
	grpc.ClientStream
}

func (x *msmControlPlaneSendClient) Send(m *Message) error {
	return x.ClientStream.SendMsg(m)
}

func (x *msmControlPlaneSendClient) Recv() (*Message, error) {
	m := new(Message)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// MsmControlPlaneServer is the server API for MsmControlPlane service.
// All implementations should embed UnimplementedMsmControlPlaneServer
// for forward compatibility
type MsmControlPlaneServer interface {
	Send(MsmControlPlane_SendServer) error
}

// UnimplementedMsmControlPlaneServer should be embedded to have forward compatible implementations.
type UnimplementedMsmControlPlaneServer struct {
}

func (UnimplementedMsmControlPlaneServer) Send(MsmControlPlane_SendServer) error {
	return status.Errorf(codes.Unimplemented, "method Send not implemented")
}

// UnsafeMsmControlPlaneServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to MsmControlPlaneServer will
// result in compilation errors.
type UnsafeMsmControlPlaneServer interface {
	mustEmbedUnimplementedMsmControlPlaneServer()
}

func RegisterMsmControlPlaneServer(s grpc.ServiceRegistrar, srv MsmControlPlaneServer) {
	s.RegisterService(&MsmControlPlane_ServiceDesc, srv)
}

func _MsmControlPlane_Send_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(MsmControlPlaneServer).Send(&msmControlPlaneSendServer{stream})
}

type MsmControlPlane_SendServer interface {
	Send(*Message) error
	Recv() (*Message, error)
	grpc.ServerStream
}

type msmControlPlaneSendServer struct {
	grpc.ServerStream
}

func (x *msmControlPlaneSendServer) Send(m *Message) error {
	return x.ServerStream.SendMsg(m)
}

func (x *msmControlPlaneSendServer) Recv() (*Message, error) {
	m := new(Message)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// MsmControlPlane_ServiceDesc is the grpc.ServiceDesc for MsmControlPlane service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var MsmControlPlane_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "msm_cp.MsmControlPlane",
	HandlerType: (*MsmControlPlaneServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Send",
			Handler:       _MsmControlPlane_Send_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "msm_cp.proto",
}
