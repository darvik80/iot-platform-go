// Telemetry Logger — gRPC External Plugin
//
// Implements the PluginService gRPC interface. The platform connects
// to this plugin as a gRPC client to send device events.
//
// Usage:
//
//	go run telemetry_logger_grpc.go --port 50051
//
// Or with Unix socket:
//
//	go run telemetry_logger_grpc.go --socket /tmp/plugin.sock
//
// Dependencies:
//   - google.golang.org/grpc
//   - google.golang.org/protobuf
//
// The plugin:
// 1. Starts gRPC server on specified port/socket
// 2. Implements OnEvent, ApplyConfig, Detach, Ping
// 3. Logs all events to console
//
// Note: This is a standalone example. For production use with
// hashicorp/go-plugin, see the reattach pattern.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// ─── Generated types (inline for example — normally from protoc) ─────────────

// In production, run: protoc --go_out=. --go-grpc_out=. plugin.proto

// PluginServiceServer is the server API for PluginService.
type PluginServiceServer interface {
	OnEvent(context.Context, *OnEventRequest) (*OnEventResponse, error)
	ApplyConfig(context.Context, *ApplyConfigRequest) (*ApplyConfigResponse, error)
	Detach(context.Context, *DetachRequest) (*DetachResponse, error)
	Ping(context.Context, *PingRequest) (*PingResponse, error)
}

type OnEventRequest struct {
	BindingId int64  `protobuf:"varint,1,opt,name=binding_id,json=bindingId,proto3" json:"binding_id,omitempty"`
	DevicePk  int64  `protobuf:"varint,2,opt,name=device_pk,json=devicePk,proto3" json:"device_pk,omitempty"`
	DeviceId  string `protobuf:"bytes,3,opt,name=device_id,json=deviceId,proto3" json:"device_id,omitempty"`
	ProductId string `protobuf:"bytes,4,opt,name=product_id,json=productId,proto3" json:"product_id,omitempty"`
	Kind      string `protobuf:"bytes,5,opt,name=kind,proto3" json:"kind,omitempty"`
	Payload   []byte `protobuf:"bytes,6,opt,name=payload,proto3" json:"payload,omitempty"`
	Online    bool   `protobuf:"varint,7,opt,name=online,proto3" json:"online,omitempty"`
	Timestamp int64  `protobuf:"varint,8,opt,name=timestamp,proto3" json:"timestamp,omitempty"`
}

func (m *OnEventRequest) ProtoMessage()  {}
func (m *OnEventRequest) Reset()         { *m = OnEventRequest{} }
func (m *OnEventRequest) String() string { return fmt.Sprintf("%+v", *m) }

type OnEventResponse struct {
	Ok    bool   `protobuf:"varint,1,opt,name=ok,proto3" json:"ok,omitempty"`
	Error string `protobuf:"bytes,2,opt,name=error,proto3" json:"error,omitempty"`
}

func (m *OnEventResponse) ProtoMessage()  {}
func (m *OnEventResponse) Reset()         { *m = OnEventResponse{} }
func (m *OnEventResponse) String() string { return fmt.Sprintf("%+v", *m) }

type ApplyConfigRequest struct {
	BindingId int64  `protobuf:"varint,1,opt,name=binding_id,json=bindingId,proto3" json:"binding_id,omitempty"`
	DevicePk  int64  `protobuf:"varint,2,opt,name=device_pk,json=devicePk,proto3" json:"device_pk,omitempty"`
	DeviceId  string `protobuf:"bytes,3,opt,name=device_id,json=deviceId,proto3" json:"device_id,omitempty"`
	ProductId string `protobuf:"bytes,4,opt,name=product_id,json=productId,proto3" json:"product_id,omitempty"`
	Config    []byte `protobuf:"bytes,5,opt,name=config,proto3" json:"config,omitempty"`
}

func (m *ApplyConfigRequest) ProtoMessage()  {}
func (m *ApplyConfigRequest) Reset()         { *m = ApplyConfigRequest{} }
func (m *ApplyConfigRequest) String() string { return fmt.Sprintf("%+v", *m) }

type ApplyConfigResponse struct {
	Ok    bool   `protobuf:"varint,1,opt,name=ok,proto3" json:"ok,omitempty"`
	Error string `protobuf:"bytes,2,opt,name=error,proto3" json:"error,omitempty"`
}

func (m *ApplyConfigResponse) ProtoMessage()  {}
func (m *ApplyConfigResponse) Reset()         { *m = ApplyConfigResponse{} }
func (m *ApplyConfigResponse) String() string { return fmt.Sprintf("%+v", *m) }

type DetachRequest struct {
	BindingId int64 `protobuf:"varint,1,opt,name=binding_id,json=bindingId,proto3" json:"binding_id,omitempty"`
	DevicePk  int64 `protobuf:"varint,2,opt,name=device_pk,json=devicePk,proto3" json:"device_pk,omitempty"`
}

func (m *DetachRequest) ProtoMessage()  {}
func (m *DetachRequest) Reset()         { *m = DetachRequest{} }
func (m *DetachRequest) String() string { return fmt.Sprintf("%+v", *m) }

type DetachResponse struct {
	Ok    bool   `protobuf:"varint,1,opt,name=ok,proto3" json:"ok,omitempty"`
	Error string `protobuf:"bytes,2,opt,name=error,proto3" json:"error,omitempty"`
}

func (m *DetachResponse) ProtoMessage()  {}
func (m *DetachResponse) Reset()         { *m = DetachResponse{} }
func (m *DetachResponse) String() string { return fmt.Sprintf("%+v", *m) }

type PingRequest struct{}

func (m *PingRequest) ProtoMessage()  {}
func (m *PingRequest) Reset()         { *m = PingRequest{} }
func (m *PingRequest) String() string { return fmt.Sprintf("%+v", *m) }

type PingResponse struct {
	Ok              bool   `protobuf:"varint,1,opt,name=ok,proto3" json:"ok,omitempty"`
	Version         string `protobuf:"bytes,2,opt,name=version,proto3" json:"version,omitempty"`
	Status          string `protobuf:"bytes,3,opt,name=status,proto3" json:"status,omitempty"`
	EventsProcessed int64  `protobuf:"varint,4,opt,name=events_processed,json=eventsProcessed,proto3" json:"events_processed,omitempty"`
	UptimeSec       int64  `protobuf:"varint,5,opt,name=uptime_sec,json=uptimeSec,proto3" json:"uptime_sec,omitempty"`
}

func (m *PingResponse) ProtoMessage()  {}
func (m *PingResponse) Reset()         { *m = PingResponse{} }
func (m *PingResponse) String() string { return fmt.Sprintf("%+v", *m) }

// ─── Plugin Implementation ───────────────────────────────────────────────────

type TelemetryLoggerPlugin struct {
	startTime   time.Time
	eventsCount int64
	mu          sync.RWMutex
}

func NewTelemetryLoggerPlugin() *TelemetryLoggerPlugin {
	return &TelemetryLoggerPlugin{
		startTime: time.Now(),
	}
}

func (p *TelemetryLoggerPlugin) OnEvent(ctx context.Context, req *OnEventRequest) (*OnEventResponse, error) {
	p.mu.Lock()
	p.eventsCount++
	count := p.eventsCount
	p.mu.Unlock()

	ts := time.Now().Format("15:04:05")

	// Parse payload
	var payloadObj any
	if len(req.Payload) > 0 {
		_ = json.Unmarshal(req.Payload, &payloadObj)
	}

	switch req.Kind {
	case "telemetry":
		log.Printf("📊 [%s] Device #%d (binding %d) — event #%d:", ts, req.DevicePk, req.BindingId, count)
		if m, ok := payloadObj.(map[string]any); ok {
			if metrics, exists := m["metrics"]; exists {
				if metricsMap, ok := metrics.(map[string]any); ok {
					for k, v := range metricsMap {
						log.Printf("   %s: %v", k, v)
					}
				}
			} else {
				for k, v := range m {
					log.Printf("   %s: %v", k, v)
				}
			}
		}
	case "status":
		status := "offline"
		if req.Online {
			status = "online"
		}
		log.Printf("🔌 [%s] Device #%d: %s", ts, req.DevicePk, status)
	case "event":
		log.Printf("⚡ [%s] Device #%d event: %s", ts, req.DevicePk, string(req.Payload))
	case "log":
		log.Printf("📝 [%s] Device #%d log: %s", ts, req.DevicePk, string(req.Payload))
	case "ota":
		log.Printf("📦 [%s] Device #%d OTA: %s", ts, req.DevicePk, string(req.Payload))
	default:
		log.Printf("📨 [%s] Device #%d [%s]: %s", ts, req.DevicePk, req.Kind, string(req.Payload))
	}

	return &OnEventResponse{Ok: true}, nil
}

func (p *TelemetryLoggerPlugin) ApplyConfig(ctx context.Context, req *ApplyConfigRequest) (*ApplyConfigResponse, error) {
	log.Printf("⚙️  Binding %d (device #%d): config applied", req.BindingId, req.DevicePk)
	if len(req.Config) > 0 {
		log.Printf("   Config: %s", string(req.Config))
	}
	return &ApplyConfigResponse{Ok: true}, nil
}

func (p *TelemetryLoggerPlugin) Detach(ctx context.Context, req *DetachRequest) (*DetachResponse, error) {
	log.Printf("🔌 Binding %d (device #%d): detached", req.BindingId, req.DevicePk)
	return &DetachResponse{Ok: true}, nil
}

func (p *TelemetryLoggerPlugin) Ping(ctx context.Context, req *PingRequest) (*PingResponse, error) {
	p.mu.RLock()
	count := p.eventsCount
	p.mu.RUnlock()

	return &PingResponse{
		Ok:              true,
		Version:         "1.0.0",
		Status:          "ok",
		EventsProcessed: count,
		UptimeSec:       int64(time.Since(p.startTime).Seconds()),
	}, nil
}

// ─── gRPC Server Registration ────────────────────────────────────────────────

// Note: In production, use protoc-generated registration code.
// This is a simplified manual registration for the example.

type pluginServiceDesc struct{}

func (pluginServiceDesc) ServiceDesc() *grpc.ServiceDesc {
	return &grpc.ServiceDesc{
		ServiceName: "plugin.PluginService",
		HandlerType: (*PluginServiceServer)(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "OnEvent",
				Handler:    onEventHandler,
			},
			{
				MethodName: "ApplyConfig",
				Handler:    applyConfigHandler,
			},
			{
				MethodName: "Detach",
				Handler:    detachHandler,
			},
			{
				MethodName: "Ping",
				Handler:    pingHandler,
			},
		},
		Streams: []grpc.StreamDesc{},
	}
}

func onEventHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	req := new(OnEventRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	return srv.(PluginServiceServer).OnEvent(ctx, req)
}

func applyConfigHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	req := new(ApplyConfigRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	return srv.(PluginServiceServer).ApplyConfig(ctx, req)
}

func detachHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	req := new(DetachRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	return srv.(PluginServiceServer).Detach(ctx, req)
}

func pingHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	req := new(PingRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	return srv.(PluginServiceServer).Ping(ctx, req)
}

// ─── Main ────────────────────────────────────────────────────────────────────

func main() {
	port := flag.Int("port", 50051, "gRPC server port")
	socket := flag.String("socket", "", "Unix socket path (overrides --port)")
	flag.Parse()

	log.Println("==================================================")
	log.Println("Telemetry Logger Plugin (gRPC)")
	log.Println("==================================================")

	// Create listener
	var lis net.Listener
	var err error

	if *socket != "" {
		// Unix socket
		os.Remove(*socket)
		lis, err = net.Listen("unix", *socket)
		if err != nil {
			log.Fatalf("Failed to listen on socket: %v", err)
		}
		log.Printf("Listening on Unix socket: %s", *socket)
	} else {
		// TCP
		lis, err = net.Listen("tcp", fmt.Sprintf(":%d", *port))
		if err != nil {
			log.Fatalf("Failed to listen: %v", err)
		}
		log.Printf("Listening on TCP port: %d", *port)
	}

	// Create gRPC server
	grpcServer := grpc.NewServer()

	// Register plugin service
	plugin := NewTelemetryLoggerPlugin()
	grpcServer.RegisterService(pluginServiceDesc{}.ServiceDesc(), plugin)

	// Enable reflection for debugging
	reflection.Register(grpcServer)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down...")
		grpcServer.GracefulStop()
	}()

	log.Println("Waiting for events from platform...")
	log.Println("--------------------------------------------------")

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC server error: %v", err)
	}
}
