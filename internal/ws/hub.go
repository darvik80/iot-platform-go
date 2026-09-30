package ws

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait = 10 * time.Second
	sendBuf   = 64
)

type client struct {
	hub       *Hub
	conn      *websocket.Conn
	send      chan []byte
	once      sync.Once
	accountID int64
	global    bool // platform admin: also receives platform-wide events
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*client]struct{}

	// AllowedHosts are extra Origin hosts (host[:port]) accepted besides the request host.
	AllowedHosts []string
}

func NewHub() *Hub {
	return &Hub{clients: map[*client]struct{}{}}
}

// OriginAllowed accepts non-browser clients (no Origin), same-host pages,
// localhost dev servers and AllowedHosts.
func (h *Hub) OriginAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	switch strings.ToLower(u.Hostname()) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	for _, h := range h.AllowedHosts {
		if strings.EqualFold(u.Host, h) {
			return true
		}
	}
	return false
}

// ServeWS subscribes the connection to events of accountID; global also adds platform-wide events.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, accountID int64, global bool) {
	upgrader := websocket.Upgrader{CheckOrigin: h.OriginAllowed}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &client{hub: h, conn: conn, send: make(chan []byte, sendBuf), accountID: accountID, global: global}
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

func (h *Hub) Broadcast(accountID int64, event string, data any) {
	msg, err := json.Marshal(map[string]any{"event": event, "data": data})
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if accountID == 0 {
			if !c.global {
				continue
			}
		} else if c.accountID != accountID {
			continue
		}
		select {
		case c.send <- msg:
		default:
			// slow dashboard: drop this event rather than block MQTT
		}
	}
}
