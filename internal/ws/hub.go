package ws

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/taskforge/internal"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	tenantID string
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]bool
	rooms   map[string]map[*Client]bool // tenantID -> clients
	log     *zap.Logger
}

func NewHub(log *zap.Logger) *Hub {
	return &Hub{
		clients: make(map[*Client]bool),
		rooms:   make(map[string]map[*Client]bool),
		log:     log,
	}
}

func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = r.Header.Get("X-Tenant-ID")
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Error("ws upgrade failed", zap.Error(err))
		return
	}

	client := &Client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 256),
		tenantID: tenantID,
	}

	h.mu.Lock()
	h.clients[client] = true
	if tenantID != "" {
		if h.rooms[tenantID] == nil {
			h.rooms[tenantID] = make(map[*Client]bool)
		}
		h.rooms[tenantID][client] = true
	}
	h.mu.Unlock()

	go client.writePump()
	go client.readPump()

	h.log.Info("ws client connected", zap.String("tenant", tenantID))
}

func (h *Hub) Broadcast(tenantID string, msg internal.WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		h.log.Error("ws marshal failed", zap.Error(err))
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if tenantID != "" {
		for client := range h.rooms[tenantID] {
			select {
			case client.send <- data:
			default:
				close(client.send)
				delete(h.rooms[tenantID], client)
				delete(h.clients, client)
			}
		}
	} else {
		for client := range h.clients {
			select {
			case client.send <- data:
			default:
			}
		}
	}
}

func (h *Hub) BroadcastJobEvent(tenantID, eventType string, job *internal.Job) {
	h.Broadcast(tenantID, internal.WSMessage{
		Type: eventType,
		Payload: map[string]interface{}{
			"id":        job.ID,
			"type":      job.Type,
			"status":    job.Status,
			"tenant_id": job.TenantID,
			"priority":  job.Priority,
			"retry":     job.RetryCount,
		},
	})
}

func (h *Hub) Remove(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		if client.tenantID != "" {
			delete(h.rooms[client.tenantID], client)
		}
		client.conn.Close()
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) readPump() {
	defer func() {
		c.hub.Remove(c)
	}()
	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}
