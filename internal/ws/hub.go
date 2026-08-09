package ws

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait = 10 * time.Second
	sendBuf   = 64
)

type client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
	once sync.Once
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*client]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: map[*client]struct{}{}}
}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &client{hub: h, conn: conn, send: make(chan []byte, sendBuf)}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	go c.writePump()
	c.readPump()
}

func (h *Hub) remove(c *client) {
	c.once.Do(func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
		close(c.send)
		_ = c.conn.Close()
	})
}

func (c *client) readPump() {
	defer c.hub.remove(c)
	c.conn.SetReadLimit(512)
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (c *client) writePump() {
	defer c.hub.remove(c)
	for msg := range c.send {
		_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

func (h *Hub) Broadcast(event string, data any) {
	msg, err := json.Marshal(map[string]any{"event": event, "data": data})
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.send <- msg:
		default:
			// slow dashboard: drop this event rather than block MQTT
		}
	}
}
