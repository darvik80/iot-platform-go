// Package pluginpb defines the gRPC plugin protocol types and service.
// Hand-written equivalent of protoc-gen-go + protoc-gen-go-grpc output.
package pluginpb

import (
	context "context"
	fmt "fmt"

	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
)

// ─── Messages ────────────────────────────────────────────────────────────────

type OnEventRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	BindingId int64  `protobuf:"varint,1,opt,name=binding_id,proto3" json:"binding_id,omitempty"`
	DevicePk  int64  `protobuf:"varint,2,opt,name=device_pk,proto3" json:"device_pk,omitempty"`
	DeviceId  string `protobuf:"bytes,3,opt,name=device_id,proto3" json:"device_id,omitempty"`
	ProductId string `protobuf:"bytes,4,opt,name=product_id,proto3" json:"product_id,omitempty"`
	Kind      string `protobuf:"bytes,5,opt,name=kind,proto3" json:"kind,omitempty"`
	Payload   []byte `protobuf:"bytes,6,opt,name=payload,proto3" json:"payload,omitempty"`
	Online    bool   `protobuf:"varint,7,opt,name=online,proto3" json:"online,omitempty"`
	Timestamp int64  `protobuf:"varint,8,opt,name=timestamp,proto3" json:"timestamp,omitempty"`
}

func (x *OnEventRequest) Reset()                             { *x = OnEventRequest{} }
func (x *OnEventRequest) String() string                     { return fmt.Sprintf("%+v", *x) }
func (x *OnEventRequest) ProtoMessage()                      {}
func (x *OnEventRequest) ProtoReflect() protoreflect.Message { return nil }
func (x *OnEventRequest) GetBindingId() int64                { return x.BindingId }
func (x *OnEventRequest) GetDevicePk() int64                 { return x.DevicePk }
func (x *OnEventRequest) GetDeviceId() string                { return x.DeviceId }
func (x *OnEventRequest) GetProductId() string               { return x.ProductId }
func (x *OnEventRequest) GetKind() string                    { return x.Kind }
func (x *OnEventRequest) GetPayload() []byte                 { return x.Payload }
func (x *OnEventRequest) GetOnline() bool                    { return x.Online }
func (x *OnEventRequest) GetTimestamp() int64                { return x.Timestamp }

type OnEventResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Ok    bool   `protobuf:"varint,1,opt,name=ok,proto3" json:"ok,omitempty"`
	Error string `protobuf:"bytes,2,opt,name=error,proto3" json:"error,omitempty"`
}

func (x *OnEventResponse) Reset()                             { *x = OnEventResponse{} }
func (x *OnEventResponse) String() string                     { return fmt.Sprintf("%+v", *x) }
func (x *OnEventResponse) ProtoMessage()                      {}
func (x *OnEventResponse) ProtoReflect() protoreflect.Message { return nil }
func (x *OnEventResponse) GetOk() bool                        { return x.Ok }
func (x *OnEventResponse) GetError() string                   { return x.Error }

type ApplyConfigRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	BindingId int64  `protobuf:"varint,1,opt,name=binding_id,proto3" json:"binding_id,omitempty"`
	DevicePk  int64  `protobuf:"varint,2,opt,name=device_pk,proto3" json:"device_pk,omitempty"`
	DeviceId  string `protobuf:"bytes,3,opt,name=device_id,proto3" json:"device_id,omitempty"`
	ProductId string `protobuf:"bytes,4,opt,name=product_id,proto3" json:"product_id,omitempty"`
	Config    []byte `protobuf:"bytes,5,opt,name=config,proto3" json:"config,omitempty"`
}

func (x *ApplyConfigRequest) Reset()                             { *x = ApplyConfigRequest{} }
func (x *ApplyConfigRequest) String() string                     { return fmt.Sprintf("%+v", *x) }
func (x *ApplyConfigRequest) ProtoMessage()                      {}
func (x *ApplyConfigRequest) ProtoReflect() protoreflect.Message { return nil }
func (x *ApplyConfigRequest) GetBindingId() int64                { return x.BindingId }
func (x *ApplyConfigRequest) GetDevicePk() int64                 { return x.DevicePk }
func (x *ApplyConfigRequest) GetDeviceId() string                { return x.DeviceId }
func (x *ApplyConfigRequest) GetProductId() string               { return x.ProductId }
func (x *ApplyConfigRequest) GetConfig() []byte                  { return x.Config }

type ApplyConfigResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Ok    bool   `protobuf:"varint,1,opt,name=ok,proto3" json:"ok,omitempty"`
	Error string `protobuf:"bytes,2,opt,name=error,proto3" json:"error,omitempty"`
}

func (x *ApplyConfigResponse) Reset()                             { *x = ApplyConfigResponse{} }
func (x *ApplyConfigResponse) String() string                     { return fmt.Sprintf("%+v", *x) }
func (x *ApplyConfigResponse) ProtoMessage()                      {}
func (x *ApplyConfigResponse) ProtoReflect() protoreflect.Message { return nil }
func (x *ApplyConfigResponse) GetOk() bool                        { return x.Ok }
func (x *ApplyConfigResponse) GetError() string                   { return x.Error }

type DetachRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	BindingId int64 `protobuf:"varint,1,opt,name=binding_id,proto3" json:"binding_id,omitempty"`
	DevicePk  int64 `protobuf:"varint,2,opt,name=device_pk,proto3" json:"device_pk,omitempty"`
}

func (x *DetachRequest) Reset()                             { *x = DetachRequest{} }
func (x *DetachRequest) String() string                     { return fmt.Sprintf("%+v", *x) }
func (x *DetachRequest) ProtoMessage()                      {}
func (x *DetachRequest) ProtoReflect() protoreflect.Message { return nil }
func (x *DetachRequest) GetBindingId() int64                { return x.BindingId }
func (x *DetachRequest) GetDevicePk() int64                 { return x.DevicePk }

type DetachResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Ok    bool   `protobuf:"varint,1,opt,name=ok,proto3" json:"ok,omitempty"`
	Error string `protobuf:"bytes,2,opt,name=error,proto3" json:"error,omitempty"`
}

func (x *DetachResponse) Reset()                             { *x = DetachResponse{} }
func (x *DetachResponse) String() string                     { return fmt.Sprintf("%+v", *x) }
func (x *DetachResponse) ProtoMessage()                      {}
func (x *DetachResponse) ProtoReflect() protoreflect.Message { return nil }
func (x *DetachResponse) GetOk() bool                        { return x.Ok }
func (x *DetachResponse) GetError() string                   { return x.Error }

type PingRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *PingRequest) Reset()                             { *x = PingRequest{} }
func (x *PingRequest) String() string                     { return fmt.Sprintf("%+v", *x) }
func (x *PingRequest) ProtoMessage()                      {}
func (x *PingRequest) ProtoReflect() protoreflect.Message { return nil }

type PingResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Ok              bool   `protobuf:"varint,1,opt,name=ok,proto3" json:"ok,omitempty"`
	Version         string `protobuf:"bytes,2,opt,name=version,proto3" json:"version,omitempty"`
	Status          string `protobuf:"bytes,3,opt,name=status,proto3" json:"status,omitempty"`
	EventsProcessed int64  `protobuf:"varint,4,opt,name=events_processed,proto3" json:"events_processed,omitempty"`
	UptimeSec       int64  `protobuf:"varint,5,opt,name=uptime_sec,proto3" json:"uptime_sec,omitempty"`
}

func (x *PingResponse) Reset()                             { *x = PingResponse{} }
func (x *PingResponse) String() string                     { return fmt.Sprintf("%+v", *x) }
func (x *PingResponse) ProtoMessage()                      {}
func (x *PingResponse) ProtoReflect() protoreflect.Message { return nil }
func (x *PingResponse) GetOk() bool                        { return x.Ok }
func (x *PingResponse) GetVersion() string                 { return x.Version }
func (x *PingResponse) GetStatus() string                  { return x.Status }
func (x *PingResponse) GetEventsProcessed() int64          { return x.EventsProcessed }
func (x *PingResponse) GetUptimeSec() int64                { return x.UptimeSec }

// ─── Service ─────────────────────────────────────────────────────────────────

// PluginServiceClient is the client API for PluginService.
type PluginServiceClient interface {
	OnEvent(ctx context.Context, in *OnEventRequest, opts ...grpc.CallOption) (*OnEventResponse, error)
	ApplyConfig(ctx context.Context, in *ApplyConfigRequest, opts ...grpc.CallOption) (*ApplyConfigResponse, error)
	Detach(ctx context.Context, in *DetachRequest, opts ...grpc.CallOption) (*DetachResponse, error)
	Ping(ctx context.Context, in *PingRequest, opts ...grpc.CallOption) (*PingResponse, error)
}

type pluginServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewPluginServiceClient(cc grpc.ClientConnInterface) PluginServiceClient {
	return &pluginServiceClient{cc}
}

const serviceName = "/plugin.PluginService/"

func (c *pluginServiceClient) OnEvent(ctx context.Context, in *OnEventRequest, opts ...grpc.CallOption) (*OnEventResponse, error) {
	out := new(OnEventResponse)
	err := c.cc.Invoke(ctx, serviceName+"OnEvent", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *pluginServiceClient) ApplyConfig(ctx context.Context, in *ApplyConfigRequest, opts ...grpc.CallOption) (*ApplyConfigResponse, error) {
	out := new(ApplyConfigResponse)
	err := c.cc.Invoke(ctx, serviceName+"ApplyConfig", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *pluginServiceClient) Detach(ctx context.Context, in *DetachRequest, opts ...grpc.CallOption) (*DetachResponse, error) {
	out := new(DetachResponse)
	err := c.cc.Invoke(ctx, serviceName+"Detach", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *pluginServiceClient) Ping(ctx context.Context, in *PingRequest, opts ...grpc.CallOption) (*PingResponse, error) {
	out := new(PingResponse)
	err := c.cc.Invoke(ctx, serviceName+"Ping", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ─── Server ──────────────────────────────────────────────────────────────────

// PluginServiceServer is the server API for PluginService.
type PluginServiceServer interface {
	OnEvent(context.Context, *OnEventRequest) (*OnEventResponse, error)
	ApplyConfig(context.Context, *ApplyConfigRequest) (*ApplyConfigResponse, error)
	Detach(context.Context, *DetachRequest) (*DetachResponse, error)
	Ping(context.Context, *PingRequest) (*PingResponse, error)
}

// UnimplementedPluginService can be embedded to have forward compatible implementations.
type UnimplementedPluginService struct{}

func (*UnimplementedPluginService) OnEvent(context.Context, *OnEventRequest) (*OnEventResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method OnEvent not implemented")
}
func (*UnimplementedPluginService) ApplyConfig(context.Context, *ApplyConfigRequest) (*ApplyConfigResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ApplyConfig not implemented")
}
func (*UnimplementedPluginService) Detach(context.Context, *DetachRequest) (*DetachResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Detach not implemented")
}
func (*UnimplementedPluginService) Ping(context.Context, *PingRequest) (*PingResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Ping not implemented")
}

// RegisterPluginServiceServer registers the service with a gRPC server.
func RegisterPluginServiceServer(s *grpc.Server, srv PluginServiceServer) {
	s.RegisterService(&_PluginService_serviceDesc, srv)
}

func _PluginService_OnEvent_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(OnEventRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PluginServiceServer).OnEvent(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PluginServiceServer).OnEvent(ctx, req.(*OnEventRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _PluginService_ApplyConfig_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ApplyConfigRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PluginServiceServer).ApplyConfig(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PluginServiceServer).ApplyConfig(ctx, req.(*ApplyConfigRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _PluginService_Detach_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DetachRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PluginServiceServer).Detach(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PluginServiceServer).Detach(ctx, req.(*DetachRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _PluginService_Ping_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(PingRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PluginServiceServer).Ping(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PluginServiceServer).Ping(ctx, req.(*PingRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _PluginService_serviceDesc = grpc.ServiceDesc{
	ServiceName: "plugin.PluginService",
	HandlerType: (*PluginServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "OnEvent", Handler: _PluginService_OnEvent_Handler},
		{MethodName: "ApplyConfig", Handler: _PluginService_ApplyConfig_Handler},
		{MethodName: "Detach", Handler: _PluginService_Detach_Handler},
		{MethodName: "Ping", Handler: _PluginService_Ping_Handler},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "plugin.proto",
}
