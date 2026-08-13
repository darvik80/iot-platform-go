package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "crearts-iot-platform/internal/grpc/plugin"
)

// grpcClient wraps a gRPC connection to an external plugin.
type grpcClient struct {
	conn   *grpc.ClientConn
	client pb.PluginServiceClient
	mu     sync.Mutex
}

// grpcPool manages gRPC connections to external plugins.
type grpcPool struct {
	mu      sync.RWMutex
	clients map[string]*grpcClient // plugin_id → client
}

func newGRPCPool() *grpcPool {
	return &grpcPool{clients: make(map[string]*grpcClient)}
}

// get returns existing client or creates a new one.
func (p *grpcPool) get(pluginID, endpoint string) (*grpcClient, error) {
	p.mu.RLock()
	c, ok := p.clients[pluginID]
	p.mu.RUnlock()
	if ok {
		return c, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check
	if c, ok = p.clients[pluginID]; ok {
		return c, nil
	}

	conn, err := grpc.NewClient(endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("grpc dial %s: %w", endpoint, err)
	}

	c = &grpcClient{conn: conn, client: pb.NewPluginServiceClient(conn)}
	p.clients[pluginID] = c
	return c, nil
}

func (p *grpcPool) close(pluginID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clients[pluginID]; ok {
		c.conn.Close()
		delete(p.clients, pluginID)
	}
}

func (p *grpcPool) closeAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, c := range p.clients {
		c.conn.Close()
		delete(p.clients, id)
	}
}

// ─── gRPC calls ──────────────────────────────────────────────────────────────

func (c *grpcClient) onEvent(ctx context.Context, bindingID, devicePK int64, deviceID, productID, kind string, payload json.RawMessage, online bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.client.OnEvent(ctx, &pb.OnEventRequest{
		BindingId: bindingID,
		DevicePk:  devicePK,
		DeviceId:  deviceID,
		ProductId: productID,
		Kind:      kind,
		Payload:   []byte(payload),
		Online:    online,
		Timestamp: time.Now().UnixMilli(),
	})
	if err != nil {
		return err
	}
	if !resp.Ok {
		return fmt.Errorf("plugin error: %s", resp.Error)
	}
	return nil
}

func (c *grpcClient) applyConfig(ctx context.Context, bindingID, devicePK int64, deviceID, productID string, config json.RawMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.client.ApplyConfig(ctx, &pb.ApplyConfigRequest{
		BindingId: bindingID,
		DevicePk:  devicePK,
		DeviceId:  deviceID,
		ProductId: productID,
		Config:    []byte(config),
	})
	if err != nil {
		return err
	}
	if !resp.Ok {
		return fmt.Errorf("plugin error: %s", resp.Error)
	}
	return nil
}

func (c *grpcClient) detach(ctx context.Context, bindingID, devicePK int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := c.client.Detach(ctx, &pb.DetachRequest{
		BindingId: bindingID,
		DevicePk:  devicePK,
	})
	if err != nil {
		return err
	}
	if !resp.Ok {
		return fmt.Errorf("plugin error: %s", resp.Error)
	}
	return nil
}

func (c *grpcClient) ping(ctx context.Context) (*pb.PingResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	return c.client.Ping(ctx, &pb.PingRequest{})
}
